package ai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Agents: named assistants an owner defines in Settings › AI › Agents
// (ai_agents), which anyone allowed can @mention in a note on a record or
// in chat (the core's mentionables). A mention starts a run (runs.go) that
// works as the person who mentioned it, through the same tools as Ask,
// and replies where it was mentioned. Each has its identity (name, @handle,
// picture or initials, description), its behaviour (instructions, model),
// what it may use (read only, or propose changes the person approves; an
// optional list of actions; all records or some clients' only), who may
// mention it, and how much one run may spend and how many steps it takes.
// A paused one answers mentions with a polite no.

type agent struct {
	ID           int64
	Name         string
	Handle       string
	Description  string
	Instructions string
	// Model is "main", "fast" or one of the provider's models.
	Model string
	// Tools is "read" (only looks) or "propose" (changes, each approved).
	Tools string
	// Allowed are the actions it may use (all the person's when empty).
	Allowed []string
	// ClientIDs are the clients whose records it may read (all when empty).
	ClientIDs []int64
	// Roles may mention it (everyone when empty).
	Roles          []string
	RunBudgetCents *int64
	MaxSteps       int
	Paused         bool
	AvatarFileID   int64
	CreatedAt      string
	UpdatedAt      string
}

const agentColumns = `id, name, handle, COALESCE(description, ''), COALESCE(instructions, ''), COALESCE(model, 'main'), COALESCE(tools, 'read'),
	COALESCE(allowed_tools, '[]'), COALESCE(client_ids, '[]'), COALESCE(roles, '[]'), run_budget_cents, COALESCE(max_steps, 8), paused, COALESCE(avatar_file_id, 0), created_at, updated_at`

func scanAgent(row interface{ Scan(...any) error }) (*agent, error) {
	g := &agent{}
	var allowed, clients, roles string
	var budget sql.NullInt64
	var created, updated store.Time
	if err := row.Scan(&g.ID, &g.Name, &g.Handle, &g.Description, &g.Instructions, &g.Model, &g.Tools, &allowed, &clients, &roles, &budget, &g.MaxSteps, &g.Paused, &g.AvatarFileID, &created, &updated); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(allowed), &g.Allowed)
	_ = json.Unmarshal([]byte(clients), &g.ClientIDs)
	_ = json.Unmarshal([]byte(roles), &g.Roles)
	if budget.Valid {
		g.RunBudgetCents = &budget.Int64
	}
	g.CreatedAt, g.UpdatedAt = stamp(created), stamp(updated)
	return g, nil
}

func stamp(t store.Time) string {
	if !t.Valid {
		return ""
	}
	return t.Time.UTC().Format("2006-01-02T15:04:05Z")
}

