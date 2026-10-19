package core

import (
	"context"
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Connected apps, for everyone (Settings › Connected apps): the AI apps and
// tokens acting as you. Make a personal token for the CLI or a script
// (shown once), see what each app did (agent_calls), pause one, and
// disconnect any. An owner also sees every connection in the install,
// clients' included, and can pause or disconnect them. A client's contact
// has the same for themselves in the portal. Managing access stays with a
// person in the browser: agents can't mint, pause or revoke tokens.

func init() { routeSets = append(routeSets, (*App).connectedAppRoutes) }

func (a *App) connectedAppRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/settings/connected_apps", a.listConnectedApps)
	a.handle(mux, "POST /api/settings/connected_apps", a.createConnectedApp)
	a.handle(mux, "DELETE /api/settings/connected_apps/{id}", a.deleteConnectedApp)
	a.handle(mux, "POST /api/settings/connected_apps/{id}/pause", a.pauseConnectedApp)
	a.handle(mux, "DELETE /api/settings/connected_apps/{id}/pause", a.resumeConnectedApp)
	a.handle(mux, "PATCH /api/settings/client_agent_approvals", a.updateClientAgentApprovals, need("manage_settings"))

	a.handle(mux, "GET /api/portal/connected_apps", a.listPortalConnectedApps, portal)
	a.handle(mux, "POST /api/portal/connected_apps", a.createPortalConnectedApp, portal)
	a.handle(mux, "DELETE /api/portal/connected_apps/{id}", a.deletePortalConnectedApp, portal)
}

// inBrowser refuses a request made with a token: tokens are managed by a
// person in the browser.
func inBrowser(c *Ctx) error {
	if c.Actor != nil && c.Actor.Token != nil {
		return &apierr.Error{Status: 403, Code: "forbidden", Message: "Access tokens are managed by a person in the browser."}
	}
	return nil
}

