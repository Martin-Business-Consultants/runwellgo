package core

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Settings › People: who is on the team, their roles, and invitations out;
// and each person's page: their details, how they sign in, and what to do
// about it. Every change here is a security event in the audit log, and a
// new password or address signs them out, so whoever had the old one is
// out too.

func init() { routeSets = append(routeSets, (*App).peopleRoutes) }

const needAnOwner = "Runwell needs an active owner. Make someone else an owner first."

func (a *App) peopleRoutes(mux *http.ServeMux) {
	mp := need("manage_people")
	a.handle(mux, "GET /api/settings/people", a.listPeople, mp, tool("list_people", "List people, invitations and what each role can do"))
	a.handle(mux, "GET /api/settings/people/{id}", a.showPerson, mp, tool("show_person", "Show someone on the team",
		note("Their name, address, role, whether they're active, how they sign in (two-factor, connected apps, open sessions) and their recent sign-ins.")))
	a.handle(mux, "PATCH /api/settings/people/{id}", a.updatePerson, mp, tool("update_person", "Change someone’s name, address or role",
		body("user", `{name, email_address, role: owner|manager|member}`, true)))
	a.handle(mux, "POST /api/settings/people/{id}/deactivation", a.deactivatePerson, mp, tool("deactivate_person", "Deactivate someone",
		note("Signs them out and keeps them out; their name stays on their history.")))
	a.handle(mux, "DELETE /api/settings/people/{id}/deactivation", a.reactivatePerson, mp, tool("reactivate_person", "Reactivate someone"))
	a.handle(mux, "POST /api/settings/people/{id}/password", a.sendPersonPasswordReset, mp)
	a.handle(mux, "PATCH /api/settings/people/{id}/password", a.setPersonPassword, mp)
	a.handle(mux, "DELETE /api/settings/people/{id}/sessions", a.signPersonOutEverywhere, mp)
	a.handle(mux, "DELETE /api/settings/people/{id}/connections", a.revokePersonConnections, mp)
}

// person is the route's {id}: someone on the team (not an agent).
func (a *App) person(ctx context.Context, c *Ctx) (*User, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, err
	}
	u, err := a.loadUser(ctx, id)
	if err != nil {
		return nil, err
	}
	if u == nil || u.IsAgent() {
		return nil, notFound("person")
	}
	return u, nil
}

// refusePeopleChangeByAgent: sign-in security is a person's to change, in
// the browser (the Rails agent_exempt actions).
func refuseAgent(c *Ctx) error {
	if c.Actor.Agent() {
		return forbidden()
	}
	return nil
}

