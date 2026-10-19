package plugin

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
	"runtime/debug"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// New is a host keeping plugins in db and reaching Runwell through env;
// modules are the plugins this binary has. A module whose manifest or
// handlers don't hold up panics: it's a bug in the build, not the install.
func New(env Env, db *store.DB, log *slog.Logger, spawn Spawn, modules ...*Module) *Host {
	h := &Host{env: env, db: db, log: log, spawn: spawn, modules: map[string]*Module{}}
	for _, m := range modules {
		if err := m.validate(); err != nil {
			panic(err)
		}
		if h.modules[m.Manifest.Key] != nil {
			panic("plugin " + m.Manifest.Key + " is registered twice")
		}
		h.modules[m.Manifest.Key] = m
	}
	return h
}

// Host installs, loads and calls plugins.
type Host struct {
	env     Env
	db      *store.DB
	log     *slog.Logger
	spawn   Spawn
	modules map[string]*Module // what this binary has, by key

	loadMu sync.Mutex // one LoadAll at a time
	mu     sync.RWMutex
	// plugins and caps are swapped whole by LoadAll, under mu.
	plugins  map[string]*Plugin
	caps     []CapGroup
	mail     mailLimiter
	mentions mentionCache
}

var keyFormat = regexp.MustCompile(`^[a-z][a-z0-9_]{1,40}$`)

const callTimeout = 10 * time.Second

// LoadAll reads the installed plugins. One the binary has whose manifest
// changed (a new build) is upgraded in place: it keeps the grants it still
// asks for, and if it asks for more it's off until someone grants the rest
// (a default plugin is granted what it asks for). One the binary doesn't
// have is left off and logged, not fatal: a plugin can't break Runwell.
func (h *Host) LoadAll(ctx context.Context) error {
	h.loadMu.Lock()
	defer h.loadMu.Unlock()
	rows, err := h.db.QueryContext(ctx, `SELECT id, key, name, version, COALESCE(description, ''), manifest, granted, enabled, created_at, updated_at FROM plugins ORDER BY key`)
	if err != nil {
		return err
	}
	type row struct {
		p        *Plugin
		manifest store.JSON
	}
	all, err := store.ScanAll(rows, func(r *sql.Rows) (row, error) {
		p := &Plugin{}
		var manifest, granted store.JSON
		if err := r.Scan(&p.ID, &p.Key, &p.Name, &p.Version, &p.Description, &manifest, &granted, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return row{}, err
		}
		// An unreadable manifest or grants isn't "asks for nothing, granted
		// nothing": the plugin is left off, and why is logged.
		if err := json.Unmarshal(manifest, &p.Manifest); err != nil {
			h.log.Warn("plugin manifest unreadable; left off", "plugin", p.Key, "err", err)
			p.Enabled = false
		}
		if len(granted) > 0 {
			if err := json.Unmarshal(granted, &p.Granted); err != nil {
				h.log.Warn("plugin grants unreadable; left off", "plugin", p.Key, "err", err)
				p.Enabled = false
			}
		}
		return row{p, manifest}, nil
	})
	if err != nil {
		return err
	}
	plugins := map[string]*Plugin{}
	var groups []CapGroup
	for _, r := range all {
		p := r.p
		p.module = h.modules[p.Key]
		switch {
		case p.module == nil:
			if p.Enabled {
				h.log.Warn("plugin isn't built into this binary; left off", "plugin", p.Key)
			}
			p.Enabled = false
		case !bytes.Equal(r.manifest, store.MustJSON(p.module.Manifest)):
			if err := h.upgrade(ctx, p); err != nil {
				h.log.Warn("plugin upgrade failed; left off", "plugin", p.Key, "err", err)
				p.Enabled = false
			}
		}
		// Its own tables, made (or brought up to date) before anything runs.
		if p.Enabled && p.Granted.Tables && p.Manifest.Schema != "" {
			if err := applySchema(ctx, h.db, p.Key, p.Manifest.Schema); err != nil {
				h.log.Warn("plugin tables couldn't be made; left off", "plugin", p.Key, "err", err)
				p.Enabled = false
			}
		}
		plugins[p.Key] = p
		if p.Enabled && len(p.Manifest.Capabilities) > 0 {
			g := CapGroup{Group: p.Name}
			for _, c := range p.Manifest.Capabilities {
				g.Caps = append(g.Caps, c.Key)
				g.Labels = append(g.Labels, c.Label)
				roles := c.Roles
				if len(roles) == 0 {
					roles = []string{"owner"}
				}
				g.Roles = append(g.Roles, roles)
			}
			groups = append(groups, g)
		}
	}
	h.mu.Lock()
	h.plugins, h.caps = plugins, groups
	h.mu.Unlock()
	h.forgetMentionables("")
	return nil
}

