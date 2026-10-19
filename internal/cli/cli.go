// Package cli is runwell working on an install from a terminal or an AI
// app, over that install's API: `runwell login`, then any of its tools by
// name (`runwell list_clients --status all`), `runwell tools` for what
// there is, `runwell doctor` for whether it works, and `runwell mcp` for
// an AI app that wants a local MCP server.
//
// The tools are the site's own (GET /api/agent/tools), so this always
// matches the version of the site it talks to, and a tool runs on the site
// as an MCP call (POST /mcp): the command line and AI apps go through the
// same code, as the signed-in person or token, with that role.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Profile is one install and the credential used on it: a personal token
// pasted in, or an OAuth token (signed in through the browser) with what
// refreshes it.
type Profile struct {
	URL   string `json:"url"`
	Token string `json:"token"`
	// OAuth's: the client this CLI registered as, and the refresh token.
	ClientID     string `json:"client_id,omitempty"`
	RefreshToken string `json:"refresh_token,omitempty"`

	name string // the profile's name, to save a refreshed token under
}

type profiles struct {
	Default  string             `json:"default"`
	Profiles map[string]Profile `json:"profiles"`
}

// configPath is where profiles are kept: ~/.config/runwell/profiles.json
// (or the platform's config directory), readable by its owner alone.
func configPath() (string, error) {
	if p := os.Getenv("RUNWELL_CONFIG"); p != "" {
		return p, nil
	}
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "runwell", "profiles.json"), nil
}

func loadProfiles() (*profiles, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}
	ps := &profiles{Profiles: map[string]Profile{}}
	raw, err := os.ReadFile(path) //nolint:gosec // the user's own config
	if errors.Is(err, os.ErrNotExist) {
		return ps, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, ps); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if ps.Profiles == nil {
		ps.Profiles = map[string]Profile{}
	}
	return ps, nil
}

func (ps *profiles) save() error {
	path, err := configPath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o600)
}

// ErrNoProfile is a command that needs a site, run before `runwell login`.
var ErrNoProfile = errors.New("not signed in to a site: run `runwell login https://runwell.example.com` (or set RUNWELL_URL and RUNWELL_TOKEN)")

// resolve is the site a command works on: RUNWELL_URL and
// RUNWELL_TOKEN when set (CI, one-offs), else the named profile
// (--profile, RUNWELL_PROFILE), else the default one.
func resolve(name string) (Profile, string, error) {
	if u, t := os.Getenv("RUNWELL_URL"), os.Getenv("RUNWELL_TOKEN"); u != "" && t != "" && name == "" {
		return Profile{URL: strings.TrimRight(u, "/"), Token: t}, "environment", nil
	}
	ps, err := loadProfiles()
	if err != nil {
		return Profile{}, "", err
	}
	if name == "" {
		name = os.Getenv("RUNWELL_PROFILE")
	}
	if name == "" {
		name = ps.Default
	}
	p, ok := ps.Profiles[name]
	p.name = name
	if !ok || name == "" {
		if name != "" && len(ps.Profiles) > 0 {
			return Profile{}, "", fmt.Errorf("no profile %q: `runwell profiles` lists them", name)
		}
		return Profile{}, "", ErrNoProfile
	}
	return p, name, nil
}

// client calls one site.
type client struct {
	Profile
	http *http.Client
}

func newClient(p Profile) *client {
	return &client{Profile: p, http: &http.Client{Timeout: 5 * time.Minute}}
}

// do is one request: its status and body. An OAuth token that has expired
// is refreshed (and kept) and the request sent again.
func (c *client) do(ctx context.Context, method, path string, body any) (int, []byte, error) {
	status, out, err := c.once(ctx, method, path, body)
	if err == nil && status == http.StatusUnauthorized && c.RefreshToken != "" && c.refresh(ctx) == nil {
		return c.once(ctx, method, path, body)
	}
	return status, out, err
}

