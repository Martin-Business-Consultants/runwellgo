package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// do does one job: check it's allowed, clone, set up, branch, write the
// prompt, run the agent, then commit, push and open a pull request when it
// changed anything, and report. Its directory goes at the end, whatever
// happened.
func (r *runner) do(ctx context.Context, cancel context.CancelCauseFunc, job *Job) {
	began := time.Now()
	log := r.log.With("job", job.ID, "repo", job.Repo)
	log.Info("job claimed", "branch", job.Branch)
	out := newStreamer(r.client, job.ID, r.red, func() { cancel(errCancelled) })
	go out.run(ctx) //nolint:gosec // G118: it does run on ctx; the Background below is finish's, reported while stopping
	result := r.work(ctx, job, out)
	result.DurationSeconds = int64(time.Since(began).Seconds())
	if result.Branch == "" {
		result.Branch = job.Branch
	}
	out.say("Took " + time.Since(began).Round(time.Second).String() + ".")
	out.close()
	result.Summary = r.red.clean(result.Summary)
	result.Error = r.red.clean(result.Error)
	// Reported even while stopping: a context of its own, briefly.
	fctx, done := context.WithTimeout(context.Background(), 30*time.Second)
	defer done()
	if err := r.client.finish(fctx, job.ID, result); err != nil {
		log.Warn("finish", "err", err)
	}
	log.Info("job done", "outcome", result.Outcome, "pr", result.PRURL, "files", len(result.FilesChanged), "error", result.Error)
}

