package core

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Settings › Plugins: the plugins this build has, each installed, granted
// what it asks for and switched on here (the Rails Settings::Plugins
// controllers), and what the app needs to draw the ones that are on: their
// places (nav, record panels, home, settings, quick actions), their screens,
// and their routes under /api/x/<key>/…, which agents also reach as tools.
//
// A plugin here is compiled into the binary, so there is nothing to
// download: installing one is recording it, and it updates when Runwell
// does.

func init() {
	routeSets = append(routeSets, (*App).pluginRoutes)
	navPlugins = (*App).pluginNav
}

const switchPluginBody = `{enabled (required): boolean, granted: {"records:read", "records:write", storage, tables, files, mail, "http:public", "http:private", secrets, tools, portal, live, notify, metrics, mentionables, "mentions:relay", machines, "content:external": boolean, http: [host]}}. Switching on with no granted grants what it asks for (show_plugin lists it).`

func (a *App) pluginRoutes(mux *http.ServeMux) {
	settings := need("manage_settings")
	a.handle(mux, "GET /api/settings/plugins", a.listPlugins, settings,
		tool("list_plugins", "List plugins: installed (with any newer release), available to install, and recent changes"))
	a.handle(mux, "GET /api/settings/plugins/{key}", a.showPlugin, settings,
		tool("show_plugin", "Show one plugin: what it does, whether it's on, and what it adds"))
	a.handle(mux, "PATCH /api/settings/plugins/{key}", a.switchPlugin, settings,
		tool("switch_plugin", "Switch a plugin on or off", body("plugin", switchPluginBody, true)))
	a.handle(mux, "DELETE /api/settings/plugins/{key}", a.removePlugin, settings,
		tool("remove_plugin", "Remove an installed plugin",
			note("It stops at once and drops off Settings › Plugins' installed list. Its data stays, so installing it again brings it back, unless purge_data is true."),
			query("purge_data", "boolean", "Delete its stored data too, for good.")))
	a.handle(mux, "POST /api/settings/plugins/installations", a.installPlugin, settings,
		tool("install_plugin", "Install a plugin",
			note("repo: a plugin's key from list_plugins' available (time_tracking), or its repository's name (Martin-Business-Consultants/runwell-time-tracking). Plugins are built into Runwell, so this records it: it starts off, and switch_plugin turns it on."),
			bodyParam("repo", "string", "The plugin's key, or owner/name of its repository.", true), next("switch_plugin")))
	a.handle(mux, "POST /api/settings/plugins/check", a.checkPlugins, settings,
		tool("check_plugin_updates", "Check every installed plugin for a newer release now"))
	a.handle(mux, "POST /api/settings/plugins/{key}/upgrade", a.upgradePlugin, settings,
		tool("update_plugin", "Update an installed plugin to its newest release",
			note("Plugins are built into Runwell: one updates when Runwell does, so this only says so.")))

	// What the app draws for the plugins that are on.
	a.handle(mux, "GET /api/plugin_extensions", a.pluginExtensions)
	a.handle(mux, "GET /api/plugin_screens/{key}", a.pluginScreens)
	// A plugin's own routes, as its manifest declares them.
	a.handle(mux, "/api/x/{key}/{rest...}", a.pluginRoute, open)
}

// ---- Settings › Plugins ------------------------------------------------------

func (a *App) listPlugins(c *Ctx) error {
	ctx := c.R.Context()
	installed := a.plugins.List()
	list, failed, on := []Obj{}, Obj{}, 0
	for _, p := range installed {
		if p.Enabled {
			on++
		}
		if !p.BuiltIn() {
			failed = failed.Set(p.Key, "It isn't in this build of Runwell, so it stays off. Its data is kept.")
		}
		list = append(list, a.pluginJSON(p))
	}
	available := []Obj{}
	for _, m := range a.plugins.Available() {
		available = append(available, O("key", m.Key, "repo", nil, "name", m.Name, "description", m.Description, "version", m.Version,
			"requested", m.Permissions, "adds", m.Additions()))
	}
	changes, err := a.pluginChanges(ctx)
	if err != nil {
		return err
	}
	if changes == nil {
		changes = []Obj{}
	}
	c.Summary = fmt.Sprintf("Runwell %s: %d of %d installed plugins on", a.cfg.Version, on, len(installed))
	c.Crumbs = []Crumb{}
	for _, m := range a.plugins.Available() {
		c.Crumbs = append(c.Crumbs, step("Install "+m.Name, "install_plugin", "repo", m.Key))
	}
	return c.JSON(200, O("summary", c.Summary, "runwell_version", a.cfg.Version, "plugins", list, "failed_to_load", failed,
		"available", available, "changes", changes))
}

