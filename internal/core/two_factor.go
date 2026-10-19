package core

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha1" //nolint:gosec // TOTP is HMAC-SHA1 (RFC 6238), as authenticator apps read it
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base32"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Two-factor sign-in: a code from an authenticator app (TOTP, 30 seconds)
// after the first step (a password, an emailed link, a provider), or one of
// ten single-use recovery codes when the phone is gone. The secret is
// sealed; recovery codes are kept only as digests and shown once. A code
// can't be used twice (otp_last_used_at), and one either side of now is
// accepted for a phone whose clock drifts. The same secrets and digests as
// the Rails core's User::TwoFactor, so a Rails person's keeps working.

func init() {
	routeSets = append(routeSets, (*App).twoFactorRoutes)
	twoFactorStep = (*App).twoFactorChallenge
	routeGate = requireTwoFactorSetup
}

const (
	otpIssuer          = "Runwell"
	recoveryCodeCount  = 10
	twoFactorExpiresIn = 5 * time.Minute
	twoFactorTries     = 5
)

func (a *App) twoFactorRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/session/two_factor", a.createSessionTwoFactor, open)
	a.handle(mux, "GET /api/settings/two_factor/new", a.newTwoFactor)
	a.handle(mux, "POST /api/settings/two_factor", a.createTwoFactor)
	a.handle(mux, "DELETE /api/settings/two_factor", a.destroyTwoFactor)
	a.handle(mux, "POST /api/settings/two_factor/recovery_codes", a.createRecoveryCodes)
	a.handle(mux, "PATCH /api/settings/two_factor_requirement", a.updateTwoFactorRequirement, need("manage_people"))
	a.handle(mux, "DELETE /api/settings/people/{id}/two_factor", a.resetPersonTwoFactor, need("manage_people"))
}

// ---- TOTP ------------------------------------------------------------------

var b32 = base32.StdEncoding.WithPadding(base32.NoPadding)

// newOTPSecret is a new secret as ROTP made them: 20 random bytes, base32.
func newOTPSecret() string { return b32.EncodeToString(secret.RandomBytes(20)) }

// totp is the 6-digit code for a 30-second step.
func totp(key []byte, step int64) string {
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(step)) //nolint:gosec // a step count, never negative
	mac := hmac.New(sha1.New, key)
	mac.Write(msg[:])
	sum := mac.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	n := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", n%1000000)
}

// otpVerify is the step a code was made for (as a time) when it checks
// out: now, or one step either side, and after the last one used.
func otpVerify(secretText, code string, after store.Time) (time.Time, bool) {
	code = strings.Join(strings.Fields(code), "")
	key, err := b32.DecodeString(strings.ToUpper(strings.TrimRight(strings.ReplaceAll(secretText, " ", ""), "=")))
	if err != nil || len(code) != 6 {
		return time.Time{}, false
	}
	now := time.Now().Unix() / 30
	for _, step := range []int64{now - 1, now, now + 1} {
		if after.Valid && step <= after.Unix()/30 {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(totp(key, step)), []byte(code)) == 1 {
			return time.Unix(step*30, 0), true
		}
	}
	return time.Time{}, false
}

// otpProvisioningURI is the otpauth:// address an authenticator app reads.
func (a *App) otpProvisioningURI(secretText, email string) string {
	issuer := otpIssuer
	if s, err := a.settings(context.Background()); err == nil && s.AppHost != "" || a.cfg.BaseURL != "" {
		issuer = otpIssuer + " (" + a.Host() + ")"
	}
	label := url.PathEscape(issuer) + ":" + url.PathEscape(email)
	return "otpauth://totp/" + label + "?secret=" + secretText + "&issuer=" + url.QueryEscape(issuer)
}

// ---- recovery codes ---------------------------------------------------------

const recoveryAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

// newRecoveryCodes are ten codes like "k3x9q-2mfp7".
func newRecoveryCodes() []string {
	codes := make([]string, recoveryCodeCount)
	for i := range codes {
		b := make([]byte, 10)
		for j := range b {
			n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(recoveryAlphabet))))
			b[j] = recoveryAlphabet[n.Int64()]
		}
		codes[i] = string(b[:5]) + "-" + string(b[5:])
	}
	return codes
}