// work is the job's steps; it answers how they went.
func (r *runner) work(ctx context.Context, job *Job, out *streamer) Result {
	if !r.cfg.allows(job.Repo) {
		msg := fmt.Sprintf("This runner doesn't allow %s: it isn't on its allowlist, so it refused the job.", job.Repo)
		out.say(msg)
		return Result{Outcome: "refused", Error: msg}
	}
	timeout := r.timeout(job)
	ctx, stop := context.WithTimeoutCause(ctx, timeout, fmt.Errorf("timed out after %s", timeout))
	defer stop()
	ended := func(step string, err error) Result {
		switch cause := context.Cause(ctx); {
		case errors.Is(cause, errCancelled):
			out.say("Cancelled.")
			return Result{Outcome: "cancelled"}
		case errors.Is(cause, errStopped):
			return Result{Outcome: "failed", Error: "The runner was stopped while " + step + "."}
		case cause != nil:
			return Result{Outcome: "failed", Error: "It " + cause.Error() + " while " + step + "."}
		}
		return Result{Outcome: "failed", Error: fmt.Sprintf("%s failed: %v", capitalize(step), err)}
	}

	dir, err := os.MkdirTemp(r.cfg.WorkDir, fmt.Sprintf("job-%d-", job.ID))
	if err != nil {
		return Result{Outcome: "failed", Error: "Couldn't make a work directory: " + err.Error()}
	}
	defer os.RemoveAll(dir)
	repo := filepath.Join(dir, "repo")

	// A fresh checkout.
	if isLocal(job.Repo) {
		out.say("Cloning " + job.Repo + "…")
		err = r.cmd(ctx, out, dir, nil, "git", "clone", strings.TrimPrefix(job.Repo, "file://"), repo)
	} else {
		out.say("Cloning " + job.Repo + " with gh…")
		err = r.cmd(ctx, out, dir, nil, "gh", "repo", "clone", job.Repo, repo)
	}
	if err != nil {
		return ended("cloning", err)
	}
	if job.BaseBranch != "" {
		if err := r.cmd(ctx, out, repo, nil, "git", "checkout", job.BaseBranch); err != nil {
			return ended("checking out "+job.BaseBranch, err)
		}
	}
	base, err := r.output(ctx, repo, "git", "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return ended("reading the base branch", err)
	}
	baseSHA, err := r.output(ctx, repo, "git", "rev-parse", "HEAD")
	if err != nil {
		return ended("reading the base commit", err)
	}

	// The tools the repository pins.
	if err := r.installTools(ctx, out, repo); err != nil {
		return ended("installing tools", err)
	}

	branch := job.Branch
	if branch == "" {
		branch = fmt.Sprintf("runwell/%d", job.ID)
	}
	if err := r.cmd(ctx, out, repo, nil, "git", "checkout", "-b", branch); err != nil {
		return ended("making the branch", err)
	}

	// The prompt, beside the checkout so it's never committed.
	promptFile := filepath.Join(dir, "prompt.md")
	summaryFile := filepath.Join(dir, "summary.md")
	if err := os.WriteFile(promptFile, []byte(prompt(job)), 0o600); err != nil {
		return ended("writing the prompt", err)
	}

	// The agent.
	command := expand(r.cfg.Agent, map[string]string{"prompt_file": promptFile, "workdir": repo, "branch": branch, "job_id": strconv.FormatInt(job.ID, 10)})
	out.say("Running the agent: " + r.cfg.Agent)
	env := []string{"RUNWELL_JOB_ID=" + strconv.FormatInt(job.ID, 10), "RUNWELL_PROMPT_FILE=" + promptFile, "RUNWELL_SUMMARY_FILE=" + summaryFile,
		"RUNWELL_BRANCH=" + branch, "RUNWELL_WORKDIR=" + repo, "RUNWELL_REPO=" + job.Repo, "RUNWELL_WORK_URL=" + job.WorkURL}
	last := &lastMessage{}
	out.watch = last.see
	agentErr := r.cmd(ctx, out, repo, env, "sh", "-c", command)
	out.watch = nil
	if agentErr != nil {
		if ctx.Err() != nil {
			return ended("the agent ran", agentErr)
		}
		return Result{Outcome: "failed", Error: "The agent stopped with an error: " + agentErr.Error(), Summary: last.text(), Branch: branch}
	}
	summary := strings.TrimSpace(readSmall(summaryFile))
	if summary == "" {
		summary = last.text()
	}

	// What it changed: committed by the agent, or left in the tree.
	if err := r.cmd(ctx, out, repo, nil, "git", "add", "-A"); err != nil {
		return ended("staging the changes", err)
	}
	names, err := r.output(ctx, repo, "git", "diff", "--cached", "--name-only", baseSHA)
	if err != nil {
		return ended("reading the changes", err)
	}
	files := nonEmpty(strings.Split(names, "\n"))
	if len(files) == 0 {
		out.say("Nothing changed.")
		return Result{Outcome: "no_changes", Summary: summary, Branch: branch}
	}
	out.say(fmt.Sprintf("%d files changed.", len(files)))
	if _, err := r.output(ctx, repo, "git", "diff", "--cached", "--quiet"); err != nil {
		if err := r.commit(ctx, out, repo, job); err != nil {
			return ended("committing", err)
		}
	}
	res := Result{Outcome: "changes", Summary: summary, Branch: branch, FilesChanged: files}
	if r.noPush {
		out.say("--no-push: the commit stays on this machine (and goes with the work directory).")
		r.wouldOpen(out, job, branch, base, summary)
		return res
	}
	out.say("Pushing " + branch + "…")
	if err := r.cmd(ctx, out, repo, nil, "git", "push", "-u", "origin", branch); err != nil {
		return ended("pushing", err)
	}
	if r.noPR || isLocal(job.Repo) {
		out.say("Pushed. No pull request (--no-pr, or not a GitHub repository).")
		r.wouldOpen(out, job, branch, base, summary)
		return res
	}
	out.say("Opening a pull request…")
	title, body := prText(job, summary, files)
	url, err := r.output(ctx, repo, "gh", "pr", "create", "--head", branch, "--base", base, "--title", title, "--body", body)
	if err != nil {
		return ended("opening the pull request", err)
	}
	res.PRURL = lastLine(url)
	out.say("Opened " + res.PRURL)
	return res
}

// timeout is how long the job may take: its own limit, or the runner's
// when that's shorter, or an hour.
func (r *runner) timeout(job *Job) time.Duration {
	timeout := time.Duration(job.TimeoutMinutes) * time.Minute
	if r.cfg.TimeoutMinutes > 0 && (timeout == 0 || time.Duration(r.cfg.TimeoutMinutes)*time.Minute < timeout) {
		timeout = time.Duration(r.cfg.TimeoutMinutes) * time.Minute
	}
	if timeout <= 0 {
		timeout = time.Hour
	}
	return timeout
}

// installTools installs the tools the repository pins with mise, when it
// can; it fails only when the job's context ended while it did.
func (r *runner) installTools(ctx context.Context, out *streamer, repo string) error {
	if !hasAny(repo, "mise.toml", ".mise.toml", ".tool-versions") {
		return nil
	}
	if _, err := exec.LookPath("mise"); err != nil {
		out.say("The repository pins tools with mise, but mise isn't installed here: going on without.")
		return nil //nolint:nilerr // no mise is no tools, said, and the job goes on
	}
	out.say("Installing the repository's tools with mise…")
	_ = r.cmd(ctx, out, repo, nil, "mise", "trust", "--yes", repo) // mise install says what's wrong, if anything
	if err := r.cmd(ctx, out, repo, nil, "mise", "install"); err != nil {
		if ctx.Err() != nil {
			return err
		}
		out.say("mise install failed: going on, the agent may manage without.")
	}
	return nil
}

