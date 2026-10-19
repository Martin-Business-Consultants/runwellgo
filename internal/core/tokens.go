package core

import (
	"context"
	"crypto/rand"
	"math/big"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Access tokens: a bearer token acting as one person, with exactly that
// person's role. The person is on staff (user_id), or a client's contact
// (contact_id), whose token reaches only their portal. Personal tokens
// don't expire until revoked; OAuth tokens expire hourly and are refreshed;
// an assistant's last one call. Only SHA-256 digests are stored, so a token
// is shown once, when it's made. A paused token is refused until it's
// resumed; every call made with one is logged (agent_calls).

// TokenKinds are a token's kinds.
var TokenKinds = []string{"personal", "oauth", "assistant"}

const (
	assistantLifetime = 10 * time.Minute
	oauthLifetime     = time.Hour
	refreshLifetime   = 90 * 24 * time.Hour
)

// AccessToken is one token's row.
type AccessToken struct {
	ID            int64
	UserID        int64
	ContactID     int64
	Name          string
	Kind          string
	ReadOnly      bool
	OauthClientID int64
	ExpiresAt     store.Time
	LastUsedAt    store.Time
	PausedAt      store.Time
	RevokedAt     store.Time
	CreatedAt     store.Time

	// Plaintext and RefreshPlaintext are set once, on a token just issued.
	Plaintext        string
	RefreshPlaintext string
}

const tokenColumns = `id, COALESCE(user_id, 0), COALESCE(contact_id, 0), name, kind, read_only, COALESCE(oauth_client_id, 0), expires_at, last_used_at, paused_at, revoked_at, created_at` //nolint:gosec // column names, not a credential

func scanToken(r interface{ Scan(...any) error }) (*AccessToken, error) {
	var t AccessToken
	err := r.Scan(&t.ID, &t.UserID, &t.ContactID, &t.Name, &t.Kind, &t.ReadOnly, &t.OauthClientID, &t.ExpiresAt, &t.LastUsedAt, &t.PausedAt, &t.RevokedAt, &t.CreatedAt)
	return &t, err
}

// liveTokens is the WHERE for tokens not revoked and not expired.
const liveTokens = `revoked_at IS NULL AND (expires_at IS NULL OR expires_at > ?)` //nolint:gosec // a WHERE clause, not a credential

// base58 is Rails' SecureRandom.base58 alphabet.
const base58 = "123456789ABCDEFGHJKLMNPQRSTUVWXYZabcdefghijkmnopqrstuvwxyz"

// newTokenPlaintext is a token as Rails made them: rw_ and 40 base58
// characters.
func newTokenPlaintext() string {
	b := make([]byte, 40)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(base58))))
		b[i] = base58[n.Int64()]
	}
	return "rw_" + string(b)
}

