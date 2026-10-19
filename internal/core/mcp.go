package core

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
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
)

// Runwell over the Model Context Protocol (Streamable HTTP, stateless):
// one tool per declared route (agent.go), offered by role, each run as the
// real request with the caller's token. Staff connect at /mcp; a client's
// own agent at /portal/mcp, where it gets the portal's tools, run as their
// contact. Bearer tokens only, never a browser's cookie: a personal one
// from Connected apps, or OAuth for connectors.
//
// Around every call: a paused token is refused, a token over its rate is
// told to wait, names given for ids are resolved, a tool that reaches the
// client previews first, a write sent again with the same idempotency_key
// answers as the first did instead of running twice, an error carries a
// stable code and a hint, and the call is logged (agent_calls) for the
// person to see.

func init() { routeSets = append(routeSets, (*App).mcpRoutes) }

func (a *App) mcpRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/agent/tools", a.listTools)
	mux.HandleFunc("/portal/mcp", a.portalMCP)
}

// mcpVersion is the MCP revision this server speaks.
const mcpVersion = "2025-06-18"

const mcpInstructions = `Runwell is project management for a business, a team or anything someone runs (a home, a wedding, a trip): who we work for (clients, and contacts who can approve), what we agreed (engagements holding agreement versions: drafts until sent, frozen once sent), what is happening (work/todos, commitments with dates, notes, documents), what came in (requests to triage), and what needs a person now (the briefing).

Start with ` + "`briefing`" + `; on a schedule, use ` + "`changes`" + ` and keep its cursor so you hear each change once. Tools take names where they take ids ("Bloom", "Priya", an engagement's title); an "ambiguous" answer lists the candidates to choose from, and ` + "`resolve`" + ` looks a name up directly. Give writes an ` + "`idempotency_key`" + ` when you might retry. Use ` + "`search`" + ` to find anything, and ` + "`me`" + ` for who you are, your role, the team's ids, and why a tool might be missing. Engagements are named by ref (WO-12, P-3, S-4). Tools that take a ` + "`record`" + ` want "Type:id", like "Client:12", as every result's ` + "`record`" + ` field shows. Rich text fields take HTML or plain text; write a person's name after @ to mention them.

Anything that reaches a client (sending an agreement, emailing a link, portal visibility) answers first with a preview (status needs_confirmation). Tell the person what will happen and call again with confirm: true only after they agree. You act as the person's agent, with exactly their role: what you do is recorded as "<Person>'s agent via <this app>". Each result has a ` + "`url`" + ` so the person can open it.

An error carries a stable ` + "`code`" + ` (usage, not_found, auth, forbidden, refused, invalid, read_only, failed) and a ` + "`hint`" + ` saying what to do; branch on the code, not the wording. A success may carry ` + "`next`" + `: suggested follow-up commands with what's known filled in.`

const portalMCPInstructions = `You act for a client of a business (or of whoever runs this Runwell), through its portal: you see what they see there and can do what they can. Call ` + "`portal_me`" + ` first: who you act for, their company, and whether they can decide agreements.

` + "`portal_engagements`" + ` lists the work agreed with them (engagements, named by ref like WO-12, or by title); ` + "`portal_engagement`" + ` shows one: what was agreed, the work shared with them, documents, and any agreement awaiting their decision exactly as it was sent. ` + "`portal_work`" + ` lists all shared work; ` + "`portal_requests`" + ` what they've asked for; ` + "`portal_send_request`" + ` asks for something new.

Deciding an agreement (` + "`portal_decide`" + `) is binding, as if they clicked it themselves. Show the person the agreement, get their decision and their typed name, and only then call it; the first call answers with a preview (needs_confirmation) and you call again with confirm: true. Never decide for them on your own judgement.

Errors carry a stable ` + "`code`" + ` (usage, not_found, ambiguous, auth, refused, invalid, read_only, paused, rate_limited, failed) and a ` + "`hint`" + `; branch on the code.`

// ---- errors ------------------------------------------------------------------

// agentErrorCodes are the codes an agent branches on (Agent::Errors); the
// CLI maps them to exit statuses.
var agentErrorCodes = []string{"usage", "not_found", "ambiguous", "auth", "forbidden", "refused", "invalid", "read_only", "paused", "rate_limited", "failed"}

