package core

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/admin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/jsonv"
	"github.com/Martin-Business-Consultants/runwellgo/internal/ratelimit"
	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// ---- request context -----------------------------------------------------

// Ctx is one request: who's asking, and how to answer.
type Ctx struct {
	App   *App
	W     http.ResponseWriter
	R     *http.Request
	Actor *Actor

	// The agent envelope's summary and next steps, when a handler has them.
	Summary string
	Crumbs  []Crumb
}

// Crumb is a next step an answer suggests: a tool and its arguments, filled
// in from what was just returned (what an MCP client calls), and the same
// as a command line (what a person types).
type Crumb struct {
	Description string         `json:"description"`
	Command     string         `json:"command"`
	Tool        string         `json:"tool,omitempty"`
	Arguments   map[string]any `json:"arguments,omitempty"`
}

// crumb is a next step that isn't a tool call (a URL to pass on).
func crumb(desc, cmd string) Crumb { return Crumb{Description: desc, Command: cmd} }

// step is a next step that's a tool call: tool, then its arguments as
// name, value pairs.
func step(desc, tool string, args ...any) Crumb {
	c := Crumb{Description: desc, Tool: tool}
	line := []string{"runwell", tool}
	if len(args) > 0 {
		c.Arguments = map[string]any{}
	}
	for i := 0; i+1 < len(args); i += 2 {
		name := fmt.Sprint(args[i])
		c.Arguments[name] = args[i+1]
		v := args[i+1]
		var s string
		switch x := v.(type) {
		case string:
			s = x
		default:
			raw, _ := json.Marshal(x)
			s = string(raw)
		}
		if strings.ContainsAny(s, " '\"{}[]$`&|;<>*?") {
			s = "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
		}
		line = append(line, "--"+strings.ReplaceAll(name, "_", "-"), s)
	}
	c.Command = strings.Join(line, " ")
	return c
}

type handlerFunc func(*Ctx) error

// isNotFound is a lookup's "there's no such thing", whichever way it said so.
func isNotFound(err error) bool {
	if errors.Is(err, store.ErrNotFound) || errors.Is(err, sql.ErrNoRows) {
		return true
	}
	e, ok := errors.AsType[*apierr.Error](err)
	return ok && e.Status == http.StatusNotFound
}

func (c *Ctx) fail(err error) {
	var e *apierr.Error
	switch {
	case errors.As(err, &e):
	case errors.Is(err, store.ErrNotFound):
		e = apierr.NotFound("Couldn't find that.")
	default:
		c.App.log.Error("request", "path", c.R.URL.Path, "request_id", requestIDFrom(c.R.Context()), "err", err)
		e = &apierr.Error{Status: 500, Code: "internal_error", Message: "Something went wrong on our side."}
	}
	body := map[string]any{"error": e.Code}
	if e.Message != "" {
		body["message"] = e.Message
	}
	if e.Errors != nil {
		body["errors"] = e.Errors
	}
	if e.Capability != "" {
		body["capability"] = e.Capability
	}
	maps.Copy(body, e.Extra)
	_ = c.JSON(e.Status, body) // the client went away; nothing to do

}

