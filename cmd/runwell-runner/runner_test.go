package main

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestAllowlist(t *testing.T) {
	c := &Config{AllowedRepos: []string{"acme/site", "tools/*", "/srv/git/mirror.git"}}
	for repo, want := range map[string]bool{
		"acme/site": true, "Acme/Site": true, "https://github.com/acme/site.git": true, "acme/site2": false,
		"tools/cli": true, "tools/cli/extra": false, "toolsx/cli": false, "/srv/git/mirror.git": true, "/srv/git/other.git": false, "": false,
	} {
		if got := c.allows(repo); got != want {
			t.Errorf("allows(%q) = %v, want %v", repo, got, want)
		}
	}
}

func TestExpandQuotes(t *testing.T) {
	got := expand("agent < {prompt_file} --dir {workdir} --x {job_id}", map[string]string{"prompt_file": "/tmp/a b/p.md", "workdir": "/w/it's", "job_id": "7"})
	if got != `agent < '/tmp/a b/p.md' --dir '/w/it'\''s' --x 7` {
		t.Fatalf("expand = %s", got)
	}
}

func TestRedactor(t *testing.T) {
	t.Setenv("MY_API_KEY", "supersecretvalue1")
	r := newRedactor("rwm_tokentokentoken")
	got := r.clean("key supersecretvalue1, token rwm_tokentokentoken, gh ghp_0123456789abcdefghij, fine")
	if strings.Contains(got, "supersecret") || strings.Contains(got, "rwm_") || strings.Contains(got, "ghp_") || !strings.Contains(got, "fine") {
		t.Fatalf("clean = %q", got)
	}
}

func TestLastMessage(t *testing.T) {
	m := &lastMessage{}
	m.see("stdout", `{"type":"assistant","message":{"content":[{"type":"text","text":"Halfway."}]}}`)
	if m.text() != "Halfway." {
		t.Fatalf("text = %q", m.text())
	}
	m.see("stdout", `{"type":"result","subtype":"success","result":"Changed two files."}`)
	if m.text() != "Changed two files." {
		t.Fatalf("text = %q", m.text())
	}
	if got := readable(`{"type":"assistant","message":{"content":[{"type":"tool_use","name":"Edit","input":{"file_path":"a.go"}}]}}`); got != "→ Edit a.go" {
		t.Fatalf("readable = %q", got)
	}
}

func TestAgentProgram(t *testing.T) {
	for in, want := range map[string]string{"claude -p < {prompt_file}": "claude", "FOO=1 codex exec": "codex", "mise exec -- aider --yes": "aider"} {
		if got := agentProgram(in); got != want {
			t.Errorf("agentProgram(%q) = %q, want %q", in, got, want)
		}
	}
}

// fakeRunwell is the Runners plugin's machine routes, enough to hand out
// one job and keep what the runner sends.
type fakeRunwell struct {
	mu     sync.Mutex
	job    *Job
	lines  []Line
	result *Result
	cancel bool
}

