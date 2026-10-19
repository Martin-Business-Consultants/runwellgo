package core

import (
	"context"
	"crypto/subtle"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// ---- roles and permissions -------------------------------------------------

// Roles are the three fixed roles, as the Rails core has them. There is no
// permission builder: an install that needs finer control adds a plugin.
var Roles = []string{"owner", "manager", "member"}

// RoleDescriptions say what each role is for.
var RoleDescriptions = map[string]string{
	"owner":   "Everything, including settings, plugins and people",
	"manager": "Sends agreements, records decisions, triages, closes and deletes",
	"member":  "Does the work: updates work, adds notes, documents and commitments",
}

// Permission is one thing a role may do: its name in words and the roles
// that hold it.
type Permission struct {
	Key   string   `json:"key"`
	Name  string   `json:"name"`
	Roles []string `json:"roles"`
}

// Permissions are the core's; plugins that are on add their own.
var Permissions = []Permission{
	{"manage_settings", "Change settings and plugins", []string{"owner"}},
	{"manage_people", "Invite people and change their roles", []string{"owner"}},
	{"send_agreements", "Send agreements and record client decisions", []string{"owner", "manager"}},
	{"close_engagements", "Close engagements", []string{"owner", "manager"}},
	{"erase_engagements", "Delete closed engagements permanently, agreements and all", []string{"owner"}},
	{"triage_requests", "Triage requests", []string{"owner", "manager"}},
	{"delete_records", "Delete records", []string{"owner", "manager"}},
}

// permissions is every permission held now: the core's and the enabled
// plugins'.
func (a *App) permissions() []Permission {
	out := slices.Clone(Permissions)
	for _, g := range a.plugins.CapGroups() {
		for i, c := range g.Caps {
			out = append(out, Permission{Key: c, Name: g.Labels[i], Roles: a.plugins.CapRoles(c)})
		}
	}
	return out
}

// can is whether role holds permission. A permission from a plugin that is
// off is simply not held; one nobody declared is a bug.
func (a *App) can(role, permission string) bool {
	for _, p := range a.permissions() {
		if p.Key == permission {
			return slices.Contains(p.Roles, role)
		}
	}
	if strings.Contains(permission, ":") { // a plugin's, while it's off
		return false
	}
	panic("unknown permission: " + permission)
}

// ---- who is asking ---------------------------------------------------------

// User is a person on staff, or a person's agent (AgentOwnerID set): the user
// that apps they connect act as.
type User struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Email         string     `json:"email_address"`
	Role          string     `json:"role"`
	DeactivatedAt store.Time `json:"deactivated_at"`
	AgentOwnerID  int64      `json:"agent_owner_id,omitempty"`
	// The agent's person, for an agent.
	Owner *User `json:"-"`
}

// IsAgent is whether u is a person's agent.
func (u *User) IsAgent() bool { return u.AgentOwnerID != 0 }

// Person is who u is on behalf of: u, or an agent's person.
func (u *User) Person() *User {
	if u.Owner != nil {
		return u.Owner
	}
	return u
}

// Active is whether u may act: not deactivated, and for an agent, its person
// neither.
func (u *User) Active() bool {
	return !u.DeactivatedAt.Valid && (u.Owner == nil || u.Owner.Active())
}

// DisplayName is the name, else the address.
func (u *User) DisplayName() string {
	if u.Name != "" {
		return u.Name
	}
	return u.Email
}

// Contact is a client's contact signed in to the portal, or acting through
// their token.
type Contact struct {
	ID           int64  `json:"id"`
	ClientID     int64  `json:"client_id"`
	Name         string `json:"name"`
	Email        string `json:"email"`
	CanApprove   bool   `json:"can_approve"`
	PortalAccess bool   `json:"portal_access"`
}

// Token is the bearer token a request came with.
type Token struct {
	ID       int64
	Name     string
	Kind     string // personal, oauth, assistant
	ReadOnly bool
}

// Actor is whoever a request acts as: a person on staff (by session), a
// person's agent (by their token), or a client's contact (portal session or
// token). Exactly one of User and Contact is set.
type Actor struct {
	User    *User
	Contact *Contact
	Token   *Token
	Session *session
}

// Agent is whether the actor came with a token: an agent acting for someone.
func (a *Actor) Agent() bool { return a != nil && a.Token != nil }

// Staff is whether the actor is on staff (a person or their agent).
func (a *Actor) Staff() bool { return a != nil && a.User != nil }