func (a *App) listPeople(c *Ctx) error {
	ctx := c.R.Context()
	rows, err := a.store.QueryContext(ctx, `SELECT `+userColumns+` FROM users WHERE agent_owner_id IS NULL ORDER BY name`)
	if err != nil {
		return err
	}
	people, err := store.ScanAll(rows, func(r *sqlRows) (*User, error) { return scanUser(r) })
	if err != nil {
		return err
	}
	roleIndex := func(r string) int {
		if i := slices.Index(Roles, r); i >= 0 {
			return i
		}
		return len(Roles)
	}
	sort.SliceStable(people, func(i, j int) bool {
		di, dj := people[i].DeactivatedAt.Valid, people[j].DeactivatedAt.Valid
		if di != dj {
			return !di
		}
		return roleIndex(people[i].Role) < roleIndex(people[j].Role)
	})
	otp, err := countBy(ctx, a.store, `SELECT id, 1 FROM users WHERE agent_owner_id IS NULL AND otp_enabled_at IS NOT NULL`)
	if err != nil {
		return err
	}
	type agent struct {
		id   int64
		name string
	}
	agents := map[int64]agent{}
	arows, err := a.store.QueryContext(ctx, `SELECT id, agent_owner_id, name, email_address FROM users WHERE agent_owner_id IS NOT NULL`)
	if err != nil {
		return err
	}
	defer arows.Close()
	for arows.Next() {
		var id, owner int64
		var name, email string
		if err := arows.Scan(&id, &owner, &name, &email); err != nil {
			return err
		}
		agents[owner] = agent{id, strOr(name, email)}
	}
	if err := arows.Err(); err != nil {
		return err
	}
	list := []Obj{}
	active := 0
	for _, p := range people {
		var ag any
		if x, ok := agents[p.ID]; ok {
			ag = O("id", x.id, "name", x.name)
		}
		if !p.DeactivatedAt.Valid {
			active++
		}
		list = append(list, O("id", p.ID, "name", p.DisplayName(), "email_address", p.Email, "role", p.Role,
			"active", !p.DeactivatedAt.Valid, "two_factor", otp[p.ID] > 0, "agent", ag))
	}
	invitations, err := a.pendingInvitations(ctx)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	roles := Obj{}
	for _, r := range Roles {
		roles = roles.Set(r, RoleDescriptions[r])
	}
	perms := Obj{}
	for _, p := range a.permissions() {
		perms = perms.Set(p.Key, O("name", p.Name, "roles", p.Roles))
	}
	c.Summary = fmt.Sprintf("%s, %s", pluralWord(active, "active person", "active people"), pluralize(len(invitations), "pending invitation"))
	return c.JSON(200, O("summary", c.Summary, "people", list, "invitations", invitations, "two_factor_required", s.RequireTwoFactor,
		"roles", roles, "permissions", perms))
}

// pluralWord is "1 active person", "3 active people".
func pluralWord(n int, one, other string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, other)
}

func (a *App) showPerson(c *Ctx) error {
	ctx := c.R.Context()
	p, err := a.person(ctx, c)
	if err != nil {
		return err
	}
	out, err := a.personJSON(ctx, c, p)
	if err != nil {
		return err
	}
	c.Summary = str(out.Get("summary"))
	return c.JSON(200, out)
}

