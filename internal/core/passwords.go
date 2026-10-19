package core

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
)

// Forgot your password: the emailed link (good once, for 15 minutes) opens
// a form for a new one, which signs the person out everywhere. Asking
// always answers the same, so nobody learns who has an account.

func init() { routeSets = append(routeSets, (*App).passwordRoutes) }

const passwordResetExpiresIn = 15 * time.Minute

func (a *App) passwordRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/passwords", a.createPasswordReset, open)
	a.handle(mux, "GET /api/passwords/{token}", a.showPasswordReset, open)
	a.handle(mux, "PUT /api/passwords/{token}", a.updatePasswordReset, open)
	a.handle(mux, "PATCH /api/passwords/{token}", a.updatePasswordReset, open)
}

func tryLater() error {
	return &apierr.Error{Status: 429, Code: "rate_limited", Message: "Try again later."}
}

// activePersonByEmail is the active person (not an agent) with email; nil
// for none.
func (a *App) activePersonByEmail(ctx context.Context, email string) (*User, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	u, err := scanUser(a.store.QueryRowContext(ctx, `SELECT `+userColumns+` FROM users WHERE email_address = ? AND agent_owner_id IS NULL AND deactivated_at IS NULL`, email))
	if isNoRows(err) {
		return nil, nil
	}
	return u, err
}

// sendPasswordReset mails p a link to choose a new password once the write
// commits; an earlier link stops working.
func (t *Tx) sendPasswordReset(p *User) error {
	if err := t.dropAccountTokens(purposePasswordReset, p.ID); err != nil {
		return err
	}
	token, err := t.issueAccountToken(purposePasswordReset, p.ID, "", passwordResetExpiresIn)
	if err != nil {
		return err
	}
	a, email := t.App, p.Email
	t.AfterCommit(func(ctx context.Context) {
		link := a.BaseURL() + "/passwords/" + token
		reason := "Someone asked to reset the password for this address. If it wasn’t you, ignore this email: your password stays as it is."
		a.sendAccountMail(ctx, accountMail{
			To: email, Subject: "Reset your password",
			Preheader: "Choose a new password. The link expires in 15 minutes.",
			Heading:   "Reset your password", Paragraphs: []string{"Choose a new password for " + email + "."},
			Button: "Choose a new password", URL: link, Small: "The link expires in 15 minutes.", Reason: reason,
			Text: "Choose a new password for " + email + ":\n" + link + "\n\nThe link expires in 15 minutes.",
		})
	})
	return nil
}

func (a *App) createPasswordReset(c *Ctx) error {
	if !allowed(a.mailed, "password:"+c.ip()) {
		return tryLater()
	}
	var body struct {
		Email string `json:"email_address"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	u, err := a.activePersonByEmail(ctx, body.Email)
	if err != nil {
		return err
	}
	if u != nil {
		if err := a.write(ctx, nil, func(t *Tx) error { return t.sendPasswordReset(u) }); err != nil {
			return err
		}
	}
	c.Summary = "If that address has an account, a link to choose a new password is on its way."
	return c.JSON(200, O("summary", c.Summary))
}

func invalidResetLink() error {
	return &apierr.Error{Status: 404, Code: "expired", Message: "That link has expired or was already used. Ask for a new one."}
}

// resetUser is the active person a reset link is for.
func (a *App) resetUser(c *Ctx) (*accountToken, *User, error) {
	ctx := c.R.Context()
	tok, err := a.accountTokenFor(ctx, a.store, purposePasswordReset, c.Param("token"))
	if err != nil {
		return nil, nil, err
	}
	if tok == nil {
		return nil, nil, invalidResetLink()
	}
	u, err := a.loadUser(ctx, tok.UserID)
	if err != nil {
		return nil, nil, err
	}
	if u == nil || !u.Active() || u.IsAgent() {
		return nil, nil, invalidResetLink()
	}
	return tok, u, nil
}

func (a *App) showPasswordReset(c *Ctx) error {
	_, u, err := a.resetUser(c)
	if err != nil {
		return err
	}
	return c.JSON(200, O("email_address", u.Email))
}

func (a *App) updatePasswordReset(c *Ctx) error {
	tok, u, err := a.resetUser(c)
	if err != nil {
		return err
	}
	var body struct {
		Password     string  `json:"password"`
		Confirmation *string `json:"password_confirmation"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	if body.Confirmation != nil && *body.Confirmation != body.Password {
		return &apierr.Error{Status: 422, Code: "invalid", Message: "Passwords did not match."}
	}
	if err := checkNewPassword(body.Password, nil); err != nil {
		return err
	}
	err = a.write(c.R.Context(), &Actor{User: u}, func(t *Tx) error {
		if ok, err := t.useAccountToken(tok.ID); err != nil || !ok {
			if err == nil {
				err = invalidResetLink()
			}
			return err
		}
		return t.setPassword(u.ID, body.Password)
	})
	if err != nil {
		return err
	}
	c.Summary = "Password changed. Sign in with it."
	return c.JSON(200, O("summary", c.Summary))
}
