package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/outbound"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Ways to sign in other than a password: Google, Microsoft (Entra ID),
// GitHub, or any OpenID Connect provider (single sign-on), with the OAuth 2
// code flow. The owner registers an app with the provider and saves its
// keys in Settings › Sign-in; people connect their own accounts in
// Settings › Your account (identities), or are matched on first use by an
// address the provider vouches for. Nobody joins this way: an account
// still starts with an invitation, and two-factor sign-in still follows.

func init() { routeSets = append(routeSets, (*App).signInProviderRoutes) }

// SignInProviders are the providers, in the order they're offered.
var SignInProviders = []struct {
	Key, Name, Icon, Console string
}{
	{"google_oauth2", "Google", "globe", "https://console.cloud.google.com/apis/credentials"},
	{"entra_id", "Microsoft", "monitor", "https://entra.microsoft.com/#view/Microsoft_AAD_RegisteredApps/ApplicationsListBlade"},
	{"github", "GitHub", "share", "https://github.com/settings/applications/new"},
	{"openid_connect", "Single sign-on", "lock", ""},
}

func providerKnown(key string) bool {
	return slices.ContainsFunc(SignInProviders, func(p struct{ Key, Name, Icon, Console string }) bool { return p.Key == key })
}

func (a *App) signInProviderRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	a.handle(mux, "GET /api/settings/sign_in", a.showSignIn, ms)
	a.handle(mux, "PATCH /api/settings/sign_in_providers/{key}", a.updateSignInProvider, ms)
	a.handle(mux, "PATCH /api/settings/providers_only", a.updateProvidersOnly, ms)
	a.handle(mux, "GET /api/sign_in_providers", a.listUsableProviders, open)
	a.handle(mux, "GET /auth/{provider}", a.startProviderSignIn, open)
	a.handle(mux, "POST /auth/{provider}", a.startProviderSignIn, open)
	a.handle(mux, "GET /auth/{provider}/callback", a.providerCallback, open)
	a.handle(mux, "POST /auth/{provider}/callback", a.providerCallback, open)
}

type signInProvider struct {
	ID           int64
	Key          string
	ClientID     string
	ClientSecret string // as stored
	TenantID     string
	Issuer       string
	Label        string
	Enabled      bool
}

func (p *signInProvider) usable() bool { return p.Enabled && p.ClientID != "" && p.ClientSecret != "" }

func (a *App) loadSignInProvider(ctx context.Context, db store.Querier, key string) (*signInProvider, error) {
	p := signInProvider{Key: key}
	err := db.QueryRowContext(ctx, `SELECT id, COALESCE(client_id, ''), COALESCE(client_secret, ''), COALESCE(tenant_id, ''), COALESCE(issuer, ''), COALESCE(label, ''), enabled
		FROM sign_in_providers WHERE key = ?`, key).Scan(&p.ID, &p.ClientID, &p.ClientSecret, &p.TenantID, &p.Issuer, &p.Label, &p.Enabled)
	if isNoRows(err) {
		return &p, nil
	}
	return &p, err
}

// providerName is a provider in words: single sign-on goes by the name the
// owner gave it (Okta, Acme SSO); the others by their own.
func (a *App) providerName(ctx context.Context, key string) string {
	if key == "openid_connect" {
		var label string
		_ = a.store.QueryRowContext(ctx, `SELECT COALESCE(label, '') FROM sign_in_providers WHERE key = ?`, key).Scan(&label)
		if label != "" {
			return label
		}
	}
	for _, p := range SignInProviders {
		if p.Key == key {
			return p.Name
		}
	}
	return humanize(key)
}

// usableProviders are the keys of the providers switched on with keys, in
// order.
func (a *App) usableProviders(ctx context.Context) ([]string, error) {
	keys, err := store.Strings(ctx, a.store, `SELECT key FROM sign_in_providers WHERE enabled = 1 AND COALESCE(client_id, '') != '' AND client_secret IS NOT NULL AND client_secret != ''`)
	if err != nil {
		return nil, err
	}
	out := []string{}
	for _, p := range SignInProviders {
		if slices.Contains(keys, p.Key) {
			out = append(out, p.Key)
		}
	}
	return out, nil
}

func (a *App) providerNames(ctx context.Context, keys []string) string {
	names := make([]string, len(keys))
	for i, k := range keys {
		names[i] = a.providerName(ctx, k)
	}
	return toSentence(names, "or")
}