// agentHints say what to do next, per code.
var agentHints = map[string]string{
	"usage":        "Check the inputs: `runwell help %{tool}`, or the tool's input schema.",
	"not_found":    "Check the id or ref. `search` or the matching list_ tool finds it.",
	"ambiguous":    "More than one record matches that name: pick one from candidates and call %{tool} again with its id.",
	"auth":         "Sign in again: the person runs `runwell login` in a terminal.",
	"forbidden":    "This person's role can't do that. `me` lists what it can; ask the person.",
	"refused":      "Runwell refused it as things stand (the summary says why). Change what it depends on, or ask the person.",
	"invalid":      "Fix the fields named in errors and call %{tool} again.",
	"read_only":    "This token is read-only. Ask the person for a token that can make changes.",
	"paused":       "The person paused this connection in Connected apps. Ask them to resume it.",
	"rate_limited": "Too many calls in a minute. Wait %{retry} seconds, then carry on; batch where a tool allows it.",
	"failed":       "Try once more; if it fails again, tell the person what you were doing.",
}

// agentHint is code's hint, filled in.
func agentHint(code, tool string, retryAfter int) string {
	h, ok := agentHints[code]
	if !ok {
		h = agentHints["failed"]
	}
	if tool == "" {
		tool = "the tool"
	}
	return strings.NewReplacer("%{tool}", tool, "%{retry}", strconv.Itoa(retryAfter)).Replace(h)
}

// codeForStatus is the code for an HTTP status with no code of its own.
func codeForStatus(status int) string {
	switch status {
	case 400:
		return "usage"
	case 401:
		return "auth"
	case 403:
		return "forbidden"
	case 404:
		return "not_found"
	case 409:
		return "refused"
	case 422:
		return "invalid"
	case 429:
		return "rate_limited"
	}
	return "failed"
}

// errorCode is an answer's code: the API's own when an agent knows it,
// else the status's.
func errorCode(apiCode string, status int) string {
	switch apiCode {
	case "bad_request":
		return "usage"
	case "unauthorized", "invalid_authenticity_token":
		return "auth"
	case "conflict":
		return "refused"
	case "invalid":
		if status == 422 {
			return "invalid"
		}
	}
	if slices.Contains(agentErrorCodes, apiCode) {
		return apiCode
	}
	if status == 422 && apiCode != "" {
		return "refused"
	}
	return codeForStatus(status)
}

func withHint(body map[string]any, code, tool string) map[string]any {
	body["code"] = code
	if _, ok := body["hint"]; !ok {
		body["hint"] = agentHint(code, tool, 60)
	}
	return body
}

// ---- rate limit ------------------------------------------------------------------

// agentRateLimit is how many tool calls one token may make a minute
// (RUNWELL_AGENT_RATE_LIMIT, 120 unless set), so a runaway loop in someone's
// bot can't take the install down.
func agentRateLimit() int {
	if v, err := strconv.Atoi(os.Getenv("RUNWELL_AGENT_RATE_LIMIT")); err == nil {
		return v
	}
	return 120
}

// ---- the servers -------------------------------------------------------------------

// mcpSurface is one MCP server: staff's, or a client's portal.
type mcpSurface struct {
	portal bool
}

// mcp is the staff MCP server (POST /mcp).
func (a *App) mcp(w http.ResponseWriter, r *http.Request) { a.serveMCP(w, r, mcpSurface{}) }

