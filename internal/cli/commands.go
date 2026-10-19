package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
)

// Env is where a command reads and writes, and this binary's version.
type Env struct {
	Stdin          io.Reader
	Stdout, Stderr io.Writer
	Version        string
}

// ErrUnknown is a command that's neither runwell's nor one of the
// site's tools.
var ErrUnknown = errors.New("unknown command")

// ErrFailed is a tool the site answered with an error (already printed).
var ErrFailed = errors.New("the tool failed")

// Commands are the client commands, for main's dispatch.
var Commands = []string{"login", "logout", "profiles", "whoami", "tools", "doctor", "mcp", "help"}

// Run runs a client command, or a tool by name.
func Run(ctx context.Context, env Env, cmd string, args []string) error {
	switch cmd {
	case "login":
		return login(ctx, env, args)
	case "logout":
		return logout(env, args)
	case "profiles":
		return listProfiles(env, args)
	case "whoami":
		return runTool(ctx, env, "me", args)
	case "help":
		if len(args) == 0 {
			return ErrUnknown
		}
		return tools(ctx, env, args)
	case "tools":
		return tools(ctx, env, args)
	case "doctor":
		return doctor(ctx, env, args)
	case "mcp":
		return mcp(ctx, env, args)
	}
	return runTool(ctx, env, cmd, args)
}

// flags pulls --profile and --json out of args.
func flags(args []string) (profile string, jsonOut bool, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--json":
			jsonOut = true
		case a == "--profile" && i+1 < len(args):
			profile = args[i+1]
			i++
		case strings.HasPrefix(a, "--profile="):
			profile = strings.TrimPrefix(a, "--profile=")
		default:
			rest = append(rest, a)
		}
	}
	return profile, jsonOut, rest
}

// ---- login ------------------------------------------------------------------

func login(ctx context.Context, env Env, args []string) error {
	var site, name, token string
	noBrowser, paste, readOnly := false, false, false
	for i := 0; i < len(args); i++ {
		switch a := args[i]; {
		case a == "--name" && i+1 < len(args):
			name, i = args[i+1], i+1
		case a == "--token" && i+1 < len(args):
			token, i = args[i+1], i+1
		case a == "--paste":
			paste = true
		case a == "--read-only":
			readOnly = true
		case a == "--no-browser":
			noBrowser = true
		case strings.HasPrefix(a, "-"):
			return fmt.Errorf("login: unknown flag %s", a)
		default:
			site = a
		}
	}
	if site == "" {
		return errors.New("usage: runwell login https://runwell.example.com [--name NAME] [--token TOKEN | --paste] [--read-only] [--no-browser]")
	}
	if !strings.Contains(site, "://") {
		site = "https://" + site
	}
	u, err := url.Parse(site)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q isn't an install's address", site)
	}
	site = strings.TrimRight(u.Scheme+"://"+u.Host+u.Path, "/")
	if name == "" {
		name = u.Hostname()
	}
	c := newClient(Profile{URL: site})
	profile := Profile{URL: site, Token: token}
	switch {
	case token == "-" || paste:
		if profile.Token, err = pasteToken(env, site); err != nil {
			return err
		}
	case token == "":
		if profile, err = browserLogin(ctx, env, c, noBrowser, readOnly); err != nil {
			return err
		}
	}
	c = newClient(profile)
	status, body, err := c.do(ctx, "GET", "/api/me", nil)
	if err != nil {
		return err
	} else if status != 200 {
		return apiError(status, body)
	}
	var me struct {
		Name string `json:"name"`
		Role string `json:"role"`
	}
	_ = json.Unmarshal(body, &me)
	ps, err := loadProfiles()
	if err != nil {
		return err
	}
	ps.Profiles[name] = profile
	if ps.Default == "" || len(ps.Profiles) == 1 {
		ps.Default = name
	}
	if err := ps.save(); err != nil {
		return err
	}
	path, _ := configPath()
	fmt.Fprintf(env.Stdout, "Signed in to %s as %s (%s), profile %q (kept in %s).\n", site, me.Name, me.Role, name, path)
	if ps.Default != name {
		fmt.Fprintf(env.Stdout, "Use it with --profile %s, or make it the default: runwell profiles --default %s\n", name, name)
	}
	fmt.Fprintln(env.Stdout, "Next: runwell briefing, runwell tools, runwell mcp setup")
	return nil
}