// tokensJSON are tokens as Connected apps lists them, with their holders
// (holder: for an owner's list of everyone's) and their last 20 calls.
func (a *App) tokensJSON(ctx context.Context, where string, holder bool, args ...any) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT `+tokenColumns+` FROM access_tokens WHERE `+liveTokens+` AND `+where+` ORDER BY created_at DESC, id DESC`,
		append([]any{store.Now().DB()}, args...)...)
	if err != nil {
		return nil, err
	}
	tokens, err := store.ScanAll(rows, func(r *sqlRows) (*AccessToken, error) { return scanToken(r) })
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(tokens))
	for i, t := range tokens {
		ids[i] = t.ID
	}
	calls, err := a.agentCallsJSON(ctx, ids, 20)
	if err != nil {
		return nil, err
	}
	out := make([]Obj, 0, len(tokens))
	for _, t := range tokens {
		kind := "Personal token"
		if t.Kind == "oauth" {
			kind = "Connected app"
		}
		o := O("id", t.ID, "name", t.Name, "kind", t.Kind, "kind_label", kind, "read_only", t.ReadOnly, "paused", t.PausedAt.Valid,
			"created_at", t.CreatedAt, "last_used_at", t.LastUsedAt)
		if holder {
			name := ""
			if t.ContactID != 0 {
				var contact, client string
				if a.store.QueryRowContext(ctx, `SELECT c.name, cl.name FROM contacts c JOIN clients cl ON cl.id = c.client_id WHERE c.id = ?`, t.ContactID).Scan(&contact, &client) == nil {
					name = contact + " (" + client + ")"
				}
			} else if u, err := a.loadUser(ctx, t.UserID); err == nil && u != nil {
				name = u.DisplayName()
			}
			o = o.Set("holder", name).Set("client", t.ContactID != 0)
		}
		list := calls[t.ID]
		if list == nil {
			list = []Obj{}
		}
		out = append(out, o.Set("calls", list))
	}
	return out, nil
}

func (a *App) connectedAppsJSON(c *Ctx) (Obj, error) {
	ctx := c.R.Context()
	person := c.Actor.User.Person()
	mine, err := a.tokensJSON(ctx, `kind != 'assistant' AND user_id = ?`, false, person.ID)
	if err != nil {
		return nil, err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	agent := person.DisplayName() + "'s agent"
	var name string
	if a.store.QueryRowContext(ctx, `SELECT name FROM users WHERE agent_owner_id = ?`, person.ID).Scan(&name) == nil && name != "" {
		agent = name
	}
	base := a.BaseURL()
	out := O("tokens", mine, "agent_name", agent, "mcp_url", base+"/mcp", "portal_mcp_url", base+"/portal/mcp",
		"cli_install_url", base+"/install/cli", "rate_limit", agentRateLimit(), "client_agent_approvals", s.ClientAgentApprovals)
	if c.Can("manage_people") {
		everyone, err := a.tokensJSON(ctx, `kind != 'assistant' AND (user_id IS NULL OR user_id != ?)`, true, person.ID)
		if err != nil {
			return nil, err
		}
		out = out.Set("everyone", everyone)
	}
	return out, nil
}

func (a *App) listConnectedApps(c *Ctx) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	out, err := a.connectedAppsJSON(c)
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

// tokenParams reads {"access_token": {"name", "read_only"}}.
func tokenParams(c *Ctx) (string, bool, error) {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return "", false, err
	}
	attrs, _ := body["access_token"].(map[string]any)
	name := strings.TrimSpace(str(attrs["name"]))
	if name == "" {
		name = "Personal token"
	}
	return truncate(name, 100), boolOf(attrs["read_only"]), nil
}

// createConnectedApp makes a personal token, shown once in the answer.
func (a *App) createConnectedApp(c *Ctx) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	name, readOnly, err := tokenParams(c)
	if err != nil {
		return err
	}
	var tok *AccessToken
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		tok, err = t.issueToken(c.Actor.User.Person().ID, 0, name, "personal", 0, readOnly)
		return err
	})
	if err != nil {
		return err
	}
	out, err := a.connectedAppsJSON(c)
	if err != nil {
		return err
	}
	return c.JSON(201, out.Set("new_token", O("id", tok.ID, "name", tok.Name, "token", tok.Plaintext, "read_only", tok.ReadOnly)))
}

// manageableToken is a live token the person may pause or disconnect: their
// own, or, for an owner, anyone's.
func (a *App) manageableToken(c *Ctx) (*AccessToken, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, err
	}
	where, args := `id = ? AND `+liveTokens, []any{id, store.Now().DB()}
	if !c.Can("manage_people") {
		where += ` AND user_id = ?`
		args = append(args, c.Actor.User.Person().ID)
	}
	tok, err := scanToken(a.store.QueryRowContext(c.R.Context(), `SELECT `+tokenColumns+` FROM access_tokens WHERE `+where, args...))
	if isNoRows(err) {
		return nil, notFound("connected app")
	}
	return tok, err
}

func (a *App) deleteConnectedApp(c *Ctx) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	tok, err := a.manageableToken(c)
	if err != nil {
		return err
	}
	if err := a.write(c.R.Context(), c.Actor, func(t *Tx) error { return t.revokeToken(tok.ID) }); err != nil {
		return err
	}
	return c.JSON(200, O("notice", tok.Name+" disconnected."))
}

func (a *App) pauseConnectedApp(c *Ctx) error { return a.setConnectedAppPaused(c, true) }

func (a *App) resumeConnectedApp(c *Ctx) error { return a.setConnectedAppPaused(c, false) }

// setConnectedAppPaused stops a connection at once without disconnecting
// it (every call answers "paused" until it's resumed), or resumes it.
func (a *App) setConnectedAppPaused(c *Ctx, paused bool) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	tok, err := a.manageableToken(c)
	if err != nil {
		return err
	}
	if err := a.write(c.R.Context(), c.Actor, func(t *Tx) error { return t.setTokenPaused(tok.ID, paused) }); err != nil {
		return err
	}
	notice := tok.Name + " is working again."
	if paused {
		notice = tok.Name + " is paused: it can’t do anything until you resume it."
	}
	return c.JSON(200, O("notice", notice))
}

// updateClientAgentApprovals is whether a client's own agent may decide
// agreements for them, after showing the agreement and taking their typed
// name. On unless the owner turns it off.
func (a *App) updateClientAgentApprovals(c *Ctx) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	body, err := c.Wrapped("setting")
	if err != nil {
		return err
	}
	on := boolOf(body["client_agent_approvals"])
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("settings", s.ID, map[string]any{"client_agent_approvals": on}) }); err != nil {
		return err
	}
	notice := "Clients decide agreements themselves; their agents can’t."
	if on {
		notice = "Clients’ agents may decide agreements, with the client’s typed name."
	}
	return c.JSON(200, O("notice", notice, "client_agent_approvals", on))
}

// ---- the portal's ------------------------------------------------------------------

func (a *App) portalConnectedAppsJSON(c *Ctx) (Obj, error) {
	ctx := c.R.Context()
	tokens, err := a.tokensJSON(ctx, `contact_id = ?`, false, c.Actor.Contact.ID)
	if err != nil {
		return nil, err
	}
	for i := range tokens {
		tokens[i] = tokens[i].Set("calls", nil)
	}
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	return O("tokens", tokens, "mcp_url", a.BaseURL()+"/portal/mcp", "can_approve", c.Actor.Contact.CanApprove,
		"client_agent_approvals", s.ClientAgentApprovals), nil
}

func (a *App) listPortalConnectedApps(c *Ctx) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	out, err := a.portalConnectedAppsJSON(c)
	if err != nil {
		return err
	}
	return c.JSON(200, out)
}

func (a *App) createPortalConnectedApp(c *Ctx) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	name, readOnly, err := tokenParams(c)
	if err != nil {
		return err
	}
	var tok *AccessToken
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		tok, err = t.issueToken(0, c.Actor.Contact.ID, name, "personal", 0, readOnly)
		return err
	})
	if err != nil {
		return err
	}
	out, err := a.portalConnectedAppsJSON(c)
	if err != nil {
		return err
	}
	return c.JSON(201, out.Set("new_token", O("id", tok.ID, "name", tok.Name, "token", tok.Plaintext, "read_only", tok.ReadOnly)))
}

func (a *App) deletePortalConnectedApp(c *Ctx) error {
	if err := inBrowser(c); err != nil {
		return err
	}
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	tok, err := scanToken(a.store.QueryRowContext(c.R.Context(), `SELECT `+tokenColumns+` FROM access_tokens WHERE id = ? AND contact_id = ? AND `+liveTokens,
		id, c.Actor.Contact.ID, store.Now().DB()))
	if isNoRows(err) {
		return notFound("connected app")
	} else if err != nil {
		return err
	}
	if err := a.write(c.R.Context(), c.Actor, func(t *Tx) error { return t.revokeToken(tok.ID) }); err != nil {
		return err
	}
	return c.JSON(200, O("notice", tok.Name+" disconnected."))
}
