package core

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// How people join after the first owner: an owner invites an email address
// with a role, and the emailed link (good once, for a week) lets them
// choose a name and password. Resending makes a fresh link; the old one
// stops working.

func init() { routeSets = append(routeSets, (*App).invitationRoutes) }

const invitationExpiresIn = 7 * 24 * time.Hour

func (a *App) invitationRoutes(mux *http.ServeMux) {
	mp := need("manage_people")
	a.handle(mux, "POST /api/settings/invitations", a.createInvitation, mp, tool("invite_person", "Invite someone to the team",
		body("invitation", `{email_address (required), role: owner|manager|member}`, true)))
	a.handle(mux, "PATCH /api/settings/invitations/{id}", a.resendInvitation, mp, tool("resend_invitation", "Resend an invitation"))
	a.handle(mux, "DELETE /api/settings/invitations/{id}", a.withdrawInvitation, mp, tool("withdraw_invitation", "Withdraw an invitation"))
	a.handle(mux, "GET /api/invitations/{token}", a.showInvitationLink, open)
	a.handle(mux, "PATCH /api/invitations/{token}", a.acceptInvitation, open)
}

type invitation struct {
	ID          int64
	Email       string
	Role        string
	InvitedByID int64
	InvitedBy   string
	SentAt      store.Time
}

func (inv *invitation) expired() bool {
	return inv.SentAt.Time.Before(time.Now().Add(-invitationExpiresIn))
}

const invitationColumns = `i.id, i.email_address, i.role, i.invited_by_id, COALESCE(NULLIF(u.name, ''), u.email_address, ''), i.sent_at`

func scanInvitation(r interface{ Scan(...any) error }) (*invitation, error) {
	var inv invitation
	err := r.Scan(&inv.ID, &inv.Email, &inv.Role, &inv.InvitedByID, &inv.InvitedBy, &inv.SentAt)
	return &inv, err
}

func (inv *invitation) json() Obj {
	return O("id", inv.ID, "email_address", inv.Email, "role", inv.Role, "sent_at", inv.SentAt, "expired", inv.expired(),
		"invited_by", userRef(inv.InvitedByID, inv.InvitedBy))
}

// pendingInvitations are the invitations not yet accepted, newest first.
func (a *App) pendingInvitations(ctx context.Context) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT `+invitationColumns+` FROM invitations i LEFT JOIN users u ON u.id = i.invited_by_id
		WHERE i.accepted_at IS NULL ORDER BY i.sent_at DESC`)
	if err != nil {
		return nil, err
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (*invitation, error) { return scanInvitation(r) })
	if err != nil {
		return nil, err
	}
	out := []Obj{}
	for _, inv := range list {
		out = append(out, inv.json())
	}
	return out, nil
}

func (a *App) pendingInvitation(ctx context.Context, db store.Querier, where string, args ...any) (*invitation, error) {
	inv, err := scanInvitation(db.QueryRowContext(ctx, `SELECT `+invitationColumns+` FROM invitations i LEFT JOIN users u ON u.id = i.invited_by_id
		WHERE i.accepted_at IS NULL AND `+where, args...))
	if isNoRows(err) {
		return nil, notFound("invitation")
	}
	return inv, err
}

// deliverInvitation makes a fresh link for inv (the old one stops working)
// and mails it once the write commits.
func (t *Tx) deliverInvitation(inv *invitation) error {
	if _, err := t.Exec(`DELETE FROM account_tokens WHERE purpose = ? AND new_email = ?`, purposeInvitation, inv.Email); err != nil {
		return err
	}
	token, err := t.issueAccountToken(purposeInvitation, 0, inv.Email, invitationExpiresIn)
	if err != nil {
		return err
	}
	a := t.App
	t.AfterCommit(func(ctx context.Context) {
		link := a.BaseURL() + "/invitations/" + token
		brand := a.BrandName(ctx)
		team := brand
		if brand == "Runwell" {
			team = "the team"
		}
		role := roleWithArticle(inv.Role)
		reason := inv.InvitedBy + " invited this address. Not expecting it? Ignore this email and nothing happens."
		a.sendAccountMail(ctx, accountMail{
			To: inv.Email, Subject: inv.InvitedBy + " invited you to Runwell", Preheader: "Choose your name and password to get started.",
			Heading:    "Join " + team + " on Runwell",
			Paragraphs: []string{inv.InvitedBy + " invited you to join as " + role + ". Accept to choose your name and password."},
			Button:     "Accept the invitation", URL: link, Small: "The link works once and expires in 7 days.", Reason: reason,
			Text: inv.InvitedBy + " invited you to join Runwell as " + role + ".\n\nAccept the invitation to choose your name and password:\n" +
				link + "\n\nThe link works once and expires in 7 days.",
		})
	})
	return nil
}

func (a *App) createInvitation(c *Ctx) error {
	body, err := c.Wrapped("invitation")
	if err != nil {
		return err
	}
	email := strings.ToLower(strings.TrimSpace(str(body["email_address"])))
	role := str(body["role"])
	if role == "" {
		role = "member"
	}
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	var inv *invitation
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		bad := apierr.Invalid{}
		switch {
		case email == "":
			bad.Add("email_address", "can't be blank")
		case !validEmail(email):
			bad.Add("email_address", "is invalid")
		default:
			if n, err := store.Count(ctx, t, `SELECT count(*) FROM invitations WHERE email_address = ? AND accepted_at IS NULL`, email); err != nil {
				return err
			} else if n > 0 {
				bad.Add("email_address", "already has an invitation")
			}
			if n, err := store.Count(ctx, t, `SELECT count(*) FROM users WHERE email_address = ?`, email); err != nil {
				return err
			} else if n > 0 {
				bad.Add("email_address", "is already on the team")
			}
		}
		if !slices.Contains(Roles, role) {
			bad.Add("role", "is not included in the list")
		}
		if err := fullInvalid(bad); err != nil {
			return err
		}
		now := store.Now()
		id, err := t.Insert(`INSERT INTO invitations (email_address, role, invited_by_id, sent_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			email, role, me.ID, now.DB(), now.DB(), now.DB())
		if err != nil {
			return err
		}
		inv = &invitation{ID: id, Email: email, Role: role, InvitedByID: me.ID, InvitedBy: me.DisplayName(), SentAt: now}
		return t.deliverInvitation(inv)
	})
	if err != nil {
		return err
	}
	c.Summary = "Invitation sent to " + email + "."
	return c.JSON(201, O("summary", c.Summary, "invitation", inv.json()))
}