func loadAgents(ctx context.Context, q plugin.Querier) ([]*agent, error) {
	rows, err := q.Query(ctx, `SELECT `+agentColumns+` FROM ai_agents ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []*agent{}
	for rows.Next() {
		g, err := scanAgent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func loadAgent(ctx context.Context, q plugin.Querier, id int64) (*agent, error) {
	g, err := scanAgent(q.QueryRow(ctx, `SELECT `+agentColumns+` FROM ai_agents WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return g, err
}

func (g *agent) avatarPath() string {
	if g.AvatarFileID == 0 {
		return ""
	}
	return fmt.Sprintf("/api/x/ai/agents/%d/avatar?v=%d", g.ID, g.AvatarFileID)
}

// mayMention is whether someone with role may mention it.
func (g *agent) mayMention(role string) bool {
	return len(g.Roles) == 0 || slices.Contains(g.Roles, role)
}

// modelName is the model a run uses, as the provider takes it.
func (g *agent) modelName(s *setting) string {
	switch g.Model {
	case "", "main":
		return s.modelFor(false)
	case "fast":
		return s.modelFor(true)
	}
	return strings.TrimPrefix(g.Model, "opencode/")
}

// mentionables are the agents, for the core's @ pickers: none while AI
// isn't set up.
func mentionables(ctx context.Context, api *plugin.API) ([]plugin.Mentionable, error) {
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	if !s.ready() {
		return nil, nil
	}
	list, err := loadAgents(ctx, api)
	if err != nil {
		return nil, err
	}
	out := []plugin.Mentionable{}
	for _, g := range list {
		desc := g.Description
		if g.Paused {
			desc = "Paused"
		}
		out = append(out, plugin.Mentionable{ID: strconv.FormatInt(g.ID, 10), Handle: g.Handle, Name: g.Name, Avatar: g.avatarPath(),
			Description: truncate(desc, 120), Kind: "agent", Roles: g.Roles, Paused: g.Paused})
	}
	return out, nil
}

// agentJSON is an agent as the settings page and agents see it; the whole
// of it (behaviour, tools, limits) only for those who manage settings.
func agentJSON(g *agent, full bool) map[string]any {
	out := map[string]any{"id": g.ID, "name": g.Name, "handle": g.Handle, "description": g.Description, "avatar_url": nullStr(g.avatarPath()),
		"paused": g.Paused, "mention": "@" + g.Handle}
	if full {
		roles := g.Roles
		if roles == nil {
			roles = []string{}
		}
		allowed := g.Allowed
		if allowed == nil {
			allowed = []string{}
		}
		clients := g.ClientIDs
		if clients == nil {
			clients = []int64{}
		}
		out["instructions"] = g.Instructions
		out["model"] = g.Model
		out["tools"] = g.Tools
		out["allowed_tools"] = allowed
		out["client_ids"] = clients
		out["roles"] = roles
		out["run_budget_cents"] = g.RunBudgetCents
		out["max_steps"] = g.MaxSteps
		out["created_at"] = g.CreatedAt
		out["updated_at"] = g.UpdatedAt
	}
	return out
}

// ---- routes ----------------------------------------------------------------------------

func listAgents(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if req.Actor == nil {
		return refuse(http.StatusUnauthorized, "Sign in first."), nil
	}
	list, err := loadAgents(ctx, api)
	if err != nil {
		return nil, err
	}
	full := req.Actor.Can("manage_settings")
	out := []map[string]any{}
	for _, g := range list {
		if !full && !g.mayMention(req.Actor.Role) {
			continue
		}
		out = append(out, agentJSON(g, full))
	}
	answer := map[string]any{"summary": fmt.Sprintf("%d %s.", len(out), map[bool]string{true: "agent", false: "agents"}[len(out) == 1]), "agents": out}
	if full && req.Query.Get("form") != "" {
		s, err := loadSetting(ctx, api)
		if err != nil {
			return nil, err
		}
		tools := []map[string]any{}
		if list, err := api.Tools(ctx, req.Actor); err == nil {
			for _, t := range list {
				tools = append(tools, map[string]any{"name": t.Name, "title": t.Title, "read_only": t.ReadOnly})
			}
		}
		clients := []map[string]any{}
		if list, err := api.Records(ctx, plugin.Query{Type: "Client", Limit: 500}); err == nil {
			for _, c := range list {
				clients = append(clients, map[string]any{"id": c["id"], "name": c["display_name"]})
			}
		}
		answer["form"] = map[string]any{"ready": s.ready(), "models": modelOptions(s), "main": s.modelFor(false), "fast": s.modelFor(true), "tools": tools, "clients": clients}
	}
	return plugin.JSON(200, answer), nil
}

func showAgent(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	g, err := loadAgent(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if g == nil || req.Actor == nil || (!req.Actor.Can("manage_settings") && !g.mayMention(req.Actor.Role)) {
		return missing("agent"), nil
	}
	return plugin.JSON(200, map[string]any{"agent": agentJSON(g, req.Actor.Can("manage_settings"))}), nil
}

var handleOK = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{1,29}$`)

// saveAgent creates an agent (id 0) or changes one from the form's fields.
func saveAgent(ctx context.Context, api *plugin.API, req *plugin.Request, g *agent) (*plugin.Response, error) {
	if req.Actor.Agent {
		return refuse(http.StatusForbidden, "Agents are set up by a person in the browser."), nil
	}
	in := wrapped(params(req), "agent")
	set := func(key string, fn func(v any)) {
		if v, ok := in[key]; ok {
			fn(v)
		}
	}
	errs := map[string][]string{}
	set("name", func(v any) { g.Name = strings.TrimSpace(str(v)) })
	set("handle", func(v any) { g.Handle = strings.ToLower(strings.TrimPrefix(strings.TrimSpace(str(v)), "@")) })
	set("description", func(v any) { g.Description = strings.TrimSpace(str(v)) })
	set("instructions", func(v any) { g.Instructions = strings.TrimSpace(str(v)) })
	set("model", func(v any) { g.Model = strings.TrimSpace(str(v)) })
	set("tools", func(v any) { g.Tools = str(v) })
	set("allowed_tools", func(v any) { g.Allowed = strs(v) })
	set("client_ids", func(v any) { g.ClientIDs = idList(v) })
	set("roles", func(v any) { g.Roles = strs(v) })
	set("paused", func(v any) { g.Paused = truthy(v) })
	set("max_steps", func(v any) {
		n := intOf(v)
		if n < 1 || n > 30 {
			errs["max_steps"] = append(errs["max_steps"], "must be between 1 and 30")
			return
		}
		g.MaxSteps = int(n)
	})
	set("run_budget_cents", func(v any) {
		text := strings.TrimSpace(str(v))
		if text == "" {
			g.RunBudgetCents = nil
			return
		}
		n := intOf(v)
		if strconv.FormatInt(n, 10) != text || n < 1 {
			errs["run_budget_cents"] = append(errs["run_budget_cents"], "must be a whole number of cents, 1 or more")
			return
		}
		g.RunBudgetCents = &n
	})
	if g.Name == "" {
		errs["name"] = append(errs["name"], "can't be blank")
	} else if len([]rune(g.Name)) > 40 {
		errs["name"] = append(errs["name"], "is too long (40 characters at most)")
	}
	if g.Model == "" {
		g.Model = "main"
	}
	if g.Tools != "read" && g.Tools != "propose" {
		errs["tools"] = append(errs["tools"], "is read or propose")
	}
	for _, r := range g.Roles {
		if !slices.Contains(plugin.Roles, r) {
			errs["roles"] = append(errs["roles"], "are owner, manager or member")
			break
		}
	}
	if len(g.Instructions) > 20000 {
		errs["instructions"] = append(errs["instructions"], "are too long (20,000 characters at most)")
	}
	if msg := handleProblem(ctx, api, g); msg != "" {
		errs["handle"] = append(errs["handle"], msg)
	}
	if len(errs) > 0 {
		return plugin.Invalid(errs), nil
	}
	allowed, _ := json.Marshal(nonNil(g.Allowed))
	clients, _ := json.Marshal(g.ClientIDs)
	if g.ClientIDs == nil {
		clients = []byte("[]")
	}
	roles, _ := json.Marshal(nonNil(g.Roles))
	now := store.Now().DB()
	message := g.Name + " saved."
	if g.ID == 0 {
		res, err := api.Exec(ctx, `INSERT INTO ai_agents (name, handle, description, instructions, model, tools, allowed_tools, client_ids, roles, run_budget_cents, max_steps, paused, created_by_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, g.Name, g.Handle, nullStr(g.Description), nullStr(g.Instructions), g.Model, g.Tools,
			string(allowed), string(clients), string(roles), g.RunBudgetCents, g.MaxSteps, g.Paused, req.Actor.ID, now, now)
		if err != nil {
			return nil, err
		}
		g.ID, _ = res.LastInsertId()
		message = fmt.Sprintf("%s is ready: mention @%s in a note or in chat.", g.Name, g.Handle)
	} else if _, err := api.Exec(ctx, `UPDATE ai_agents SET name = ?, handle = ?, description = ?, instructions = ?, model = ?, tools = ?, allowed_tools = ?, client_ids = ?, roles = ?,
		run_budget_cents = ?, max_steps = ?, paused = ?, updated_at = ? WHERE id = ?`, g.Name, g.Handle, nullStr(g.Description), nullStr(g.Instructions), g.Model, g.Tools,
		string(allowed), string(clients), string(roles), g.RunBudgetCents, g.MaxSteps, g.Paused, now, g.ID); err != nil {
		return nil, err
	}
	api.MentionablesChanged()
	api.Log(fmt.Sprintf("agent @%s saved by %s (paused %v)", g.Handle, req.Actor.Name, g.Paused))
	g, err := loadAgent(ctx, api, g.ID)
	if err != nil {
		return nil, err
	}
	return plugin.JSON(200, map[string]any{"message": message, "summary": message, "agent": agentJSON(g, true)}), nil
}