func (c *Ctx) envelope() bool {
	if c.R.Header.Get("X-Agent-Envelope") == "1" {
		return true
	}
	switch strings.ToLower(c.R.URL.Query().Get("envelope")) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// JSON answers with v, in the agent envelope when the caller asked for it.
func (c *Ctx) JSON(status int, v any) error {
	if c.envelope() {
		ok := status < 400
		summary := c.Summary
		if summary == "" {
			summary = defaultSummary(c.R, status, v)
		}
		crumbs := c.Crumbs
		if crumbs == nil || !ok {
			crumbs = []Crumb{}
		}
		v = O("status", map[bool]string{true: "ok", false: "error"}[ok], "summary", summary, "data", v, "breadcrumbs", crumbs)
	}
	body, err := marshal(v)
	if err != nil {
		return err
	}
	c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	c.W.WriteHeader(status)
	_, err = c.W.Write(body)
	return err
}

func defaultSummary(r *http.Request, status int, v any) string {
	if status >= 400 {
		if m, ok := v.(map[string]any); ok {
			if msg, ok := m["message"].(string); ok {
				return msg
			}
			return fmt.Sprint(m["error"])
		}
		return http.StatusText(status)
	}
	return r.Method + " " + r.URL.Path + " — " + http.StatusText(status) + "."
}

// NoContent answers 204.
func (c *Ctx) NoContent() error {
	c.W.WriteHeader(http.StatusNoContent)
	return nil
}

// maxBody is the most a JSON body may be.
const maxBody = 32 << 20

// Body decodes the request's JSON into v; a body that isn't JSON is a 400,
// one over 32 MB a 413.
func (c *Ctx) Body(v any) error { return c.body(v, false) }

// OptionalBody is Body for a request that may come with none (v is left
// as it is); one that does come must still be JSON.
func (c *Ctx) OptionalBody(v any) error { return c.body(v, true) }

func (c *Ctx) body(v any, optional bool) error {
	data, err := io.ReadAll(http.MaxBytesReader(c.W, c.R.Body, maxBody))
	if err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return apierr.TooLarge(maxBody)
		}
		return apierr.BadRequest("Couldn't read the request body.")
	}
	if len(strings.TrimSpace(string(data))) == 0 {
		if optional {
			return nil
		}
		return apierr.BadRequest("The request needs a JSON body.")
	}
	if err := json.Unmarshal(data, v); err != nil {
		return apierr.BadRequest("The body isn't JSON: " + err.Error())
	}
	return nil
}

// Wrapped decodes {"<key>": {…}} into a map, 400 when the key is missing,
// as Rails' params.require did.
func (c *Ctx) Wrapped(key string) (map[string]any, error) {
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return nil, err
	}
	inner, ok := body[key].(map[string]any)
	if !ok {
		return nil, apierr.BadRequest("param is missing or the value is empty: " + key)
	}
	return inner, nil
}

// Query is the query string's key.
func (c *Ctx) Query(key string) string { return c.R.URL.Query().Get(key) }

// Param is the route's {key}.
func (c *Ctx) Param(key string) string { return c.R.PathValue(key) }

// IntParam is the route's {key} as an id; a 404 when it isn't one.
func (c *Ctx) IntParam(key string) (int64, error) {
	n, err := strconv.ParseInt(c.R.PathValue(key), 10, 64)
	if err != nil {
		return 0, apierr.NotFound("Couldn't find " + key + " " + c.R.PathValue(key) + ".")
	}
	return n, nil
}