func (a *App) personJSON(ctx context.Context, c *Ctx, p *User) (Obj, error) {
	srows, err := a.store.QueryContext(ctx, `SELECT id, COALESCE(user_agent, ''), COALESCE(ip_address, ''), created_at, updated_at FROM sessions WHERE user_id = ? ORDER BY updated_at DESC`, p.ID)
	if err != nil {
		return nil, err
	}
	sessions, err := store.ScanAll(srows, func(r *sqlRows) (Obj, error) {
		var id int64
		var ua, ip string
		var created, updated store.Time
		if err := r.Scan(&id, &ua, &ip, &created, &updated); err != nil {
			return nil, err
		}
		return O("id", id, "user_agent", truncate(ua, 80), "ip_address", ip, "created_at", created), nil
	})
	if err != nil {
		return nil, err
	}
	trows, err := a.store.QueryContext(ctx, `SELECT id, name, kind, read_only, last_used_at FROM access_tokens
		WHERE user_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?) AND kind != 'assistant' ORDER BY created_at DESC`, p.ID, store.Now().DB())
	if err != nil {
		return nil, err
	}
	connections, err := store.ScanAll(trows, func(r *sqlRows) (Obj, error) {
		var id int64
		var name, kind string
		var ro bool
		var used store.Time
		if err := r.Scan(&id, &name, &kind, &ro, &used); err != nil {
			return nil, err
		}
		return O("id", id, "name", name, "kind", kind, "read_only", ro, "last_used_at", used), nil
	})
	if err != nil {
		return nil, err
	}
	erows, err := a.store.QueryContext(ctx, `SELECT id, kind, payload, occurred_at FROM events WHERE subject_type = 'User' AND subject_id = ?
		AND kind IN ('user.signed_in', 'user.sign_in_failed') ORDER BY occurred_at DESC LIMIT 8`, p.ID)
	if err != nil {
		return nil, err
	}
	signIns, err := store.ScanAll(erows, func(r *sqlRows) (Obj, error) {
		var id int64
		var kind string
		var payload store.JSON
		var at store.Time
		if err := r.Scan(&id, &kind, &payload, &at); err != nil {
			return nil, err
		}
		m := payload.Map()
		return O("id", id, "ok", kind == "user.signed_in", "at", at, "method", m["method"], "ip", m["ip"]), nil
	})
	if err != nil {
		return nil, err
	}
	tf, err := a.loadTwoFactor(ctx, a.store, p.ID)
	if err != nil {
		return nil, err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	state := "active"
	if !p.Active() {
		state = "deactivated"
	}
	summary := fmt.Sprintf("%s, %s, %s: %s, %s", p.DisplayName(), p.Role, state, pluralize(len(sessions), "open session"), pluralize(len(connections), "connected app"))
	if tf.On() {
		summary += ", two-factor on"
	}
	summary += "."
	return a.agentRef("User", p.ID, "", p.DisplayName(), fmt.Sprintf("/settings/people/%d", p.ID)).
		Set("summary", summary).Set("name", p.Name).Set("email_address", p.Email).Set("role", p.Role).
		Set("active", p.Active()).Set("two_factor", tf.On()).Set("open_sessions", len(sessions)).
		Set("connected_apps", orEmpty(connections)).Set("recent_sign_ins", orEmpty(signIns)).
		Set("sessions", orEmpty(sessions)).Set("two_factor_required", s.RequireTwoFactor).
		Set("you", c.Actor.User.Person().ID == p.ID).Set("role_description", RoleDescriptions[p.Role]), nil
}

// otherActiveOwners counts the active owners on the team besides id.
func otherActiveOwners(ctx context.Context, db store.Querier, id int64) (int, error) {
	return store.Count(ctx, db, `SELECT count(*) FROM users WHERE role = 'owner' AND deactivated_at IS NULL AND agent_owner_id IS NULL AND id != ?`, id)
}

// updateUserProfile changes name, address and role (whatever fields has),
// with the Rails core's rules and events: role changes and profile changes
// are recorded, the last owner can't step down, and a new address signs
// them out unless they changed it themselves.
func (t *Tx) updateUserProfile(p *User, fields map[string]any) error {
	inv := apierr.Invalid{}
	set := map[string]any{}
	changed := []string{}
	if has(fields, "name") {
		name := strings.TrimSpace(str(fields["name"]))
		if name == "" {
			inv.Add("name", "can't be blank")
		} else if name != p.Name {
			set["name"] = name
			changed = append(changed, "name")
		}
	}
	if has(fields, "email_address") {
		email := strings.ToLower(strings.TrimSpace(str(fields["email_address"])))
		switch {
		case email == "":
			inv.Add("email_address", "can't be blank")
		case !validEmail(email):
			inv.Add("email_address", "is invalid")
		case email != p.Email:
			n, err := store.Count(t.Ctx, t, `SELECT count(*) FROM users WHERE email_address = ? AND id != ?`, email, p.ID)
			if err != nil {
				return err
			}
			if n > 0 {
				inv.Add("email_address", "has already been taken")
			} else {
				set["email_address"] = email
				changed = append(changed, "email_address")
			}
		}
	}
	roleFrom := ""
	if has(fields, "role") {
		role := str(fields["role"])
		if !slices.Contains(Roles, role) {
			inv.Add("role", "is not included in the list")
		} else if role != p.Role {
			if p.Role == "owner" && !p.DeactivatedAt.Valid {
				n, err := otherActiveOwners(t.Ctx, t, p.ID)
				if err != nil {
					return err
				}
				if n == 0 {
					inv.Add("base", needAnOwner)
				}
			}
			set["role"] = role
			roleFrom = p.Role
		}
	}
	if err := fullInvalid(inv); err != nil {
		return err
	}
	if len(set) == 0 {
		return nil
	}
	if err := t.update("users", p.ID, set); err != nil {
		return err
	}
	if roleFrom != "" {
		// An agent has its person's role exactly.
		if _, err := t.Exec(`UPDATE users SET role = ? WHERE agent_owner_id = ?`, set["role"], p.ID); err != nil {
			return err
		}
		if err := t.Event("User", p.ID, "user.role_changed", map[string]any{"from": roleFrom, "to": set["role"]}); err != nil {
			return err
		}
	}
	if len(changed) > 0 {
		self := t.Actor != nil && t.Actor.User != nil && t.Actor.User.Person().ID == p.ID
		if slices.Contains(changed, "email_address") && !self {
			if _, err := t.Exec(`DELETE FROM sessions WHERE user_id = ?`, p.ID); err != nil {
				return err
			}
		}
		if err := t.Event("User", p.ID, "user.profile_changed", map[string]any{"changed": changed}); err != nil {
			return err
		}
	}
	if v, ok := set["name"]; ok {
		p.Name = v.(string)
	}
	if v, ok := set["email_address"]; ok {
		p.Email = v.(string)
	}
	if v, ok := set["role"]; ok {
		p.Role = v.(string)
	}
	return nil
}

func (a *App) updatePerson(c *Ctx) error {
	ctx := c.R.Context()
	p, err := a.person(ctx, c)
	if err != nil {
		return err
	}
	body, err := c.Wrapped("user")
	if err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.updateUserProfile(p, body) }); err != nil {
		return err
	}
	c.Summary = "Saved " + p.DisplayName() + ": " + roleWithArticle(p.Role) + "."
	out, err := a.personJSON(ctx, c, p)
	if err != nil {
		return err
	}
	return c.JSON(200, out.Set("summary", c.Summary))
}