// handleProblem is what's wrong with an agent's handle: the format, or a
// clash with another agent or a person (their first name, their name run
// together, or their email's name).
func handleProblem(ctx context.Context, api *plugin.API, g *agent) string {
	if !handleOK.MatchString(g.Handle) {
		return "must be 2 to 30 lowercase letters, digits, _ . or -, like researcher"
	}
	var n int
	if err := api.QueryRow(ctx, `SELECT count(*) FROM ai_agents WHERE handle = ? AND id <> ?`, g.Handle, g.ID).Scan(&n); err == nil && n > 0 {
		return "is another agent's"
	}
	if people, err := api.Records(ctx, plugin.Query{Type: "User", Limit: 500}); err == nil {
		for _, p := range people {
			name := strings.ToLower(str(p["name"]))
			first, _, _ := strings.Cut(name, " ")
			local, _, _ := strings.Cut(strings.ToLower(str(p["email"])), "@")
			if g.Handle == first || g.Handle == strings.ReplaceAll(name, " ", "") || g.Handle == local {
				return "is too like " + str(p["name"]) + "'s name: pick one nobody would mistake for a person"
			}
		}
	}
	return ""
}

func strs(v any) []string {
	out := []string{}
	switch l := v.(type) {
	case []any:
		for _, x := range l {
			if s := strings.TrimSpace(str(x)); s != "" && !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	case string:
		for s := range strings.SplitSeq(l, ",") {
			if s = strings.TrimSpace(s); s != "" && !slices.Contains(out, s) {
				out = append(out, s)
			}
		}
	}
	return out
}

func idList(v any) []int64 {
	out := []int64{}
	for _, s := range strs(v) {
		if id := intOf(s); id > 0 && !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func nonNil(l []string) []string {
	if l == nil {
		return []string{}
	}
	return l
}

func createAgent(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return saveAgent(ctx, api, req, &agent{Model: "main", Tools: "read", MaxSteps: 8})
}

func updateAgent(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	g, err := loadAgent(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if g == nil {
		return missing("agent"), nil
	}
	return saveAgent(ctx, api, req, g)
}

func deleteAgent(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	g, err := loadAgent(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if g == nil {
		return missing("agent"), nil
	}
	if g.AvatarFileID != 0 {
		_ = api.Detach(ctx, "Ai::Agent", g.ID, g.AvatarFileID)
	}
	// Its runs stay, for what they cost and did; it's gone from the pickers.
	if _, err := api.Exec(ctx, `DELETE FROM ai_agents WHERE id = ?`, g.ID); err != nil {
		return nil, err
	}
	api.MentionablesChanged()
	api.Log(fmt.Sprintf("agent @%s deleted by %s", g.Handle, req.Actor.Name))
	return plugin.JSON(200, map[string]any{"message": g.Name + " deleted.", "summary": g.Name + " deleted.", "deleted": true}), nil
}

func isImage(t string) bool {
	switch t {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

func uploadAgentAvatar(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	g, err := loadAgent(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if g == nil {
		return missing("agent"), nil
	}
	typ := strings.TrimSpace(strings.Split(req.Header("Content-Type"), ";")[0])
	if !isImage(typ) {
		typ = http.DetectContentType(req.Body)
	}
	switch {
	case len(req.Body) == 0:
		return plugin.Invalid(map[string][]string{"avatar": {"is empty: pick an image"}}), nil
	case !isImage(typ):
		return plugin.Invalid(map[string][]string{"avatar": {"must be a PNG, JPEG, GIF or WebP image"}}), nil
	case len(req.Body) > 4<<20:
		return plugin.Invalid(map[string][]string{"avatar": {"is larger than 4 MB: pick a smaller image"}}), nil
	}
	name := path.Base(req.Query.Get("filename"))
	if name == "" || name == "." || name == "/" {
		name = "avatar"
	}
	f, err := api.Attach(ctx, "Ai::Agent", g.ID, "avatar", name, typ, bytes.NewReader(req.Body))
	if err != nil {
		return failed(err)
	}
	if _, err := api.Exec(ctx, `UPDATE ai_agents SET avatar_file_id = ?, updated_at = ? WHERE id = ?`, f.ID, store.Now().DB(), g.ID); err != nil {
		return nil, err
	}
	if g.AvatarFileID != 0 {
		_ = api.Detach(ctx, "Ai::Agent", g.ID, g.AvatarFileID)
	}
	api.MentionablesChanged()
	g.AvatarFileID = f.ID
	return plugin.JSON(200, map[string]any{"message": "Picture saved.", "agent": agentJSON(g, true)}), nil
}

func removeAgentAvatar(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	g, err := loadAgent(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if g == nil {
		return missing("agent"), nil
	}
	if g.AvatarFileID != 0 {
		_ = api.Detach(ctx, "Ai::Agent", g.ID, g.AvatarFileID)
		if _, err := api.Exec(ctx, `UPDATE ai_agents SET avatar_file_id = NULL, updated_at = ? WHERE id = ?`, store.Now().DB(), g.ID); err != nil {
			return nil, err
		}
		api.MentionablesChanged()
	}
	g.AvatarFileID = 0
	return plugin.JSON(200, map[string]any{"message": "Picture removed: it shows its initials.", "agent": agentJSON(g, true)}), nil
}

func serveAgentAvatar(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	g, err := loadAgent(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if g == nil || g.AvatarFileID == 0 {
		return missing("picture"), nil
	}
	rc, f, err := api.OpenFile(ctx, "Ai::Agent", g.ID, g.AvatarFileID)
	if err != nil {
		if res := plugin.Answer(err); res != nil {
			return res, nil
		}
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return &plugin.Response{Status: http.StatusOK, Body: data, Headers: map[string]string{
		"Content-Type": f.ContentType, "Content-Disposition": mime.FormatMediaType("inline", map[string]string{"filename": f.Filename}),
		"Cache-Control": "private, max-age=86400",
	}}, nil
}