// Source is where a write came from, as events record it: app, agent or
// portal.
func (a *Actor) Source() string {
	switch {
	case a == nil:
		return "system"
	case a.Token != nil:
		return "agent"
	case a.Contact != nil:
		return "portal"
	}
	return "app"
}

// UserID is the staff user's id, 0 for anyone else.
func (a *Actor) UserID() int64 {
	if a == nil || a.User == nil {
		return 0
	}
	return a.User.ID
}

// Label is who it is in words: "Ted", "Ted's agent", a contact's name,
// "system".
func (a *Actor) Label() string {
	switch {
	case a == nil:
		return "system"
	case a.User != nil:
		return a.User.DisplayName()
	case a.Contact != nil:
		return a.Contact.Name
	}
	return "system"
}

// AppLabel is the app acting, for an agent ("Claude"): the token's name.
func (a *Actor) AppLabel() string {
	if a == nil || a.Token == nil {
		return ""
	}
	return a.Token.Name
}

// systemActor does what the app does by itself (the scheduler, a job).
var systemActor *Actor

// ---- route options ---------------------------------------------------------

type route struct {
	auth   bool      // false: open to anyone
	portal bool      // a client's contact, not staff
	perm   string    // a permission the staff actor must hold
	tool   *toolSpec // the agent tool this route is (agent.go)
}

// open is a route anyone may call: sign-in, an approval link, setup.
func open(r *route) { r.auth = false }

// portal is a route for a client's contact (the portal and its MCP), not
// staff.
func portal(r *route) { r.portal = true }

// need is a route that also needs a permission (manage_settings, …).
func need(permission string) func(*route) { return func(r *route) { r.perm = permission } }

// Need refuses unless the actor is on staff and their role holds
// permission: what a handler checks when its rule depends on the record.
func (c *Ctx) Need(permission string) error {
	if c.Actor == nil || c.Actor.User == nil {
		return unauthorized()
	}
	if !c.Can(permission) {
		return forbidden()
	}
	return nil
}

// Can is whether the actor (on staff, active) holds permission.
func (c *Ctx) Can(permission string) bool {
	if c.Actor == nil || c.Actor.User == nil || !c.Actor.User.Active() {
		return false
	}
	return c.App.can(c.Actor.User.Person().Role, permission)
}

func unauthorized() error {
	return &apierr.Error{Status: 401, Code: "unauthorized", Message: "Sign in, or send Authorization: Bearer <token>."}
}

func forbidden() error {
	return &apierr.Error{Status: 403, Code: "forbidden", Message: "Your role can't do that. Call `me` to see what it can."}
}

// ---- authentication --------------------------------------------------------

const (
	sessionCookie = "rw_session"
	portalCookie  = "rw_portal"
	// Two weeks idle signs a session out.
	sessionIdle = 14 * 24 * time.Hour
)

type session struct {
	ID     int64
	CSRF   string
	Portal bool
}

// authenticate finds who's asking: a bearer token, else the session cookie
// for the kind of route it is (staff or portal).
func (a *App) authenticate(c *Ctx, portalRoute bool) (*Actor, error) {
	if h := c.R.Header.Get("Authorization"); strings.HasPrefix(h, "Bearer ") {
		plain := strings.TrimSpace(strings.TrimPrefix(h, "Bearer "))
		actor, err := a.actorForToken(c.R.Context(), plain)
		if err != nil {
			return nil, err
		}
		if actor == nil {
			return nil, &apierr.Error{Status: 401, Code: "unauthorized", Message: "That token isn't valid: it was revoked, paused, expired, or never existed."}
		}
		return actor, nil
	}
	name := sessionCookie
	if portalRoute {
		name = portalCookie
	}
	cookie, _ := c.R.Cookie(name)
	if cookie == nil || cookie.Value == "" {
		return nil, nil
	}
	actor, err := a.actorForSession(c.R.Context(), cookie.Value, portalRoute)
	if err != nil || actor == nil {
		return nil, err
	}
	// A write signed in by the browser's cookie must carry the session's
	// CSRF token, or a sibling site's page could act as whoever is signed in.
	if c.R.Method != http.MethodGet && c.R.Method != http.MethodHead {
		if subtle.ConstantTimeCompare([]byte(c.R.Header.Get("X-CSRF-Token")), []byte(actor.Session.CSRF)) != 1 {
			return nil, &apierr.Error{Status: 422, Code: "invalid_authenticity_token", Message: "A write signed in by the browser's session needs the page's CSRF token (X-CSRF-Token), or use a Bearer token."}
		}
	}
	return actor, nil
}