// commit commits what's staged, as the job, as the runner when git
// doesn't know who's committing.
func (r *runner) commit(ctx context.Context, out *streamer, repo string, job *Job) error {
	title := job.Title
	if title == "" {
		title = "Runwell job " + strconv.FormatInt(job.ID, 10)
	}
	msg := fmt.Sprintf("%s\n\nDone by a Runwell runner for job %d.", title, job.ID)
	if job.WorkURL != "" {
		msg += "\n" + job.WorkURL
	}
	args := []string{"commit", "-q", "-m", msg}
	if name, _ := r.output(ctx, repo, "git", "config", "user.name"); name == "" {
		args = append([]string{"-c", "user.name=Runwell runner", "-c", "user.email=runner@runwell.invalid"}, args...)
	}
	return r.cmd(ctx, out, repo, nil, "git", args...)
}

// wouldOpen logs the pull request it would have opened.
func (r *runner) wouldOpen(out *streamer, job *Job, branch, base, summary string) {
	title, body := prText(job, summary, nil)
	out.say(fmt.Sprintf("Would open a pull request %s → %s: %q", branch, base, title))
	for l := range strings.SplitSeq(body, "\n") {
		out.say("  " + l)
	}
}

// prompt is what the agent is given: the instructions, the work's brief,
// and how to finish.
func prompt(job *Job) string {
	var b strings.Builder
	b.WriteString("# What to do\n\n")
	b.WriteString(strings.TrimSpace(job.Instructions))
	b.WriteString("\n\n")
	if strings.TrimSpace(job.Brief) != "" {
		b.WriteString("# The work, from Runwell\n\n")
		b.WriteString(strings.TrimSpace(job.Brief))
		b.WriteString("\n\n")
	}
	b.WriteString("# How to finish\n\n")
	b.WriteString("You're working in a fresh checkout of " + job.Repo + ", on the branch " + job.Branch + ". ")
	b.WriteString("Make the change, run the project's own checks if it has them, and leave your work in the tree (committing is fine). ")
	b.WriteString("Don't push or open a pull request: the runner does, and a person approves it in Runwell. ")
	b.WriteString("Last, write a short summary of what you changed and anything a reviewer should know to the file named by $RUNWELL_SUMMARY_FILE.\n")
	return b.String()
}

// prText is the pull request's title and body.
func prText(job *Job, summary string, files []string) (string, string) {
	title := job.Title
	if title == "" {
		title = fmt.Sprintf("Runwell job %d", job.ID)
	}
	var b strings.Builder
	if summary != "" {
		b.WriteString(summary + "\n\n")
	}
	if job.WorkURL != "" {
		fmt.Fprintf(&b, "Work in Runwell: %s\n", job.WorkURL)
	}
	if job.JobURL != "" {
		fmt.Fprintf(&b, "Runner job: %s\n", job.JobURL)
	}
	if job.RequestedBy != "" {
		fmt.Fprintf(&b, "Asked for by %s.\n", job.RequestedBy)
	}
	if len(files) > 0 {
		fmt.Fprintf(&b, "\n%d files changed.\n", len(files))
	}
	b.WriteString("\n_Done by a Runwell runner. Approve or decline it in Runwell._")
	return title, b.String()
}

// cleanup removes a declined (or cancelled) job's pull request and branch.
func (r *runner) cleanup(ctx context.Context, c Cleanup) {
	log := r.log.With("job", c.JobID, "repo", c.Repo)
	var err error
	switch {
	case !r.cfg.allows(c.Repo):
		err = errors.New("this runner doesn't allow " + c.Repo)
	case c.PRURL != "":
		_, err = r.output(ctx, "", "gh", "pr", "close", c.PRURL, "--delete-branch", "--comment", "Declined in Runwell.")
	case c.Branch != "" && !r.noPush:
		remote := strings.TrimPrefix(c.Repo, "file://")
		if !isLocal(c.Repo) {
			remote = "https://github.com/" + c.Repo + ".git"
		}
		// git push wants a repository to run in, any: an empty one.
		var dir string
		if dir, err = os.MkdirTemp(r.cfg.WorkDir, fmt.Sprintf("cleanup-%d-", c.JobID)); err == nil {
			defer os.RemoveAll(dir)
			if _, err = r.output(ctx, dir, "git", "init", "-q"); err == nil {
				_, err = r.output(ctx, dir, "git", "push", remote, "--delete", c.Branch)
			}
		}
	}
	msg := ""
	if err != nil {
		msg = err.Error()
		log.Warn("cleanup", "err", err)
	} else {
		log.Info("cleaned up", "branch", c.Branch, "pr", c.PRURL)
	}
	if err := r.client.cleaned(ctx, c.JobID, r.red.clean(msg)); err != nil {
		log.Warn("cleanup report", "err", err)
	}
}

