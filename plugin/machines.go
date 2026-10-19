package plugin

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// "machines": a plugin that works with machines rather than people (a
// runner that does work on a server its owner controls) issues each one a
// token of its own. The core keeps only the token's digest, in
// plugin_machine_tokens under the plugin's key, and authenticates a route
// marked Machine: true against those tokens alone: Authorization: Bearer
// rwm_…, no session, no CSRF, no person. The route sees req.Machine (the
// token's id and label), never a Caller. A plugin can't see or use another
// plugin's tokens, a person's token never reaches a machine route, and a
// machine token never reaches anything else. Removing the plugin revokes
// its tokens.

// MachinePrefix starts every machine token, so one is told from a
// person's at a glance (and by a secret scanner).
const MachinePrefix = "rwm_"

// Machine is the machine calling a machine route: its token's id and the
// label the plugin gave it.
type Machine struct {
	TokenID int64  `json:"token_id"`
	Label   string `json:"label"`
}

// MachineToken is one of the plugin's tokens, as Tokens lists them (never
// the token itself: only its digest is kept).
type MachineToken struct {
	ID         int64
	Label      string
	CreatedAt  time.Time
	LastUsedAt time.Time // zero when never used
	Revoked    bool
}

// IssueToken makes a token for a machine (machines), labelled for the
// plugin's own use ("runner 4"): the token, shown once (only its digest is
// kept), and its id, to revoke it by.
func (a *API) IssueToken(ctx context.Context, label string) (string, int64, error) {
	if err := a.need("machines", a.grants.Machines); err != nil {
		return "", 0, err
	}
	label = strings.TrimSpace(label)
	if label == "" {
		return "", 0, errors.New("a machine token needs a label")
	}
	plain := MachinePrefix + secret.Token(32)
	now := store.Now().DB()
	res, err := a.h.db.ExecContext(ctx, `INSERT INTO plugin_machine_tokens (plugin_key, label, token_digest, created_at) VALUES (?,?,?,?)`,
		a.key, truncate(label, 200), secret.Digest(plain), now)
	if err != nil {
		return "", 0, err
	}
	id, err := res.LastInsertId()
	return plain, id, err
}

// RevokeToken stops one of the plugin's tokens working, at once (machines).
func (a *API) RevokeToken(ctx context.Context, id int64) error {
	if err := a.need("machines", a.grants.Machines); err != nil {
		return err
	}
	_, err := a.h.db.ExecContext(ctx, `UPDATE plugin_machine_tokens SET revoked_at = ? WHERE plugin_key = ? AND id = ? AND revoked_at IS NULL`,
		store.Now().DB(), a.key, id)
	return err
}

// Tokens are the plugin's machine tokens, newest first (machines).
func (a *API) Tokens(ctx context.Context) ([]MachineToken, error) {
	if err := a.need("machines", a.grants.Machines); err != nil {
		return nil, err
	}
	rows, err := a.h.db.QueryContext(ctx, `SELECT id, label, created_at, last_used_at, revoked_at IS NOT NULL FROM plugin_machine_tokens WHERE plugin_key = ? ORDER BY id DESC`, a.key)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sql.Rows) (MachineToken, error) {
		var t MachineToken
		var created, used store.Time
		err := r.Scan(&t.ID, &t.Label, &created, &used, &t.Revoked)
		t.CreatedAt, t.LastUsedAt = created.Time, used.Time
		return t, err
	})
}

// MachineFor is the machine holding plain, one of key's live tokens; nil
// when it isn't one (another plugin's, a person's, revoked, or made up).
// The core calls it for a machine route, while the plugin is on and
// granted machines.
func (h *Host) MachineFor(ctx context.Context, key, plain string) (*Machine, error) {
	if !strings.HasPrefix(plain, MachinePrefix) {
		return nil, nil
	}
	p := h.Get(key)
	if p == nil || !p.Enabled || !p.Granted.Machines {
		return nil, nil
	}
	m := &Machine{}
	var used store.Time
	err := h.db.QueryRowContext(ctx, `SELECT id, label, last_used_at FROM plugin_machine_tokens WHERE plugin_key = ? AND token_digest = ? AND revoked_at IS NULL`,
		key, secret.Digest(plain)).Scan(&m.TokenID, &m.Label, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	// When it was last used, to the minute: a machine polling every few
	// seconds doesn't write each time.
	if now := time.Now(); used.Before(now.Add(-time.Minute)) {
		_, _ = h.db.ExecContext(ctx, `UPDATE plugin_machine_tokens SET last_used_at = ? WHERE id = ?`, store.Now().DB(), m.TokenID)
	}
	return m, nil
}

// revokeMachines revokes every token a plugin issued (it's being removed).
func (h *Host) revokeMachines(ctx context.Context, key string) error {
	_, err := h.db.ExecContext(ctx, `UPDATE plugin_machine_tokens SET revoked_at = ? WHERE plugin_key = ? AND revoked_at IS NULL`, store.Now().DB(), key)
	return err
}