func (c *Ctx) truthy(key string) bool {
	switch strings.ToLower(c.Query(key)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// paging reads ?page= and ?per= with the given default and cap.
func (c *Ctx) paging(def, most int) (page, per int) {
	page, _ = strconv.Atoi(c.Query("page"))
	per, _ = strconv.Atoi(c.Query("per"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		page = 100000
	}
	if per < 1 {
		per = def
	}
	per = min(per, most)
	return
}

// ip is the client's address. Behind a proxy it's the rightmost
// X-Forwarded-For entry, the one our proxy appended: everything left of it
// is whatever the client chose to send.
func (c *Ctx) ip() string {
	if c.App.cfg.TrustProxy {
		if fwd := c.R.Header.Values("X-Forwarded-For"); len(fwd) > 0 {
			last := fwd[len(fwd)-1]
			if ip := strings.TrimSpace(last[strings.LastIndex(last, ",")+1:]); ip != "" {
				return ip
			}
		}
	}
	host := c.R.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		host = host[:i]
	}
	return strings.Trim(host, "[]")
}

// ---- routing -------------------------------------------------------------

func (a *App) handle(mux *http.ServeMux, pattern string, h handlerFunc, opts ...func(*route)) {
	rt := route{auth: true}
	for _, o := range opts {
		o(&rt)
	}
	if rt.tool != nil {
		a.register(pattern, rt.tool)
	}
	mux.HandleFunc(pattern, func(w http.ResponseWriter, r *http.Request) {
		c := &Ctx{App: a, W: w, R: r}
		defer func() {
			if rec := recover(); rec != nil {
				// A handler cutting the connection on purpose (a broken
				// download) is net/http's to handle.
				if isAbort(rec) {
					panic(rec)
				}
				a.log.Error("panic", "path", r.URL.Path, "request_id", requestIDFrom(r.Context()), "panic", rec, "stack", string(debug.Stack()))
				c.fail(fmt.Errorf("panic: %v", rec))
			}
		}()
		actor, err := a.authenticate(c, rt.portal)
		if err != nil && rt.auth {
			c.fail(err)
			return
		}
		c.Actor = actor
		if rt.auth {
			if err := rt.allows(c); err != nil {
				c.fail(err)
				return
			}
			if routeGate != nil {
				if err := routeGate(c, pattern); err != nil {
					c.fail(err)
					return
				}
			}
		}
		if err := h(c); err != nil {
			c.fail(err)
		}
	})
}

// allows checks the actor against the route: signed in, the right kind
// (staff or a portal contact), the permission, and a read-only token only
// reading.
func (rt *route) allows(c *Ctx) error {
	actor := c.Actor
	switch {
	case actor == nil:
		return unauthorized()
	case rt.portal && actor.Contact == nil, !rt.portal && actor.User == nil:
		return forbidden()
	case actor.Token != nil && actor.Token.ReadOnly && c.R.Method != http.MethodGet && c.R.Method != http.MethodHead:
		return &apierr.Error{Status: 403, Code: "read_only", Message: "This connection is read-only: it can look, not change anything."}
	case rt.perm != "" && !c.Can(rt.perm):
		return forbidden()
	}
	return nil
}

// routeGate, when set, may refuse a signed-in staff request after the
// route allowed it (the account area: two-factor sign-in required and not
// set up yet).
var routeGate func(c *Ctx, pattern string) error

// Handler is the whole install: the API, the MCP servers, the app.
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	a.mux, a.tools = mux, nil
	for _, routes := range routeSets {
		routes(a, mux)
	}
	mux.HandleFunc("/mcp", a.mcp)
	mux.HandleFunc("GET /up", func(w http.ResponseWriter, r *http.Request) {
		if err := a.store.PingContext(r.Context()); err != nil {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		w.Write([]byte("ok"))
	})
	mux.HandleFunc("GET /robots.txt", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte("User-agent: *\nDisallow: /\n"))
	})
	// Any other /api path is the API's 404, in its shape; everything else is
	// the app (staff, portal, approval pages), which routes in the browser.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") || r.URL.Path == "/api" {
			c := &Ctx{App: a, W: w, R: r}
			c.fail(apierr.NotFound("No route matches " + r.Method + " " + r.URL.Path + "."))
			return
		}
		if a.cfg.UI != nil {
			// Plugins that are on and granted content:external widen the
			// page's policy by the sites their screens load from.
			if extra := a.plugins.ContentSources(); extra != nil {
				r = r.WithContext(admin.WithSources(r.Context(), extra))
			}
			a.cfg.UI.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
	return a.logRequests(a.recoverPanics(a.cors(securityHeaders(mux))))
}

// routeSets are the API's routes, one set per area of the app: each file
// adds its own in an init (routeSets = append(routeSets, (*App).clientRoutes)).
var routeSets []func(*App, *http.ServeMux)

// ---- request logging -----------------------------------------------------

type requestIDKey struct{}

// requestIDFrom is the request's id (X-Request-ID), "" outside one.
func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// requestID keeps a caller's (a proxy's) X-Request-ID when it looks like
// one, and makes one otherwise.
func requestID(r *http.Request) string {
	id := r.Header.Get("X-Request-ID")
	if len(id) == 0 || len(id) > 128 {
		return hex.EncodeToString(secret.RandomBytes(12))
	}
	for _, ch := range id {
		if (ch < 'a' || ch > 'z') && (ch < 'A' || ch > 'Z') && (ch < '0' || ch > '9') && !strings.ContainsRune("-_.:", ch) {
			return hex.EncodeToString(secret.RandomBytes(12))
		}
	}
	return id
}