// requestBase is where this request reached Runwell: the address set in
// Settings › Address (or APP_URL), else the request's own.
func (a *App) requestBase(c *Ctx) string {
	if s, err := a.settings(c.R.Context()); a.cfg.BaseURL != "" || (err == nil && s.AppHost != "") {
		return a.BaseURL()
	}
	scheme := "http"
	if c.R.TLS != nil || (a.cfg.TrustProxy && c.R.Header.Get("X-Forwarded-Proto") == "https") {
		scheme = "https"
	}
	return scheme + "://" + c.R.Host
}

func (a *App) showSignIn(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	list := []Obj{}
	for _, info := range SignInProviders {
		p, err := a.loadSignInProvider(ctx, a.store, info.Key)
		if err != nil {
			return err
		}
		list = append(list, O("key", info.Key, "name", a.providerName(ctx, info.Key), "console", nullable(info.Console),
			"client_id", p.ClientID, "secret_saved", p.ClientSecret != "", "tenant_id", p.TenantID, "issuer", p.Issuer, "label", p.Label,
			"enabled", p.Enabled, "usable", p.usable(), "callback_url", a.requestBase(c)+"/auth/"+info.Key+"/callback"))
	}
	return c.JSON(200, O("providers", list, "providers_only", s.ProvidersOnly))
}