// ---- commands --------------------------------------------------------------------------

// cmd runs a command in dir, its output going to the job's log.
func (r *runner) cmd(ctx context.Context, out *streamer, dir string, env []string, name string, args ...string) error {
	c := exec.CommandContext(ctx, name, args...) //nolint:gosec // git, gh, mise, or sh running the operator's configured agent
	c.Dir = dir
	c.Env = append(childEnv(), env...)
	c.Stdin = nil
	c.WaitDelay = 10 * time.Second
	ownGroup(c)
	stdout, err := c.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := c.StderrPipe()
	if err != nil {
		return err
	}
	if err := c.Start(); err != nil {
		return err
	}
	// Stopped (cancelled, timed out): SIGTERM to its group at once, and
	// SIGKILL to whatever's left of it ten seconds on.
	finished := make(chan struct{})
	defer close(finished)
	go func() {
		select {
		case <-finished:
		case <-ctx.Done():
			select {
			case <-finished:
			case <-time.After(10 * time.Second):
				killGroup(c)
			}
		}
	}()
	var wg sync.WaitGroup
	wg.Add(2)
	go func() { defer wg.Done(); out.read("stdout", stdout) }()
	go func() { defer wg.Done(); out.read("stderr", stderr) }()
	wg.Wait()
	return c.Wait()
}

// output runs a command and answers its trimmed output.
func (r *runner) output(ctx context.Context, dir, name string, args ...string) (string, error) {
	c := exec.CommandContext(ctx, name, args...) //nolint:gosec // git or gh, as the runner's own steps call them
	c.Dir = dir
	c.Env = childEnv()
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	if err := c.Run(); err != nil {
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return "", fmt.Errorf("%w: %s", err, lastLine(s))
		}
		return "", err
	}
	return strings.TrimSpace(stdout.String()), nil
}

// childEnv is this process's environment without the runner's own token:
// the agent never holds it.
func childEnv() []string {
	out := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "RUNWELL_RUNNER_TOKEN=") {
			out = append(out, kv)
		}
	}
	return out
}

// expand fills a command template's {name}s, each shell-quoted.
func expand(tmpl string, vars map[string]string) string {
	for k, v := range vars {
		tmpl = strings.ReplaceAll(tmpl, "{"+k+"}", shellQuote(v))
	}
	return tmpl
}