// recoveryDigest is how a code is kept: HMAC-SHA256 of "id:code".
func (a *App) recoveryDigest(userID int64, code string) string {
	mac := hmac.New(sha256.New, a.railsSecretKeyBase())
	fmt.Fprintf(mac, "%d:%s", userID, strings.ReplaceAll(code, "-", ""))
	return hex.EncodeToString(mac.Sum(nil))
}

func (a *App) recoveryDigests(userID int64, codes []string) []string {
	out := make([]string, len(codes))
	for i, c := range codes {
		out[i] = a.recoveryDigest(userID, c)
	}
	return out
}

// ---- a person's two-factor state -------------------------------------------

type twoFactor struct {
	Secret    string // opened
	EnabledAt store.Time
	LastUsed  store.Time
	Codes     []string // digests
}

func (a *App) loadTwoFactor(ctx context.Context, db store.Querier, userID int64) (*twoFactor, error) {
	var tf twoFactor
	var sealed *string
	var codes store.JSON
	err := db.QueryRowContext(ctx, `SELECT otp_secret, otp_enabled_at, otp_last_used_at, otp_recovery_codes FROM users WHERE id = ?`, userID).
		Scan(&sealed, &tf.EnabledAt, &tf.LastUsed, &codes)
	if err != nil {
		return nil, err
	}
	if sealed != nil {
		tf.Secret, _ = a.openSecret(*sealed)
	}
	for _, c := range codes.List() {
		tf.Codes = append(tf.Codes, str(c))
	}
	return &tf, nil
}

// On is whether two-factor sign-in is on.
func (tf *twoFactor) On() bool { return tf.EnabledAt.Valid }

// enableTwoFactor turns it on once a code from the new secret checks out:
// the recovery codes to show once, nil when the code is wrong. Every other
// session of theirs ends.
func (t *Tx) enableTwoFactor(userID int64, secretText, code string, keepSession int64) ([]string, error) {
	at, ok := otpVerify(secretText, code, store.Time{})
	if !ok {
		return nil, nil
	}
	codes := newRecoveryCodes()
	now := store.Now().DB()
	if _, err := t.Exec(`UPDATE users SET otp_secret = ?, otp_enabled_at = ?, otp_last_used_at = ?, otp_recovery_codes = ?, updated_at = ? WHERE id = ?`,
		t.App.secrets.Seal(secretText), now, store.TimeOf(at).DB(), string(store.MustJSON(t.App.recoveryDigests(userID, codes))), now, userID); err != nil {
		return nil, err
	}
	if _, err := t.Exec(`DELETE FROM sessions WHERE user_id = ? AND id != ?`, userID, keepSession); err != nil {
		return nil, err
	}
	return codes, t.Event("User", userID, "user.two_factor_enabled", nil)
}

// disableTwoFactor turns it off: "disabled" by the person, "reset" by an
// owner for them.
func (t *Tx) disableTwoFactor(userID int64) error {
	var enabled store.Time
	if err := t.QueryRow(`SELECT otp_enabled_at FROM users WHERE id = ?`, userID).Scan(&enabled); err != nil {
		return err
	}
	if _, err := t.Exec(`UPDATE users SET otp_secret = NULL, otp_enabled_at = NULL, otp_last_used_at = NULL, otp_recovery_codes = NULL, updated_at = ? WHERE id = ?`,
		store.Now().DB(), userID); err != nil {
		return err
	}
	if !enabled.Valid {
		return nil
	}
	kind := "user.two_factor_reset"
	if t.Actor.UserID() == userID {
		kind = "user.two_factor_disabled"
	}
	return t.Event("User", userID, kind, nil)
}

var sixDigits = regexp.MustCompile(`^\d{6}$`)

