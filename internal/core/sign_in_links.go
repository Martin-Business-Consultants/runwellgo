package core

import (
	"context"
	"net/http"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Signing in by emailed link, for people who switched it on (Settings ›
// Your account). Asking always answers the same, so nobody learns who has
// an account. The link opens a page with a button rather than signing in
// on the GET, since mail scanners open links before people do; it works
// once, for 15 minutes, and two-factor sign-in still follows.

func init() { routeSets = append(routeSets, (*App).signInLinkRoutes) }

const signInLinkExpiresIn = 15 * time.Minute

func (a *App) signInLinkRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/session/link", a.createSignInLink, open)
	a.handle(mux, "GET /api/session/link/{token}", a.showSignInLink, open)
	a.handle(mux, "POST /api/session/link/{token}", a.useSignInLink, open)
}

// passwordSignInFor is whether user may sign in with a password or a link:
// always, unless the install signs in through providers only (owners keep
// theirs).
func (a *App) passwordSignInFor(ctx context.Context, user *User) bool {
	s, err := a.settings(ctx)
	return err == nil && (!s.ProvidersOnly || user.Person().Role == "owner")
}

func (a *App) passwordless(ctx context.Context, userID int64) bool {
	var on bool
	_ = a.store.QueryRowContext(ctx, `SELECT passwordless FROM users WHERE id = ?`, userID).Scan(&on)
	return on
}

func (a *App) createSignInLink(c *Ctx) error {
	if !allowed(a.mailed, "link:"+c.ip()) {
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
	if u != nil && a.passwordless(ctx, u.ID) && a.passwordSignInFor(ctx, u) {
		err := a.write(ctx, nil, func(t *Tx) error {
			if err := t.dropAccountTokens(purposeSignInLink, u.ID); err != nil {
				return err
			}
			token, err := t.issueAccountToken(purposeSignInLink, u.ID, "", signInLinkExpiresIn)
			if err != nil {
				return err
			}
			email := u.Email
			t.AfterCommit(func(ctx context.Context) {
				link := a.BaseURL() + "/session/link/" + token
				a.sendAccountMail(ctx, accountMail{
					To: email, Subject: "Your sign-in link", Preheader: "Your link to sign in. It works once, for 15 minutes.",
					Heading: "Sign in to " + a.BrandName(ctx), Paragraphs: []string{"Here’s your link to sign in, no password needed."},
					Button: "Sign in", URL: link, Small: "It works once, for 15 minutes.",
					Reason: "Someone asked to sign in with this address. If it wasn’t you, ignore this email: nothing happens.",
					Text:   "Here's your link to sign in, no password needed:\n" + link + "\n\nIt works once, for 15 minutes.",
				})
			})
			return nil
		})
		if err != nil {
			return err
		}
	}
	c.Summary = "If that address can sign in by link, one is on its way. It works for 15 minutes."
	return c.JSON(200, O("summary", c.Summary))
}

func expiredLink() error {
	return &apierr.Error{Status: 404, Code: "expired", Message: "That link has expired or was already used. Ask for a new one."}
}

// linkUser is who a sign-in link is for, while it may still sign them in.
func (a *App) linkUser(c *Ctx) (*accountToken, *User, error) {
	ctx := c.R.Context()
	tok, err := a.accountTokenFor(ctx, a.store, purposeSignInLink, c.Param("token"))
	if err != nil || tok == nil {
		return nil, nil, firstErr(err, expiredLink())
	}
	u, err := a.loadUser(ctx, tok.UserID)
	if err != nil {
		return nil, nil, err
	}
	if u == nil || !u.Active() || u.IsAgent() || !a.passwordless(ctx, u.ID) || !a.passwordSignInFor(ctx, u) {
		return nil, nil, expiredLink()
	}
	return tok, u, nil
}

func firstErr(errs ...error) error {
	for _, e := range errs {
		if e != nil {
			return e
		}
	}
	return nil
}

func (a *App) showSignInLink(c *Ctx) error {
	_, u, err := a.linkUser(c)
	if err != nil {
		return err
	}
	return c.JSON(200, O("name", u.DisplayName(), "email_address", u.Email))
}

func (a *App) useSignInLink(c *Ctx) error {
	tok, u, err := a.linkUser(c)
	if err != nil {
		return err
	}
	err = a.write(c.R.Context(), nil, func(t *Tx) error {
		if ok, err := t.useAccountToken(tok.ID); err != nil || !ok {
			return firstErr(err, expiredLink())
		}
		_, err := t.Exec(`UPDATE users SET link_signed_in_at = ? WHERE id = ?`, store.Now().DB(), u.ID)
		return err
	})
	if err != nil {
		return err
	}
	return a.signInAfterFirstStep(c, u, "link")
}