// revokeLiveTokens revokes a person's live tokens (all but the assistant's
// when apps only): how many.
func (t *Tx) revokeLiveTokens(userID int64, appsOnly bool) (int, error) {
	q := `UPDATE access_tokens SET revoked_at = ?, updated_at = ? WHERE user_id = ? AND revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)`
	if appsOnly {
		q += ` AND kind != 'assistant'`
	}
	now := store.Now().DB()
	res, err := t.Exec(q, now, now, userID, now)
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	return int(n), nil
}

func (a *App) deactivatePerson(c *Ctx) error {
	ctx := c.R.Context()
	p, err := a.loadUserParam(ctx, c)
	if err != nil {
		return err
	}
	if p.ID == c.Actor.User.Person().ID {
		return &apierr.Error{Status: 422, Code: "refused", Message: "You can’t deactivate yourself."}
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if p.Role == "owner" {
			n, err := otherActiveOwners(ctx, t, p.ID)
			if err != nil {
				return err
			}
			if n == 0 {
				inv := apierr.Invalid{}
				inv.Add("base", needAnOwner)
				return fullInvalid(inv)
			}
		}
		now := store.Now().DB()
		if _, err := t.Exec(`UPDATE users SET deactivated_at = ?, updated_at = ? WHERE id = ? OR agent_owner_id = ?`, now, now, p.ID, p.ID); err != nil {
			return err
		}
		if err := t.Event("User", p.ID, "user.deactivated", nil); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM sessions WHERE user_id = ?`, p.ID); err != nil {
			return err
		}
		_, err := t.revokeLiveTokens(p.ID, false)
		return err
	})
	if err != nil {
		return err
	}
	c.Summary = p.DisplayName() + " is deactivated."
	return c.JSON(200, O("summary", c.Summary, "active", false))
}

func (a *App) reactivatePerson(c *Ctx) error {
	ctx := c.R.Context()
	p, err := a.loadUserParam(ctx, c)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if _, err := t.Exec(`UPDATE users SET deactivated_at = NULL, updated_at = ? WHERE id = ? OR agent_owner_id = ?`, store.Now().DB(), p.ID, p.ID); err != nil {
			return err
		}
		return t.Event("User", p.ID, "user.reactivated", nil)
	})
	if err != nil {
		return err
	}
	c.Summary = p.DisplayName() + " can sign in again."
	return c.JSON(200, O("summary", c.Summary, "active", true))
}

// loadUserParam is the route's {id} as any user (the Rails User.find).
func (a *App) loadUserParam(ctx context.Context, c *Ctx) (*User, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, err
	}
	var u *User
	u, err = scanUser(a.store.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if isNoRows(err) {
		return nil, notFound("person")
	}
	return u, err
}

// otherPerson is the route's person, refused when it's the caller.
func (a *App) otherPerson(c *Ctx, refusal string) (*User, error) {
	if err := refuseAgent(c); err != nil {
		return nil, err
	}
	p, err := a.person(c.R.Context(), c)
	if err != nil {
		return nil, err
	}
	if p.ID == c.Actor.User.Person().ID {
		return nil, &apierr.Error{Status: 422, Code: "refused", Message: refusal}
	}
	return p, nil
}

func (a *App) sendPersonPasswordReset(c *Ctx) error {
	p, err := a.otherPerson(c, "Change your own password in Your account.")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.sendPasswordReset(p); err != nil {
			return err
		}
		return t.Event("User", p.ID, "user.password_reset_sent", nil)
	})
	if err != nil {
		return err
	}
	c.Summary = p.DisplayName() + " has an email with a link to choose a new password."
	return c.JSON(200, O("summary", c.Summary))
}

// checkNewPassword is a new password and its confirmation, as
// has_secure_password checks them.
func checkNewPassword(password string, confirmation *string) error {
	inv := apierr.Invalid{}
	switch {
	case password == "":
		inv.Add("password", "can't be blank")
	case len(password) > 72:
		inv.Add("password", "is too long (maximum is 72 bytes)")
	}
	if confirmation != nil && *confirmation != password {
		inv.Add("password_confirmation", "doesn't match Password")
	}
	return fullInvalid(inv)
}

// setPassword sets a person's password and signs them out everywhere.
func (t *Tx) setPassword(userID int64, password string) error {
	digest, err := secret.HashPassword(password)
	if err != nil {
		return err
	}
	if _, err := t.Exec(`UPDATE users SET password_digest = ?, updated_at = ? WHERE id = ?`, digest, store.Now().DB(), userID); err != nil {
		return err
	}
	if err := t.dropAccountTokens(purposePasswordReset, userID); err != nil {
		return err
	}
	_, err = t.Exec(`DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (a *App) setPersonPassword(c *Ctx) error {
	p, err := a.otherPerson(c, "Change your own password in Your account.")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("user")
	if err != nil {
		return err
	}
	confirmation := str(body["password_confirmation"])
	if err := checkNewPassword(str(body["password"]), &confirmation); err != nil {
		return err
	}
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		if err := t.setPassword(p.ID, str(body["password"])); err != nil {
			return err
		}
		return t.Event("User", p.ID, "user.password_set", nil)
	})
	if err != nil {
		return err
	}
	c.Summary = p.DisplayName() + "’s password is changed, and they’re signed out everywhere."
	return c.JSON(200, O("summary", c.Summary))
}

func (a *App) signPersonOutEverywhere(c *Ctx) error {
	p, err := a.otherPerson(c, "Sign yourself out from the menu instead.")
	if err != nil {
		return err
	}
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		res, err := t.Exec(`DELETE FROM sessions WHERE user_id = ?`, p.ID)
		if err != nil {
			return err
		}
		n, _ := res.RowsAffected()
		return t.Event("User", p.ID, "user.signed_out_everywhere", map[string]any{"sessions": n})
	})
	if err != nil {
		return err
	}
	c.Summary = p.DisplayName() + " is signed out everywhere."
	return c.JSON(200, O("summary", c.Summary))
}

func (a *App) revokePersonConnections(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	p, err := a.person(c.R.Context(), c)
	if err != nil {
		return err
	}
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		n, err := t.revokeLiveTokens(p.ID, true)
		if err != nil {
			return err
		}
		return t.Event("User", p.ID, "user.connections_revoked", map[string]any{"tokens": n})
	})
	if err != nil {
		return err
	}
	c.Summary = "Every app " + p.DisplayName() + " connected is cut off."
	return c.JSON(200, O("summary", c.Summary))
}
