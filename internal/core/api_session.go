package core

import (
	"context"
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Signing in and out, the first owner's sign-up, and who is signed in.

func init() { routeSets = append(routeSets, (*App).sessionRoutes) }

func (a *App) sessionRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/session", a.showSession, open)
	a.handle(mux, "POST /api/session", a.createSession, open)
	a.handle(mux, "DELETE /api/session", a.destroySession)
	a.handle(mux, "POST /api/signup", a.createSignup, open)
	a.handle(mux, "GET /api/me", a.showMe, tool("me", "Who am I, and what can I do",
		note("Your name and role, what your role allows, and the team (ids for owners of work and commitments). Call this when something is forbidden or a list is empty.")))
}

// showSession is what the app asks on load: who's signed in (and the CSRF
// token its writes carry), or whether the install still needs its first
// owner.
func (a *App) showSession(c *Ctx) error {
	ctx := c.R.Context()
	users, err := store.Count(ctx, a.store, `SELECT count(*) FROM users`)
	if err != nil {
		return err
	}
	out := O("needs_setup", users == 0, "version", a.cfg.Version, "brand", a.BrandName(ctx))
	if c.Actor != nil && c.Actor.User != nil && c.Actor.Session != nil {
		me, err := a.meJSON(ctx, c)
		if err != nil {
			return err
		}
		out = out.Set("me", me).Set("csrf", c.Actor.Session.CSRF)
	}
	return c.JSON(200, out)
}