func (a *App) updateSignInProvider(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	key := c.Param("key")
	if !providerKnown(key) {
		return notFound("sign-in provider")
	}
	body, err := c.Wrapped("sign_in_provider")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	var p *signInProvider
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if p, err = a.loadSignInProvider(ctx, t, key); err != nil {
			return err
		}
		for k, dst := range map[string]*string{"client_id": &p.ClientID, "tenant_id": &p.TenantID, "issuer": &p.Issuer, "label": &p.Label} {
			if has(body, k) {
				*dst = strings.TrimSpace(str(body[k]))
			}
		}
		if s := strings.TrimSpace(str(body["client_secret"])); s != "" {
			p.ClientSecret = a.secrets.Seal(s)
		}
		if has(body, "enabled") {
			p.Enabled = boolOf(body["enabled"])
		}
		bad := apierr.Invalid{}
		if p.Enabled {
			if p.ClientID == "" {
				bad.Add("client_id", "can't be blank")
			}
			if p.ClientSecret == "" {
				bad.Add("client_secret", "can't be blank")
			}
			if key == "openid_connect" {
				if p.Issuer == "" {
					bad.Add("issuer", "can't be blank")
				} else if !strings.HasPrefix(p.Issuer, "https://") {
					bad.Add("issuer", "should be the provider's https:// address")
				}
			}
		}
		if err := fullInvalid(bad); err != nil {
			e, _ := errors.AsType[*apierr.Error](err) // fullInvalid's error is always one
			e.Message = a.providerName(ctx, key) + ": " + e.Message
			return e
		}
		now := store.Now().DB()
		if p.ID == 0 {
			if p.ID, err = t.Insert(`INSERT INTO sign_in_providers (key, client_id, client_secret, tenant_id, issuer, label, enabled, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				key, nullable(p.ClientID), nullable(p.ClientSecret), nullable(p.TenantID), nullable(p.Issuer), nullable(p.Label), p.Enabled, now, now); err != nil {
				return err
			}
		} else if err := t.update("sign_in_providers", p.ID, map[string]any{"client_id": nullable(p.ClientID), "client_secret": nullable(p.ClientSecret),
			"tenant_id": nullable(p.TenantID), "issuer": nullable(p.Issuer), "label": nullable(p.Label), "enabled": p.Enabled}); err != nil {
			return err
		}
		s, err := a.settings(ctx)
		if err != nil {
			return err
		}
		return t.Event("Setting", s.ID, "settings.sign_in_provider", map[string]any{"provider": key, "on": p.usable()})
	})
	if err != nil {
		return err
	}
	name := a.providerName(ctx, key)
	c.Summary = name + " sign-in saved, and off."
	if p.usable() {
		c.Summary = name + " sign-in is on."
	}
	return c.JSON(200, O("summary", c.Summary))
}

func (a *App) updateProvidersOnly(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	body, err := c.Wrapped("setting")
	if err != nil {
		return err
	}
	on := boolOf(body["providers_only"])
	ctx := c.R.Context()
	keys, err := a.usableProviders(ctx)
	if err != nil {
		return err
	}
	if on && len(keys) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Switch on a provider first, or nobody but owners could sign in."}
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.update("settings", s.ID, map[string]any{"providers_only": on}); err != nil {
			return err
		}
		return t.Event("Setting", s.ID, "settings.providers_only", map[string]any{"on": on})
	})
	if err != nil {
		return err
	}
	c.Summary = "Passwords and links work again."
	if on {
		c.Summary = "Only " + a.providerNames(ctx, keys) + " from now on (owners keep their password)."
	}
	return c.JSON(200, O("summary", c.Summary, "providers_only", on))
}

// listUsableProviders is what the sign-in page offers.
func (a *App) listUsableProviders(c *Ctx) error {
	ctx := c.R.Context()
	keys, err := a.usableProviders(ctx)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	list := []Obj{}
	for _, k := range keys {
		list = append(list, O("key", k, "name", a.providerName(ctx, k)))
	}
	return c.JSON(200, O("providers", list, "providers_only", s.ProvidersOnly && len(keys) > 0, "names", a.providerNames(ctx, keys)))
}

// ---- identities -------------------------------------------------------------

func (a *App) identitiesJSON(ctx context.Context, userID int64) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT id, provider, COALESCE(email, ''), last_used_at FROM identities WHERE user_id = ? ORDER BY id`, userID)
	if err != nil {
		return nil, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var provider, email string
		var used store.Time
		if err := r.Scan(&id, &provider, &email, &used); err != nil {
			return nil, err
		}
		return O("id", id, "provider", provider, "name", a.providerName(ctx, provider), "email", nullable(email), "last_used_at", used), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

func (a *App) destroyIdentity(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	var provider string
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.QueryRow(`SELECT provider FROM identities WHERE id = ? AND user_id = ?`, id, me.ID).Scan(&provider); err != nil {
			if isNoRows(err) {
				return notFound("connection")
			}
			return err
		}
		if _, err := t.Exec(`DELETE FROM identities WHERE id = ?`, id); err != nil {
			return err
		}
		return t.Event("User", me.ID, "user.identity_disconnected", map[string]any{"provider": provider})
	})
	if err != nil {
		return err
	}
	c.Summary = a.providerName(ctx, provider) + " disconnected."
	return c.JSON(200, O("summary", c.Summary))
}

// ---- the OAuth 2 / OpenID Connect code flow ---------------------------------

const oauthCookie = "rw_oauth"

// oauthState is what the browser carries to the provider and back, sealed
// in a short-lived cookie.
type oauthState struct {
	Provider string `json:"p"`
	State    string `json:"s"`
	Verifier string `json:"v"`
	Nonce    string `json:"n"`
	At       int64  `json:"t"`
}

// providerEndpoints are where a provider authorizes and hands out tokens.
type providerEndpoints struct {
	Authorize, Token, UserInfo string
	Scope                      string
	PKCE                       bool
}

func (a *App) endpointsFor(ctx context.Context, p *signInProvider) (*providerEndpoints, error) {
	switch p.Key {
	case "google_oauth2":
		return &providerEndpoints{"https://accounts.google.com/o/oauth2/v2/auth", "https://oauth2.googleapis.com/token", "https://openidconnect.googleapis.com/v1/userinfo", "openid email profile", true}, nil
	case "entra_id":
		tenant := p.TenantID
		if tenant == "" {
			tenant = "common"
		}
		base := "https://login.microsoftonline.com/" + url.PathEscape(tenant) + "/oauth2/v2.0"
		return &providerEndpoints{base + "/authorize", base + "/token", "", "openid profile email", true}, nil
	case "github":
		return &providerEndpoints{"https://github.com/login/oauth/authorize", "https://github.com/login/oauth/access_token", "https://api.github.com/user", "user:email", false}, nil
	case "openid_connect":
		var doc struct {
			Authorization string `json:"authorization_endpoint"`
			Token         string `json:"token_endpoint"`
			UserInfo      string `json:"userinfo_endpoint"`
		}
		if err := getJSON(ctx, strings.TrimRight(p.Issuer, "/")+"/.well-known/openid-configuration", "", &doc); err != nil {
			return nil, err
		}
		if doc.Authorization == "" || doc.Token == "" {
			return nil, errors.New("the issuer's discovery document has no endpoints")
		}
		return &providerEndpoints{doc.Authorization, doc.Token, doc.UserInfo, "openid email profile", true}, nil
	}
	return nil, fmt.Errorf("no provider %s", p.Key)
}

func getJSON(ctx context.Context, u, bearer string, v any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", "Runwell")
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	res, err := outbound.Client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("%s answered %s", req.URL.Host, res.Status)
	}
	return json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(v)
}