// openBrowser opens the site's sign-in page (an argument, never a shell
// line); where it can't, the address printed above does.
func openBrowser(ctx context.Context, u string) {
	name, args := "xdg-open", []string{u}
	switch runtime.GOOS {
	case "darwin":
		name = "open"
	case "windows":
		name, args = "rundll32", []string{"url.dll,FileProtocolHandler", u}
	}
	_ = exec.CommandContext(ctx, name, args...).Start() //nolint:gosec // a fixed program, the site's own address
}

func logout(env Env, args []string) error {
	ps, err := loadProfiles()
	if err != nil {
		return err
	}
	name := ps.Default
	if len(args) > 0 {
		name = strings.TrimPrefix(args[len(args)-1], "--profile=")
	}
	if _, ok := ps.Profiles[name]; !ok {
		return fmt.Errorf("no profile %q", name)
	}
	delete(ps.Profiles, name)
	if ps.Default == name {
		ps.Default = ""
		if names := sortedNames(ps.Profiles); len(names) > 0 {
			ps.Default = names[0]
		}
	}
	if err := ps.save(); err != nil {
		return err
	}
	fmt.Fprintf(env.Stdout, "Signed out of %q (the token still works until it's disconnected in Settings › Connected apps).\n", name)
	return nil
}

func listProfiles(env Env, args []string) error {
	ps, err := loadProfiles()
	if err != nil {
		return err
	}
	if len(ps.Profiles) == 0 {
		return ErrNoProfile
	}
	// --default NAME: the profile commands use unless told otherwise.
	if len(args) == 2 && args[0] == "--default" {
		if _, ok := ps.Profiles[args[1]]; !ok {
			return fmt.Errorf("no profile %q", args[1])
		}
		ps.Default = args[1]
		if err := ps.save(); err != nil {
			return err
		}
	}
	for _, n := range sortedNames(ps.Profiles) {
		mark := " "
		if n == ps.Default {
			mark = "*"
		}
		fmt.Fprintf(env.Stdout, "%s %-24s %s\n", mark, n, ps.Profiles[n].URL)
	}
	return nil
}

// ---- tools ------------------------------------------------------------------

func tools(ctx context.Context, env Env, args []string) error {
	profile, jsonOut, rest := flags(args)
	p, _, err := resolve(profile)
	if err != nil {
		return err
	}
	list, version, err := newClient(p).catalogue(ctx)
	if err != nil {
		return err
	}
	if jsonOut {
		return json.NewEncoder(env.Stdout).Encode(map[string]any{"tools": list, "version": version})
	}
	if len(rest) > 0 {
		i := slices.IndexFunc(list, func(t Tool) bool { return t.Name == toolName(rest[0]) })
		if i < 0 {
			return fmt.Errorf("no tool %s on %s", rest[0], p.URL)
		}
		printTool(env.Stdout, list[i])
		return nil
	}
	fmt.Fprintf(env.Stdout, "%s (Runwell %s): %d tools. `runwell tools NAME` for one in full; `--json` for all of them.\n\n", p.URL, version, len(list))
	for _, t := range list {
		fmt.Fprintf(env.Stdout, "  %-26s %s\n", t.Name, t.Summary)
	}
	return nil
}