// refresh trades the refresh token for a new pair and saves it in the
// profile.
func (c *client) refresh(ctx context.Context) error {
	form := url.Values{"grant_type": {"refresh_token"}, "client_id": {c.ClientID}, "refresh_token": {c.RefreshToken}}
	tok, err := tokenRequest(ctx, c.http, c.URL, form)
	if err != nil {
		return err
	}
	c.Token, c.RefreshToken = tok.AccessToken, tok.RefreshToken
	if c.name == "" || c.name == "environment" {
		return nil
	}
	ps, err := loadProfiles()
	if err != nil {
		return err
	}
	p := ps.Profiles[c.name]
	p.Token, p.RefreshToken = c.Token, c.RefreshToken
	ps.Profiles[c.name] = p
	return ps.save()
}

func (c *client) once(ctx context.Context, method, path string, body any) (int, []byte, error) {
	var r io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return 0, nil, err
		}
		r = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.URL+path, r)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	req.Header.Set("User-Agent", "runwell")
	res, err := c.http.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("can't reach %s: %w", c.URL, err)
	}
	defer res.Body.Close()
	out, err := io.ReadAll(io.LimitReader(res.Body, 256<<20))
	return res.StatusCode, out, err
}

// Tool is one of the site's tools, as its catalogue has it.
type Tool struct {
	Name    string `json:"name"`
	Summary string `json:"summary"`
	Notes   string `json:"notes"`
	Method  string `json:"method"`
	Path    string `json:"path"`
	Params  []struct {
		Name     string `json:"name"`
		In       string `json:"in"`
		Type     string `json:"type"`
		Desc     string `json:"description"`
		Required bool   `json:"required"`
	} `json:"params"`
	// InputSchema is the tool's arguments as MCP has them: the fields of a
	// wrapped body (--client.name), with their types.
	InputSchema map[string]any `json:"input_schema"`
}

func (c *client) catalogue(ctx context.Context) ([]Tool, string, error) {
	status, body, err := c.do(ctx, "GET", "/api/agent/tools", nil)
	if err != nil {
		return nil, "", err
	}
	if status != 200 {
		return nil, "", apiError(status, body)
	}
	var out struct {
		Tools   []Tool `json:"tools"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, "", fmt.Errorf("the site's tools: %w", err)
	}
	return out.Tools, out.Version, nil
}

// call runs a tool on the site (as an MCP tools/call): the answer, in the
// envelope, and whether the site called it an error.
func (c *client) call(ctx context.Context, tool string, args map[string]any) ([]byte, bool, error) {
	status, body, err := c.do(ctx, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": tool, "arguments": args}})
	if err != nil {
		return nil, false, err
	}
	if status != 200 {
		return nil, false, apiError(status, body)
	}
	var res struct {
		Result struct {
			Content []struct {
				Text string `json:"text"`
			} `json:"content"`
			IsError bool `json:"isError"`
		} `json:"result"`
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, false, err
	}
	if res.Error != nil {
		return nil, false, errors.New(res.Error.Message)
	}
	var text string
	if len(res.Result.Content) > 0 {
		text = res.Result.Content[0].Text
	}
	return []byte(text), res.Result.IsError, nil
}

// apiError is an error answer in words.
func apiError(status int, body []byte) error {
	var e struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	var env struct {
		Code    string `json:"code"`
		Summary string `json:"summary"`
		Hint    string `json:"hint"`
	}
	if json.Unmarshal(body, &env) == nil && env.Summary != "" {
		if env.Hint != "" {
			return fmt.Errorf("%s (%s): %s", env.Summary, env.Code, env.Hint)
		}
		return fmt.Errorf("%s (%d)", env.Summary, status)
	}
	if json.Unmarshal(body, &e) == nil && e.Message != "" {
		if status == 401 {
			return fmt.Errorf("%s (%d): sign in again with `runwell login`", e.Message, status)
		}
		return fmt.Errorf("%s (%d)", e.Message, status)
	}
	return fmt.Errorf("the site answered %d: %s", status, strings.TrimSpace(string(body[:min(len(body), 300)])))
}

// sortedNames is a map's keys, sorted.
func sortedNames[V any](m map[string]V) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