// actorForToken is the live token's holder: a person on staff acting as
// their agent, or a client's contact. Nil for a token that's revoked,
// paused, expired, or whose holder can't act any more.
func (a *App) actorForToken(ctx context.Context, plain string) (*Actor, error) {
	if plain == "" {
		return nil, nil
	}
	var t Token
	var userID, contactID store.NullInt
	var lastUsed store.Time
	err := a.store.QueryRowContext(ctx, `SELECT id, name, kind, read_only, user_id, contact_id, last_used_at FROM access_tokens
		WHERE token_digest = ? AND revoked_at IS NULL AND paused_at IS NULL AND (expires_at IS NULL OR expires_at > ?)`,
		secret.Digest(plain), store.Now().DB()).Scan(&t.ID, &t.Name, &t.Kind, &t.ReadOnly, &userID, &contactID, &lastUsed)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	actor := &Actor{Token: &t}
	switch {
	case userID.Valid:
		person, err := a.loadUser(ctx, userID.Int64)
		if err != nil || person == nil || !person.Active() {
			return nil, err
		}
		agent, err := a.agentFor(ctx, person)
		if err != nil {
			return nil, err
		}
		actor.User = agent
	case contactID.Valid:
		contact, err := a.loadContact(ctx, contactID.Int64)
		if err != nil || contact == nil || !contact.PortalAccess {
			return nil, err
		}
		actor.Contact = contact
	default:
		return nil, nil
	}
	if !lastUsed.Valid || time.Since(lastUsed.Time) > time.Minute {
		a.touch(ctx, `UPDATE access_tokens SET last_used_at = ? WHERE id = ?`, store.Now().DB(), t.ID)
	}
	return actor, nil
}

// touch records a last-used time. It's bookkeeping: a failure is logged,
// and the request it came with goes on.
func (a *App) touch(ctx context.Context, query string, args ...any) {
	if _, err := a.store.ExecContext(ctx, query, args...); err != nil {
		a.log.Warn("touch", "err", err)
	}
}

const userColumns = `id, name, email_address, role, deactivated_at, COALESCE(agent_owner_id, 0)`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	if err := row.Scan(&u.ID, &u.Name, &u.Email, &u.Role, &u.DeactivatedAt, &u.AgentOwnerID); err != nil {
		return nil, err
	}
	return &u, nil
}

