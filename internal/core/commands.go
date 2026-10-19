package core

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// What the one-off commands do (runwell user …), outside any request.

// CreateUser adds a person on staff with a role; the first is always an
// owner.
func (a *App) CreateUser(ctx context.Context, name, email, password, role string) (int64, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !validEmail(email) {
		return 0, fmt.Errorf("%q isn't an email address", email)
	}
	if !slices.Contains(Roles, role) {
		return 0, fmt.Errorf("the role is owner, manager or member, not %q", role)
	}
	if len(password) < 8 {
		return 0, fmt.Errorf("the password needs at least 8 characters")
	}
	if name == "" {
		name, _, _ = strings.Cut(email, "@")
	}
	digest, err := secret.HashPassword(password)
	if err != nil {
		return 0, err
	}
	var id int64
	err = a.write(ctx, nil, func(t *Tx) error {
		n, err := store.Count(ctx, t, `SELECT count(*) FROM users`)
		if err != nil {
			return err
		}
		if n == 0 {
			role = "owner"
		}
		now := store.Now().DB()
		id, err = t.Insert(`INSERT INTO users (email_address, password_digest, name, role, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, email, digest, name, role, now, now)
		if err != nil {
			return fmt.Errorf("couldn't add %s (is the address taken?): %w", email, err)
		}
		return t.Event("User", id, "user.created", map[string]any{"role": role, "via": "command line"})
	})
	return id, err
}

// SetPassword gives someone locked out a new password and signs them out
// everywhere.
func (a *App) SetPassword(ctx context.Context, email, password string) error {
	if len(password) < 8 {
		return fmt.Errorf("the password needs at least 8 characters")
	}
	digest, err := secret.HashPassword(password)
	if err != nil {
		return err
	}
	return a.write(ctx, nil, func(t *Tx) error {
		var id int64
		if err := t.QueryRow(`SELECT id FROM users WHERE email_address = ? AND agent_owner_id IS NULL`, strings.ToLower(strings.TrimSpace(email))).Scan(&id); err != nil {
			return fmt.Errorf("no one signs in as %s", email)
		}
		if _, err := t.Exec(`UPDATE users SET password_digest = ?, updated_at = ? WHERE id = ?`, digest, store.Now().DB(), id); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
			return err
		}
		return t.Event("User", id, "user.password_changed", map[string]any{"via": "command line"})
	})
}

// DisableTwoFactor turns someone's second factor off.
func (a *App) DisableTwoFactor(ctx context.Context, email string) error {
	return a.write(ctx, nil, func(t *Tx) error {
		var id int64
		if err := t.QueryRow(`SELECT id FROM users WHERE email_address = ? AND agent_owner_id IS NULL`, strings.ToLower(strings.TrimSpace(email))).Scan(&id); err != nil {
			return fmt.Errorf("no one signs in as %s", email)
		}
		if _, err := t.Exec(`UPDATE users SET otp_secret = NULL, otp_enabled_at = NULL, otp_last_used_at = NULL, otp_recovery_codes = NULL, updated_at = ? WHERE id = ?`, store.Now().DB(), id); err != nil {
			return err
		}
		return t.Event("User", id, "user.two_factor_disabled", map[string]any{"via": "command line"})
	})
}
