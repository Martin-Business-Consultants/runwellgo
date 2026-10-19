package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// A tool the runner uses, and how to ask its version.
type tool struct {
	name     string
	args     []string
	required bool
	why      string
}

var tools = []tool{
	{"git", []string{"--version"}, true, "clones, commits and pushes"},
	{"gh", []string{"--version"}, true, "clones from GitHub and opens pull requests"},
	{"rg", []string{"--version"}, false, "fast search, which coding agents use"},
	{"mise", []string{"--version"}, false, "installs the tools a repository pins (mise.toml, .tool-versions)"},
}

var versionRE = regexp.MustCompile(`\d+\.\d+(\.\d+)?`)

// toolVersions are the tools found here, with their versions.
func toolVersions(ctx context.Context) map[string]string {
	out := map[string]string{}
	for _, t := range tools {
		if v, err := toolVersion(ctx, t.name, t.args...); err == nil {
			out[t.name] = v
		}
	}
	return out
}

func toolVersion(ctx context.Context, name string, args ...string) (string, error) {
	if _, err := exec.LookPath(name); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", err
	}
	if v := versionRE.FindString(string(b)); v != "" {
		return v, nil
	}
	return strings.TrimSpace(firstOf(string(b))), nil
}

func firstOf(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

// agentProgram is the agent command's program (its first word).
func agentProgram(cmd string) string {
	f := strings.Fields(cmd)
	for len(f) > 0 && strings.Contains(f[0], "=") { // FOO=bar claude …
		f = f[1:]
	}
	if len(f) == 0 {
		return ""
	}
	if f[0] == "mise" && len(f) > 2 && f[1] == "exec" { // mise exec -- claude …
		for i, w := range f {
			if w == "--" && i+1 < len(f) {
				return f[i+1]
			}
		}
	}
	return f[0]
}

func doctorCmd(ctx context.Context, args []string, out io.Writer) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	path := fs.String("config", defaultConfigPath(), "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	problems := 0
	say := func(ok bool, what, detail string) {
		mark := "ok  "
		if !ok {
			mark = "MISSING"
			problems++
		}
		fmt.Fprintf(out, "%-8s %-10s %s\n", mark, what, detail)
	}
	fmt.Fprintf(out, "runwell-runner %s on %s/%s\n\n", version, runtime.GOOS, runtime.GOARCH)
	for _, t := range tools {
		v, err := toolVersion(ctx, t.name, t.args...)
		switch {
		case err == nil:
			say(true, t.name, v)
		case t.required:
			say(false, t.name, "needed: "+t.why)
		default:
			fmt.Fprintf(out, "%-8s %-10s %s\n", "optional", t.name, "not found: "+t.why)
		}
	}
	if _, err := exec.LookPath("gh"); err == nil {
		c, cancel := context.WithTimeout(ctx, 15*time.Second)
		b, err := exec.CommandContext(c, "gh", "auth", "status").CombinedOutput()
		cancel()
		if err != nil {
			say(false, "gh auth", "not signed in: run `gh auth login` (and `gh auth setup-git`) ("+strings.TrimSpace(firstOf(string(b)))+")")
		} else {
			say(true, "gh auth", "signed in")
		}
	}
	c, err := loadConfig(*path)
	if err != nil {
		say(false, "config", err.Error())
		return fmt.Errorf("%d problem(s)", problems)
	}
	say(true, "config", *path)
	if p := agentProgram(c.Agent); p == "" {
		say(false, "agent", "agent_command is blank")
	} else if _, err := exec.LookPath(p); err != nil {
		say(false, "agent", p+" isn't on the PATH (agent_command: "+c.Agent+")")
	} else {
		say(true, "agent", c.Agent)
	}
	if err := os.MkdirAll(c.WorkDir, 0o700); err != nil {
		say(false, "work dir", err.Error())
	} else {
		say(true, "work dir", c.WorkDir)
	}
	fmt.Fprintf(out, "%-8s %-10s %s\n", "", "repos", strings.Join(c.AllowedRepos, ", "))
	beat, err := newClient(c).heartbeat(ctx, info(ctx, c, 0))
	if err != nil {
		var ae *apiError
		if errors.As(err, &ae) && ae.Status == 401 {
			say(false, "runwell", "the token isn't valid (removed, or the Runners plugin is off): add the runner again in Settings › Runners")
		} else {
			say(false, "runwell", err.Error())
		}
	} else {
		say(true, "runwell", fmt.Sprintf("%s answers: this is %q%s", c.URL, beat.Name, map[bool]string{true: " (paused)", false: ""}[beat.Paused]))
	}
	if problems > 0 {
		return fmt.Errorf("%d problem(s)", problems)
	}
	fmt.Fprintln(out, "\nReady: runwell-runner run")
	return nil
}

// info is what a heartbeat tells Runwell about this runner.
func info(ctx context.Context, c *Config, busy int) map[string]any {
	host, _ := os.Hostname()
	if c.Name != "" {
		host = c.Name
	}
	return map[string]any{"version": version, "os": runtime.GOOS, "arch": runtime.GOARCH, "hostname": host,
		"tools": toolVersions(ctx), "agent": filepath.Base(agentProgram(c.Agent)), "allowed_repos": c.AllowedRepos, "busy": busy, "max_jobs": c.Concurrency}
}