// upgrade brings an installed plugin's record up to the module this binary
// has.
func (h *Host) upgrade(ctx context.Context, p *Plugin) error {
	m := p.module.Manifest
	granted := intersectGrants(p.Granted, m.Permissions)
	// It stays on while it holds what it needs: reaching the server's own
	// network is only ever ticked by an owner (Configure), so a plugin that
	// asks for it was switched on without it, and keeps running without it.
	need := m.Permissions
	need.PrivateHTTP = false
	enabled := p.Enabled && grantsCover(granted, need)
	if p.module.Default {
		// First party: what it asks for is granted, and it stays as on or
		// off as it was.
		granted, enabled = m.Permissions, p.Enabled
	}
	now := store.Now()
	if _, err := h.db.ExecContext(ctx, `UPDATE plugins SET name=?, version=?, description=?, manifest=?, granted=?, enabled=?, updated_at=? WHERE id=?`,
		m.Name, m.Version, m.Description, string(store.MustJSON(m)), string(store.MustJSON(granted)), enabled, now.DB(), p.ID); err != nil {
		return err
	}
	if p.Version != m.Version {
		h.env.Audit(ctx, nil, "plugin.upgraded", p.ID, p.Key, map[string]any{"from_version": p.Version, "to_version": m.Version, "on": enabled})
	}
	p.Name, p.Version, p.Description, p.Manifest, p.Granted, p.Enabled, p.UpdatedAt = m.Name, m.Version, m.Description, m, granted, enabled, now
	return nil
}

// CapGroups is the enabled plugins' capabilities.
func (h *Host) CapGroups() []CapGroup {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.caps
}

// CapRoles is the roles that hold an enabled plugin's capability; none
// when no enabled plugin declares it.
func (h *Host) CapRoles(capability string) []string {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, g := range h.caps {
		for i, c := range g.Caps {
			if c == capability {
				return g.Roles[i]
			}
		}
	}
	return nil
}

// List is every installed plugin, by key.
func (h *Host) List() []*Plugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]*Plugin, 0, len(h.plugins))
	for _, p := range h.plugins {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b *Plugin) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// Available is the manifests of the plugins this binary has that aren't
// installed, by key.
func (h *Host) Available() []Manifest {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := []Manifest{}
	for key, m := range h.modules {
		if h.plugins[key] == nil {
			out = append(out, m.Manifest)
		}
	}
	slices.SortFunc(out, func(a, b Manifest) int { return strings.Compare(a.Key, b.Key) })
	return out
}

// Get is the plugin installed as key, nil when there's none.
func (h *Host) Get(key string) *Plugin {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.plugins[key]
}

// enabled is the plugin installed as key when it's on, and the API its
// grants allow, acting for actor; nil when it's off.
func (h *Host) enabled(key string, actor Actor) (*Plugin, *API) {
	p := h.Get(key)
	if p == nil || !p.Enabled || p.module == nil {
		return nil, nil
	}
	return p, &API{h: h, key: p.Key, grants: p.Granted, actor: actor}
}

// guard runs fn within the time limit, a panic in it answering an error
// rather than taking the server down.
func guard(ctx context.Context, what string, fn func(context.Context) error) (err error) {
	ctx, cancel := context.WithTimeout(ctx, callTimeout)
	defer cancel()
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("%s panicked: %v\n%s", what, r, debug.Stack())
		}
	}()
	return fn(ctx)
}