func (f *fakeRunwell) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer rwm_test" {
		w.WriteHeader(http.StatusUnauthorized)
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/x/runners/machine")
	w.Header().Set("Content-Type", "application/json")
	switch {
	case path == "/heartbeat":
		io.WriteString(w, `{"name":"test","poll_seconds":1,"cancel":[],"cleanup":[]}`)
	case path == "/claim":
		_ = json.NewEncoder(w).Encode(map[string]any{"job": f.job})
		f.job = nil
	case strings.HasSuffix(path, "/events"):
		var body struct{ Events []Line }
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.lines = append(f.lines, body.Events...)
		_ = json.NewEncoder(w).Encode(map[string]any{"cancel": f.cancel})
	case strings.HasSuffix(path, "/cancel"):
		_ = json.NewEncoder(w).Encode(map[string]any{"cancel": f.cancel})
	case strings.HasSuffix(path, "/finish"):
		var res Result
		_ = json.NewDecoder(r.Body).Decode(&res)
		f.result = &res
		io.WriteString(w, `{}`)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=t", "GIT_AUTHOR_EMAIL=t@t", "GIT_COMMITTER_NAME=t", "GIT_COMMITTER_EMAIL=t@t")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// A job, end to end against a local bare repository: the agent (a script)
// edits a file, the runner commits it on the job's branch, pushes it, and
// reports the change with the agent's summary; a job for a repository off
// the allowlist is refused.
func TestRunnerDoesAJob(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("no git")
	}
	dir := t.TempDir()
	bare := filepath.Join(dir, "site.git")
	git(t, dir, "init", "-q", "--bare", "-b", "main", bare)
	seed := filepath.Join(dir, "seed")
	git(t, dir, "clone", "-q", bare, seed)
	os.WriteFile(filepath.Join(seed, "README.md"), []byte("hi\n"), 0o644)
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-qm", "init")
	git(t, seed, "push", "-q", "origin", "main")
	agent := filepath.Join(dir, "agent.sh")
	os.WriteFile(agent, []byte("#!/bin/sh\necho \"prompt has $(grep -c . \"$1\") lines\"\necho hello > hello.txt\necho 'Said hello.' > \"$RUNWELL_SUMMARY_FILE\"\n"), 0o755)

	fake := &fakeRunwell{job: &Job{ID: 7, Title: "Say hello", Repo: bare, Branch: "runwell/7-say-hello", Instructions: "Say hello.", Brief: "# Brief", TimeoutMinutes: 1}}
	srv := httptest.NewServer(fake)
	defer srv.Close()
	cfg := &Config{URL: srv.URL, Token: "rwm_test", WorkDir: filepath.Join(dir, "work"), AllowedRepos: []string{bare}, Concurrency: 1, Agent: agent + " {prompt_file}"}
	os.MkdirAll(cfg.WorkDir, 0o700)
	r := &runner{cfg: cfg, client: newClient(cfg), log: slog.New(slog.NewTextHandler(io.Discard, nil)), red: newRedactor(cfg.Token),
		noPR: true, once: true, running: map[int64]context.CancelCauseFunc{}, cleaned: map[int64]bool{}, poll: 50 * time.Millisecond}
	ctx, cancel := context.WithTimeout(t.Context(), 60*time.Second)
	defer cancel()
	if err := r.loop(ctx); err != nil {
		t.Fatal(err)
	}
	fake.mu.Lock()
	res, lines := fake.result, fake.lines
	fake.mu.Unlock()
	if res == nil || res.Outcome != "changes" || res.Summary != "Said hello." || len(res.FilesChanged) != 1 || res.FilesChanged[0] != "hello.txt" {
		t.Fatalf("result: %+v", res)
	}
	var log strings.Builder
	for _, l := range lines {
		log.WriteString(l.Text + "\n")
	}
	if !strings.Contains(log.String(), "prompt has") {
		t.Fatalf("the agent's output didn't reach the log:\n%s", log.String())
	}
	out, err := exec.CommandContext(t.Context(), "git", "--git-dir", bare, "log", "--format=%s", "runwell/7-say-hello").Output()
	if err != nil || !strings.HasPrefix(string(out), "Say hello") {
		t.Fatalf("the branch wasn't pushed: %s %v", out, err)
	}
	entries, _ := os.ReadDir(cfg.WorkDir)
	if len(entries) != 0 {
		t.Fatalf("the work directory was left: %v", entries)
	}

	// A repository it doesn't allow is refused, whatever Runwell says.
	fake.mu.Lock()
	fake.job, fake.result = &Job{ID: 8, Repo: "evil/repo", Branch: "runwell/8", Instructions: "x", TimeoutMinutes: 1}, nil
	fake.mu.Unlock()
	if err := r.loop(ctx); err != nil {
		t.Fatal(err)
	}
	if fake.result == nil || fake.result.Outcome != "refused" {
		t.Fatalf("an unallowed repository: %+v", fake.result)
	}
}