func printTool(w io.Writer, t Tool) {
	fmt.Fprintf(w, "runwell %s", t.Name)
	for _, p := range t.Params {
		flag := "--" + strings.ReplaceAll(p.Name, "_", "-") + " " + strings.ToUpper(p.Type)
		if !p.Required {
			flag = "[" + flag + "]"
		}
		fmt.Fprint(w, " "+flag)
	}
	fmt.Fprintf(w, "\n\n%s\n", t.Summary)
	if t.Notes != "" {
		fmt.Fprintf(w, "\n%s\n", t.Notes)
	}
	if len(t.Params) > 0 {
		fmt.Fprintln(w)
		props, _ := t.InputSchema["properties"].(map[string]any)
		for _, p := range t.Params {
			fmt.Fprintf(w, "  --%-18s %s\n", strings.ReplaceAll(p.Name, "_", "-"), p.Desc)
			// A wrapped body's fields, one flag each: --client.name Bloom.
			schema, _ := props[p.Name].(map[string]any)
			fields, _ := schema["properties"].(map[string]any)
			required, _ := schema["required"].([]any)
			for _, f := range sortedNames(fields) {
				spec, _ := fields[f].(map[string]any)
				note := fmt.Sprint(spec["type"])
				if enum, ok := spec["enum"].([]any); ok {
					parts := make([]string, len(enum))
					for i, e := range enum {
						parts[i] = fmt.Sprint(e)
					}
					note = strings.Join(parts, "|")
				}
				if slices.Contains(required, any(f)) {
					note += ", required"
				}
				fmt.Fprintf(w, "    --%s.%-14s %s\n", p.Name, f, note)
			}
		}
	}
	if t.Method != "ANY" {
		fmt.Fprintf(w, "\n(%s %s)\n", t.Method, t.Path)
	}
	fmt.Fprintln(w, "\nObjects and lists are JSON: a value, @file.json, or - for stdin. --json prints the answer as the site sent it.")
}

func toolName(s string) string { return strings.ReplaceAll(s, "-", "_") }

// runTool parses a tool's arguments against its catalogue entry, runs it on
// the site, and prints the answer.
func runTool(ctx context.Context, env Env, name string, args []string) error {
	profile, jsonOut, rest := flags(args)
	p, _, err := resolve(profile)
	if errors.Is(err, ErrNoProfile) {
		return fmt.Errorf("%w %q: not runwell's, and not signed in to a site to look for a tool of that name (runwell login https://runwell.example.com)", ErrUnknown, name)
	}
	if err != nil {
		return err
	}
	c := newClient(p)
	list, _, err := c.catalogue(ctx)
	if err != nil {
		return err
	}
	i := slices.IndexFunc(list, func(t Tool) bool { return t.Name == toolName(name) })
	if i < 0 {
		return fmt.Errorf("%w %q: not runwell's, and %s has no such tool (runwell tools lists them)", ErrUnknown, name, p.URL)
	}
	t := list[i]
	toolArgs, err := parseArgs(env, t, rest)
	if err != nil {
		return fmt.Errorf("%s: %w (runwell tools %s)", t.Name, err, t.Name)
	}
	out, isErr, err := c.call(ctx, t.Name, toolArgs)
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Fprintln(env.Stdout, string(out))
	} else {
		printAnswer(env, out, isErr)
	}
	if isErr {
		return ErrFailed
	}
	return nil
}

