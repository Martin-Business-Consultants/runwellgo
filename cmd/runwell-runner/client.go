package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// client talks to the Runners plugin's machine routes with the runner's
// token.
type client struct {
	base  string // https://runwell.example.com/api/x/runners/machine
	token string
	http  *http.Client
}

func newClient(c *Config) *client {
	return &client{base: strings.TrimSuffix(c.URL, "/") + "/api/x/runners/machine", token: c.Token, http: &http.Client{Timeout: 30 * time.Second}}
}

// apiError is Runwell refusing a call.
type apiError struct {
	Status  int
	Message string
}

func (e *apiError) Error() string { return fmt.Sprintf("runwell answered %d: %s", e.Status, e.Message) }

func (c *client) call(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "runwell-runner/"+version)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 {
		var e struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(data, &e)
		if e.Message == "" {
			e.Message = strings.TrimSpace(string(data[:min(len(data), 300)]))
		}
		return &apiError{Status: res.StatusCode, Message: e.Message}
	}
	if out != nil && len(data) > 0 {
		return json.Unmarshal(data, out)
	}
	return nil
}

// Beat is what a heartbeat answers.
type Beat struct {
	Name        string    `json:"name"`
	Paused      bool      `json:"paused"`
	PollSeconds int       `json:"poll_seconds"`
	Cancel      []int64   `json:"cancel"`
	Cleanup     []Cleanup `json:"cleanup"`
}

// Cleanup is a declined job whose branch and pull request should go.
type Cleanup struct {
	JobID  int64  `json:"job_id"`
	Repo   string `json:"repo"`
	Branch string `json:"branch"`
	PRURL  string `json:"pr_url"`
}

// Job is a job as the runner gets it.
type Job struct {
	ID             int64  `json:"id"`
	Record         string `json:"record"`
	Title          string `json:"title"`
	Repo           string `json:"repo"`
	BaseBranch     string `json:"base_branch"`
	Branch         string `json:"branch"`
	Instructions   string `json:"instructions"`
	Brief          string `json:"brief"`
	TimeoutMinutes int    `json:"timeout_minutes"`
	WorkURL        string `json:"work_url"`
	JobURL         string `json:"job_url"`
	RequestedBy    string `json:"requested_by"`
}

// Line is one line of a job's log.
type Line struct {
	Stream string `json:"stream"`
	Text   string `json:"text"`
	At     string `json:"at"`
}

// Result is how a job went.
type Result struct {
	Outcome         string   `json:"outcome"` // changes, no_changes, failed, refused, cancelled
	PRURL           string   `json:"pr_url,omitempty"`
	Branch          string   `json:"branch,omitempty"`
	Summary         string   `json:"summary,omitempty"`
	FilesChanged    []string `json:"files_changed"`
	DurationSeconds int64    `json:"duration_seconds"`
	Error           string   `json:"error,omitempty"`
}

func (c *client) heartbeat(ctx context.Context, info map[string]any) (*Beat, error) {
	var b Beat
	return &b, c.call(ctx, "POST", "/heartbeat", info, &b)
}

func (c *client) claim(ctx context.Context, repos []string) (*Job, error) {
	var out struct {
		Job *Job `json:"job"`
	}
	err := c.call(ctx, "POST", "/claim", map[string]any{"allowed_repos": repos}, &out)
	return out.Job, err
}

// events sends log lines, and answers whether to stop.
func (c *client) events(ctx context.Context, id int64, lines []Line) (bool, error) {
	var out struct {
		Cancel bool `json:"cancel"`
	}
	err := c.call(ctx, "POST", fmt.Sprintf("/jobs/%d/events", id), map[string]any{"events": lines}, &out)
	return out.Cancel, err
}

func (c *client) cancelled(ctx context.Context, id int64) (bool, error) {
	var out struct {
		Cancel bool `json:"cancel"`
	}
	err := c.call(ctx, "GET", fmt.Sprintf("/jobs/%d/cancel", id), nil, &out)
	return out.Cancel, err
}

func (c *client) finish(ctx context.Context, id int64, r Result) error {
	if r.FilesChanged == nil {
		r.FilesChanged = []string{}
	}
	return c.call(ctx, "POST", fmt.Sprintf("/jobs/%d/finish", id), r, nil)
}

func (c *client) cleaned(ctx context.Context, id int64, errText string) error {
	return c.call(ctx, "POST", fmt.Sprintf("/jobs/%d/cleaned", id), map[string]any{"error": errText}, nil)
}
