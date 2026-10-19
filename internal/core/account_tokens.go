package core

import (
	"context"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// One-time links mailed to a person (account_tokens): a password reset, a
// sign-in link, an invitation's, and the half-signed-in step between a
// password and a two-factor code. Only the digest is kept; a link works
// once, until it expires.

const (
	purposePasswordReset = "password_reset"
	purposeSignInLink    = "sign_in_link"
	purposeInvitation    = "invitation"
	purposeTwoFactor     = "two_factor"
)

// accountToken is one live link.
type accountToken struct {
	ID        int64
	UserID    int64
	NewEmail  string // an invitation's address; a two-factor step's method
	CreatedAt store.Time
	ExpiresAt store.Time
}

// issueAccountToken makes a link for purpose and answers its plaintext.
func (t *Tx) issueAccountToken(purpose string, userID int64, newEmail string, ttl time.Duration) (string, error) {
	plain := secret.Token(32)
	now := store.Now()
	var uid any
	if userID != 0 {
		uid = userID
	}
	_, err := t.Exec(`INSERT INTO account_tokens (user_id, purpose, token_digest, new_email, expires_at, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		uid, purpose, secret.Digest(plain), nullable(newEmail), store.TimeOf(now.Add(ttl)).DB(), now.DB())
	return plain, err
}

// accountTokenFor is the live link plain stands for, nil for none (used,
// expired, another purpose, never made).
func (a *App) accountTokenFor(ctx context.Context, db store.Querier, purpose, plain string) (*accountToken, error) {
	if plain == "" {
		return nil, nil
	}
	var tok accountToken
	var uid store.NullInt
	var email *string
	err := db.QueryRowContext(ctx, `SELECT id, user_id, new_email, created_at, expires_at FROM account_tokens
		WHERE token_digest = ? AND purpose = ? AND used_at IS NULL AND expires_at > ?`, secret.Digest(plain), purpose, store.Now().DB()).
		Scan(&tok.ID, &uid, &email, &tok.CreatedAt, &tok.ExpiresAt)
	if isNoRows(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	tok.UserID = uid.Int64
	if email != nil {
		tok.NewEmail = *email
	}
	return &tok, nil
}

// useAccountToken spends a link; false when another request spent it first.
func (t *Tx) useAccountToken(id int64) (bool, error) {
	res, err := t.Exec(`UPDATE account_tokens SET used_at = ? WHERE id = ? AND used_at IS NULL`, store.Now().DB(), id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n == 1, nil
}

// dropAccountTokens ends a person's live links of a purpose (a new one
// replaces them; a new password ends the resets).
func (t *Tx) dropAccountTokens(purpose string, userID int64) error {
	_, err := t.Exec(`DELETE FROM account_tokens WHERE purpose = ? AND user_id = ?`, purpose, userID)
	return err
}
