// Package plugin is Runwell's plugin API, and the host that runs plugins: Go
// packages compiled into the binary, each a Module that does only what it
// declared and an owner granted. Adding one is passing its Module to
// runwell.Main (the binary's main.go, cmd/runwell); installing it, granting
// it and switching it on happen at runtime, in Settings › Plugins, with no
// restart. Within a major version of Runwell this API only grows: names
// aren't removed or changed (docs/plugins.md).
//
// What a plugin reaches of Runwell is the *API it's handed on each call,
// holding the host's fixed set of capabilities:
//
//	records:read   API.Records / API.Record — clients, contacts, engagements, agreement
//	               versions, scope items, work, commitments and requests, as agents see them
//	records:write  API.AddNote / AddWork / AddCommitment / RecordEvent — through the core's
//	               own rules, recorded in history as whoever made the call
//	storage        API.KVGet / KVSet / KVDelete / KVList / KVValues / KVCount — its own namespace
//	mail           API.Mail — email through Settings › Email
//	http           API.HTTP — outbound requests, only to the hosts it declared
//	http:public    API.PublicHTTP — any public internet address, never this server's network
//	http:private   API.PrivateHTTP — any address, this server's own network included (a
//	               server of the owner's on it); never granted by switching on, only ticked
//	tables         API.Exec / Query / QueryRow / Tx — SQL on its own tables
//	files          API.Attach / Attachments / OpenFile / Detach — files on its own records
//	secrets        API.Seal / Unseal — values sealed with the install's key (an API key)
//	tools          API.Tools / RunTool — the agent tools the caller is offered, run as
//	               them (their role, never more); a change runs only inside that person's
//	               own request (their approval), never in the background
//	               API.PortalTools / RunPortalTool — the same for a portal contact
//	portal         routes with Portal: true, served to a client's contact signed in to
//	               their portal (req.Contact)
//	live           API.Publish / Online — events pushed to people's open tabs (the core's
//	               /api/live stream), and who has one open
//	notify         API.Notify — a notification in a person's bell (/notifications),
//	               emailed when the plugin asks
//	metrics        API.Metrics — read-only numbers for reports: each client's counts today,
//	               each engagement's state and agreed value, dated facts for trends
//	mentionables   Module.Mentionables / API.Reply — names people can @mention, heard as
//	               mention.created (with the person who mentioned it as the caller) and
//	               answered where they were made: a note on the record, or a reply in
//	               the relaying plugin's thread
//	content:external  Manifest.Content — the other sites its own screens load scripts,
//	               styles, fonts, images or data from (a map's), added to the app's
//	               Content-Security-Policy while it's on and granted (content.go)
//	mentions:relay API.RelayMention / Module.OnMentionReply — mentions in its own content
//	               (a chat message) passed on as their writer, and the replies taken in
//	emit           API.Emit — an event of its own (<key>.<what>) other plugins hook; always
//	machines       API.IssueToken / RevokeToken / Tokens — tokens for machines it works
//	               with (not people), kept hashed by the core; routes with Machine: true
//	               answer only a Bearer rwm_… token of its own (req.Machine)
//	rich text      API.RichText — HTML cleaned the core's way, its text and mentions; always
//	log, site      API.Log, API.Site — always
//
// A capability that wasn't granted answers an error instead of acting. A
// plugin is called through what its manifest declares: hooks (Runwell's
// events, such as client.created, to its OnEvent), routes (served under
// /api/x/<key>/…, to the handler each names, optionally an agent tool),
// tasks (run on a schedule) and screens (described as data: tables, forms
// and details the app draws, no code in the app), placed by its extension
// points: a nav page, record panels, a settings page, briefing sections and
// quick actions. A call that panics answers an error, and each call's
// context ends after the time limit. See docs/plugins.md.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Module is a plugin, as its package builds it.
type Module struct {
	Manifest Manifest
	// Handlers serve its routes, by the name each route gives as Export.
	Handlers map[string]Handler
	// OnEvent hears the events its manifest hooks; nil when it hooks none.
	OnEvent func(ctx context.Context, api *API, event string, data json.RawMessage) error
	// Tasks run on the schedule its manifest's tasks give, by name.
	Tasks map[string]Task
	// Default plugins are installed on first run, granted what they ask for
	// and switched on.
	Default bool
	// Mentionables are the names it offers to @mention (mentionables,
	// mentions.go); mentioning one is heard as mention.created.
	Mentionables func(ctx context.Context, api *API) ([]Mentionable, error)
	// OnMentionReply takes a reply to a mention it relayed (mentions:relay)
	// and answers the reply's id there.
	OnMentionReply func(ctx context.Context, api *API, r MentionReply) (string, error)
}