// Serve hands req to the handler an enabled plugin's route names, its
// writes made as actor (the caller, as the core knows them).
func (h *Host) Serve(ctx context.Context, key, export string, req *Request, actor Actor) (*Response, error) {
	p, api := h.enabled(key, actor)
	if p == nil {
		return nil, fmt.Errorf("plugin %s isn't enabled", key)
	}
	handler := p.module.Handlers[export]
	if handler == nil {
		return nil, fmt.Errorf("plugin %s has no handler %s", key, export)
	}
	// The API knows the request it serves: who called (so tools run only as
	// them) and, while the call lasts, that it's that person's own request.
	api.caller, api.contact, api.method = req.Actor, req.Contact, req.Method
	live := &atomic.Bool{}
	live.Store(true)
	api.live = live
	defer live.Store(false)
	var res *Response
	err := guard(ctx, "plugin "+key+"/"+export, func(ctx context.Context) error {
		var err error
		res, err = handler(ctx, api, req)
		return err
	})
	if err == nil && res == nil {
		err = fmt.Errorf("plugin %s/%s answered nothing", key, export)
	}
	return res, err
}

// Emit hands an event to every enabled plugin that hooks it, in the
// background: a slow or failing plugin never holds up a save.
func (h *Host) Emit(ctx context.Context, event string, data any) {
	body, err := json.Marshal(data)
	if err != nil {
		h.log.Warn("plugin hook", "event", event, "err", err)
		return
	}
	for _, p := range h.List() {
		if !p.Enabled || !hooks(p.Manifest.Hooks, event) {
			continue
		}
		h.spawn(ctx, "plugin hook", func(ctx context.Context) {
			p, api := h.enabled(p.Key, nil)
			if p == nil {
				return
			}
			err := guard(ctx, "plugin "+p.Key+" on "+event, func(ctx context.Context) error {
				return p.module.OnEvent(ctx, api, event, body)
			})
			if err != nil {
				h.log.Warn("plugin hook", "plugin", p.Key, "event", event, "err", err)
			}
		})
	}
}

// hooks is whether a plugin's hooks hear event: by name, or "*.<what>"
// for any plugin's <key>.<what> (never the core's, which have no plugin
// key in front).
func hooks(list []string, event string) bool {
	if slices.Contains(list, event) {
		return true
	}
	key, rest, ok := strings.Cut(event, ".")
	if !ok || !keyFormat.MatchString(key) {
		return false
	}
	for _, h := range list {
		if strings.HasPrefix(h, "*.") && h[2:] == rest && strings.Count(rest, ".") >= 1 {
			return true
		}
	}
	return false
}

// Install installs a plugin this binary has, off until a person grants what
// it asks for and enables it.
func (h *Host) Install(ctx context.Context, actor Actor, key string) (*Plugin, error) {
	m := h.modules[key]
	if m == nil {
		return nil, apierr.NotFound("This build has no plugin " + key + ".")
	}
	if p := h.Get(key); p != nil {
		return p, nil
	}
	now := store.Now().DB()
	_, err := h.db.ExecContext(ctx, `INSERT INTO plugins (key, name, version, description, manifest, granted, enabled, created_at, updated_at) VALUES (?,?,?,?,?,?,0,?,?)
		ON CONFLICT(key) DO NOTHING`,
		m.Manifest.Key, m.Manifest.Name, m.Manifest.Version, m.Manifest.Description, string(store.MustJSON(m.Manifest)), string(store.MustJSON(Grants{HTTP: []string{}})), now, now)
	if err != nil {
		return nil, err
	}
	h.env.Audit(ctx, actor, "plugin.installed", 0, key, map[string]any{"to_version": m.Manifest.Version})
	if err := h.LoadAll(ctx); err != nil {
		return nil, err
	}
	return h.Get(key), nil
}

// intersectGrants is what's both granted and asked for.
func intersectGrants(granted, asked Grants) Grants {
	out := Grants{RecordsRead: granted.RecordsRead && asked.RecordsRead, RecordsWrite: granted.RecordsWrite && asked.RecordsWrite,
		Storage: granted.Storage && asked.Storage, Tables: granted.Tables && asked.Tables, Files: granted.Files && asked.Files,
		Mail: granted.Mail && asked.Mail, PublicHTTP: granted.PublicHTTP && asked.PublicHTTP, PrivateHTTP: granted.PrivateHTTP && asked.PrivateHTTP,
		Secrets: granted.Secrets && asked.Secrets, Tools: granted.Tools && asked.Tools, Portal: granted.Portal && asked.Portal,
		Live: granted.Live && asked.Live, Notify: granted.Notify && asked.Notify,
		Metrics:      granted.Metrics && asked.Metrics,
		Mentionables: granted.Mentionables && asked.Mentionables, MentionsRelay: granted.MentionsRelay && asked.MentionsRelay, Machines: granted.Machines && asked.Machines,
		ContentExternal: granted.ContentExternal && asked.ContentExternal, HTTP: []string{}}
	for _, host := range granted.HTTP {
		if slices.Contains(asked.HTTP, host) {
			out.HTTP = append(out.HTTP, host)
		}
	}
	return out
}

