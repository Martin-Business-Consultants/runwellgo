package core

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"mime/multipart"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
)

// The agent surface: the tools an AI agent (or a person at a terminal) uses
// to work on the site. A tool is declared on its route (h(…, tool(…))), so
// the catalogue is the API that runs, not a description of it; the
// catalogue (GET /api/agent/tools) feeds `runwell <tool>` and the MCP
// server (POST /mcp), which runs a call through the same routes, as the
// caller, with the caller's role: a tool can never do more than the API
// lets that caller do.

// toolSpec is one tool: what it's called, what it does, the route it runs,
// and what it takes.
type toolSpec struct {
	Name    string      `json:"name"`
	Summary string      `json:"summary"`
	Notes   string      `json:"notes,omitempty"`
	Method  string      `json:"method"`
	Path    string      `json:"path"`
	Params  []toolParam `json:"params"`
	// The body's own key ({"client": {…}}), filled from the param of that name.
	Wrap string `json:"wrap,omitempty"`
	// Fixed into the body under Wrap ({"status": "done"}).
	Preset map[string]any `json:"preset,omitempty"`
	// The body is multipart (a file upload).
	Multipart bool `json:"multipart,omitempty"`
	// Confirm is what the call reaches (a client): the tool previews until
	// it's called again with confirm: true, once the person agreed.
	Confirm string `json:"confirm,omitempty"`
	// Next are the tools worth calling after this one.
	Next []string `json:"next_tools,omitempty"`
	// InputSchema is what MCP sends it (inputSchema), for the CLI too.
	InputSchema map[string]any `json:"input_schema,omitempty"`

	// The route it was declared on: who may call it (agent_tools.go).
	rt *route
}

// toolParam is one argument: in the path, the query or the body.
type toolParam struct {
	Name     string `json:"name"`
	In       string `json:"in"`   // path, query, body, file
	Type     string `json:"type"` // string, integer, boolean, object, array, file
	Desc     string `json:"description"`
	Required bool   `json:"required,omitempty"`
}

type toolOpt func(*toolSpec)

// tool declares the route's tool.
func tool(name, summary string, opts ...toolOpt) func(*route) {
	return func(r *route) {
		t := &toolSpec{Name: name, Summary: summary, rt: r}
		for _, o := range opts {
			o(t)
		}
		r.tool = t
	}
}

func query(name, typ, desc string) toolOpt {
	return func(t *toolSpec) {
		t.Params = append(t.Params, toolParam{Name: name, In: "query", Type: typ, Desc: desc})
	}
}

// body is the JSON object the route takes, sent as {wrap: value}.
func body(wrap, desc string, required bool) toolOpt {
	return func(t *toolSpec) {
		t.Wrap = wrap
		t.Params = append(t.Params, toolParam{Name: wrap, In: "body", Type: "object", Desc: desc, Required: required})
	}
}

func note(s string) toolOpt { return func(t *toolSpec) { t.Notes = s } }

// confirm marks a tool that reaches a client: it previews first.
func confirm(what string) toolOpt { return func(t *toolSpec) { t.Confirm = what } }

// next names the tools worth calling after this one.
func next(names ...string) toolOpt { return func(t *toolSpec) { t.Next = names } }

// bodyParam is a top-level body field (not wrapped): {"ids": [...]}.
func bodyParam(name, typ, desc string, required bool) toolOpt {
	return func(t *toolSpec) {
		t.Params = append(t.Params, toolParam{Name: name, In: "body", Type: typ, Desc: desc, Required: required})
	}
}

// file is a multipart file field: a path from the command line, or
// {"filename", "content_base64"} over MCP.
func file(name, desc string) toolOpt {
	return func(t *toolSpec) {
		t.Multipart = true
		t.Params = append(t.Params, toolParam{Name: name, In: "file", Type: "file", Desc: desc, Required: true})
	}
}

// formField is a multipart text field beside the file.
func formField(name, desc string) toolOpt {
	return func(t *toolSpec) {
		t.Params = append(t.Params, toolParam{Name: name, In: "body", Type: "string", Desc: desc})
	}
}