// portalMCP is a client's own agent's (POST /portal/mcp).
func (a *App) portalMCP(w http.ResponseWriter, r *http.Request) {
	a.serveMCP(w, r, mcpSurface{portal: true})
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// mcpSession is one request to an MCP server: who's calling, with what.
type mcpSession struct {
	a       *App
	r       *http.Request
	surface mcpSurface
	actor   *Actor
	auth    string
	tools   []*toolSpec
}

// serveMCP answers one POST: a JSON-RPC message (or a batch), as JSON.
func (a *App) serveMCP(w http.ResponseWriter, r *http.Request, surface mcpSurface) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", "POST")
		http.Error(w, "POST JSON-RPC here; this server doesn't stream.", http.StatusMethodNotAllowed)
		return
	}
	resource := a.BaseURL() + "/.well-known/oauth-protected-resource"
	if surface.portal {
		resource += "/portal/mcp"
	}
	unauthorized := func() {
		w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+resource+`"`)
		summary := "Sign in first: this needs a valid token."
		if surface.portal {
			summary = "Sign in first: this needs a valid client token."
		}
		writeJSON(w, 401, map[string]any{"status": "error", "code": "auth", "summary": summary, "hint": agentHint("auth", "", 60)})
	}
	h := r.Header.Get("Authorization")
	if !strings.HasPrefix(h, "Bearer ") {
		unauthorized()
		return
	}
	plain := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
	tok, err := a.tokenByPlain(r.Context(), plain)
	if err != nil {
		writeJSON(w, 500, rpcError(nil, -32603, "Something went wrong on our side."))
		return
	}
	if tok != nil && tok.PausedAt.Valid && (tok.UserID != 0) != surface.portal {
		writeJSON(w, 403, map[string]any{"status": "error", "code": "paused", "summary": "This connection is paused.", "hint": agentHint("paused", "", 60)})
		return
	}
	c := &Ctx{App: a, W: w, R: r}
	actor, err := a.authenticate(c, surface.portal)
	if err != nil || actor == nil || (surface.portal && actor.Contact == nil) || (!surface.portal && actor.User == nil) {
		unauthorized()
		return
	}
	s := &mcpSession{a: a, r: r, surface: surface, actor: actor, auth: h, tools: a.toolsFor(actor)}
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<20))
	if err != nil {
		writeJSON(w, 413, rpcError(nil, -32600, "the request is too large"))
		return
	}
	if trimmed := bytes.TrimSpace(raw); len(trimmed) > 0 && trimmed[0] == '[' {
		var batch []rpcRequest
		if err := json.Unmarshal(trimmed, &batch); err != nil {
			writeJSON(w, 400, rpcError(nil, -32700, "not JSON-RPC"))
			return
		}
		var out []any
		for _, req := range batch {
			if res := s.rpc(req); res != nil {
				out = append(out, res)
			}
		}
		if len(out) == 0 {
			w.WriteHeader(http.StatusAccepted)
			return
		}
		writeJSON(w, 200, out)
		return
	}
	var req rpcRequest
	if err := json.Unmarshal(raw, &req); err != nil {
		writeJSON(w, 400, rpcError(nil, -32700, "not JSON-RPC"))
		return
	}
	res := s.rpc(req)
	if res == nil {
		w.WriteHeader(http.StatusAccepted)
		return
	}
	writeJSON(w, 200, res)
}

// rpc answers one message; nil for a notification.
func (s *mcpSession) rpc(req rpcRequest) any {
	if len(req.ID) == 0 {
		return nil // a notification (notifications/initialized and kin)
	}
	a := s.a
	ok := func(result any) any { return map[string]any{"jsonrpc": "2.0", "id": req.ID, "result": result} }
	var params map[string]any
	_ = json.Unmarshal(req.Params, &params)
	switch req.Method {
	case "initialize":
		info := map[string]any{"name": "runwell", "title": "Runwell", "version": a.cfg.Version}
		instructions := mcpInstructions
		if s.surface.portal {
			name := s.clientName()
			info = map[string]any{"name": "runwell-portal", "title": name + " at " + a.BrandName(s.r.Context()), "version": a.cfg.Version}
			instructions = portalMCPInstructions
		}
		return ok(map[string]any{
			"protocolVersion": mcpVersion,
			"capabilities":    map[string]any{"tools": map[string]any{"listChanged": false}, "prompts": map[string]any{"listChanged": false}, "resources": map[string]any{"listChanged": false}},
			"serverInfo":      info,
			"instructions":    instructions,
		})
	case "ping":
		return ok(map[string]any{})
	case "tools/list":
		tools := []any{}
		for _, t := range s.tools {
			tools = append(tools, map[string]any{"name": t.Name, "title": t.Summary, "description": t.description(), "inputSchema": t.inputSchema(), "annotations": t.annotations()})
		}
		return ok(map[string]any{"tools": tools})
	case "tools/call":
		var p struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return rpcError(req.ID, -32602, "params must be {name, arguments}")
		}
		t := s.find(p.Name)
		if t == nil {
			return rpcError(req.ID, -32602, "Tool not found: "+p.Name)
		}
		body := s.dispatch(t, p.Arguments)
		text, _ := marshal(body)
		return ok(map[string]any{"content": []any{map[string]any{"type": "text", "text": string(text)}}, "structuredContent": body, "isError": body["status"] == "error"})
	case "resources/list":
		out := []any{}
		for _, rs := range s.resources() {
			if !strings.Contains(rs.uri, "{") {
				out = append(out, map[string]any{"uri": rs.uri, "name": rs.name, "title": rs.title, "description": rs.description, "mimeType": "application/json"})
			}
		}
		return ok(map[string]any{"resources": out})
	case "resources/templates/list":
		out := []any{}
		for _, rs := range s.resources() {
			if strings.Contains(rs.uri, "{") {
				out = append(out, map[string]any{"uriTemplate": rs.uri, "name": rs.name, "title": rs.title, "description": rs.description, "mimeType": "application/json"})
			}
		}
		return ok(map[string]any{"resourceTemplates": out})
	case "resources/read":
		uri := str(params["uri"])
		name, args := resourceCall(s.resources(), uri)
		t := s.find(name)
		if t == nil {
			return rpcError(req.ID, -32002, "Resource not found: "+uri)
		}
		body := s.dispatch(t, args)
		if body["status"] == "error" {
			return rpcError(req.ID, -32002, "Resource not found: "+uri)
		}
		text, _ := marshal(body)
		return ok(map[string]any{"contents": []any{map[string]any{"uri": uri, "mimeType": "application/json", "text": string(text)}}})
	case "prompts/list":
		out := []any{}
		for _, p := range s.prompts() {
			args := []any{}
			for _, arg := range p.args {
				args = append(args, map[string]any{"name": arg.name, "description": arg.description, "required": arg.required})
			}
			out = append(out, map[string]any{"name": p.name, "title": p.title, "description": p.description, "arguments": args})
		}
		return ok(map[string]any{"prompts": out})
	case "prompts/get":
		name := str(params["name"])
		i := slices.IndexFunc(s.prompts(), func(p mcpPrompt) bool { return p.name == name })
		if i < 0 {
			return rpcError(req.ID, -32602, "Prompt not found: "+name)
		}
		p := s.prompts()[i]
		args := map[string]string{}
		if m, ok := params["arguments"].(map[string]any); ok {
			for k, v := range m {
				args[k] = str(v)
			}
		}
		for _, arg := range p.args {
			if arg.required && args[arg.name] == "" {
				return rpcError(req.ID, -32602, "Missing required arguments: "+arg.name)
			}
		}
		return ok(map[string]any{"description": p.description, "messages": []any{
			map[string]any{"role": "user", "content": map[string]any{"type": "text", "text": p.text(args)}},
		}})
	}
	return rpcError(req.ID, -32601, "Method not found: "+req.Method)
}

