package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Config is the runner's config file.
type Config struct {
	// URL is Runwell's address; Token the runner's own (rwm_…), from
	// Settings › Runners.
	URL   string `json:"url"`
	Token string `json:"token"`
	// WorkDir is where each job's fresh checkout is made (and removed).
	WorkDir string `json:"work_dir"`
	// AllowedRepos are the only repositories it touches: owner/name,
	// owner/* for all of an owner's, or a local path (a test, a mirror).
	AllowedRepos []string `json:"allowed_repos"`
	// Concurrency is how many jobs at once (1 unless set).
	Concurrency int `json:"concurrency"`
	// Agent is the command run in the checkout, with {prompt_file},
	// {workdir}, {branch} and {job_id} replaced.
	Agent string `json:"agent_command"`
	// TimeoutMinutes caps a job here, whatever Runwell allows (0: Runwell's).
	TimeoutMinutes int `json:"timeout_minutes,omitempty"`
	// NoPush and NoPR are for a dry run, or a remote that isn't GitHub.
	NoPush bool `json:"no_push,omitempty"`
	NoPR   bool `json:"no_pr,omitempty"`
	// Name is how it introduces itself (the hostname unless set).
	Name string `json:"name,omitempty"`
}

const defaultAgent = "claude -p --output-format stream-json --verbose --permission-mode acceptEdits < {prompt_file}"

func defaultConfigPath() string {
	if p := os.Getenv("RUNWELL_RUNNER_CONFIG"); p != "" {
		return p
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "runwell-runner", "config.json")
}

func loadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("no config at %s: run `runwell-runner init` first", path)
	} else if err != nil {
		return nil, err
	}
	var c Config
	if err := json.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if t := os.Getenv("RUNWELL_RUNNER_TOKEN"); t != "" {
		c.Token = t // a secret store's, rather than the file's
	}
	return &c, c.check()
}

func (c *Config) check() error {
	var errs []string
	if u, err := url.Parse(c.URL); err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		errs = append(errs, "url must be Runwell's address, like https://runwell.example.com")
	}
	if !strings.HasPrefix(c.Token, "rwm_") {
		errs = append(errs, "token must be the runner's token from Settings › Runners (rwm_…)")
	}
	if len(c.AllowedRepos) == 0 {
		errs = append(errs, "allowed_repos must name at least one repository (owner/name or owner/*)")
	}
	if c.WorkDir == "" {
		errs = append(errs, "work_dir can't be blank")
	}
	if strings.TrimSpace(c.Agent) == "" {
		errs = append(errs, "agent_command can't be blank")
	}
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// allows is whether repo is on the runner's own allowlist.
func (c *Config) allows(repo string) bool {
	repo = normalRepo(repo)
	if repo == "" {
		return false
	}
	for _, p := range c.AllowedRepos {
		p = normalRepo(p)
		if p == repo || (strings.HasSuffix(p, "/*") && !isLocal(p) && strings.HasPrefix(repo, strings.TrimSuffix(p, "*")) && !strings.Contains(strings.TrimPrefix(repo, strings.TrimSuffix(p, "*")), "/")) {
			return true
		}
	}
	return false
}

func isLocal(repo string) bool {
	return strings.HasPrefix(repo, "/") || strings.HasPrefix(repo, "file://")
}

// normalRepo is owner/name in lowercase (a local path as it is).
func normalRepo(s string) string {
	s = strings.TrimSpace(s)
	if isLocal(s) {
		return strings.TrimSuffix(s, "/")
	}
	s = strings.ToLower(s)
	for _, p := range []string{"https://github.com/", "http://github.com/", "git@github.com:", "github.com/"} {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
}

func initCmd(args []string, out io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	path := fs.String("config", defaultConfigPath(), "")
	c := Config{}
	fs.StringVar(&c.URL, "url", "", "")
	fs.StringVar(&c.Token, "token", "", "")
	fs.StringVar(&c.WorkDir, "work-dir", "", "")
	repos := fs.String("repos", "", "")
	fs.IntVar(&c.Concurrency, "concurrency", 1, "")
	fs.StringVar(&c.Agent, "agent", defaultAgent, "")
	fs.IntVar(&c.TimeoutMinutes, "timeout", 0, "")
	fs.StringVar(&c.Name, "name", "", "")
	force := fs.Bool("force", false, "")
	if err := parseFlags(fs, args); err != nil {
		return err
	}
	for _, r := range strings.FieldsFunc(*repos, func(r rune) bool { return r == ',' || r == ' ' }) {
		c.AllowedRepos = append(c.AllowedRepos, normalRepo(r))
	}
	c.URL = strings.TrimSuffix(strings.TrimSpace(c.URL), "/")
	if c.WorkDir == "" {
		cache, err := os.UserCacheDir()
		if err != nil {
			cache = os.TempDir()
		}
		c.WorkDir = filepath.Join(cache, "runwell-runner", "jobs")
	}
	if err := c.check(); err != nil {
		return usageError{err}
	}
	if _, err := os.Stat(*path); err == nil && !*force {
		return fmt.Errorf("%s is there already: --force to replace it", *path)
	}
	if err := os.MkdirAll(filepath.Dir(*path), 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(c.WorkDir, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	// The token is in it: only this user reads it.
	if err := os.WriteFile(*path, append(b, '\n'), 0o600); err != nil {
		return err
	}
	fmt.Fprintf(out, "Wrote %s.\nNext: runwell-runner doctor, then runwell-runner run.\n", *path)
	return nil
}