func (a *App) resendInvitation(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	var inv *invitation
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if inv, err = a.pendingInvitation(ctx, t, `i.id = ?`, id); err != nil {
			return err
		}
		inv.SentAt = store.Now()
		if err := t.update("invitations", inv.ID, map[string]any{"sent_at": inv.SentAt.DB()}); err != nil {
			return err
		}
		return t.deliverInvitation(inv)
	})
	if err != nil {
		return err
	}
	c.Summary = "Sent a new link to " + inv.Email + "."
	return c.JSON(200, O("summary", c.Summary, "invitation", inv.json()))
}

func (a *App) withdrawInvitation(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	var inv *invitation
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if inv, err = a.pendingInvitation(ctx, t, `i.id = ?`, id); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM account_tokens WHERE purpose = ? AND new_email = ?`, purposeInvitation, inv.Email); err != nil {
			return err
		}
		_, err := t.Exec(`DELETE FROM invitations WHERE id = ?`, inv.ID)
		return err
	})
	if err != nil {
		return err
	}
	c.Summary = "Invitation to " + inv.Email + " withdrawn."
	return c.JSON(200, O("summary", c.Summary))
}

// invitationForLink is the pending invitation an emailed link stands for.
func (a *App) invitationForLink(c *Ctx) (*accountToken, *invitation, error) {
	ctx := c.R.Context()
	gone := &apierr.Error{Status: 404, Code: "expired", Message: "That invitation has expired or was already used. Ask for a new one."}
	tok, err := a.accountTokenFor(ctx, a.store, purposeInvitation, c.Param("token"))
	if err != nil || tok == nil {
		return nil, nil, firstErr(err, gone)
	}
	inv, err := a.pendingInvitation(ctx, a.store, `i.email_address = ?`, tok.NewEmail)
	if isNotFound(err) || (err == nil && inv.expired()) {
		return nil, nil, gone
	}
	return tok, inv, err
}

func (a *App) showInvitationLink(c *Ctx) error {
	_, inv, err := a.invitationForLink(c)
	if err != nil {
		return err
	}
	return c.JSON(200, O("email_address", inv.Email, "role", inv.Role, "invited_by", userRef(inv.InvitedByID, inv.InvitedBy)))
}

func (a *App) acceptInvitation(c *Ctx) error {
	if !allowed(a.signIns, "ip:"+c.ip()) {
		return tryLater()
	}
	tok, inv, err := a.invitationForLink(c)
	if err != nil {
		return err
	}
	body, err := c.Wrapped("user")
	if err != nil {
		return err
	}
	name, password := strings.TrimSpace(str(body["name"])), str(body["password"])
	ctx := c.R.Context()
	var id int64
	err = a.write(ctx, nil, func(t *Tx) error {
		bad := apierr.Invalid{}
		if name == "" {
			bad.Add("name", "can't be blank")
		}
		if err := checkNewPassword(password, nil); err != nil {
			e, _ := err.(*apierr.Error)
			for _, m := range e.Errors["password"] {
				bad.Add("password", m)
			}
		}
		if n, err := store.Count(ctx, t, `SELECT count(*) FROM users WHERE email_address = ?`, inv.Email); err != nil {
			return err
		} else if n > 0 {
			bad.Add("email_address", "has already been taken")
		}
		if err := fullInvalid(bad); err != nil {
			return err
		}
		if ok, err := t.useAccountToken(tok.ID); err != nil || !ok {
			return firstErr(err, apierr.Conflict("That invitation was just used."))
		}
		digest, err := secret.HashPassword(password)
		if err != nil {
			return err
		}
		now := store.Now().DB()
		if id, err = t.Insert(`INSERT INTO users (email_address, password_digest, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			inv.Email, digest, name, inv.Role, now, now); err != nil {
			return err
		}
		return t.update("invitations", inv.ID, map[string]any{"accepted_at": now, "user_id": id})
	})
	if err != nil {
		return err
	}
	if err := a.signOut(c); err != nil {
		return err
	}
	user, err := a.loadUser(ctx, id)
	if err != nil {
		return err
	}
	c.Summary = "Welcome to Runwell, " + user.DisplayName() + "."
	return a.finishSignIn(c, user, "invitation")
}