func (s *mcpSession) find(name string) *toolSpec {
	for _, t := range s.tools {
		if t.Name == name {
			return t
		}
	}
	return nil
}

// clientName is the portal contact's client.
func (s *mcpSession) clientName() string {
	var name string
	if s.actor.Contact != nil {
		_ = s.a.store.QueryRowContext(s.r.Context(), `SELECT name FROM clients WHERE id = ?`, s.actor.Contact.ClientID).Scan(&name)
	}
	return name
}

// ---- one call ------------------------------------------------------------------------

// controlArgs steer a call; they never reach the route.
var controlArgs = []string{"confirm", "idempotency_key"}

// dispatch runs one tool call as the caller, guarded and logged.
func (s *mcpSession) dispatch(t *toolSpec, args map[string]any) map[string]any {
	started := time.Now()
	if args == nil {
		args = map[string]any{}
	}
	body := s.guarded(t, args)
	if tok := s.actor.Token; tok != nil {
		s.a.recordAgentCall(s.r.Context(), tok.ID, t.Name, body, started)
	}
	return body
}

func (s *mcpSession) guarded(t *toolSpec, args map[string]any) map[string]any {
	if tok := s.actor.Token; tok != nil {
		if ok, retry := s.a.agentCalls.Allow("token:" + strconv.FormatInt(tok.ID, 10)); !ok {
			return map[string]any{"status": "error", "code": "rate_limited", "summary": "Too many calls this minute.", "retry_after": retry,
				"hint": agentHint("rate_limited", t.Name, retry)}
		}
	}
	return s.run(t, args)
}

func (s *mcpSession) run(t *toolSpec, args map[string]any) map[string]any {
	ctx := s.r.Context()
	args, answer := s.withNamesResolved(t, args)
	if answer != nil {
		return answer
	}
	if t.Confirm != "" && args["confirm"] != true {
		return s.preview(t, args)
	}
	var key string
	if !t.read() {
		key = strings.TrimSpace(str(args["idempotency_key"]))
	}
	tok := s.actor.Token
	if key != "" && tok != nil {
		prior, answer, found, err := s.a.priorAnswer(ctx, tok.ID, key)
		if err != nil {
			return withHint(map[string]any{"status": "error", "summary": "Something went wrong running " + t.Name + "."}, "failed", t.Name)
		}
		if found {
			if prior == t.Name {
				answer["replayed"] = true
				return answer
			}
			return withHint(map[string]any{"status": "error", "summary": "That idempotency_key was already used for " + prior + ". Use a new key for a new change."}, "usage", t.Name)
		}
	}
	callArgs := map[string]any{}
	for k, v := range args {
		if !slices.Contains(controlArgs, k) {
			callArgs[k] = v
		}
	}
	body := s.answer(t, callArgs)
	if key != "" && tok != nil && body["status"] == "ok" {
		s.a.rememberAnswer(ctx, tok.ID, key, t.Name, body)
	}
	return body
}