// verifySecondFactor is whether code (from the app, or a recovery code,
// used up) checks out.
func (t *Tx) verifySecondFactor(userID int64, code string) (bool, error) {
	tf, err := t.App.loadTwoFactor(t.Ctx, t, userID)
	if err != nil || !tf.On() {
		return false, err
	}
	code = strings.ToLower(strings.NewReplacer(" ", "", "\t", "", "-", "").Replace(code))
	if sixDigits.MatchString(code) {
		if at, ok := otpVerify(tf.Secret, code, tf.LastUsed); ok {
			_, err := t.Exec(`UPDATE users SET otp_last_used_at = ? WHERE id = ?`, store.TimeOf(at).DB(), userID)
			return err == nil, err
		}
	}
	digest := t.App.recoveryDigest(userID, code)
	i := slices.IndexFunc(tf.Codes, func(d string) bool { return subtle.ConstantTimeCompare([]byte(d), []byte(digest)) == 1 })
	if i < 0 {
		return false, nil
	}
	left := slices.Delete(slices.Clone(tf.Codes), i, i+1)
	_, err = t.Exec(`UPDATE users SET otp_recovery_codes = ? WHERE id = ?`, string(store.MustJSON(left)), userID)
	return err == nil, err
}

// ---- signing in, second step ------------------------------------------------

// twoFactorChallenge answers a sign-in whose person has two-factor on with
// a challenge, good for five minutes and five tries.
func (a *App) twoFactorChallenge(c *Ctx, user *User, method string) (bool, error) {
	ctx := c.R.Context()
	tf, err := a.loadTwoFactor(ctx, a.store, user.ID)
	if err != nil || !tf.On() {
		return false, err
	}
	var challenge string
	err = a.write(ctx, nil, func(t *Tx) error {
		if err := t.dropAccountTokens(purposeTwoFactor, user.ID); err != nil {
			return err
		}
		challenge, err = t.issueAccountToken(purposeTwoFactor, user.ID, method, twoFactorExpiresIn)
		return err
	})
	if err != nil {
		return true, err
	}
	c.Summary = "One more step: the code from the authenticator app."
	return true, c.JSON(200, O("two_factor_required", true, "challenge", challenge))
}