// Handler answers one of a plugin's routes.
type Handler func(ctx context.Context, api *API, req *Request) (*Response, error)

// Task is work a plugin does on a schedule (a nightly sync).
type Task func(ctx context.Context, api *API) error

// Request is a call to a plugin's route.
type Request struct {
	Method string
	Path   string
	// Params are the route's {name} segments, by name.
	Params  map[string]string
	Query   url.Values
	Headers map[string]string // Content-Type, Accept, User-Agent, Referer, Origin, X-Requested-With
	Body    []byte            // up to the route's MaxBody
	IP      string
	Actor   *Caller // nil on a public route called by no one signed in
	// Contact is the client's contact signed in to their portal, on a
	// portal route (and only there).
	Contact *Contact
	// Machine is the machine calling a machine route with one of the
	// plugin's own tokens (machines), and only there.
	Machine *Machine
}

// Contact is a client's contact calling a portal route from their portal.
type Contact struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	ClientID   int64  `json:"client_id"`
	ClientName string `json:"client_name"`
	// CanApprove is whether they may decide on agreements.
	CanApprove bool `json:"can_approve"`
	// Agent is true when their own agent calls, with their portal token.
	Agent bool `json:"agent"`
}

// Header is the request's header name, in any case.
func (r *Request) Header(name string) string {
	for k, v := range r.Headers {
		if strings.EqualFold(k, name) {
			return v
		}
	}
	return ""
}

// Caller is who's calling a route: a person on staff, or their agent.
type Caller struct {
	// ID is the person's user id (an agent's person, for an agent).
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Role  string `json:"role"` // owner, manager or member
	Agent bool   `json:"agent"`
	// AgentID is the agent user's own id when an agent calls (ID stays its
	// person's); Via is the app it came through ("Claude"), when it says.
	AgentID      int64    `json:"agent_id,omitempty"`
	Via          string   `json:"via,omitempty"`
	Capabilities []string `json:"capabilities"` // the permissions the role holds
}

// Can is whether the caller's role holds permission.
func (c *Caller) Can(permission string) bool {
	return c != nil && slices.Contains(c.Capabilities, permission)
}

// Response is a route's answer. Set-Cookie is dropped, and it's served with
// Content-Security-Policy: sandbox.
type Response struct {
	Status  int
	Headers map[string]string
	Body    []byte
}

// JSON is a response with v as its JSON body.
func JSON(status int, v any) *Response {
	b, err := json.Marshal(v)
	if err != nil {
		return &Response{Status: http.StatusInternalServerError, Headers: map[string]string{"Content-Type": "application/json; charset=utf-8"}, Body: []byte(`{"error":"internal"}`)}
	}
	return &Response{Status: status, Headers: map[string]string{"Content-Type": "application/json; charset=utf-8"}, Body: b}
}

// Invalid is a 422 the app shows beside the fields: {"errors": {field: [message]}}.
func Invalid(errs map[string][]string) *Response {
	return JSON(http.StatusUnprocessableEntity, map[string]any{"error": "invalid", "message": "Check the highlighted fields.", "errors": errs})
}

// Answer is err as a response when it's one the caller should see (a
// write's validation, a record that isn't there, a refused capability), and
// nil when it's the plugin failing (answer with the error itself: the core
// logs it and says the plugin failed).
func Answer(err error) *Response {
	if e, ok := errors.AsType[*apierr.Error](err); ok {
		body := map[string]any{"error": e.Code, "message": e.Message}
		if e.Errors != nil {
			body["errors"] = e.Errors
		}
		return JSON(e.Status, body)
	}
	if errors.Is(err, ErrNoRecord) {
		return JSON(http.StatusNotFound, map[string]any{"error": "not_found", "message": "Couldn't find that record."})
	}
	return nil
}