// redirect answers with a redirect to path in the app, with a message for
// the screen there.
func redirectTo(c *Ctx, path, key, message string) error {
	if message != "" {
		sep := "?"
		if strings.Contains(path, "?") {
			sep = "&"
		}
		path += sep + key + "=" + url.QueryEscape(message)
	}
	http.Redirect(c.W, c.R, path, http.StatusSeeOther)
	return nil
}

// signedInStaff is the person signed in by the session cookie, if any (an
// /auth route is open, and a GET carries no CSRF token).
func (a *App) signedInStaff(c *Ctx) *User {
	cookie, _ := c.R.Cookie(sessionCookie)
	if cookie == nil || cookie.Value == "" {
		return nil
	}
	actor, err := a.actorForSession(c.R.Context(), cookie.Value, false)
	if err != nil || actor == nil {
		return nil
	}
	return actor.User
}

func (a *App) providerFailure(c *Ctx, key string) error {
	msg := "Signing in with " + a.providerName(c.R.Context(), key) + " didn’t work. Try again, or use your password."
	if a.signedInStaff(c) != nil {
		return redirectTo(c, "/settings/account?tab=security", "alert", msg)
	}
	return redirectTo(c, "/", "alert", msg)
}

func (a *App) startProviderSignIn(c *Ctx) error {
	ctx := c.R.Context()
	key := c.Param("provider")
	if !providerKnown(key) {
		return notFound("sign-in provider")
	}
	p, err := a.loadSignInProvider(ctx, a.store, key)
	if err != nil {
		return err
	}
	if !p.usable() {
		return apierr.NotFound(a.providerName(ctx, key) + " sign-in isn't set up")
	}
	ep, err := a.endpointsFor(ctx, p)
	if err != nil {
		a.log.Warn("sign-in provider", "provider", key, "err", err)
		return a.providerFailure(c, key)
	}
	st := oauthState{Provider: key, State: secret.Token(24), Verifier: secret.Token(48), Nonce: secret.Token(16), At: time.Now().Unix()}
	a.setCookie(c, oauthCookie, a.secrets.Seal(string(store.MustJSON(st))), 600)
	q := url.Values{"response_type": {"code"}, "client_id": {p.ClientID}, "redirect_uri": {a.requestBase(c) + "/auth/" + key + "/callback"},
		"scope": {ep.Scope}, "state": {st.State}}
	if key != "github" {
		q.Set("nonce", st.Nonce)
	}
	if ep.PKCE {
		sum := sha256.Sum256([]byte(st.Verifier))
		q.Set("code_challenge", base64.RawURLEncoding.EncodeToString(sum[:]))
		q.Set("code_challenge_method", "S256")
	}
	sep := "?"
	if strings.Contains(ep.Authorize, "?") {
		sep = "&"
	}
	http.Redirect(c.W, c.R, ep.Authorize+sep+q.Encode(), http.StatusSeeOther)
	return nil
}

// providerAccount is who the provider says signed in: its id for them, the
// address it has, and whether it vouches for that address.
type providerAccount struct {
	UID      string
	Email    string
	Verified bool
}