// answer runs the route and reads its envelope, adding the code and hint to
// an error and the next tools to a success.
func (s *mcpSession) answer(t *toolSpec, args map[string]any) map[string]any {
	status, raw, err := s.a.call(s.r.Context(), t, args, s.auth, s.r.RemoteAddr)
	if err != nil {
		msg := err.Error()
		if e, ok := errors.AsType[*apierr.Error](err); ok {
			msg = e.Message
			if e.Status == 400 {
				return withHint(map[string]any{"status": "error", "summary": "Missing or wrong " + msg}, "usage", t.Name)
			}
		}
		return withHint(map[string]any{"status": "error", "summary": "Something went wrong running " + t.Name + ": " + truncate(msg, 200)}, "failed", t.Name)
	}
	body := map[string]any{}
	if len(bytes.TrimSpace(raw)) == 0 {
		body = map[string]any{"status": "ok", "summary": fmt.Sprintf("Done (%d).", status)}
	} else if json.Unmarshal(raw, &body) != nil {
		body = map[string]any{"status": "error", "summary": fmt.Sprintf("%s answered with a page, not JSON (%d).", t.Name, status)}
	}
	if _, ok := body["status"]; !ok {
		body["status"] = map[bool]string{true: "ok", false: "error"}[status < 400]
	}
	if body["status"] == "error" {
		data, _ := body["data"].(map[string]any)
		apiCode := str(body["code"])
		if apiCode == "" && data != nil {
			apiCode = str(data["error"])
			if data["code"] != nil {
				apiCode = str(data["code"])
			}
		}
		if data != nil {
			if errs, ok := data["errors"]; ok {
				body["errors"] = errs
			}
			if h, ok := data["hint"]; ok {
				body["hint"] = h
			}
		}
		if status == 429 {
			if retry, ok := data["retry_after"]; ok {
				body["retry_after"] = retry
			}
		}
		return withHint(body, errorCode(apiCode, status), t.Name)
	}
	if len(t.Next) > 0 {
		body["next_tools"] = t.Next
		body["next"] = s.suggestions(t, args, body)
	}
	return body
}

// suggestions are what to run next, as runwell commands with what's known
// filled in (the ids this call used, or the record it returned) and
// placeholders for the rest.
func (s *mcpSession) suggestions(t *toolSpec, args map[string]any, body map[string]any) []any {
	known := map[string]any{}
	for k, v := range args {
		known[k] = v
	}
	if rec := returnedRecord(body); rec != nil {
		known["id"], known["record"] = rec["id"], rec["record"]
		if rec["type"] == "Todo" {
			known["todo_id"] = rec["id"]
		}
	}
	out := []any{}
	for _, name := range t.Next {
		nxt := s.find(name)
		if nxt == nil {
			continue
		}
		parts := []string{"runwell", name}
		for _, p := range nxt.Params {
			switch {
			case p.In == "path":
				v := "<" + p.Name + ">"
				if k, ok := known[p.Name]; ok && k != nil {
					v = fmt.Sprint(k)
				}
				parts = append(parts, "--"+p.Name, v)
			case p.Name == "record":
				v := "<Type:id>"
				if k, ok := known["record"]; ok && k != nil {
					v = fmt.Sprint(k)
				}
				parts = append(parts, "--record", v)
			case p.Type == "object" && p.Required:
				schema := shapeSchema(p.Desc)
				req, _ := schema["required"].([]string)
				for _, f := range req {
					parts = append(parts, "--"+p.Name+"."+f, "<"+f+">")
				}
			}
		}
		out = append(out, map[string]any{"tool": name, "title": nxt.Summary, "command": strings.Join(parts, " ")})
	}
	return out
}