// Manifest is what a plugin says it is, needs and adds.
type Manifest struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Author      string `json:"author,omitempty"`
	Homepage    string `json:"homepage,omitempty"`
	Permissions Grants `json:"permissions"`
	// Hooks are the events it hears (client.created, engagement.closed, …);
	// "*.<what>" hears any plugin's <key>.<what> (*.approval.requested).
	Hooks  []string `json:"hooks"`
	Routes []Route  `json:"routes"`
	// Capabilities are the permissions it adds, each "<key>:<name>", held
	// by the roles it names; its routes can require them.
	Capabilities []Capability `json:"capabilities"`
	// Tasks run on a schedule: Every is a duration ("1h") or "nightly".
	Tasks []TaskSpec `json:"tasks"`
	// Schema makes its own tables (tables): CREATE TABLE IF NOT EXISTS and
	// CREATE INDEX IF NOT EXISTS statements, every name <key>_…, run each
	// time it loads switched on. A plugin that came from the Rails core
	// keeps its tables' names and columns, so their rows carry over.
	Schema string `json:"schema,omitempty"`
	// Content are the other sites its own screens load from, by CSP
	// directive ({"script-src": ["maps.googleapis.com"]}), added to the
	// app's Content-Security-Policy while it's on and granted
	// content:external (content.go).
	Content map[string][]string `json:"content,omitempty"`

	// Screens are what the app draws for it, as data (screens.go).
	Screens []Screen `json:"screens"`
	// Where they show: a page in the sidebar, tabs on records, cards on
	// home, a page in Settings, entries in the quick actions.
	Nav          *NavPage      `json:"nav,omitempty"`
	Panels       []Panel       `json:"panels"`
	Home         []HomeSection `json:"home"`
	Settings     *SettingsPage `json:"settings,omitempty"`
	QuickActions []QuickAction `json:"quick_actions"`
}

// Capability is a permission a plugin adds, shown on Settings › People with
// the core's.
type Capability struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Roles hold it (owner, manager, member): owners alone when empty.
	Roles []string `json:"roles,omitempty"`
}

// Grants is what a plugin asks for, and the subset an owner granted.
type Grants struct {
	RecordsRead  bool `json:"records:read"`
	RecordsWrite bool `json:"records:write"`
	Storage      bool `json:"storage"`
	// Tables: SQL on its own tables (named <key>_…), made from its
	// manifest's Schema (sql.go).
	Tables bool `json:"tables"`
	// Files: files kept in Runwell's file store, attached to its own
	// records (files.go).
	Files bool     `json:"files"`
	Mail  bool     `json:"mail"`
	HTTP  []string `json:"http"`
	// PublicHTTP: any public internet address (public.go), never this
	// server's own network.
	PublicHTTP bool `json:"http:public"`
	// PrivateHTTP: any address, the server's own network included (a
	// server the owner runs beside Runwell). Switching a plugin on never
	// grants it: an owner ticks it.
	PrivateHTTP bool `json:"http:private"`
	// Secrets: values sealed with the install's key (secrets.go).
	Secrets bool `json:"secrets"`
	// Tools: the agent tools the caller is offered, run as them (tools.go).
	Tools bool `json:"tools"`
	// Portal: routes served to a client's contact in their portal.
	Portal bool `json:"portal"`
	// Live: events pushed to the people it names, in their open tabs
	// (live.go), and whether they have one open.
	Live bool `json:"live"`
	// Notify: notifications in a person's bell, as the core's are (live.go).
	Notify bool `json:"notify"`
	// Metrics: the install's numbers for reports, read-only (metrics.go).
	Metrics bool `json:"metrics"`
	// Mentionables: names people can @mention (an AI agent), heard as
	// mention.created and answered where they were made (mentions.go).
	Mentionables bool `json:"mentionables"`
	// MentionsRelay: mentions in its own content passed on to the plugin
	// offering them, and their replies taken in (mentions.go).
	MentionsRelay bool `json:"mentions:relay"`
	// Machines: tokens for machines it works with, and routes they call
	// (machines.go).
	Machines bool `json:"machines,omitempty"`
	// ContentExternal: the app's pages may load what its manifest's Content
	// names from other sites (content.go).
	ContentExternal bool `json:"content:external"`
}

// MarshalJSON writes no hosts as [], never null.
func (g Grants) MarshalJSON() ([]byte, error) {
	type plain Grants
	if g.HTTP == nil {
		g.HTTP = []string{}
	}
	return json.Marshal(plain(g))
}