// statusWriter notes what was answered, for the log.
type statusWriter struct {
	http.ResponseWriter
	status int
	bytes  int64
}

// WriteHeader notes the first status written.
func (w *statusWriter) WriteHeader(s int) {
	if w.status == 0 {
		w.status = s
	}
	w.ResponseWriter.WriteHeader(s)
}

func (w *statusWriter) Write(b []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	n, err := w.ResponseWriter.Write(b)
	w.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the connection (flushing,
// deadlines).
func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// logRequests gives every request an id and logs one line when it's
// answered (/up, polled by health checks, isn't).
func (a *App) logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		id := requestID(r)
		w.Header().Set("X-Request-ID", id)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, id))
		sw := &statusWriter{ResponseWriter: w}
		defer func() {
			if r.URL.Path == "/up" {
				return
			}
			status := sw.status
			if status == 0 {
				status = http.StatusOK
			}
			c := &Ctx{App: a, W: w, R: r}
			a.log.Info("request", "method", r.Method, "path", r.URL.Path, "status", status, "bytes", sw.bytes,
				"duration_ms", time.Since(start).Milliseconds(), "ip", c.ip(), "request_id", id)
		}()
		next.ServeHTTP(sw, r)
	})
}

// recoverPanics answers a panic anywhere (the files, the admin, /up) with
// a 500 and logs where it came from. A handler cutting the connection on
// purpose (http.ErrAbortHandler) is net/http's to handle.
// isAbort is whether a recovered panic is http.ErrAbortHandler: a handler
// cutting the connection on purpose.
func isAbort(rec any) bool {
	err, ok := rec.(error)
	return ok && errors.Is(err, http.ErrAbortHandler)
}

func (a *App) recoverPanics(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			rec := recover()
			if rec == nil {
				return
			}
			if isAbort(rec) {
				panic(rec)
			}
			a.log.Error("panic", "path", r.URL.Path, "request_id", requestIDFrom(r.Context()), "panic", rec, "stack", string(debug.Stack()))
			if sw, ok := w.(*statusWriter); !ok || sw.status == 0 {
				http.Error(w, "Something went wrong on our side.", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "strict-origin-when-cross-origin")
		h.Set("X-Frame-Options", "DENY")
		next.ServeHTTP(w, r)
	})
}

// cors lets plugins' public routes (/api/x/<key>/…, a form on another
// site) be called from any browser; everything else is same-origin.
func (a *App) cors(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if origin := r.Header.Get("Origin"); origin != "" && strings.HasPrefix(r.URL.Path, "/api/x/") {
			h := w.Header()
			h.Set("Access-Control-Allow-Origin", origin)
			h.Set("Vary", "Origin")
			h.Set("Access-Control-Allow-Headers", "Authorization, Content-Type")
			h.Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ---- small helpers -------------------------------------------------------

// str, boolOf and intOf are jsonv's readers, short because nearly every
// handler reads its body with them.
func str(v any) string { return jsonv.String(v) }

func boolOf(v any) bool { return jsonv.Bool(v) }

func intOf(v any) (int64, bool) { return jsonv.Int(v) }

func has(m map[string]any, key string) bool {
	_, ok := m[key]
	return ok
}

func firstOf(m map[string]any, keys ...string) any {
	for _, k := range keys {
		if v, ok := m[k]; ok && v != nil && v != "" {
			return v
		}
	}
	return nil
}

func asList(v any) []any {
	l, _ := v.([]any)
	return l
}

// allowed is whether l lets key through now.
func allowed(l *ratelimit.Limiter, key string) bool {
	ok, _ := l.Allow(key)
	return ok
}