// returnedRecord is the answer's main record: the first object with a
// record and an id.
func returnedRecord(body map[string]any) map[string]any {
	for _, top := range []any{body["data"], body} {
		m, ok := top.(map[string]any)
		if !ok {
			continue
		}
		if m["record"] != nil && m["id"] != nil {
			return m
		}
		for _, v := range m {
			if rec, ok := v.(map[string]any); ok && rec["record"] != nil && rec["id"] != nil {
				return rec
			}
		}
	}
	return nil
}

// pathTypes are the record types a route's first segment holds, for
// resolving an {id} given as a name.
var pathTypes = map[string]string{"clients": "Client", "contacts": "Contact", "engagements": "Engagement", "work": "Todo", "todos": "Todo",
	"requests": "Request", "commitments": "Commitment", "scope_items": "ScopeItem", "people": "User", "users": "User"}

// withNamesResolved swaps names given for ids for the ids: path ids and
// refs, id parameters anywhere (client_id, owner_id…), and "Type:name"
// records. An answer when a name matches several records, or none.
func (s *mcpSession) withNamesResolved(t *toolSpec, args map[string]any) (map[string]any, map[string]any) {
	var clientScope int64
	if s.actor.Contact != nil {
		clientScope = s.actor.Contact.ClientID
	}
	pathType := ""
	if seg := strings.Split(strings.TrimPrefix(strings.TrimPrefix(t.Path, "/api/"), "portal/"), "/"); len(seg) > 0 {
		pathType = pathTypes[seg[0]]
	}
	isPath := func(name string) bool {
		return slices.ContainsFunc(t.Params, func(p toolParam) bool { return p.In == "path" && p.Name == name })
	}
	var resolve func(key string, v any, path bool) (any, error)
	resolve = func(key string, v any, path bool) (any, error) {
		switch x := v.(type) {
		case map[string]any:
			out := make(map[string]any, len(x))
			for k, inner := range x {
				r, err := resolve(k, inner, false)
				if err != nil {
					return nil, err
				}
				out[k] = r
			}
			return out, nil
		case []any:
			out := make([]any, len(x))
			for i, inner := range x {
				r, err := resolve(key, inner, false)
				if err != nil {
					return nil, err
				}
				out[i] = r
			}
			return out, nil
		case string:
			if strings.TrimSpace(x) == "" {
				return x, nil
			}
			typ := idParams[key]
			switch {
			case path && (key == "ref" || key == "engagement_ref"):
				if engagementRef.MatchString(x) {
					return x, nil
				}
				r, err := s.a.resolveName(s.r.Context(), "Engagement", x, clientScope)
				return r.Ref, err
			case path && key == "id" && pathType != "" && (clientScope == 0 || slices.Contains([]string{"Engagement", "Todo", "Request"}, pathType)):
				return s.a.idFor(s.r.Context(), pathType, x, clientScope)
			case typ == "User" && (strings.EqualFold(x, "me") || strings.EqualFold(x, "none")):
				if strings.EqualFold(x, "none") {
					return "", nil
				}
				if s.actor.User != nil {
					return s.actor.User.Person().ID, nil
				}
				return x, nil
			case typ != "" && (clientScope == 0 || slices.Contains([]string{"Engagement", "Todo", "Request"}, typ)):
				return s.a.idFor(s.r.Context(), typ, x, clientScope)
			case key == "record":
				if rt, name, ok := strings.Cut(x, ":"); ok && slices.Contains(resolverTypes, rt) && !isID(name) {
					r, err := s.a.resolveName(s.r.Context(), rt, name, clientScope)
					if err != nil {
						return nil, err
					}
					return fmt.Sprintf("%s:%d", rt, r.ID), nil
				}
			}
		}
		return v, nil
	}
	out := make(map[string]any, len(args))
	for k, v := range args {
		r, err := resolve(k, v, isPath(k))
		if err != nil {
			var amb *errAmbiguous
			var missing *errMissing
			switch {
			case errors.As(err, &amb):
				cands := []any{}
				for _, c := range amb.candidates {
					cands = append(cands, s.a.describeResolved(s.r.Context(), c))
				}
				return nil, map[string]any{"status": "error", "code": "ambiguous", "summary": amb.Error(), "candidates": cands, "hint": agentHint("ambiguous", t.Name, 60)}
			case errors.As(err, &missing):
				return nil, withHint(map[string]any{"status": "error", "summary": missing.Error()}, "not_found", t.Name)
			}
			return nil, withHint(map[string]any{"status": "error", "summary": "Something went wrong running " + t.Name + "."}, "failed", t.Name)
		}
		out[k] = r
	}
	return out, nil
}