// parseArgs reads --name value pairs (and positional values for the
// required path parameters, in order) into a tool's arguments.
func parseArgs(env Env, t Tool, args []string) (map[string]any, error) {
	out := map[string]any{}
	byName := map[string]int{}
	var positional []int
	for i, p := range t.Params {
		byName[p.Name] = i
		if p.In == "path" {
			positional = append(positional, i)
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if !strings.HasPrefix(a, "--") {
			if len(positional) == 0 {
				return nil, fmt.Errorf("unexpected %q", a)
			}
			p := t.Params[positional[0]]
			positional = positional[1:]
			out[p.Name] = a
			continue
		}
		key, val, hasVal := strings.Cut(strings.TrimPrefix(a, "--"), "=")
		key = toolName(key)
		// --confirm (once the person agreed to what a preview said) and
		// --idempotency-key steer the call itself.
		if key == "confirm" && !hasVal {
			out["confirm"] = true
			continue
		}
		if key == "idempotency_key" {
			if !hasVal && i+1 < len(args) {
				val, i = args[i+1], i+1
			}
			out["idempotency_key"] = val
			continue
		}
		// --client.name Bloom: one field of a wrapped body.
		if head, field, nested := strings.Cut(key, "."); nested {
			idx, ok := byName[head]
			if !ok || t.Params[idx].Type != "object" {
				return nil, fmt.Errorf("no --%s", strings.ReplaceAll(head, "_", "-"))
			}
			if !hasVal {
				if i+1 >= len(args) {
					return nil, fmt.Errorf("--%s needs a value", a[2:])
				}
				val, i = args[i+1], i+1
			}
			v, err := value(env, fieldType(t, head, field), val)
			if err != nil {
				return nil, fmt.Errorf("--%s: %w", a[2:], err)
			}
			obj, _ := out[head].(map[string]any)
			if obj == nil {
				obj = map[string]any{}
			}
			obj[field] = v
			out[head] = obj
			continue
		}
		idx, ok := byName[key]
		if !ok {
			return nil, fmt.Errorf("no --%s", strings.ReplaceAll(key, "_", "-"))
		}
		p := t.Params[idx]
		if !hasVal {
			switch {
			case p.Type == "boolean" && (i+1 >= len(args) || strings.HasPrefix(args[i+1], "--")):
				val = "true"
			case i+1 < len(args):
				val, i = args[i+1], i+1
			default:
				return nil, fmt.Errorf("--%s needs a value", a[2:])
			}
		}
		v, err := value(env, p.Type, val)
		if err != nil {
			return nil, fmt.Errorf("--%s: %w", strings.ReplaceAll(key, "_", "-"), err)
		}
		out[p.Name] = v
		positional = slices.DeleteFunc(positional, func(j int) bool { return j == idx })
	}
	for _, p := range t.Params {
		if _, ok := out[p.Name]; p.Required && !ok {
			return nil, fmt.Errorf("--%s is required", strings.ReplaceAll(p.Name, "_", "-"))
		}
	}
	return out, nil
}

// fieldType is a wrapped body's field's type, from the tool's input schema
// (string when it doesn't say).
func fieldType(t Tool, param, field string) string {
	props, _ := t.InputSchema["properties"].(map[string]any)
	p, _ := props[param].(map[string]any)
	fields, _ := p["properties"].(map[string]any)
	f, _ := fields[field].(map[string]any)
	switch v := f["type"].(type) {
	case string:
		return v
	case []any:
		if len(v) > 0 && v[0] == "integer" {
			return "id"
		}
	}
	return "string"
}

// value is a command-line value as its type: JSON for objects and lists
// (inline, @file or - for stdin), a file's contents for a file.
func value(env Env, typ, raw string) (any, error) {
	switch typ {
	case "id":
		if n, err := strconv.Atoi(raw); err == nil {
			return n, nil
		}
		return raw, nil // a name, which the install looks up
	case "integer":
		if n, err := strconv.Atoi(raw); err == nil {
			return n, nil
		}
		return raw, nil
	case "boolean":
		return strconv.ParseBool(raw)
	case "object", "array":
		data := []byte(raw)
		switch {
		case raw == "-":
			b, err := io.ReadAll(env.Stdin)
			if err != nil {
				return nil, err
			}
			data = b
		case strings.HasPrefix(raw, "@"):
			b, err := os.ReadFile(raw[1:])
			if err != nil {
				return nil, err
			}
			data = b
		}
		var v any
		if err := json.Unmarshal(data, &v); err != nil {
			return nil, fmt.Errorf("isn't JSON: %w", err)
		}
		return v, nil
	case "file":
		b, err := os.ReadFile(raw) //nolint:gosec // the file the person named
		if err != nil {
			return nil, err
		}
		return map[string]any{"filename": filepath.Base(raw), "content_base64": base64.StdEncoding.EncodeToString(b)}, nil
	}
	return raw, nil
}

// printAnswer is an envelope for a person: the summary, the data, and the
// next steps.
func printAnswer(env Env, raw []byte, isErr bool) {
	var e struct {
		Status     string          `json:"status"`
		Summary    string          `json:"summary"`
		Data       json.RawMessage `json:"data"`
		Message    string          `json:"message"`
		Code       string          `json:"code"`
		Hint       string          `json:"hint"`
		Candidates []struct {
			Record string `json:"record"`
			Name   string `json:"name"`
			Client string `json:"client"`
		} `json:"candidates"`
		Errors      map[string][]string `json:"errors"`
		CallAgain   map[string]any      `json:"call_again_with"`
		Breadcrumbs []struct {
			Description string `json:"description"`
			Command     string `json:"command"`
		} `json:"breadcrumbs"`
	}
	if json.Unmarshal(raw, &e) != nil || (e.Summary == "" && e.Data == nil && e.Message == "") {
		w := env.Stdout
		if isErr {
			w = env.Stderr
		}
		fmt.Fprintln(w, strings.TrimSpace(string(raw)))
		return
	}
	if isErr {
		msg := e.Message
		if msg == "" {
			msg = e.Summary
		}
		if msg == "" {
			msg = string(raw)
		}
		fmt.Fprintln(env.Stderr, msg)
		for field, list := range e.Errors {
			for _, m := range list {
				fmt.Fprintf(env.Stderr, "  %s %s\n", field, m)
			}
		}
		for _, c := range e.Candidates {
			fmt.Fprintf(env.Stderr, "  %s  %s %s\n", c.Record, c.Name, c.Client)
		}
		if e.Hint != "" {
			fmt.Fprintf(env.Stderr, "%s (%s)\n", e.Hint, e.Code)
		}
		return
	}
	if e.Status == "needs_confirmation" {
		fmt.Fprintln(env.Stdout, e.Summary)
		fmt.Fprintln(env.Stdout, "Run it again with --confirm once the person agrees.")
		return
	}
	if e.Summary != "" {
		fmt.Fprintln(env.Stdout, e.Summary)
	}
	if len(e.Data) > 0 && string(e.Data) != "null" {
		var pretty bytes.Buffer
		if json.Indent(&pretty, e.Data, "", "  ") == nil {
			fmt.Fprintln(env.Stdout, pretty.String())
		}
	}
	if len(e.Breadcrumbs) > 0 {
		fmt.Fprintln(env.Stdout, "\nNext:")
		for _, b := range e.Breadcrumbs {
			fmt.Fprintf(env.Stdout, "  %s\n    %s\n", b.Description, b.Command)
		}
	}
}

// ---- doctor -----------------------------------------------------------------

func doctor(ctx context.Context, env Env, args []string) error {
	profile, _, _ := flags(args)
	ok := true
	check := func(pass bool, what, fix string) {
		mark := "ok  "
		if !pass {
			mark, ok = "FAIL", false
		}
		fmt.Fprintf(env.Stdout, "%s %s\n", mark, what)
		if !pass && fix != "" {
			fmt.Fprintf(env.Stdout, "     %s\n", fix)
		}
	}
	p, name, err := resolve(profile)
	check(err == nil, "signed in to a site ("+name+")", "runwell login https://runwell.example.com")
	if err != nil {
		return ErrFailed
	}
	c := newClient(p)
	status, _, err := c.do(ctx, "GET", "/up", nil)
	check(err == nil && status == 200, p.URL+" answers", "is the address right, and is it running?")
	status, body, err := c.do(ctx, "GET", "/api/me", nil)
	check(err == nil && status == 200, "the token works", "sign in again: runwell login "+p.URL)
	if err == nil && status == 200 {
		var m map[string]any
		_ = json.Unmarshal(body, &m)
		fmt.Fprintf(env.Stdout, "     as %v, role %v\n", m["name"], m["role"])
	}
	list, version, err := c.catalogue(ctx)
	check(err == nil && len(list) > 0, fmt.Sprintf("its tools (%d)", len(list)), "")
	if err == nil {
		same := strings.TrimPrefix(version, "v") == strings.TrimPrefix(env.Version, "v")
		check(same || env.Version == "dev" || version == "dev", fmt.Sprintf("this runwell (%s) and the site (%s) match", env.Version, version),
			"tools come from the site, so they work anyway; to match, run runwell update (or install the site's version)")
	}
	status, _, err = c.do(ctx, "POST", "/mcp", map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{}})
	check(err == nil && status == 200, "its MCP server answers", "")
	if !ok {
		return ErrFailed
	}
	fmt.Fprintln(env.Stdout, "\nAll good. AI apps: runwell mcp setup")
	return nil
}