func (a *App) providerCallback(c *Ctx) error {
	ctx := c.R.Context()
	key := c.Param("provider")
	if !providerKnown(key) {
		return notFound("sign-in provider")
	}
	cookie, _ := c.R.Cookie(oauthCookie)
	a.setCookie(c, oauthCookie, "", -1)
	var st oauthState
	if cookie == nil {
		return a.providerFailure(c, key)
	}
	plain, ok := a.secrets.Open(cookie.Value)
	if !ok || json.Unmarshal([]byte(plain), &st) != nil || st.Provider != key || time.Since(time.Unix(st.At, 0)) > 10*time.Minute ||
		c.R.FormValue("state") != st.State || c.R.FormValue("code") == "" {
		return a.providerFailure(c, key)
	}
	p, err := a.loadSignInProvider(ctx, a.store, key)
	if err != nil {
		return err
	}
	acct, err := a.exchangeCode(c, p, st, c.R.FormValue("code"))
	if err != nil || acct.UID == "" {
		a.log.Warn("sign-in provider", "provider", key, "err", err)
		return a.providerFailure(c, key)
	}
	if me := a.signedInStaff(c); me != nil {
		return a.connectIdentity(c, me, key, acct)
	}
	user, err := a.identityUser(ctx, key, acct)
	if err != nil {
		return err
	}
	if user == nil {
		name := a.providerName(ctx, key)
		return redirectTo(c, "/", "alert", "No one here signs in with that "+name+" account. Sign in another way and connect it in Settings › Your account, or ask an owner for an invitation.")
	}
	return a.providerSignIn(c, user, key)
}