// preview is a call that reaches the client, not made yet: what would
// happen, in words, so the agent can ask the person first.
func (s *mcpSession) preview(t *toolSpec, args map[string]any) map[string]any {
	ctx := s.r.Context()
	about := []any{}
	pathType := ""
	if seg := strings.Split(strings.TrimPrefix(strings.TrimPrefix(t.Path, "/api/"), "portal/"), "/"); len(seg) > 0 {
		pathType = pathTypes[seg[0]]
	}
	describe := func(typ string, v any) {
		if typ == "" || v == nil {
			return
		}
		var id int64
		switch x := v.(type) {
		case float64:
			id = int64(x)
		case int64:
			id = x
		case string:
			if typ == "Engagement" && !isID(x) {
				_ = s.a.store.QueryRowContext(ctx, `SELECT id FROM engagements WHERE ref = ?`, strings.ToUpper(x)).Scan(&id)
			} else {
				id, _ = strconv.ParseInt(x, 10, 64)
			}
		}
		if id == 0 {
			return
		}
		if info, ok, err := s.a.subjectInfo(ctx, typ, id); err == nil && ok {
			about = append(about, info.Name)
		}
	}
	if pathType != "" {
		describe(pathType, args["id"])
	}
	for _, k := range []string{"ref", "engagement_ref"} {
		describe("Engagement", args[k])
	}
	for k, v := range args {
		if strings.HasSuffix(k, "_id") {
			describe(idParams[k], v)
		}
	}
	again := map[string]any{}
	for k, v := range args {
		again[k] = v
	}
	again["confirm"] = true
	return map[string]any{"status": "needs_confirmation",
		"summary":         "Not done yet. " + t.Confirm + " Tell the person, and call " + t.Name + " again with confirm: true once they agree.",
		"about":           about,
		"call_again_with": again}
}

// ---- resources ------------------------------------------------------------------------

// mcpResource is a record a harness can attach as context: runwell://
// engagements/WO-12 and the like, read through the matching show tool (so
// names work and the read is logged like any call).
type mcpResource struct {
	uri, name, title, description, tool, param string
}

var staffResources = []mcpResource{
	{"runwell://briefing", "briefing", "Briefing", "What needs a person now.", "briefing", ""},
	{"runwell://engagements/{ref}", "engagement", "Engagement", "An engagement by its ref (WO-12) or title: agreement, work, commitments, notes.", "show_engagement", "ref"},
	{"runwell://clients/{id}", "client", "Client", "A client by id or name: contacts, engagements, commitments, notes.", "show_client", "id"},
	{"runwell://work/{id}", "work", "Work", "A piece of work by id or title.", "show_work", "id"},
	{"runwell://requests/{id}", "request", "Request", "A client request by id or subject.", "show_request", "id"},
}

var clientResources = []mcpResource{
	{"runwell://portal/engagements/{ref}", "engagement", "Engagement", "One of your engagements by its ref or title.", "portal_engagement", "ref"},
}

func (s *mcpSession) resources() []mcpResource {
	if s.surface.portal {
		return clientResources
	}
	return staffResources
}

// resourceCall is the tool and arguments a runwell:// URI stands for.
func resourceCall(set []mcpResource, uri string) (string, map[string]any) {
	for _, rs := range set {
		if rs.param == "" {
			if uri == rs.uri {
				return rs.tool, map[string]any{}
			}
			continue
		}
		prefix, suffix, _ := strings.Cut(rs.uri, "{"+rs.param+"}")
		if strings.HasPrefix(uri, prefix) && strings.HasSuffix(uri, suffix) && len(uri) > len(prefix)+len(suffix) {
			v, err := url.PathUnescape(strings.TrimSuffix(strings.TrimPrefix(uri, prefix), suffix))
			if err != nil {
				continue
			}
			return rs.tool, map[string]any{rs.param: v}
		}
	}
	return "", nil
}

// ---- prompts ----------------------------------------------------------------------------

// mcpPrompt is a ready-made request a harness can offer as a one-tap start:
// in the person's words, what they want and which tools get it.
type mcpPrompt struct {
	name, title, description string
	args                     []promptArg
	text                     func(args map[string]string) string
}

type promptArg struct {
	name, description string
	required          bool
}