func shellQuote(s string) string {
	if s != "" && strings.IndexFunc(s, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') && (r < '0' || r > '9') && !strings.ContainsRune("-_./:=@", r)
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func hasAny(dir string, names ...string) bool {
	for _, n := range names {
		if _, err := os.Stat(filepath.Join(dir, n)); err == nil {
			return true
		}
	}
	return false
}

func readSmall(path string) string {
	f, err := os.Open(path) //nolint:gosec // the job's own summary file
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(io.LimitReader(f, 64<<10))
	return string(b)
}

func nonEmpty(list []string) []string {
	out := []string{}
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndexByte(s, '\n'); i >= 0 {
		return strings.TrimSpace(s[i+1:])
	}
	return s
}

func capitalize(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---- the agent's last message ----------------------------------------------------------

// lastMessage keeps what the agent said last: the result of a Claude Code
// stream-json run, its last text, or else its last lines of plain output.
type lastMessage struct {
	mu     sync.Mutex
	result string
	said   string
	lines  []string
}

func (m *lastMessage) see(stream, line string) {
	if stream != "stdout" {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if strings.HasPrefix(line, "{") {
		var ev struct {
			Type    string `json:"type"`
			Result  string `json:"result"`
			Message struct {
				Content []struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"content"`
			} `json:"message"`
		}
		if json.Unmarshal([]byte(line), &ev) == nil && ev.Type != "" {
			switch ev.Type {
			case "result":
				if ev.Result != "" {
					m.result = ev.Result
				}
			case "assistant":
				for _, c := range ev.Message.Content {
					if c.Type == "text" && strings.TrimSpace(c.Text) != "" {
						m.said = c.Text
					}
				}
			}
			return
		}
	}
	if strings.TrimSpace(line) != "" {
		m.lines = append(m.lines, line)
		if len(m.lines) > 15 {
			m.lines = m.lines[1:]
		}
	}
}

func (m *lastMessage) text() string {
	m.mu.Lock()
	defer m.mu.Unlock()
	switch {
	case m.result != "":
		return strings.TrimSpace(m.result)
	case m.said != "":
		return strings.TrimSpace(m.said)
	}
	return strings.TrimSpace(strings.Join(m.lines, "\n"))
}

// ---- the log -----------------------------------------------------------------------------

// streamer batches a job's output and sends it to Runwell about once a
// second, secrets taken out, and asks whether to stop between batches.
type streamer struct {
	c        *client
	id       int64
	red      *redactor
	onCancel func()
	watch    func(stream, line string)

	mu    sync.Mutex
	lines []Line
	done  chan struct{}
	ended chan struct{}
}

func newStreamer(c *client, id int64, red *redactor, onCancel func()) *streamer {
	return &streamer{c: c, id: id, red: red, onCancel: onCancel, done: make(chan struct{}), ended: make(chan struct{})}
}

func (s *streamer) add(stream, text string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.lines) > 5000 { // Runwell is unreachable: keep the newest
		s.lines = s.lines[1:]
	}
	s.lines = append(s.lines, Line{Stream: stream, Text: s.red.clean(text), At: time.Now().UTC().Format(time.RFC3339Nano)})
}

// say logs a step of the runner's own.
func (s *streamer) say(text string) { s.add("runner", text) }

// read takes a command's output line by line.
func (s *streamer) read(stream string, r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if w := s.watch; w != nil {
			w(stream, line)
		}
		s.add(stream, readable(line))
	}
}

func (s *streamer) run(ctx context.Context) {
	defer close(s.ended)
	t := time.NewTicker(time.Second)
	defer t.Stop()
	quiet := 0
	for {
		select {
		case <-s.done:
			s.flush(context.Background())
			return
		case <-t.C:
			if s.flush(ctx) {
				quiet = 0
				continue
			}
			// Nothing to send: ask whether to stop every few seconds.
			if quiet++; quiet%3 == 0 {
				c, cancel := context.WithTimeout(ctx, 10*time.Second)
				stop, err := s.c.cancelled(c, s.id)
				cancel()
				if err == nil && stop {
					s.onCancel()
				}
			}
		}
	}
}

// flush sends what's waiting; false when there was nothing.
func (s *streamer) flush(ctx context.Context) bool {
	s.mu.Lock()
	lines := s.lines
	s.lines = nil
	s.mu.Unlock()
	if len(lines) == 0 {
		return false
	}
	for len(lines) > 0 {
		n := min(len(lines), 500)
		c, cancel := context.WithTimeout(context.WithoutCancel(ctx), 15*time.Second)
		stop, err := s.c.events(c, s.id, lines[:n])
		cancel()
		if err != nil {
			// Put them back to try again next time.
			s.mu.Lock()
			s.lines = append(lines, s.lines...)
			s.mu.Unlock()
			return true
		}
		if stop {
			s.onCancel()
		}
		lines = lines[n:]
	}
	return true
}

func (s *streamer) close() {
	close(s.done)
	<-s.ended
}

// readable shortens a Claude Code stream-json line to what a person
// reads: what it said, the tool it used and on what, and the result.
func readable(line string) string {
	if !strings.HasPrefix(line, `{"type":`) {
		return line
	}
	var ev struct {
		Type    string `json:"type"`
		Subtype string `json:"subtype"`
		Result  string `json:"result"`
		Message struct {
			Content []struct {
				Type  string         `json:"type"`
				Text  string         `json:"text"`
				Name  string         `json:"name"`
				Input map[string]any `json:"input"`
			} `json:"content"`
		} `json:"message"`
	}
	if json.Unmarshal([]byte(line), &ev) != nil {
		return line
	}
	switch ev.Type {
	case "assistant":
		var parts []string
		for _, c := range ev.Message.Content {
			switch c.Type {
			case "text":
				parts = append(parts, strings.TrimSpace(c.Text))
			case "tool_use":
				what := ""
				for _, k := range []string{"file_path", "path", "command", "pattern", "description"} {
					if v, ok := c.Input[k].(string); ok && v != "" {
						what = " " + v
						break
					}
				}
				parts = append(parts, "→ "+c.Name+what)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, "\n")
		}
	case "user":
		return "  (tool result)"
	case "result":
		return "Result: " + strings.TrimSpace(ev.Result)
	case "system":
		return "Agent " + strings.TrimSpace(ev.Subtype)
	}
	return line
}