func (a *App) createSession(c *Ctx) error {
	var body struct {
		Email    string `json:"email_address"`
		Password string `json:"password"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(body.Email))
	if !allowed(a.signIns, "ip:"+c.ip()) || !allowed(a.signIns, "email:"+email) {
		return &apierr.Error{Status: 429, Code: "rate_limited", Message: "Try again later."}
	}
	ctx := c.R.Context()
	var id int64
	var digest string
	err := a.store.QueryRowContext(ctx, `SELECT id, password_digest FROM users WHERE email_address = ? AND agent_owner_id IS NULL`, email).Scan(&id, &digest)
	if err != nil && !isNoRows(err) {
		return err
	}
	if id == 0 {
		secret.CheckPassword(secret.DummyHash(), body.Password) // the same time as a real account
		return wrongPassword()
	}
	if !secret.CheckPassword(digest, body.Password) {
		a.failedSignIn(ctx, c, id, "password")
		return wrongPassword()
	}
	user, err := a.loadUser(ctx, id)
	if err != nil {
		return err
	}
	if user == nil || !user.Active() {
		return wrongPassword()
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	if s.ProvidersOnly && user.Role != "owner" {
		return &apierr.Error{Status: 422, Code: "providers_only", Message: "This install signs in with a sign-in provider, not a password."}
	}
	return a.signInAfterFirstStep(c, user, "password")
}

func wrongPassword() error {
	return &apierr.Error{Status: 422, Code: "invalid_credentials", Message: "Try another email address or password."}
}

// signInAfterFirstStep signs user in once the first step checked out (a
// password, an emailed link, a provider), or asks for the code from their
// authenticator app when they have two-factor sign-in on (twoFactorStep,
// which the account area sets).
func (a *App) signInAfterFirstStep(c *Ctx, user *User, method string) error {
	if twoFactorStep != nil {
		if handled, err := twoFactorStep(a, c, user, method); handled || err != nil {
			return err
		}
	}
	return a.finishSignIn(c, user, method)
}

// twoFactorStep, when set, answers a sign-in whose person has two-factor
// on with the challenge (handled true); the account area sets it.
var twoFactorStep func(a *App, c *Ctx, user *User, method string) (handled bool, err error)

// finishSignIn opens the session, records the sign-in for the audit log, and
// answers what showSession would.
func (a *App) finishSignIn(c *Ctx, user *User, method string) error {
	ctx := c.R.Context()
	csrf, err := a.signIn(c, user.ID)
	if err != nil {
		return err
	}
	a.sawAddress(c.R)
	actor := &Actor{User: user}
	err = a.write(ctx, actor, func(t *Tx) error {
		var otp store.Time
		_ = t.QueryRow(`SELECT otp_enabled_at FROM users WHERE id = ?`, user.ID).Scan(&otp)
		return t.Event("User", user.ID, "user.signed_in", map[string]any{"method": method, "two_factor": otp.Valid, "ip": c.ip(), "browser": truncate(c.R.UserAgent(), 120)})
	})
	if err != nil {
		return err
	}
	c.Actor = &Actor{User: user, Session: &session{CSRF: csrf}}
	me, err := a.meJSON(ctx, c)
	if err != nil {
		return err
	}
	return c.JSON(200, O("needs_setup", false, "version", a.cfg.Version, "brand", a.BrandName(ctx), "me", me, "csrf", csrf))
}

// failedSignIn records a wrong password or code for a real account, for the
// audit log. Nobody is signed in, so the event has no actor.
func (a *App) failedSignIn(ctx context.Context, c *Ctx, userID int64, reason string) {
	err := a.write(ctx, nil, func(t *Tx) error {
		return t.Event("User", userID, "user.sign_in_failed", map[string]any{"reason": reason, "ip": c.ip(), "browser": truncate(c.R.UserAgent(), 120)})
	})
	if err != nil {
		a.log.Warn("failed sign-in", "err", err)
	}
}

func (a *App) destroySession(c *Ctx) error {
	if err := a.signOut(c); err != nil {
		return err
	}
	return c.NoContent()
}

// Starts are where an install begins (chosen at sign-up, or again in
// Settings › Names): the words for its core things and whether agreements
// carry prices.
var Starts = []struct {
	Key, Name, Description string
	Prices                 bool
	Terminology            map[string]any
}{
	{"business", "A business", "Clients, projects and work orders, with agreements they approve and prices on them.", true, map[string]any{}},
	{"team", "A team", "Departments you deliver for, initiatives and their deliverables, tasks. No prices.", false, map[string]any{
		"client":     map[string]any{"one": "Department", "other": "Departments"},
		"engagement": map[string]any{"one": "Initiative", "other": "Initiatives"},
		"scope_item": map[string]any{"one": "Deliverable", "other": "Deliverables"},
		"work":       map[string]any{"one": "Task", "other": "Tasks"},
		"labels": map[string]any{
			"work_order": map[string]any{"enabled": false},
			"service":    map[string]any{"one": "Program", "other": "Programs", "prefix": "PG"},
		},
	}},
	{"personal", "Personal life", "Areas of life (home, family, a wedding, a trip), plans and their steps, tasks and promises. No prices.", false, map[string]any{
		"client":     map[string]any{"one": "Area", "other": "Areas"},
		"engagement": map[string]any{"one": "Plan", "other": "Plans"},
		"scope_item": map[string]any{"one": "Step", "other": "Steps"},
		"work":       map[string]any{"one": "Task", "other": "Tasks"},
		"commitment": map[string]any{"one": "Promise", "other": "Promises"},
		"labels": map[string]any{
			"project":    map[string]any{"one": "Plan", "other": "Plans"},
			"work_order": map[string]any{"enabled": false},
			"service":    map[string]any{"one": "Routine", "other": "Routines", "prefix": "R"},
		},
	}},
}

// applyStart sets the words and prices for start.
func (t *Tx) applyStart(key string) error {
	for _, s := range Starts {
		if s.Key == key {
			_, err := t.Exec(`UPDATE settings SET terminology = ?, prices = ?, updated_at = ?`, string(store.MustJSON(s.Terminology)), s.Prices, store.Now().DB())
			return err
		}
	}
	return apierr.BadRequest("no start " + key)
}

// createSignup makes the first owner: signing up works once, and after
// that people join by invitation.
func (a *App) createSignup(c *Ctx) error {
	if !allowed(a.signIns, "ip:"+c.ip()) {
		return &apierr.Error{Status: 429, Code: "rate_limited", Message: "Try again later."}
	}
	body, err := c.Wrapped("signup")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	if _, err := a.settings(ctx); err != nil { // the row, before the write needs it
		return err
	}
	name, email, password := strings.TrimSpace(str(body["name"])), strings.ToLower(strings.TrimSpace(str(body["email_address"]))), str(body["password"])
	start := str(body["start"])
	if start == "" {
		start = "business"
	}
	invalid := apierr.Invalid{}
	if name == "" {
		invalid.Add("name", "can't be blank")
	}
	if !validEmail(email) {
		invalid.Add("email_address", "is invalid")
	}
	if len(password) < 8 {
		invalid.Add("password", "is too short (minimum is 8 characters)")
	}
	if err := invalid.Err(); err != nil {
		return err
	}
	digest, err := secret.HashPassword(password)
	if err != nil {
		return err
	}
	var id int64
	err = a.write(ctx, nil, func(t *Tx) error {
		n, err := store.Count(ctx, t, `SELECT count(*) FROM users`)
		if err != nil {
			return err
		}
		if n > 0 {
			return &apierr.Error{Status: 403, Code: "invite_only", Message: "Runwell is invite-only. Ask an owner for an invitation."}
		}
		now := store.Now().DB()
		if id, err = t.Insert(`INSERT INTO users (email_address, password_digest, name, role, created_at, updated_at) VALUES (?, ?, ?, 'owner', ?, ?)`,
			email, digest, name, now, now); err != nil {
			return err
		}
		return t.applyStart(start)
	})
	if err != nil {
		return err
	}
	user, err := a.loadUser(ctx, id)
	if err != nil {
		return err
	}
	return a.finishSignIn(c, user, "signup")
}

// showMe is who the caller is: their role, what it lets them do, and the
// team (for owners of work and commitments).
func (a *App) showMe(c *Ctx) error {
	me, err := a.meJSON(c.R.Context(), c)
	if err != nil {
		return err
	}
	c.Summary = "You are " + c.Actor.Label() + " (" + c.Actor.User.Role + ")."
	return c.JSON(200, me.Set("read_only", c.Actor.Token != nil && c.Actor.Token.ReadOnly).Set("withheld_tools", a.withheldTools(c.Actor)))
}

func (a *App) meJSON(ctx context.Context, c *Ctx) (Obj, error) {
	u := c.Actor.User
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	perms := []string{}
	for _, p := range a.permissions() {
		if c.Can(p.Key) {
			perms = append(perms, p.Key)
		}
	}
	team, err := a.team(ctx)
	if err != nil {
		return nil, err
	}
	var textSize, navPlugins any
	var otp store.Time
	var ts string
	var nav store.JSON
	if err := a.store.QueryRowContext(ctx, `SELECT COALESCE(text_size, ''), nav_plugins, otp_enabled_at FROM users WHERE id = ?`, u.Person().ID).Scan(&ts, &nav, &otp); err != nil {
		return nil, err
	}
	if ts != "" {
		textSize = ts
	}
	if len(nav) > 0 {
		navPlugins = nav.List()
	}
	return O("id", u.ID, "name", u.DisplayName(), "email_address", u.Email, "role", u.Role, "agent", u.IsAgent(),
		"person_id", u.Person().ID, "permissions", perms, "two_factor", otp.Valid, "two_factor_required", s.RequireTwoFactor, "text_size", textSize, "nav_plugins", navPlugins,
		"terms", s.TermsJSON(), "prices", s.Prices, "index_view", s.IndexView, "team", team), nil
}

// team is everyone active on staff (people, not agents), by name.
func (a *App) team(ctx context.Context) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT `+userColumns+` FROM users WHERE deactivated_at IS NULL AND agent_owner_id IS NULL ORDER BY name`)
	if err != nil {
		return nil, err
	}
	users, err := store.ScanAll(rows, func(r *sqlRows) (*User, error) { return scanUser(r) })
	if err != nil {
		return nil, err
	}
	out := []Obj{}
	for _, u := range users {
		out = append(out, O("id", u.ID, "name", u.DisplayName(), "email_address", u.Email, "role", u.Role))
	}
	return out, nil
}

func validEmail(s string) bool {
	at := strings.LastIndex(s, "@")
	return at > 0 && at < len(s)-1 && !strings.ContainsAny(s, " \t\r\n<>,;")
}

// truncate cuts s to n runes, with an ellipsis.
func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