// grantsEqual says whether two grants are the same.
func grantsEqual(a, b Grants) bool { return grantsCover(a, b) && grantsCover(b, a) }

// grantsCover says whether granted holds all that's asked for.
func grantsCover(granted, asked Grants) bool { //nolint:gocyclo // one condition per grant, as Grants lists them
	if asked.RecordsRead && !granted.RecordsRead || asked.RecordsWrite && !granted.RecordsWrite || asked.Storage && !granted.Storage ||
		asked.Tables && !granted.Tables || asked.Files && !granted.Files || asked.Mail && !granted.Mail || asked.PublicHTTP && !granted.PublicHTTP ||
		asked.PrivateHTTP && !granted.PrivateHTTP || asked.Secrets && !granted.Secrets || asked.Tools && !granted.Tools || asked.Portal && !granted.Portal ||
		asked.Live && !granted.Live || asked.Notify && !granted.Notify || asked.Metrics && !granted.Metrics ||
		asked.Mentionables && !granted.Mentionables || asked.MentionsRelay && !granted.MentionsRelay ||
		asked.Machines && !granted.Machines || asked.ContentExternal && !granted.ContentExternal {
		return false
	}
	for _, host := range asked.HTTP {
		if !slices.Contains(granted.HTTP, host) {
			return false
		}
	}
	return true
}

// Configure sets what's granted (never more than was asked) and whether
// it's on. Switching on with no grant given grants what it asks for: the
// owner switching it on has been shown what that is.
func (h *Host) Configure(ctx context.Context, actor Actor, key string, grant *Grants, enabled *bool) (*Plugin, error) {
	p := h.Get(key)
	if p == nil {
		return nil, apierr.NotFound("No plugin " + key + ".")
	}
	on := p.Enabled
	if enabled != nil {
		on = *enabled
	}
	granted := p.Granted
	if grant != nil {
		granted = intersectGrants(*grant, p.Manifest.Permissions)
	} else if on && !p.Enabled {
		// Reaching the server's own network is never granted in passing:
		// an owner ticks it.
		granted = p.Manifest.Permissions
		granted.PrivateHTTP = false
	}
	if on && p.module == nil {
		return p, apierr.Conflict("This build doesn't have the plugin " + key + ", so it can't be switched on.")
	}
	if _, err := h.db.ExecContext(ctx, `UPDATE plugins SET granted = ?, enabled = ?, updated_at = ? WHERE key = ?`, string(store.MustJSON(granted)), on, store.Now().DB(), key); err != nil {
		return nil, err
	}
	if on != p.Enabled {
		// What the Rails core recorded for the switch.
		h.env.Audit(ctx, actor, "settings.plugin", p.ID, key, map[string]any{"on": on})
	}
	if !grantsEqual(granted, p.Granted) {
		h.env.Audit(ctx, actor, "plugin.granted", p.ID, key, map[string]any{"granted": granted})
	}
	if err := h.LoadAll(ctx); err != nil {
		return nil, err
	}
	return h.Get(key), nil
}

// Remove uninstalls a plugin, and its storage too when purgeData.
func (h *Host) Remove(ctx context.Context, actor Actor, key string, purgeData bool) error {
	if _, err := h.db.ExecContext(ctx, `DELETE FROM plugins WHERE key = ?`, key); err != nil {
		return err
	}
	if purgeData {
		if _, err := h.db.ExecContext(ctx, `DELETE FROM plugin_kv WHERE plugin_key = ?`, key); err != nil {
			return err
		}
	}
	// A removed plugin's machines can't call in again, purged or not.
	if err := h.revokeMachines(ctx, key); err != nil {
		return err
	}
	if err := h.forgetDefault(ctx, key); err != nil {
		return err
	}
	h.env.Audit(ctx, actor, "plugin.removed", 0, key, map[string]any{"purge_data": purgeData})
	return h.LoadAll(ctx)
}