var pathParam = regexp.MustCompile(`\{([a-z_]+)(\.\.\.)?\}`)

// register records a route's tool, its path parameters read off the pattern.
func (a *App) register(pattern string, t *toolSpec) {
	method, path, _ := strings.Cut(pattern, " ")
	t.Method, t.Path = method, path
	var params []toolParam
	for _, m := range pathParam.FindAllStringSubmatch(path, -1) {
		desc := "The " + strings.ReplaceAll(m[1], "_", " ") + "."
		switch m[1] {
		case "ref":
			desc = "The engagement's ref, like WO-12."
		case "id":
			desc = "The record's id."
		}
		params = append(params, toolParam{Name: m[1], In: "path", Type: "string", Desc: desc, Required: true})
	}
	t.Params = append(params, t.Params...)
	if !slices.ContainsFunc(a.tools, func(o *toolSpec) bool { return o.Name == t.Name }) {
		a.tools = append(a.tools, t)
	}
}

// apiRequestTool reaches whatever the named tools don't: any /api route.
var apiRequestTool = &toolSpec{
	Name: "api_request", Summary: "Call any /api route the named tools don't cover, as you, with your role.",
	Notes:  "path starts with /api/. `me` says who you are and what your role may do. Prefer a named tool when there is one: its answer comes with next steps.",
	Method: "ANY", Path: "/api/…",
	Params: []toolParam{
		{Name: "method", In: "body", Type: "string", Desc: "GET, POST, PATCH, PUT or DELETE.", Required: true},
		{Name: "path", In: "body", Type: "string", Desc: "The route, e.g. /api/settings/names.", Required: true},
		{Name: "query", In: "body", Type: "object", Desc: "Query parameters, name → value."},
		{Name: "body", In: "body", Type: "object", Desc: "The JSON body."},
	},
}

// catalogue is every tool, in the order declared, then the enabled
// plugins' (api_plugins.go), api_request last.
func (a *App) catalogue() []*toolSpec {
	return append(append(slices.Clone(a.tools), a.pluginTools()...), apiRequestTool)
}

// listTools is the caller's catalogue (agent_tools.go): the tools their role
// is offered, with each one's input schema.
func (a *App) listTools(c *Ctx) error {
	tools := []toolSpec{}
	for _, t := range a.toolsFor(c.Actor) {
		cp := *t
		cp.InputSchema = t.inputSchema()
		tools = append(tools, cp)
	}
	c.Summary = fmt.Sprintf("%d tools.", len(tools))
	c.Crumbs = []Crumb{crumb("Start with what needs a person now", "runwell briefing")}
	return c.JSON(200, O("tools", tools, "version", a.cfg.Version))
}

// call runs t with args through the routes, authenticated as auth (the
// caller's own Authorization header): the answer's status and body, in
// the agent envelope.
func (a *App) call(ctx context.Context, t *toolSpec, args map[string]any, auth, remote string) (int, []byte, error) {
	method, path := t.Method, ""
	q := url.Values{}
	var payload io.Reader
	contentType := "application/json"
	if t == apiRequestTool {
		var err error
		if method, path, payload, err = rawRequest(args, q); err != nil {
			return 0, nil, err
		}
	} else {
		var err error
		if path, err = toolPath(t, args); err != nil {
			return 0, nil, err
		}
		for _, p := range t.Params {
			if v, ok := args[p.Name]; ok && v != nil && p.In == "query" {
				q.Set(p.Name, fmt.Sprint(v))
			}
		}
		if t.Multipart {
			payload, contentType, err = multipartBody(t, args)
		} else {
			payload, err = jsonBody(t, args)
		}
		if err != nil {
			return 0, nil, err
		}
	}
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	r, err := http.NewRequestWithContext(ctx, method, path, payload)
	if err != nil {
		return 0, nil, err
	}
	if payload == nil {
		r.Body = http.NoBody // as a server's request always has one
	}
	r.Header.Set("Authorization", auth)
	r.Header.Set("Accept", "application/json")
	r.Header.Set("X-Agent-Envelope", "1")
	if payload != nil {
		r.Header.Set("Content-Type", contentType)
	}
	r.RemoteAddr = remote
	w := &recorder{header: http.Header{}, status: 200}
	a.mux.ServeHTTP(w, r)
	return w.status, w.body.Bytes(), nil
}