var staffPrompts = []mcpPrompt{
	{"plan_my_day", "Plan my day", "What needs me today, and what to do first.", nil, func(map[string]string) string {
		return "Call `briefing`, then `list_work` for my open work by due date. Tell me, in a short list, what needs me today (questions to answer, requests to triage, anything overdue or blocked) and what to start with. Don't change anything."
	}},
	{"standup", "Standup", "What changed across the team since yesterday, by engagement.", []promptArg{{"since", "How far back, like 24h or 3d (default 24h)", false}}, func(args map[string]string) string {
		return "Call `changes` with since: " + strOr(args["since"], "24h") + " (and again with after: the cursor while more is true). Summarise by engagement: what moved, what came in, what was sent or decided, who did it. Then list anything blocked or overdue from `briefing`. Keep it short enough to read aloud."
	}},
	{"triage_requests", "Triage requests", "Go through open requests and propose what each should become.", nil, func(map[string]string) string {
		return "Call `list_requests` for open requests. For each, read it with `show_request` and propose one of: a new engagement, a change to an engagement's draft, a todo, a commitment, or dismissing it, with why. Ask me before promoting or dismissing anything, one request at a time."
	}},
	{"client_update", "Weekly client update", "Draft this week's update for a client, from what happened.", []promptArg{{"client", "The client's name", true}}, func(args map[string]string) string {
		c := args["client"]
		return "Draft a weekly update for " + c + ". Use `show_client` (id: \"" + c + "\") and `changes` with client_id: \"" + c + "\" and since: 7d. Cover what was delivered, what's in progress, what we need from them, and what's next, in plain language a client reads in a minute. Show me the draft; don't send anything."
	}},
	{"catch_up", "Catch me up on an engagement", "Where an engagement stands: agreed, done, in progress, waiting.", []promptArg{{"engagement", "Its ref (WO-12) or title", true}}, func(args map[string]string) string {
		return "Call `show_engagement` with ref: \"" + args["engagement"] + "\". Tell me what was agreed, what's done, what's in progress and with whom, what we're waiting on (commitments), and anything overdue or blocked. Short."
	}},
}

var clientPrompts = []mcpPrompt{
	{"project_status", "Where are things at?", "The state of the work being done for you.", nil, func(map[string]string) string {
		return "Call `portal_engagements`, then `portal_engagement` for each that's active. Tell me in plain language what's done, what's in progress, and anything waiting on me (an agreement to decide, or something they asked for)."
	}},
	{"send_a_request", "Ask for something", "Turn what you need into a request they'll triage.", []promptArg{{"what", "What you need, in your words", true}}, func(args map[string]string) string {
		return "I want to ask them for this: " + args["what"] + ". Write it as a request with a short, clear subject and a description of what I need and by when, show me, and send it with `portal_send_request` once I agree."
	}},
}

func (s *mcpSession) prompts() []mcpPrompt {
	if s.surface.portal {
		return clientPrompts
	}
	return staffPrompts
}

// ---- running a tool as someone, from inside ------------------------------------------------

// runToolAs runs one tool as a person on staff (or a contact), the way their
// own agent would over MCP: with a token for this call alone (kind
// "assistant", kept out of Connected apps), revoked straight after. name is
// what the history says it came through ("Runwell AI"). For a plugin's
// in-app assistant.
func (a *App) runToolAs(ctx context.Context, userID, contactID int64, toolName string, args map[string]any, name string) (map[string]any, error) {
	var tok *AccessToken
	if err := a.write(ctx, nil, func(t *Tx) error {
		var err error
		tok, err = t.issueToken(userID, contactID, name, "assistant", 0, false)
		return err
	}); err != nil {
		return nil, err
	}
	defer func() {
		if err := a.write(context.WithoutCancel(ctx), nil, func(t *Tx) error { return t.revokeToken(tok.ID) }); err != nil {
			a.log.Warn("assistant token", "err", err)
		}
	}()
	r, err := http.NewRequestWithContext(ctx, http.MethodPost, "/mcp", nil)
	if err != nil {
		return nil, err
	}
	r.Header.Set("Authorization", "Bearer "+tok.Plaintext)
	c := &Ctx{App: a, R: r, W: &recorder{header: http.Header{}}}
	actor, err := a.authenticate(c, contactID != 0)
	if err != nil || actor == nil {
		return nil, errors.New("the assistant's token didn't work")
	}
	s := &mcpSession{a: a, r: r, surface: mcpSurface{portal: contactID != 0}, actor: actor, auth: "Bearer " + tok.Plaintext, tools: a.toolsFor(actor)}
	t := s.find(toolName)
	if t == nil {
		return nil, fmt.Errorf("no tool %s for this person", toolName)
	}
	return s.dispatch(t, args), nil
}

func rpcError(id json.RawMessage, code int, message string) any {
	return map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