// pluginChanges are the last installs, upgrades and removals, from the
// audit log (what the Rails core kept as PluginChange).
func (a *App) pluginChanges(ctx context.Context) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT e.id, e.kind, e.payload, e.occurred_at, e.actor_user_id, COALESCE(NULLIF(u.name, ''), u.email_address, e.actor) FROM events e LEFT JOIN users u ON u.id = e.actor_user_id
		WHERE e.subject_type = 'Setting' AND e.kind IN ('plugin.installed', 'plugin.upgraded', 'plugin.removed') ORDER BY e.occurred_at DESC, e.id DESC LIMIT 10`)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var kind string
		var payload store.JSON
		var at store.Time
		var userID store.NullInt
		var name *string
		if err := r.Scan(&id, &kind, &payload, &at, &userID, &name); err != nil {
			return nil, err
		}
		p := payload.Map()
		action := map[string]string{"plugin.installed": "install", "plugin.upgraded": "update", "plugin.removed": "remove"}[kind]
		key := str(p["plugin"])
		pname := key
		if m := a.plugins.Get(key); m != nil {
			pname = m.Name
		}
		verb := map[string]string{"install": "Installed", "update": "Updated", "remove": "Removed"}[action]
		var by any
		if userID.Valid && name != nil {
			by = O("id", userID.Int64, "name", *name)
		}
		return O("id", id, "key", key, "repo", nil, "action", action, "from_version", p["from_version"], "to_version", p["to_version"],
			"status", "done", "message", nil, "summary", verb+" "+pname, "requested_by", by, "created_at", at, "finished_at", at), nil
	})
}

func (a *App) installedPlugin(c *Ctx) (*plugin.Plugin, error) {
	p := a.plugins.Get(c.Param("key"))
	if p == nil {
		return nil, apierr.NotFound("Couldn't find the plugin " + c.Param("key") + ".")
	}
	return p, nil
}

func (a *App) showPlugin(c *Ctx) error {
	p, err := a.installedPlugin(c)
	if err != nil {
		return err
	}
	c.Summary = fmt.Sprintf("%s %s: %s", p.Name, p.Version, map[bool]string{true: "on", false: "off"}[p.Enabled])
	o := a.pluginJSON(p).Set("summary", c.Summary)
	return c.JSON(200, o)
}

func (a *App) switchPlugin(c *Ctx) error {
	p, err := a.installedPlugin(c)
	if err != nil {
		return err
	}
	var body struct {
		Plugin struct {
			Enabled any            `json:"enabled"`
			Granted *plugin.Grants `json:"granted"`
		} `json:"plugin"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	var enabled *bool
	if body.Plugin.Enabled != nil {
		on := boolOf(body.Plugin.Enabled)
		enabled = &on
	}
	if enabled == nil && body.Plugin.Granted == nil {
		return apierr.BadRequest("param is missing or the value is empty: plugin")
	}
	p, err = a.plugins.Configure(c.R.Context(), c.Actor, p.Key, body.Plugin.Granted, enabled)
	if err != nil {
		return err
	}
	if enabled != nil {
		c.Summary = p.Name + map[bool]string{true: " switched on.", false: " switched off."}[p.Enabled]
	} else {
		c.Summary = p.Name + ": permissions saved."
	}
	return c.JSON(200, a.pluginJSON(p).Set("summary", c.Summary))
}

func (a *App) removePlugin(c *Ctx) error {
	p, err := a.installedPlugin(c)
	if err != nil {
		return err
	}
	purge := c.truthy("purge_data")
	if err := a.plugins.Remove(c.R.Context(), c.Actor, p.Key, purge); err != nil {
		return err
	}
	if purge {
		c.Summary = "Removed " + p.Name + ", and its data with it."
	} else {
		c.Summary = "Removed " + p.Name + ". Its data stays, so installing it again brings it back."
	}
	return c.JSON(200, O("summary", c.Summary, "key", p.Key))
}