// toolPath is t's route with its path parameters filled from args.
func toolPath(t *toolSpec, args map[string]any) (string, error) {
	var missing []string
	path := pathParam.ReplaceAllStringFunc(t.Path, func(m string) string {
		sub := pathParam.FindStringSubmatch(m)
		v := strings.Trim(fmt.Sprint(args[sub[1]]), "/")
		if args[sub[1]] == nil || v == "" {
			missing = append(missing, sub[1])
			return ""
		}
		if sub[2] == "" {
			return url.PathEscape(v)
		}
		parts := strings.Split(v, "/")
		for i := range parts {
			parts[i] = url.PathEscape(parts[i])
		}
		return strings.Join(parts, "/")
	})
	if len(missing) > 0 {
		return "", apierr.BadRequest("missing " + strings.Join(missing, ", "))
	}
	return path, nil
}

// rawRequest is api_request's call: its method, /api path, query (into q)
// and JSON body.
func rawRequest(args map[string]any, q url.Values) (string, string, io.Reader, error) {
	method, path := strings.ToUpper(str(args["method"])), str(args["path"])
	if !strings.HasPrefix(path, "/api/") {
		return "", "", nil, apierr.BadRequest("path must start with /api/")
	}
	if qs, ok := args["query"].(map[string]any); ok {
		for k, v := range qs {
			q.Set(k, fmt.Sprint(v))
		}
	}
	b, ok := args["body"]
	if !ok || b == nil {
		return method, path, nil, nil
	}
	raw, err := json.Marshal(b)
	if err != nil {
		return "", "", nil, err
	}
	return method, path, bytes.NewReader(raw), nil
}

// jsonBody is the request body a tool sends: its body param under Wrap,
// with the preset fields, beside any top-level body params.
func jsonBody(t *toolSpec, args map[string]any) (io.Reader, error) {
	out := map[string]any{}
	for _, p := range t.Params {
		if v, ok := args[p.Name]; ok && p.In == "body" && p.Name != t.Wrap {
			out[p.Name] = v
		}
	}
	if t.Wrap != "" || t.Preset != nil {
		switch v := args[t.Wrap].(type) {
		case []any:
			out[t.Wrap] = v
		case map[string]any:
			inner := maps.Clone(v)
			maps.Copy(inner, t.Preset)
			out[t.Wrap] = inner
		case nil:
			out[t.Wrap] = maps.Clone(t.Preset)
		default:
			return nil, apierr.BadRequest(t.Wrap + " must be a JSON object")
		}
	}
	if len(out) == 0 {
		return nil, nil
	}
	raw, err := json.Marshal(out)
	return bytes.NewReader(raw), err
}

// multipartBody is a file upload: the file param ({"filename",
// "content_base64"}) and the text fields.
func multipartBody(t *toolSpec, args map[string]any) (io.Reader, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, p := range t.Params {
		v, ok := args[p.Name]
		if !ok || v == nil {
			continue
		}
		switch p.In {
		case "file":
			f, _ := v.(map[string]any)
			name, data := str(f["filename"]), str(f["content_base64"])
			raw, err := base64.StdEncoding.DecodeString(data)
			if err != nil || name == "" {
				return nil, "", apierr.BadRequest(p.Name + ` must be {"filename": "…", "content_base64": "…"}`)
			}
			fw, err := mw.CreateFormFile(p.Name, name)
			if err != nil {
				return nil, "", err
			}
			if _, err := fw.Write(raw); err != nil {
				return nil, "", err
			}
		case "body":
			if err := mw.WriteField(p.Name, fmt.Sprint(v)); err != nil {
				return nil, "", err
			}
		}
	}
	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, mw.FormDataContentType(), nil
}

// recorder keeps a routed answer.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (r *recorder) Header() http.Header         { return r.header }
func (r *recorder) Write(b []byte) (int, error) { return r.body.Write(b) }
func (r *recorder) WriteHeader(s int)           { r.status = s }