// Route is one a plugin serves under /api/x/<key>/.
type Route struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Export string `json:"export"` // the handler, in Module.Handlers
	// Public routes answer anyone (a form on another site). Portal routes
	// answer a client's contact signed in to their portal (portal). The rest
	// need someone on staff signed in, holding Capability when it names one
	// (the plugin's own, or a core permission such as manage_settings).
	Public bool `json:"public"`
	Portal bool `json:"portal,omitempty"`
	// Machine routes answer only a machine holding one of the plugin's own
	// tokens (Authorization: Bearer rwm_…; machines): no session, no CSRF.
	Machine    bool   `json:"machine,omitempty"`
	Capability string `json:"capability,omitempty"`
	// MaxBody is the largest request body it takes: 1 MB unless it says
	// more, at most 10 MB.
	MaxBody int64 `json:"max_body,omitempty"`
	// Tool makes the route an agent tool (MCP, the CLI), named as the
	// Rails plugin named it.
	Tool *Tool `json:"tool,omitempty"`
}

// Tool is a route as agents call it.
type Tool struct {
	Name        string `json:"name"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	// Body is the JSON object the route takes, under Wrap ({"entry": {…}}),
	// described for the agent: `{duration (required), worked_on, note}`.
	Wrap string `json:"wrap,omitempty"`
	Body string `json:"body,omitempty"`
	// Query are query parameters, beside the body.
	Query []ToolParam `json:"query,omitempty"`
	// Params are top-level body fields beside Wrap ({"record": "Client:12"}).
	Params []ToolParam `json:"params,omitempty"`
}

// ToolParam is one argument of a tool.
type ToolParam struct {
	Name        string `json:"name"`
	Type        string `json:"type,omitempty"` // string (default), integer, boolean
	Description string `json:"description"`
	Required    bool   `json:"required,omitempty"`
}

// TaskSpec is a task in the manifest: Module.Tasks[Name], run every Every.
type TaskSpec struct {
	Name  string `json:"name"`
	Every string `json:"every"` // a Go duration of at least a minute ("15m", "6h"), or "nightly"
}

// Interval is how often the task runs.
func (t TaskSpec) Interval() time.Duration {
	if t.Every == "nightly" {
		return 24 * time.Hour
	}
	d, _ := time.ParseDuration(t.Every)
	return d
}

// NavPage is a plugin's page in the sidebar: a screen it describes, at
// /p/<key>/<page>, or one of its own (a Svelte screen from its module's
// ui/, which the app's build copies into ui/src/plugins/<key>) at Path.
type NavPage struct {
	Label string `json:"label"`
	Page  string `json:"page,omitempty"` // a screen's id
	// Path is the app path of its own screen ("/my-day"), instead of Page.
	Path string `json:"path,omitempty"`
	// Icon is a lucide icon's name (clock, calendar, chart-bar…); a puzzle
	// piece when blank or unknown.
	Icon string `json:"icon,omitempty"`
	// Badge is a GET route of its own answering {"count": n} for the
	// person: the number beside the page in the sidebar (unread), none at 0.
	Badge string `json:"badge,omitempty"`
}

// Slots are where a panel can go.
var Slots = map[string]string{
	"client_panel":     "Client panel",
	"engagement_panel": "Engagement panel",
	"todo_panel":       "Work panel",
	"home_top":         "Home",
}

// Panel is a screen on a record's page (a tab) or above home's briefing.
// Its screen's source is called with record=<Type>:<id> (none on home).
type Panel struct {
	Slot   string `json:"slot"`
	Label  string `json:"label"`
	Screen string `json:"screen"`
}

// HomeSection is a section of the briefing on home: a screen, drawn as a
// card, for whoever may call its source.
type HomeSection struct {
	Title  string `json:"title"`
	Screen string `json:"screen"`
}

// SettingsPage is a page under Settings (for people who may manage
// settings), at /settings/plugins/<key>.
type SettingsPage struct {
	Label  string `json:"label"`
	Screen string `json:"screen"`
}

// QuickAction is an entry in the quick actions: a form screen opened for
// the record on screen when it's one of Types (Client, Engagement, Todo),
// or for none.
type QuickAction struct {
	Label  string   `json:"label"`
	Icon   string   `json:"icon,omitempty"`
	Types  []string `json:"types,omitempty"`
	Screen string   `json:"screen"`
}

const maxBody = 10 << 20

var (
	capFormat   = regexp.MustCompile(`^[a-z][a-z0-9_]*:[a-z][a-z0-9_]*$`)
	eventFormat = regexp.MustCompile(`^[a-z][a-z_]*(\.[a-z][a-z_]*)+$`)
	// anyPluginFormat hooks an event any plugin emits: *.approval.requested.
	anyPluginFormat = regexp.MustCompile(`^\*(\.[a-z][a-z_]*)+$`)
	idFormat        = regexp.MustCompile(`^[a-z][a-z0-9_-]{0,40}$`)
	toolFormat      = regexp.MustCompile(`^[a-z][a-z0-9_]{1,60}$`)
)

// Roles are the roles a capability can name.
var Roles = []string{"owner", "manager", "member"}

// RecordTypes are the records a quick action can be for.
var RecordTypes = []string{"Client", "Engagement", "Todo"}

// Plugin is one installed plugin.
type Plugin struct {
	ID          int64
	Key         string
	Name        string
	Version     string
	Description string
	Manifest    Manifest
	Granted     Grants
	Enabled     bool
	CreatedAt   store.Time
	UpdatedAt   store.Time

	module *Module // nil when the binary doesn't have it
}

// CapGroup is a plugin's capabilities, as its manifest declares them; the
// enabled plugins' join the core's permissions.
type CapGroup struct {
	Group  string
	Caps   []string
	Labels []string
	Roles  [][]string
}

// Actor is who did something, as the core knows them (*core.Actor), passed
// back to Env; nil for the system (an event, a task).
type Actor any

// Env is what plugins reach of Runwell beyond their own storage.
type Env interface {
	Setting(ctx context.Context, key string) (map[string]any, error)
	// ReplaceSetting writes data as key's, replacing what was there.
	ReplaceSetting(ctx context.Context, key string, data map[string]any) error
	// Site is API.Site's answer: the install's name, address, contact.
	Site(ctx context.Context) (Site, error)
	SendMail(ctx context.Context, m mail.Message) error
	// Records answers a query for records, as agents see them.
	Records(ctx context.Context, q Query) ([]map[string]any, error)
	// Write runs one of the write verbs (records.go) as actor.
	Write(ctx context.Context, actor Actor, plugin string, w any) (map[string]any, error)
	// Audit records what someone did to a plugin.
	Audit(ctx context.Context, actor Actor, action string, id int64, label string, meta map[string]any)
	// Files (files.go): stored the core's way, attached to a plugin's records.
	AttachFile(ctx context.Context, recordType string, recordID int64, name, filename, contentType string, r io.Reader) (*File, error)
	Attachments(ctx context.Context, recordType string, recordID int64, name string) ([]File, error)
	OpenFile(ctx context.Context, recordType string, recordID, fileID int64) (io.ReadCloser, *File, error)
	DetachFile(ctx context.Context, recordType string, recordID, fileID int64) error
	// Tools are the agent tools actor is offered: a person's (or their
	// agent's) by role, a contact's portal ones (tools.go).
	Tools(ctx context.Context, actor Actor) ([]ToolInfo, error)
	// RunTool runs one as actor, recorded as made through via (the plugin).
	RunTool(ctx context.Context, actor Actor, via, name string, args map[string]any) (map[string]any, error)
	// Seal and Unseal are the install's key at work (secrets.go).
	Seal(plain string) string
	Unseal(sealed string) (string, bool)
}

// Site is the install a plugin runs on.
type Site struct {
	Name     string `json:"name"`
	URL      string `json:"url"`
	Locale   string `json:"locale"`
	TimeZone string `json:"time_zone"`
	// Location is that zone, to work out dates in (TimeZone may be a Rails
	// zone name, which Go doesn't know).
	Location     *time.Location `json:"-"`
	ContactEmail string         `json:"contact_email"`
	// MaxUpload is the largest file one upload may be, in bytes
	// (RUNWELL_MAX_UPLOAD_MB).
	MaxUpload int64 `json:"max_upload"`
	// Terms are the install's words for its things (Settings › Names):
	// Terms["client"] is {"one": "Client", "other": "Clients"}, and so on
	// for engagement, scope_item, work and commitment.
	Terms map[string]map[string]string `json:"terms"`
}

// Spawn runs fn in the background for as long as the app lives; false when
// it's closed (fn doesn't run).
type Spawn func(ctx context.Context, name string, fn func(context.Context)) bool

// validate checks a module: its manifest, a handler for each route, and a
// function for each task.
func (m *Module) validate() error {
	if err := m.Manifest.validate(); err != nil {
		return err
	}
	for _, r := range m.Manifest.Routes {
		if m.Handlers[r.Export] == nil {
			return fmt.Errorf("plugin %s: route %s %s names %q, which has no handler", m.Manifest.Key, r.Method, r.Path, r.Export)
		}
	}
	for _, t := range m.Manifest.Tasks {
		if m.Tasks[t.Name] == nil {
			return fmt.Errorf("plugin %s: task %q has no function in Tasks", m.Manifest.Key, t.Name)
		}
	}
	if len(m.Manifest.Hooks) > 0 && m.OnEvent == nil {
		return fmt.Errorf("plugin %s hooks events but has no OnEvent", m.Manifest.Key)
	}
	if m.Mentionables != nil && (!m.Manifest.Permissions.Mentionables || m.OnEvent == nil) {
		return fmt.Errorf("plugin %s offers mentionables: it needs permissions.mentionables and an OnEvent for mention.created", m.Manifest.Key)
	}
	if m.OnMentionReply != nil && !m.Manifest.Permissions.MentionsRelay {
		return fmt.Errorf("plugin %s takes mention replies: it needs permissions.mentions:relay", m.Manifest.Key)
	}
	return nil
}

// validate checks what a manifest says: its key, name and version, its
// routes, tools, capabilities and tasks, its screens and where they show,
// and the hosts it asks to reach.
func (m *Manifest) validate() error {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	if !keyFormat.MatchString(m.Key) {
		add("key must be lowercase letters, digits and _, starting with a letter")
	}
	if m.Schema != "" {
		if err := checkSchema(m.Key, m.Schema); err != nil {
			add("schema: %v", err)
		}
		if !m.Permissions.Tables {
			add("schema needs permissions.tables")
		}
	}
	if m.Name == "" {
		add("name can't be blank")
	}
	if m.Version == "" {
		add("version can't be blank")
	}
	for i, c := range m.Capabilities {
		if !capFormat.MatchString(c.Key) || !strings.HasPrefix(c.Key, m.Key+":") || c.Label == "" {
			add("capabilities[%d] needs a label and a key \"%s:<name>\"", i, m.Key)
		}
		for _, r := range c.Roles {
			if !slices.Contains(Roles, r) {
				add("capabilities[%d].roles are owner, manager or member, not %q", i, r)
			}
		}
	}
	for _, h := range m.Hooks {
		if !eventFormat.MatchString(h) && !anyPluginFormat.MatchString(h) {
			add("hooks name events like client.created (or *.approval.requested, any plugin's), not %q", h)
		}
	}
	errs = append(errs, m.validateRoutes()...)
	for i, t := range m.Tasks {
		if t.Name == "" || (t.Every != "nightly" && t.Interval() < time.Minute) {
			add("tasks[%d] needs a name and every: a duration of a minute or more, or \"nightly\"", i)
		}
	}
	for _, host := range m.Permissions.HTTP {
		if host == "*" || strings.Contains(host, "/") {
			add("permissions.http names hosts, not \"*\" or URLs: %s", host)
		}
	}
	if m.Nav != nil && m.Nav.Badge != "" {
		if r := m.Route("GET", m.Nav.Badge); r == nil || r.Public || r.Portal {
			add("nav.badge must be a GET route of its own, for people on staff")
		}
	}
	errs = append(errs, m.validateContent()...)
	errs = append(errs, m.validateScreens()...)
	if len(errs) > 0 {
		return fmt.Errorf("plugin %s: %s", m.Key, strings.Join(errs, "; "))
	}
	return nil
}

// validateRoutes checks the routes: their paths, methods and limits, and
// what each needs to be a portal route, a machine route or an agent tool.
func (m *Manifest) validateRoutes() []string {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	tools := map[string]bool{}
	for i, r := range m.Routes {
		if r.Export == "" || !strings.HasPrefix(r.Path, "/") {
			add("routes[%d] needs a path starting with / and an export", i)
		}
		switch strings.ToUpper(r.Method) {
		case "GET", "POST", "PATCH", "PUT", "DELETE":
		default:
			add("routes[%d].method is GET, POST, PATCH, PUT or DELETE", i)
		}
		if r.MaxBody > maxBody {
			add("routes[%d].max_body is at most 10 MB", i)
		}
		if strings.Contains(r.Capability, ":") && !slices.ContainsFunc(m.Capabilities, func(c Capability) bool { return c.Key == r.Capability }) {
			add("routes[%d].capability must be one the plugin declares under capabilities", i)
		}
		if r.Portal {
			switch {
			case !m.Permissions.Portal:
				add("routes[%d] is a portal route: it needs permissions.portal", i)
			case r.Public || r.Capability != "" || r.Tool != nil:
				add("routes[%d] is a portal route: it can't be public, need a capability or be an agent tool", i)
			}
		}
		if r.Machine {
			switch {
			case !m.Permissions.Machines:
				add("routes[%d] is a machine route: it needs permissions.machines", i)
			case r.Public || r.Portal || r.Capability != "" || r.Tool != nil:
				add("routes[%d] is a machine route: it can't be public, a portal route, need a capability or be an agent tool", i)
			}
		}
		if r.Tool != nil {
			switch {
			case r.Public:
				add("routes[%d] is public: an agent tool needs a route that's signed in", i)
			case !toolFormat.MatchString(r.Tool.Name) || r.Tool.Title == "":
				add("routes[%d].tool needs a title and a name in lowercase letters, digits and _", i)
			case tools[r.Tool.Name]:
				add("routes[%d].tool %s is named twice", i, r.Tool.Name)
			}
			tools[r.Tool.Name] = true
		}
	}
	return errs
}

// Route is the route serving method and path, nil when there's none.
func (m *Manifest) Route(method, path string) *Route {
	r, _ := m.Match(method, path)
	return r
}

// Match is the route for method and path, and the values of its {name}
// segments (/priorities/{todo_id} matches /priorities/12 with todo_id 12).
// An exact path wins over one with parameters.
func (m *Manifest) Match(method, path string) (*Route, map[string]string) {
	var found *Route
	var params map[string]string
	for i := range m.Routes {
		r := &m.Routes[i]
		if !strings.EqualFold(r.Method, method) {
			continue
		}
		if r.Path == path {
			return r, map[string]string{}
		}
		if found == nil {
			if p, ok := matchPath(r.Path, path); ok {
				found, params = r, p
			}
		}
	}
	return found, params
}

// matchPath matches a route's path, with {name} segments, against a path.
func matchPath(pattern, path string) (map[string]string, bool) {
	want := strings.Split(strings.Trim(pattern, "/"), "/")
	got := strings.Split(strings.Trim(path, "/"), "/")
	if len(want) != len(got) || !strings.Contains(pattern, "{") {
		return nil, false
	}
	params := map[string]string{}
	for i, w := range want {
		if strings.HasPrefix(w, "{") && strings.HasSuffix(w, "}") {
			v, err := url.PathUnescape(got[i])
			if err != nil || v == "" {
				return nil, false
			}
			params[w[1:len(w)-1]] = v
			continue
		}
		if w != got[i] {
			return nil, false
		}
	}
	return params, true
}

// Screen is the screen with id, nil when there's none.
func (m *Manifest) Screen(id string) *Screen {
	for i := range m.Screens {
		if m.Screens[i].ID == id {
			return &m.Screens[i]
		}
	}
	return nil
}

// Additions is what a plugin adds, in words, for Settings › Plugins (the
// Rails Runwell::Plugins.additions).
func (m *Manifest) Additions() []string {
	out := []string{}
	for _, p := range m.Panels {
		out = append(out, Slots[p.Slot])
	}
	if m.Nav != nil {
		out = append(out, "Nav: "+m.Nav.Label)
	}
	for _, h := range m.Home {
		out = append(out, "Home: "+h.Title)
	}
	for _, q := range m.QuickActions {
		out = append(out, "Quick action: "+q.Label)
	}
	for _, c := range m.Capabilities {
		out = append(out, "Permission: "+c.Label)
	}
	if m.Settings != nil {
		out = append(out, "Settings: "+m.Settings.Label)
	}
	for _, t := range m.Tasks {
		if t.Every == "nightly" {
			out = append(out, "Nightly: "+t.Name)
		} else {
			out = append(out, "Every "+t.Every+": "+t.Name)
		}
	}
	for _, r := range m.Routes {
		if r.Tool != nil {
			out = append(out, "Agent tool: "+r.Tool.Name)
		}
	}
	if hosts := m.contentHosts(); len(hosts) > 0 {
		out = append(out, "Loads content from: "+strings.Join(hosts, ", "))
	}
	return out
}

// BuiltIn says whether this binary has the plugin; one it doesn't (a
// plugin from an older build) stays off.
func (p *Plugin) BuiltIn() bool { return p.module != nil }