// ---- MCP --------------------------------------------------------------------

// mcp is a local MCP server for AI apps that start one (stdio): each
// message goes to the site's own /mcp, signed in as the profile. `mcp
// setup` says how to add it to an app.
func mcp(ctx context.Context, env Env, args []string) error {
	profile, _, rest := flags(args)
	if len(rest) > 0 && rest[0] == "setup" {
		return mcpSetup(env, profile, rest[1:])
	}
	p, _, err := resolve(profile)
	if err != nil {
		return err
	}
	c := newClient(p)
	in := bufio.NewScanner(env.Stdin)
	in.Buffer(make([]byte, 1<<20), 64<<20)
	for in.Scan() {
		line := bytes.TrimSpace(in.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg any
		if err := json.Unmarshal(line, &msg); err != nil {
			fmt.Fprintln(env.Stdout, `{"jsonrpc":"2.0","id":null,"error":{"code":-32700,"message":"not JSON"}}`)
			continue
		}
		status, body, err := c.do(ctx, "POST", "/mcp", msg)
		if err != nil {
			fmt.Fprintf(env.Stderr, "runwell mcp: %v\n", err)
			if id := idOf(msg); id != nil {
				out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32603, "message": err.Error()}})
				fmt.Fprintln(env.Stdout, string(out))
			}
			continue
		}
		if status == 202 || len(bytes.TrimSpace(body)) == 0 {
			continue // a notification
		}
		if status != 200 {
			if id := idOf(msg); id != nil {
				out, _ := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": -32603, "message": apiError(status, body).Error()}})
				fmt.Fprintln(env.Stdout, string(out))
			}
			continue
		}
		fmt.Fprintln(env.Stdout, string(bytes.TrimSpace(body)))
	}
	return in.Err()
}