func (a *App) createSessionTwoFactor(c *Ctx) error {
	if !allowed(a.signIns, "ip:"+c.ip()) {
		return &apierr.Error{Status: 429, Code: "rate_limited", Message: "Try again later."}
	}
	var body struct {
		Challenge string `json:"challenge"`
		Code      string `json:"code"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	tok, err := a.accountTokenFor(ctx, a.store, purposeTwoFactor, body.Challenge)
	if err != nil {
		return err
	}
	var user *User
	if tok != nil {
		if user, err = a.loadUser(ctx, tok.UserID); err != nil {
			return err
		}
	}
	if tok == nil || user == nil || !user.Active() {
		return &apierr.Error{Status: 422, Code: "expired", Message: "Sign in again: that took too long."}
	}
	var ok bool
	err = a.write(ctx, &Actor{User: user}, func(t *Tx) error {
		if ok, err = t.verifySecondFactor(user.ID, body.Code); err != nil || !ok {
			return err
		}
		_, err = t.useAccountToken(tok.ID)
		return err
	})
	if err != nil {
		return err
	}
	if !ok {
		a.failedSignIn(ctx, c, user.ID, "code")
		tries, err := store.Count(ctx, a.store, `SELECT count(*) FROM events WHERE subject_type = 'User' AND subject_id = ? AND kind = 'user.sign_in_failed'
			AND json_extract(payload, '$.reason') = 'code' AND occurred_at >= ?`, user.ID, tok.CreatedAt.DB())
		if err != nil {
			return err
		}
		if tries >= twoFactorTries {
			if err := a.write(ctx, nil, func(t *Tx) error { return t.dropAccountTokens(purposeTwoFactor, user.ID) }); err != nil {
				return err
			}
			return &apierr.Error{Status: 422, Code: "expired", Message: "Too many wrong codes. Sign in again."}
		}
		return &apierr.Error{Status: 422, Code: "invalid_code", Message: "That code didn’t work. Try the newest one from your app, or a recovery code."}
	}
	method := tok.NewEmail
	if method == "" {
		method = "password"
	}
	tf, err := a.loadTwoFactor(ctx, a.store, user.ID)
	if err != nil {
		return err
	}
	if n := len(tf.Codes); n < 3 {
		c.Summary = fmt.Sprintf("%d recovery codes left. Make new ones in Settings › Your account.", n)
	}
	return a.finishSignIn(c, user, method)
}

// requireTwoFactorSetup: when the install requires two-factor sign-in and
// the person signed in with a browser hasn't set it up, every staff route
// but setting it up (and signing out) is refused until they do. Tokens
// already made keep working.
func requireTwoFactorSetup(c *Ctx, pattern string) error {
	if c.Actor == nil || c.Actor.User == nil || c.Actor.Session == nil || c.Actor.User.IsAgent() {
		return nil
	}
	switch pattern {
	case "GET /api/session", "DELETE /api/session", "GET /api/me", "GET /api/nav", "GET /api/settings/two_factor/new",
		"POST /api/settings/two_factor", "PATCH /api/settings/two_factor_requirement", "GET /api/time_zones":
		return nil
	}
	ctx := c.R.Context()
	s, err := c.App.settings(ctx)
	if err != nil || !s.RequireTwoFactor {
		return err
	}
	var enabled store.Time
	if err := c.App.store.QueryRowContext(ctx, `SELECT otp_enabled_at FROM users WHERE id = ?`, c.Actor.User.ID).Scan(&enabled); err != nil || enabled.Valid {
		return err
	}
	return &apierr.Error{Status: 403, Code: "two_factor_setup_required", Message: "Set up two-factor sign-in to carry on: this install requires it."}
}

// ---- Settings › Your account › Two-factor sign-in ----------------------------

// pendingSecret seals a secret being set up to the person, so the form can
// carry it back with the first code (the Rails core kept it in the session).
func (a *App) pendingSecret(userID int64, secretText string) string {
	return a.secrets.Seal(fmt.Sprintf("otp:%d:%s", userID, secretText))
}

func (a *App) openPendingSecret(userID int64, sealed string) (string, bool) {
	plain, ok := a.secrets.Open(sealed)
	prefix := fmt.Sprintf("otp:%d:", userID)
	if !ok || !strings.HasPrefix(plain, prefix) {
		return "", false
	}
	return strings.TrimPrefix(plain, prefix), true
}

func (a *App) newTwoFactor(c *Ctx) error {
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	tf, err := a.loadTwoFactor(ctx, a.store, me.ID)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	if tf.On() {
		c.Summary = "Two-factor sign-in is already on."
		return c.JSON(200, O("summary", c.Summary, "on", true))
	}
	secretText := newOTPSecret()
	uri := a.otpProvisioningURI(secretText, me.Email)
	qr, err := qrEncode(uri)
	if err != nil {
		return err
	}
	var grouped []string
	for i := 0; i < len(secretText); i += 4 {
		grouped = append(grouped, secretText[i:min(i+4, len(secretText))])
	}
	return c.JSON(200, O("on", false, "secret", secretText, "secret_grouped", strings.Join(grouped, " "), "uri", uri,
		"qr_svg", qr.svg(), "pending", a.pendingSecret(me.ID, secretText), "required", s.RequireTwoFactor, "can_stop_requiring", c.Can("manage_people")))
}

func (a *App) createTwoFactor(c *Ctx) error {
	if c.Actor.Agent() {
		return forbidden()
	}
	var body struct {
		Pending string `json:"pending"`
		Code    string `json:"code"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	me := c.Actor.User.Person()
	secretText, ok := a.openPendingSecret(me.ID, body.Pending)
	if !ok {
		return apierr.BadRequest("Start setting up two-factor sign-in again.")
	}
	var codes []string
	keep := int64(0)
	if c.Actor.Session != nil {
		keep = c.Actor.Session.ID
	}
	err := a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		var err error
		codes, err = t.enableTwoFactor(me.ID, secretText, body.Code, keep)
		return err
	})
	if err != nil {
		return err
	}
	if codes == nil {
		return &apierr.Error{Status: 422, Code: "invalid_code", Message: "That code didn’t match. Check your phone’s clock and try the newest code."}
	}
	c.Summary = "Two-factor sign-in is on."
	return c.JSON(200, O("summary", c.Summary, "recovery_codes", codes))
}