// issueToken makes a token for a person on staff (userID) or a contact
// (contactID); its plaintext is readable once, on the answer.
func (t *Tx) issueToken(userID, contactID int64, name, kind string, oauthClientID int64, readOnly bool) (*AccessToken, error) {
	tok := &AccessToken{UserID: userID, ContactID: contactID, Name: name, Kind: kind, OauthClientID: oauthClientID, ReadOnly: readOnly, Plaintext: newTokenPlaintext()}
	now := store.Now()
	var expires, refreshDigest, refreshExpires, user, contact, client any
	switch kind {
	case "assistant":
		expires = store.TimeOf(time.Now().Add(assistantLifetime)).DB()
	case "oauth":
		tok.RefreshPlaintext = newTokenPlaintext()
		expires = store.TimeOf(time.Now().Add(oauthLifetime)).DB()
		refreshDigest = secret.Digest(tok.RefreshPlaintext)
		refreshExpires = store.TimeOf(time.Now().Add(refreshLifetime)).DB()
	}
	if userID != 0 {
		user = userID
	}
	if contactID != 0 {
		contact = contactID
	}
	if oauthClientID != 0 {
		client = oauthClientID
	}
	id, err := t.Insert(`INSERT INTO access_tokens (user_id, contact_id, name, kind, token_digest, refresh_token_digest, oauth_client_id, expires_at, refresh_expires_at, read_only, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, user, contact, name, kind, secret.Digest(tok.Plaintext), refreshDigest, client, expires, refreshExpires, readOnly, now.DB(), now.DB())
	if err != nil {
		return nil, err
	}
	tok.ID, tok.CreatedAt = id, now
	return tok, nil
}

// tokenByPlain is the live token a bearer string is (paused or not); nil
// when there's none.
func (a *App) tokenByPlain(ctx context.Context, plain string) (*AccessToken, error) {
	if plain == "" {
		return nil, nil
	}
	tok, err := scanToken(a.store.QueryRowContext(ctx, `SELECT `+tokenColumns+` FROM access_tokens WHERE token_digest = ? AND `+liveTokens, secret.Digest(plain), store.Now().DB()))
	if isNoRows(err) {
		return nil, nil
	}
	return tok, err
}

// setTokenPaused pauses a token (every call answers "paused") or resumes it.
func (t *Tx) setTokenPaused(id int64, paused bool) error {
	var at any
	if paused {
		at = store.Now().DB()
	}
	_, err := t.Exec(`UPDATE access_tokens SET paused_at = ?, updated_at = ? WHERE id = ?`, at, store.Now().DB(), id)
	return err
}

// revokeToken stops a token for good.
func (t *Tx) revokeToken(id int64) error {
	_, err := t.Exec(`UPDATE access_tokens SET revoked_at = ?, updated_at = ? WHERE id = ? AND revoked_at IS NULL`, store.Now().DB(), store.Now().DB(), id)
	return err
}

// tokenScope is what an OAuth answer says the token may do.
func tokenScope(readOnly, contact bool) string {
	s := "runwell"
	if readOnly {
		s = "runwell:read"
	}
	if contact {
		s += " runwell:portal"
	}
	return s
}

// refreshToken trades a refresh token for a new pair, revoking the old one
// (it stays paused if it was); nil when it isn't a live refresh token of
// this client's, or its holder can't act any more.
func (a *App) refreshToken(ctx context.Context, plain string, clientID int64) (*AccessToken, error) {
	var fresh *AccessToken
	err := a.write(ctx, nil, func(t *Tx) error {
		old, err := scanToken(t.QueryRow(`SELECT `+tokenColumns+` FROM access_tokens WHERE refresh_token_digest = ? AND oauth_client_id = ? AND revoked_at IS NULL AND refresh_expires_at > ?`,
			secret.Digest(plain), clientID, store.Now().DB()))
		if isNoRows(err) {
			return nil
		} else if err != nil {
			return err
		}
		if ok, err := a.holderActive(ctx, old.UserID, old.ContactID); err != nil || !ok {
			return err
		}
		if err := t.revokeToken(old.ID); err != nil {
			return err
		}
		if fresh, err = t.issueToken(old.UserID, old.ContactID, old.Name, "oauth", old.OauthClientID, old.ReadOnly); err != nil {
			return err
		}
		if old.PausedAt.Valid {
			_, err = t.Exec(`UPDATE access_tokens SET paused_at = ? WHERE id = ?`, old.PausedAt.DB(), fresh.ID)
		}
		return err
	})
	return fresh, err
}

// holderActive is whether a token's holder can still use it: an active
// person on staff, or a contact who still has portal access.
func (a *App) holderActive(ctx context.Context, userID, contactID int64) (bool, error) {
	if userID != 0 {
		u, err := a.loadUser(ctx, userID)
		return err == nil && u != nil && u.Active(), err
	}
	ct, err := a.loadContact(ctx, contactID)
	return err == nil && ct != nil && ct.PortalAccess && ct.Email != "", err
}