// pluginKeyFor reads install_plugin's repo: a key, or a repository's name
// (owner/runwell-time-tracking → time_tracking).
func pluginKeyFor(repo string) string {
	repo = strings.TrimSpace(strings.TrimSuffix(repo, ".git"))
	if i := strings.LastIndex(repo, "/"); i >= 0 {
		repo = repo[i+1:]
	}
	for _, prefix := range []string{"runwell-", "plugin-"} {
		repo = strings.TrimPrefix(repo, prefix)
	}
	return strings.ToLower(strings.ReplaceAll(repo, "-", "_"))
}

func (a *App) installPlugin(c *Ctx) error {
	var body struct {
		Repo   string `json:"repo"`
		Plugin struct {
			Key string `json:"key"`
		} `json:"plugin"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	repo := body.Repo
	if repo == "" {
		repo = body.Plugin.Key
	}
	if strings.TrimSpace(repo) == "" {
		return apierr.BadRequest("Name the plugin to install: {\"repo\": \"time_tracking\"}.")
	}
	key := pluginKeyFor(repo)
	if p := a.plugins.Get(key); p != nil {
		return apierr.Conflict(p.Name + " is already installed.")
	}
	p, err := a.plugins.Install(c.R.Context(), c.Actor, key)
	if err != nil {
		return err
	}
	c.Summary = fmt.Sprintf("Installed %s %s. It starts off: switch it on here.", p.Name, p.Version)
	c.Crumbs = []Crumb{step("Switch it on", "switch_plugin", "key", p.Key, "plugin", map[string]any{"enabled": true})}
	return c.JSON(201, a.pluginJSON(p).Set("summary", c.Summary))
}

func (a *App) checkPlugins(c *Ctx) error {
	c.Summary = "Every plugin is at its newest release."
	return c.JSON(200, O("summary", c.Summary, "newer", []string{},
		"note", "Plugins are built into Runwell, so each updates when Runwell does."))
}

func (a *App) upgradePlugin(c *Ctx) error {
	p, err := a.installedPlugin(c)
	if err != nil {
		return err
	}
	return apierr.Conflict(p.Name + " is built into Runwell and updates when Runwell does: it's at its newest release for this build.")
}

// pluginJSON is a plugin as Settings › Plugins shows it (settings/plugins/
// index.json's entries), with what the Go host adds: what it asks for and
// was granted, the events it hears and the tools it brings.
func (a *App) pluginJSON(p *plugin.Plugin) Obj {
	m := p.Manifest
	tools := []string{}
	for _, r := range m.Routes {
		if r.Tool != nil {
			tools = append(tools, r.Tool.Name)
		}
	}
	var settingsPath, pagePath any
	if m.Settings != nil {
		settingsPath = "/settings/plugins/" + p.Key
	}
	if m.Nav != nil {
		pagePath = "/p/" + p.Key + "/" + m.Nav.Page
	}
	return O("key", p.Key, "name", p.Name, "version", p.Version, "description", p.Description,
		"author", nullable(m.Author), "requires", nil, "homepage", nullable(m.Homepage), "repo", nil, "newer_release", nil,
		"compatible", p.BuiltIn(), "enabled", p.Enabled, "adds", m.Additions(),
		"requested", m.Permissions, "granted", p.Granted, "built_in", p.BuiltIn(), "hooks", orEmpty(m.Hooks), "tools", tools,
		"capabilities", orEmptyCaps(m.Capabilities), "has_settings", m.Settings != nil, "settings_path", settingsPath, "page_path", pagePath,
		"created_at", p.CreatedAt, "updated_at", p.UpdatedAt)
}

func orEmptyCaps(s []plugin.Capability) []plugin.Capability {
	if s == nil {
		return []plugin.Capability{}
	}
	return s
}

// ---- what the app draws ----------------------------------------------------

// mayCall is whether the actor may call a plugin's route: anyone for a
// public one; else someone on staff, holding its capability when it names
// one, and only reading on a read-only connection.
func (a *App) mayCall(c *Ctx, r *plugin.Route, method string) bool {
	if r.Public {
		return true
	}
	if r.Portal || r.Machine { // a client's contact's or a machine's, never staff's
		return false
	}
	if c.Actor == nil || c.Actor.User == nil || !c.Actor.User.Active() {
		return false
	}
	if c.Actor.Token != nil && c.Actor.Token.ReadOnly && method != http.MethodGet {
		return false
	}
	if r.Capability == "" {
		return true
	}
	if !strings.Contains(r.Capability, ":") && !slices.ContainsFunc(Permissions, func(p Permission) bool { return p.Key == r.Capability }) {
		return false // a core permission there isn't: nobody
	}
	return c.Can(r.Capability)
}

// openScreens are the plugin's screens the actor may open: those whose
// source route they may call.
func (a *App) openScreens(c *Ctx, p *plugin.Plugin) map[string]plugin.Screen {
	out := map[string]plugin.Screen{}
	for _, s := range p.Manifest.Screens {
		if r := p.Manifest.Route("GET", s.Source); r != nil && a.mayCall(c, r, http.MethodGet) {
			out[s.ID] = s
		}
	}
	return out
}

// enabledPlugins are the plugins that are on and built in.
func (a *App) enabledPlugins() []*plugin.Plugin {
	var out []*plugin.Plugin
	for _, p := range a.plugins.List() {
		if p.Enabled && p.BuiltIn() {
			out = append(out, p)
		}
	}
	return out
}

// navOrder is how the person arranged plugins' links (Settings › Your
// account › Your menu, users.nav_plugins {order, hidden}): their order
// first, the rest after, hidden ones out.
func (a *App) navOrder(ctx context.Context, actor *Actor, keys []string) ([]string, error) {
	var raw store.JSON
	if err := a.store.QueryRowContext(ctx, `SELECT nav_plugins FROM users WHERE id = ?`, actor.User.Person().ID).Scan(&raw); err != nil && !isNoRows(err) {
		return nil, err
	}
	var order, hidden []string
	for _, v := range raw.List() { // an older shape: just the order
		order = append(order, str(v))
	}
	if m := raw.Map(); len(m) > 0 {
		for _, v := range asList(m["order"]) {
			order = append(order, str(v))
		}
		for _, v := range asList(m["hidden"]) {
			hidden = append(hidden, str(v))
		}
	}
	var out []string
	for _, k := range order {
		if slices.Contains(keys, k) && !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	for _, k := range keys {
		if !slices.Contains(out, k) {
			out = append(out, k)
		}
	}
	return slices.DeleteFunc(out, func(k string) bool { return slices.Contains(hidden, k) }), nil
}

// pluginNav is the plugins' pages in the sidebar, for /api/nav: [{href,
// label, key, icon}], as the person arranged them.
func (a *App) pluginNav(ctx context.Context, actor *Actor) ([]Obj, error) {
	if actor == nil || actor.User == nil {
		return []Obj{}, nil
	}
	c := &Ctx{App: a, Actor: actor}
	items := map[string]Obj{}
	var keys []string
	for _, p := range a.enabledPlugins() {
		n := p.Manifest.Nav
		if n == nil {
			continue
		}
		if _, ok := a.openScreens(c, p)[n.Page]; !ok && n.Path == "" {
			continue
		}
		keys = append(keys, p.Key)
		items[p.Key] = O("href", navHref(p.Key, n), "label", n.Label, "key", p.Key, "icon", nullable(n.Icon), "badge", a.pluginBadge(ctx, c, p))
	}
	order, err := a.navOrder(ctx, actor, keys)
	if err != nil {
		return nil, err
	}
	out := []Obj{}
	for _, k := range order {
		out = append(out, items[k])
	}
	return out, nil
}

// pluginExtensions is where the plugins that are on show for the actor:
// the nav, record panels (as tabs) and home's cards, settings pages and
// quick actions. The app reads it once someone is signed in.
func (a *App) pluginExtensions(c *Ctx) error {
	ctx := c.R.Context()
	nav, err := a.pluginNav(ctx, c.Actor)
	if err != nil {
		return err
	}
	panels, home, settings, quick := []Obj{}, []Obj{}, []Obj{}, []Obj{}
	for _, p := range a.enabledPlugins() {
		m := p.Manifest
		screens := a.openScreens(c, p)
		for _, pn := range m.Panels {
			if s, ok := screens[pn.Screen]; ok {
				panels = append(panels, O("key", p.Key, "plugin", p.Name, "slot", pn.Slot, "label", pn.Label, "screen", s.ID))
			}
		}
		for _, h := range m.Home {
			if s, ok := screens[h.Screen]; ok {
				home = append(home, O("key", p.Key, "plugin", p.Name, "title", h.Title, "screen", s.ID))
			}
		}
		if m.Settings != nil && c.Can("manage_settings") {
			settings = append(settings, O("key", p.Key, "label", m.Settings.Label, "href", "/settings/plugins/"+p.Key))
		}
		for _, q := range m.QuickActions {
			if s, ok := screens[q.Screen]; ok {
				types := q.Types
				if types == nil {
					types = []string{}
				}
				quick = append(quick, O("key", p.Key, "label", q.Label, "icon", nullable(q.Icon), "types", types, "screen", s.ID))
			}
		}
	}
	// Which plugins are on: a built-in plugin's own screens (ui/src/plugins)
	// show only while it is.
	enabled := []string{}
	for _, p := range a.enabledPlugins() {
		enabled = append(enabled, p.Key)
	}
	return c.JSON(200, O("enabled", enabled, "nav", nav, "panels", panels, "home", home, "settings", settings, "quick_actions", quick))
}

// pluginScreens is what the app draws a plugin's screens from: those the
// actor may open, and the routes they may write to (so it shows only the
// forms and buttons that would work).
func (a *App) pluginScreens(c *Ctx) error {
	p := a.plugins.Get(c.Param("key"))
	if p == nil || !p.Enabled || !p.BuiltIn() {
		return apierr.NotFound("Couldn't find the plugin " + c.Param("key") + ", or it's off.")
	}
	open := a.openScreens(c, p)
	if len(open) == 0 {
		return forbidden()
	}
	screens := []plugin.Screen{}
	for _, s := range p.Manifest.Screens {
		if _, ok := open[s.ID]; ok {
			screens = append(screens, s)
		}
	}
	writes := []string{}
	for i := range p.Manifest.Routes {
		r := &p.Manifest.Routes[i]
		method := strings.ToUpper(r.Method)
		if method != http.MethodGet && a.mayCall(c, r, method) {
			writes = append(writes, method+" "+r.Path)
		}
	}
	return c.JSON(200, O("plugin", O("key", p.Key, "name", p.Name, "version", p.Version, "nav", p.Manifest.Nav, "settings", p.Manifest.Settings,
		"screens", screens, "writes", writes)))
}

// ---- a plugin's routes -------------------------------------------------------

// pluginRoute hands a request to the plugin handler its manifest names.
func (a *App) pluginRoute(c *Ctx) error {
	p := a.plugins.Get(c.Param("key"))
	if p == nil || !p.Enabled || !p.BuiltIn() {
		return apierr.NotFound("Couldn't find the plugin " + c.Param("key") + ", or it's off.")
	}
	path := "/" + c.Param("rest")
	rt, params := p.Manifest.Match(c.R.Method, path)
	if rt == nil {
		return apierr.NotFound("The plugin " + p.Key + " has no route " + c.R.Method + " " + path + ".")
	}
	machine, err := a.pluginRouteCaller(c, p, rt, path)
	if err != nil {
		return err
	}
	limit := rt.MaxBody
	if limit <= 0 {
		limit = 1 << 20
	}
	var body []byte
	if c.R.Body != nil { // an agent's call, routed in-process, may have none
		var err error
		if body, err = io.ReadAll(io.LimitReader(c.R.Body, limit+1)); err != nil {
			return err
		}
	}
	if int64(len(body)) > limit {
		return &apierr.Error{Status: 413, Code: "too_large", Message: fmt.Sprintf("The request is larger than this route takes (%d MB).", max(limit>>20, 1))}
	}
	headers := pluginHeaders(c.R)
	query := c.R.URL.Query()
	query.Del("envelope")
	req := &plugin.Request{Method: c.R.Method, Path: path, Params: params, Query: query, Headers: headers, Body: body, IP: c.ip(), Machine: machine}
	var actor plugin.Actor
	if rt.Portal {
		ct := c.Actor.Contact
		req.Contact = &plugin.Contact{ID: ct.ID, Name: ct.Name, Email: ct.Email, ClientID: ct.ClientID, CanApprove: ct.CanApprove, Agent: c.Actor.Token != nil}
		_ = a.store.QueryRowContext(c.R.Context(), `SELECT name FROM clients WHERE id = ?`, ct.ClientID).Scan(&req.Contact.ClientName)
		actor = c.Actor
	} else if c.Actor != nil && c.Actor.User != nil && !rt.Machine {
		req.Actor = a.pluginCaller(c)
		actor = c.Actor
	}
	res, err := a.plugins.Serve(c.R.Context(), p.Key, rt.Export, req, actor)
	if err != nil {
		a.log.Warn("plugin route", "plugin", p.Key, "err", err)
		return &apierr.Error{Status: 502, Code: "plugin_error", Message: "The plugin failed to answer."}
	}
	status := res.Status
	if status == 0 {
		status = http.StatusOK
	}
	ctype := ""
	for k, v := range res.Headers {
		if strings.EqualFold(k, "Content-Type") {
			ctype = v
		}
	}
	// An agent's call gets its answer in the envelope, as the core's do.
	if c.envelope() && (ctype == "" || strings.HasPrefix(ctype, "application/json")) && len(bytes.TrimSpace(res.Body)) > 0 {
		var data any
		if json.Unmarshal(res.Body, &data) == nil {
			if m, ok := data.(map[string]any); ok {
				if s, ok := m["summary"].(string); ok {
					c.Summary = s
				} else if s, ok := m["message"].(string); ok {
					c.Summary = s
				}
			}
			// The plugin's own bytes, keys in its order.
			return c.JSON(status, json.RawMessage(bytes.TrimSpace(res.Body)))
		}
	}
	for k, v := range res.Headers {
		if !strings.EqualFold(k, "Set-Cookie") {
			c.W.Header().Set(k, v)
		}
	}
	if ctype == "" {
		c.W.Header().Set("Content-Type", "application/json; charset=utf-8")
	}
	c.W.Header().Set("Content-Security-Policy", "sandbox")
	c.W.WriteHeader(status)
	_, err = c.W.Write(res.Body)
	return err
}

// pluginHeaders are the request headers a plugin's handler sees.
func pluginHeaders(r *http.Request) map[string]string {
	headers := map[string]string{}
	for _, k := range []string{"Content-Type", "Accept", "User-Agent", "Referer", "Origin", "X-Requested-With"} {
		if v := r.Header.Get(k); v != "" {
			headers[k] = v
		}
	}
	return headers
}

// pluginRouteCaller checks who may call rt: anyone (rate limited), a
// client's contact on a portal route, the plugin's own machine (which it
// is), or staff allowed the route.
func (a *App) pluginRouteCaller(c *Ctx, p *plugin.Plugin, rt *plugin.Route, path string) (*plugin.Machine, error) {
	if rt.Public && c.Actor == nil {
		if ok, wait := a.pluginCalls.Allow(p.Key + " " + c.ip()); !ok {
			c.W.Header().Set("Retry-After", strconv.Itoa(wait))
			return nil, &apierr.Error{Status: 429, Code: "rate_limited", Message: "Too many requests; try again in a minute."}
		}
	}
	if rt.Portal {
		// A client's contact signed in to their portal (or their portal
		// agent's token), while the plugin may serve the portal.
		if !p.Granted.Portal {
			return nil, apierr.NotFound("The plugin " + p.Key + " has no route " + c.R.Method + " " + path + ".")
		}
		actor, err := a.authenticate(c, true)
		if err != nil {
			return nil, err
		}
		if actor == nil || actor.Contact == nil {
			return nil, unauthorized()
		}
		c.Actor = actor
	}
	var machine *plugin.Machine
	if rt.Machine {
		// A machine holding one of this plugin's own tokens, and nobody
		// else: not a person's session or token.
		h := c.R.Header.Get("Authorization")
		var err error
		if after, ok := strings.CutPrefix(h, "Bearer "); ok {
			machine, err = a.plugins.MachineFor(c.R.Context(), p.Key, strings.TrimSpace(after))
		}
		if err != nil {
			return nil, err
		}
		if machine == nil {
			return nil, &apierr.Error{Status: 401, Code: "unauthorized", Message: "This route takes a machine token (Authorization: Bearer rwm_…) from this plugin; that one isn't valid or was revoked."}
		}
		c.Actor = nil
	}
	if !rt.Public && !rt.Portal && !rt.Machine {
		if c.Actor == nil {
			return nil, unauthorized()
		}
		if !a.mayCall(c, rt, c.R.Method) {
			return nil, forbidden()
		}
	}
	return machine, nil
}

// pluginCaller is the staff actor as a plugin sees its caller.
func (a *App) pluginCaller(c *Ctx) *plugin.Caller {
	u := c.Actor.User
	caps := []string{}
	for _, perm := range a.permissions() {
		if c.Can(perm.Key) {
			caps = append(caps, perm.Key)
		}
	}
	caller := &plugin.Caller{ID: u.Person().ID, Name: u.Person().DisplayName(), Role: u.Person().Role, Agent: u.IsAgent() || c.Actor.Token != nil, Capabilities: caps}
	if u.IsAgent() {
		caller.AgentID = u.ID
	}
	caller.Via = c.Actor.AppLabel()
	return caller
}

// pluginBadge is the number beside a plugin's page in the sidebar: what
// its Nav.Badge route answers for the person ({"count": n}), nil for none
// (or when it fails or takes too long: the sidebar never waits on it).
func (a *App) pluginBadge(ctx context.Context, c *Ctx, p *plugin.Plugin) any {
	path := p.Manifest.Nav.Badge
	if path == "" {
		return nil
	}
	rt := p.Manifest.Route("GET", path)
	if rt == nil || !a.mayCall(c, rt, http.MethodGet) {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req := &plugin.Request{Method: http.MethodGet, Path: path, Params: map[string]string{}, Query: url.Values{}, Headers: map[string]string{}, Actor: a.pluginCaller(c)}
	res, err := a.plugins.Serve(ctx, p.Key, rt.Export, req, c.Actor)
	if err != nil || res.Status >= 300 {
		return nil
	}
	var body struct {
		Count int `json:"count"`
	}
	if json.Unmarshal(res.Body, &body) != nil || body.Count <= 0 {
		return nil
	}
	return body.Count
}

// pluginTools are the enabled plugins' routes that are agent tools, for the
// catalogue (agent.go): a core tool's name always wins.
func (a *App) pluginTools() []*toolSpec {
	var out []*toolSpec
	for _, p := range a.enabledPlugins() {
		for _, r := range p.Manifest.Routes {
			t := r.Tool
			if t == nil || slices.ContainsFunc(a.tools, func(o *toolSpec) bool { return o.Name == t.Name }) {
				continue
			}
			spec := &toolSpec{Name: t.Name, Summary: t.Title, Notes: t.Description, Method: strings.ToUpper(r.Method), Path: "/api/x/" + p.Key + r.Path}
			// The route's {name} segments are the tool's path arguments.
			for _, m := range pathParam.FindAllStringSubmatch(r.Path, -1) {
				spec.Params = append(spec.Params, toolParam{Name: m[1], In: "path", Type: "string", Desc: "The " + strings.ReplaceAll(m[1], "_", " ") + ".", Required: true})
			}
			for _, q := range t.Query {
				spec.Params = append(spec.Params, toolParam{Name: q.Name, In: "query", Type: q.Type, Desc: q.Description, Required: q.Required})
			}
			for _, q := range t.Params {
				spec.Params = append(spec.Params, toolParam{Name: q.Name, In: "body", Type: q.Type, Desc: q.Description, Required: q.Required})
			}
			if t.Wrap != "" {
				spec.Wrap = t.Wrap
				spec.Params = append(spec.Params, toolParam{Name: t.Wrap, In: "body", Type: "object", Desc: t.Body, Required: true})
			}
			out = append(out, spec)
		}
	}
	return out
}

// navHref is where a plugin's sidebar page opens: its own screen's path, or
// the screen it describes.
func navHref(key string, n *plugin.NavPage) string {
	if n.Path != "" {
		return n.Path
	}
	return "/p/" + key + "/" + n.Page
}