// passwordFrom checks the person's own password, for what asks for it.
func (a *App) checkOwnPassword(c *Ctx, password string) error {
	var digest string
	if err := a.store.QueryRowContext(c.R.Context(), `SELECT password_digest FROM users WHERE id = ?`, c.Actor.User.Person().ID).Scan(&digest); err != nil {
		return err
	}
	if !secret.CheckPassword(digest, password) {
		return &apierr.Error{Status: 422, Code: "wrong_password", Message: "That password isn’t right."}
	}
	return nil
}

func (a *App) destroyTwoFactor(c *Ctx) error {
	if c.Actor.Agent() {
		return forbidden()
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	if s.RequireTwoFactor {
		return &apierr.Error{Status: 422, Code: "required", Message: "This install requires two-factor sign-in, so it stays on."}
	}
	if err := a.checkOwnPassword(c, body.Password); err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.disableTwoFactor(c.Actor.User.Person().ID) }); err != nil {
		return err
	}
	c.Summary = "Two-factor sign-in is off."
	return c.JSON(200, O("summary", c.Summary))
}

func (a *App) createRecoveryCodes(c *Ctx) error {
	if c.Actor.Agent() {
		return forbidden()
	}
	var body struct {
		Password string `json:"password"`
	}
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	tf, err := a.loadTwoFactor(ctx, a.store, me.ID)
	if err != nil {
		return err
	}
	if !tf.On() {
		return &apierr.Error{Status: 422, Code: "off", Message: "Two-factor sign-in is off."}
	}
	if err := a.checkOwnPassword(c, body.Password); err != nil {
		return err
	}
	codes := newRecoveryCodes()
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if _, err := t.Exec(`UPDATE users SET otp_recovery_codes = ?, updated_at = ? WHERE id = ?`, string(store.MustJSON(a.recoveryDigests(me.ID, codes))), store.Now().DB(), me.ID); err != nil {
			return err
		}
		return t.Event("User", me.ID, "user.recovery_codes_renewed", nil)
	})
	if err != nil {
		return err
	}
	c.Summary = "New recovery codes. The old ones no longer work."
	return c.JSON(200, O("summary", c.Summary, "recovery_codes", codes))
}

func (a *App) updateTwoFactorRequirement(c *Ctx) error {
	if c.Actor.Agent() {
		return forbidden()
	}
	body, err := c.Wrapped("setting")
	if err != nil {
		return err
	}
	required := boolOf(body["require_two_factor"])
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.update("settings", s.ID, map[string]any{"require_two_factor": required}); err != nil {
			return err
		}
		return t.Event("Setting", s.ID, "settings.two_factor_required", map[string]any{"required": required})
	})
	if err != nil {
		return err
	}
	c.Summary = "Two-factor sign-in is optional again."
	if required {
		c.Summary = "Two-factor sign-in is now required. Anyone without it sets it up on their next page."
	}
	return c.JSON(200, O("summary", c.Summary, "two_factor_required", required))
}

func (a *App) resetPersonTwoFactor(c *Ctx) error {
	if c.Actor.Agent() {
		return forbidden()
	}
	ctx := c.R.Context()
	person, err := a.person(ctx, c)
	if err != nil {
		return err
	}
	if person.ID == c.Actor.User.Person().ID {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Turn your own off in Settings › Your account."}
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.disableTwoFactor(person.ID); err != nil {
			return err
		}
		_, err := t.Exec(`DELETE FROM sessions WHERE user_id = ?`, person.ID)
		return err
	})
	if err != nil {
		return err
	}
	c.Summary = person.DisplayName() + "’s two-factor sign-in is reset. They’ll set it up again."
	return c.JSON(200, O("summary", c.Summary))
}