// loadUser is the user with id, with an agent's person; nil when there's
// none.
func (a *App) loadUser(ctx context.Context, id int64) (*User, error) {
	u, err := scanUser(a.store.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if u.AgentOwnerID != 0 {
		if u.Owner, err = a.loadUser(ctx, u.AgentOwnerID); err != nil {
			return nil, err
		}
		if u.Owner == nil {
			return nil, nil
		}
		// An agent has its person's role exactly.
		u.Role = u.Owner.Role
	}
	return u, nil
}

// agentFor is person's agent, made the first time it's needed: the user
// their tokens act as, so the history says "Ted's agent".
func (a *App) agentFor(ctx context.Context, person *User) (*User, error) {
	var id int64
	err := a.store.QueryRowContext(ctx, `SELECT id FROM users WHERE agent_owner_id = ?`, person.ID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		id, err = a.createAgent(ctx, person)
	}
	if err != nil {
		return nil, err
	}
	return a.loadUser(ctx, id)
}

func (a *App) createAgent(ctx context.Context, person *User) (int64, error) {
	name := person.Name
	if name == "" {
		name, _, _ = strings.Cut(person.Email, "@")
	}
	local, domain, _ := strings.Cut(person.Email, "@")
	digest, err := secret.HashPassword(secret.Token(32))
	if err != nil {
		return 0, err
	}
	now := store.Now().DB()
	for i := 1; i <= 20; i++ {
		email := local + "+agent@" + domain
		if i > 1 {
			email = fmt.Sprintf("%s+agent%d@%s", local, i, domain)
		}
		res, err := a.store.ExecContext(ctx, `INSERT INTO users (email_address, password_digest, name, role, agent_owner_id, created_at, updated_at)
			SELECT ?, ?, ?, ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM users WHERE email_address = ?)`,
			email, digest, name+"'s agent", person.Role, person.ID, now, now, email)
		if err != nil {
			// Another request made it first (agent_owner_id is unique).
			var id int64
			if a.store.QueryRowContext(ctx, `SELECT id FROM users WHERE agent_owner_id = ?`, person.ID).Scan(&id) == nil {
				return id, nil
			}
			return 0, err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return res.LastInsertId()
		}
	}
	return 0, fmt.Errorf("no free agent address for %s", person.Email)
}

func (a *App) loadContact(ctx context.Context, id int64) (*Contact, error) {
	var ct Contact
	var email sql.NullString
	err := a.store.QueryRowContext(ctx, `SELECT id, client_id, name, email, can_approve, portal_access FROM contacts WHERE id = ? AND archived_at IS NULL`, id).
		Scan(&ct.ID, &ct.ClientID, &ct.Name, &email, &ct.CanApprove, &ct.PortalAccess)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	ct.Email = email.String
	return &ct, nil
}

// actorForSession is the signed-in person (or portal contact) for a
// session cookie.
func (a *App) actorForSession(ctx context.Context, cookie string, portalRoute bool) (*Actor, error) {
	table, owner := "sessions", "user_id"
	if portalRoute {
		table, owner = "portal_sessions", "contact_id"
	}
	s := session{Portal: portalRoute}
	var ownerID int64
	var seen, created store.Time
	err := a.store.QueryRowContext(ctx, `SELECT id, COALESCE(csrf, ''), `+owner+`, last_seen_at, created_at FROM `+table+` WHERE token_digest = ?`, secret.Digest(cookie)).
		Scan(&s.ID, &s.CSRF, &ownerID, &seen, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	last := seen
	if !last.Valid {
		last = created
	}
	if time.Since(last.Time) > sessionIdle {
		_, err := a.store.ExecContext(ctx, `DELETE FROM `+table+` WHERE id = ?`, s.ID)
		return nil, err
	}
	if !seen.Valid || time.Since(seen.Time) > time.Minute {
		a.touch(ctx, `UPDATE `+table+` SET last_seen_at = ?, updated_at = ? WHERE id = ?`, store.Now().DB(), store.Now().DB(), s.ID)
	}
	if portalRoute {
		contact, err := a.loadContact(ctx, ownerID)
		if err != nil || contact == nil || !contact.PortalAccess {
			return nil, err
		}
		return &Actor{Contact: contact, Session: &s}, nil
	}
	user, err := a.loadUser(ctx, ownerID)
	if err != nil || user == nil || !user.Active() || user.IsAgent() {
		return nil, err
	}
	return &Actor{User: user, Session: &s}, nil
}

// signIn opens a staff session for the user and sets its cookie; the CSRF
// token the app sends with writes.
func (a *App) signIn(c *Ctx, userID int64) (string, error) {
	return a.openSession(c, "sessions", "user_id", sessionCookie, userID)
}

// signInContact opens a portal session for a client's contact.
func (a *App) signInContact(c *Ctx, contactID int64) (string, error) {
	return a.openSession(c, "portal_sessions", "contact_id", portalCookie, contactID)
}

func (a *App) openSession(c *Ctx, table, owner, cookieName string, id int64) (string, error) {
	cookie := secret.Token(32)
	csrf := secret.Token(24)
	now := store.Now().DB()
	_, err := a.store.ExecContext(c.R.Context(), `INSERT INTO `+table+` (token_digest, csrf, `+owner+`, ip_address, user_agent, last_seen_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		secret.Digest(cookie), csrf, id, c.ip(), c.R.UserAgent(), now, now, now)
	if err != nil {
		return "", err
	}
	a.setCookie(c, cookieName, cookie, int(sessionIdle.Seconds()))
	return csrf, nil
}

// signOut ends the request's session and clears its cookie.
func (a *App) signOut(c *Ctx) error {
	if c.Actor == nil || c.Actor.Session == nil {
		return nil
	}
	table, name := "sessions", sessionCookie
	if c.Actor.Session.Portal {
		table, name = "portal_sessions", portalCookie
	}
	if _, err := a.store.ExecContext(c.R.Context(), `DELETE FROM `+table+` WHERE id = ?`, c.Actor.Session.ID); err != nil {
		return err
	}
	a.setCookie(c, name, "", -1)
	return nil
}

// setCookie sets a session cookie (maxAge -1 clears it): HttpOnly,
// SameSite=Lax, and Secure when the request came over HTTPS.
func (a *App) setCookie(c *Ctx, name, value string, maxAge int) {
	// Secure when this request came over HTTPS (directly, or through a
	// proxy saying so), not because the saved address is https: a copy of a
	// live install, run on localhost over http, must still sign in.
	secure := c.R.TLS != nil || c.R.Header.Get("X-Forwarded-Proto") == "https"
	//nolint:gosec // Secure over HTTPS only: a plain-HTTP localhost install couldn't sign in otherwise
	http.SetCookie(c.W, &http.Cookie{
		Name: name, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}