// exchangeCode trades the code for tokens and reads who signed in.
func (a *App) exchangeCode(c *Ctx, p *signInProvider, st oauthState, code string) (*providerAccount, error) {
	ctx := c.R.Context()
	ep, err := a.endpointsFor(ctx, p)
	if err != nil {
		return nil, err
	}
	clientSecret, ok := a.openSecret(p.ClientSecret)
	if !ok {
		return nil, errors.New("the client secret can't be read here: save it again in Settings › Sign-in")
	}
	form := url.Values{"grant_type": {"authorization_code"}, "code": {code}, "redirect_uri": {a.requestBase(c) + "/auth/" + p.Key + "/callback"},
		"client_id": {p.ClientID}, "client_secret": {clientSecret}}
	if ep.PKCE {
		form.Set("code_verifier", st.Verifier)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ep.Token, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := outbound.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	var tok struct {
		AccessToken string `json:"access_token"`
		IDToken     string `json:"id_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok); err != nil {
		return nil, err
	}
	if res.StatusCode != http.StatusOK || tok.AccessToken == "" {
		return nil, fmt.Errorf("the token endpoint answered %s %s", res.Status, tok.Error)
	}
	if p.Key == "github" {
		return githubAccount(ctx, tok.AccessToken)
	}
	// The ID token came straight from the provider over TLS, for our
	// client secret, so its claims are read as they are.
	claims := map[string]any{}
	if parts := strings.Split(tok.IDToken, "."); len(parts) == 3 {
		if raw, err := base64.RawURLEncoding.DecodeString(parts[1]); err == nil {
			_ = json.Unmarshal(raw, &claims)
		}
	}
	if n := str(claims["nonce"]); n != "" && n != st.Nonce {
		return nil, errors.New("the ID token's nonce doesn't match")
	}
	if (str(claims["email"]) == "" || str(claims["sub"]) == "") && ep.UserInfo != "" {
		info := map[string]any{}
		if err := getJSON(ctx, ep.UserInfo, tok.AccessToken, &info); err == nil {
			for k, v := range info {
				if _, ok := claims[k]; !ok {
					claims[k] = v
				}
			}
		}
	}
	acct := &providerAccount{UID: str(claims["sub"]), Email: str(claims["email"])}
	verified := fmt.Sprint(claims["email_verified"])
	switch p.Key {
	case "google_oauth2":
		acct.Verified = verified == "true"
	case "entra_id":
		// Microsoft's id is the tenant and the object id (omniauth-entra-id's
		// uid); its address isn't vouched for, so it must be connected first.
		acct.UID = str(claims["oid"])
		if tid := str(claims["tid"]); tid != "" {
			acct.UID = tid + acct.UID
		}
		if acct.Email == "" {
			acct.Email = str(claims["preferred_username"])
		}
	case "openid_connect":
		// The owner chose this directory as the install's own, so its
		// addresses count unless it says one isn't verified.
		acct.Verified = verified != "false"
	}
	return acct, nil
}

// githubAccount is the GitHub user and their verified primary address.
func githubAccount(ctx context.Context, token string) (*providerAccount, error) {
	var user struct {
		ID    int64  `json:"id"`
		Email string `json:"email"`
	}
	if err := getJSON(ctx, "https://api.github.com/user", token, &user); err != nil {
		return nil, err
	}
	acct := &providerAccount{UID: strconv.FormatInt(user.ID, 10)}
	var emails []struct {
		Email    string `json:"email"`
		Primary  bool   `json:"primary"`
		Verified bool   `json:"verified"`
	}
	if getJSON(ctx, "https://api.github.com/user/emails", token, &emails) == nil {
		for _, e := range emails {
			if e.Primary && e.Verified {
				acct.Email, acct.Verified = e.Email, true
			}
		}
	}
	return acct, nil
}

// identityUser is who a provider's answer signs in: whoever connected that
// account, else an active person whose address the provider vouches for
// (who is connected to it from now on).
func (a *App) identityUser(ctx context.Context, provider string, acct *providerAccount) (*User, error) {
	var id, userID int64
	err := a.store.QueryRowContext(ctx, `SELECT id, user_id FROM identities WHERE provider = ? AND uid = ?`, provider, acct.UID).Scan(&id, &userID)
	if err == nil {
		a.touch(ctx, `UPDATE identities SET last_used_at = ?, updated_at = ? WHERE id = ?`, store.Now().DB(), store.Now().DB(), id)
		u, err := a.loadUser(ctx, userID)
		if err != nil || u == nil || !u.Active() {
			return nil, err
		}
		return u, nil
	} else if !isNoRows(err) {
		return nil, err
	}
	if !acct.Verified || acct.Email == "" {
		return nil, nil
	}
	u, err := a.activePersonByEmail(ctx, acct.Email)
	if err != nil || u == nil {
		return nil, err
	}
	now := store.Now().DB()
	_, err = a.store.ExecContext(ctx, `INSERT INTO identities (user_id, provider, uid, email, last_used_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		u.ID, provider, acct.UID, acct.Email, now, now, now)
	return u, err
}

func (a *App) connectIdentity(c *Ctx, me *User, provider string, acct *providerAccount) error {
	ctx := c.R.Context()
	name := a.providerName(ctx, provider)
	var id, owner int64
	err := a.store.QueryRowContext(ctx, `SELECT id, user_id FROM identities WHERE provider = ? AND uid = ?`, provider, acct.UID).Scan(&id, &owner)
	if err != nil && !isNoRows(err) {
		return err
	}
	if id != 0 && owner != me.ID {
		return redirectTo(c, "/settings/account?tab=security", "alert", "That "+name+" account is connected to someone else.")
	}
	err = a.write(ctx, &Actor{User: me}, func(t *Tx) error {
		now := store.Now().DB()
		if id == 0 {
			if _, err := t.Exec(`INSERT INTO identities (user_id, provider, uid, email, last_used_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				me.ID, provider, acct.UID, nullable(acct.Email), now, now, now); err != nil {
				return err
			}
		} else if err := t.update("identities", id, map[string]any{"email": nullable(acct.Email), "last_used_at": now}); err != nil {
			return err
		}
		return t.Event("User", me.ID, "user.identity_connected", map[string]any{"provider": provider})
	})
	if err != nil {
		return err
	}
	return redirectTo(c, "/settings/account?tab=security", "notice", name+" connected. You can sign in with it now.")
}

// providerSignIn signs user in after a provider vouched for them, or sends
// them to the code from their authenticator app.
func (a *App) providerSignIn(c *Ctx, user *User, method string) error {
	ctx := c.R.Context()
	tf, err := a.loadTwoFactor(ctx, a.store, user.ID)
	if err != nil {
		return err
	}
	if tf.On() {
		var challenge string
		err := a.write(ctx, nil, func(t *Tx) error {
			if err := t.dropAccountTokens(purposeTwoFactor, user.ID); err != nil {
				return err
			}
			challenge, err = t.issueAccountToken(purposeTwoFactor, user.ID, method, twoFactorExpiresIn)
			return err
		})
		if err != nil {
			return err
		}
		return redirectTo(c, "/", "two_factor", challenge)
	}
	if _, err := a.signIn(c, user.ID); err != nil {
		return err
	}
	err = a.write(ctx, &Actor{User: user}, func(t *Tx) error {
		return t.Event("User", user.ID, "user.signed_in", map[string]any{"method": method, "two_factor": false, "ip": c.ip(), "browser": truncate(c.R.UserAgent(), 120)})
	})
	if err != nil {
		return err
	}
	return redirectTo(c, "/", "", "")
}