func idOf(msg any) any {
	if m, ok := msg.(map[string]any); ok {
		return m["id"]
	}
	return nil
}

func mcpSetup(env Env, profile string, args []string) error {
	p, name, err := resolve(profile)
	if err != nil {
		return err
	}
	exe, _ := os.Executable()
	if exe == "" {
		exe = "runwell"
	}
	server := "runwell"
	if name != "" && name != "environment" {
		server = "runwell-" + strings.ReplaceAll(strings.SplitN(name, ".", 2)[0], " ", "-")
	}
	local := fmt.Sprintf("%s mcp --profile %s", exe, name)
	cfg, _ := json.MarshalIndent(map[string]any{"mcpServers": map[string]any{server: map[string]any{"command": exe, "args": []string{"mcp", "--profile", name}}}}, "", "  ")
	which := ""
	if len(args) > 0 {
		which = args[0]
	}
	all := which == ""
	w := env.Stdout
	fmt.Fprintf(w, "Runwell at %s, as profile %q.\n", p.URL, name)
	if all || which == "claude-code" {
		fmt.Fprintf(w, "\nClaude Code, one command:\n\n    claude mcp add --scope user %s -- %s\n", server, local)
		fmt.Fprintf(w, "\n  or straight to the install (it signs you in through the browser; run /mcp in Claude Code):\n\n    claude mcp add --transport http %s %s/mcp\n", server, p.URL)
	}
	if all || which == "codex" {
		fmt.Fprintf(w, "\nCodex:\n\n    codex mcp add %s -- %s\n", server, local)
	}
	if all || which == "claude-desktop" || which == "cursor" || which == "other" {
		fmt.Fprintf(w, "\nClaude Desktop (Settings › Developer › Edit Config), Cursor (~/.cursor/mcp.json) and other apps that start a local server:\n\n%s\n", indent(string(cfg)))
	}
	return nil
}

func indent(s string) string {
	return "    " + strings.ReplaceAll(s, "\n", "\n    ")
}
