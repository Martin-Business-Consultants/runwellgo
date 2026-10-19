package core

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/ratelimit"
	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// OAuth 2.1 for connectors (Claude, ChatGPT, Claude Code, the runwell CLI):
// discovery (RFC 9728 and 8414), dynamic client registration (RFC 7591,
// public clients only: no secret, PKCE instead), a person's yes in the app
// (/oauth/authorize, or /portal/oauth/authorization for a client's
// contact), the token endpoint (a code with its PKCE verifier, or a refresh
// token) and revocation (RFC 7009).

func init() { routeSets = append(routeSets, (*App).oauthRoutes) }

func (a *App) oauthRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /.well-known/oauth-protected-resource", a.protectedResource)
	mux.HandleFunc("GET /.well-known/oauth-protected-resource/{resource...}", a.protectedResource)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server", a.authorizationServer)
	mux.HandleFunc("GET /.well-known/oauth-authorization-server/{issuer...}", a.authorizationServer)
	mux.HandleFunc("POST /oauth/clients", a.registerOauthClient)
	mux.HandleFunc("POST /oauth/token", a.oauthToken)
	mux.HandleFunc("POST /oauth/revocation", a.oauthRevocation)
	// Rails' address for the consent page: the app's screen is /oauth/authorize.
	mux.HandleFunc("GET /oauth/authorization", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/oauth/authorize?"+r.URL.RawQuery, http.StatusFound)
	})

	a.handle(mux, "GET /api/oauth/authorization", a.showOauthAuthorization, open)
	a.handle(mux, "POST /api/oauth/authorization", a.createOauthAuthorization)
	a.handle(mux, "GET /api/portal/oauth/authorization", a.showPortalOauthAuthorization, open, portal)
	a.handle(mux, "POST /api/portal/oauth/authorization", a.createPortalOauthAuthorization, portal)
}

const grantLifetime = 10 * time.Minute

var (
	// Registering: 20 an hour from one address, as Rails had it (counted
	// here a minute at a time).
	oauthRegistrations = ratelimit.New(20)
	// The token endpoint: 60 a minute from one address.
	oauthTokenCalls = ratelimit.New(60)
)

// ---- discovery ------------------------------------------------------------------

// protectedResource is the staff MCP endpoint, or a client's portal one
// (/.well-known/oauth-protected-resource/portal/mcp).
func (a *App) protectedResource(w http.ResponseWriter, r *http.Request) {
	base := a.BaseURL()
	if strings.HasPrefix(r.PathValue("resource"), "portal") {
		writeJSON(w, 200, O("resource", base+"/portal/mcp", "authorization_servers", []string{base}, "bearer_methods_supported", []string{"header"},
			"resource_name", a.BrandName(r.Context())+" client portal", "scopes_supported", []string{"runwell:portal", "runwell:read"}))
		return
	}
	writeJSON(w, 200, O("resource", base+"/mcp", "authorization_servers", []string{base}, "bearer_methods_supported", []string{"header"},
		"resource_name", "Runwell", "scopes_supported", []string{"runwell", "runwell:read"}))
}

func (a *App) authorizationServer(w http.ResponseWriter, r *http.Request) {
	base := a.BaseURL()
	writeJSON(w, 200, O(
		"issuer", base,
		"authorization_endpoint", base+"/oauth/authorize",
		"token_endpoint", base+"/oauth/token",
		"registration_endpoint", base+"/oauth/clients",
		"revocation_endpoint", base+"/oauth/revocation",
		"response_types_supported", []string{"code"},
		"grant_types_supported", []string{"authorization_code", "refresh_token"},
		"code_challenge_methods_supported", []string{"S256"},
		"token_endpoint_auth_methods_supported", []string{"none"},
		"scopes_supported", []string{"runwell", "runwell:read", "runwell:portal"},
	))
}

// ---- clients ----------------------------------------------------------------------

// OauthClient is an app that registered itself to sign people in.
type OauthClient struct {
	ID           int64
	UID          string
	Name         string
	RedirectURIs []string
}

func (a *App) oauthClientByUID(ctx context.Context, uid string) (*OauthClient, error) {
	if uid == "" {
		return nil, nil
	}
	var c OauthClient
	var uris store.JSON
	err := a.store.QueryRowContext(ctx, `SELECT id, uid, name, redirect_uris FROM oauth_clients WHERE uid = ?`, uid).Scan(&c.ID, &c.UID, &c.Name, &uris)
	if isNoRows(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	for _, u := range uris.List() {
		c.RedirectURIs = append(c.RedirectURIs, str(u))
	}
	return &c, nil
}

// loopbackHosts are this computer: a desktop app (Claude Code, the runwell
// CLI) signs in by listening on it, never on a domain (RFC 8252).
var loopbackHosts = []string{"localhost", "127.0.0.1", "::1"}

func isLoopback(uri string) bool {
	u, err := url.Parse(uri)
	return err == nil && slices.Contains(loopbackHosts, u.Hostname())
}

// redirectURIErrors checks a registration's addresses: https anywhere, or
// http only back to this machine.
func redirectURIErrors(uris []string) []string {
	var errs []string
	if len(uris) == 0 {
		errs = append(errs, "Redirect uris are required")
	}
	for _, uri := range uris {
		u, err := url.Parse(uri)
		if err != nil {
			errs = append(errs, "Redirect uris include an invalid address")
			continue
		}
		ok := u.Scheme == "https" && u.Host != "" || (u.Scheme == "http" && slices.Contains(loopbackHosts, u.Hostname()))
		if !ok || u.Fragment != "" || strings.Contains(uri, "#") {
			errs = append(errs, "Redirect uris must be https or a local loopback address")
		}
	}
	return errs
}

// toSentence is Rails' to_sentence: "a", "a and b", "a, b, and c".
func toSentence(list []string, connector ...string) string {
	word := "and"
	if len(connector) > 0 {
		word = connector[0]
	}
	switch len(list) {
	case 0:
		return ""
	case 1:
		return list[0]
	case 2:
		return list[0] + " " + word + " " + list[1]
	}
	if word == "and" {
		return strings.Join(list[:len(list)-1], ", ") + ", and " + list[len(list)-1]
	}
	return strings.Join(list[:len(list)-1], ", ") + " " + word + " " + list[len(list)-1]
}

// oauthParams reads a request's parameters, form or JSON, as OAuth
// endpoints take both.
func oauthParams(r *http.Request) (map[string]any, error) {
	out := map[string]any{}
	ct, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if ct == "application/json" {
		raw, err := io.ReadAll(http.MaxBytesReader(nil, r.Body, 1<<20))
		if err != nil {
			return nil, err
		}
		if len(strings.TrimSpace(string(raw))) > 0 {
			if err := json.Unmarshal(raw, &out); err != nil {
				return nil, err
			}
		}
	} else {
		r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
		if err := r.ParseForm(); err != nil {
			return nil, err
		}
		for k, v := range r.Form {
			key := strings.TrimSuffix(k, "[]")
			if len(v) == 1 && !strings.HasSuffix(k, "[]") {
				out[key] = v[0]
			} else {
				list := make([]any, len(v))
				for i, s := range v {
					list[i] = s
				}
				out[key] = list
			}
		}
	}
	for k, v := range r.URL.Query() {
		if _, ok := out[k]; !ok && len(v) > 0 {
			out[k] = v[0]
		}
	}
	return out, nil
}

// registerOauthClient is dynamic client registration: a connector registers
// itself before asking a person to approve it.
func (a *App) registerOauthClient(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{App: a, W: w, R: r}
	if !allowed(oauthRegistrations, "ip:"+c.ip()) {
		writeJSON(w, 429, map[string]any{"error": "rate_limited"})
		return
	}
	p, err := oauthParams(r)
	if err != nil {
		writeJSON(w, 400, map[string]any{"error": "invalid_request"})
		return
	}
	name := strings.TrimSpace(str(p["client_name"]))
	if name == "" {
		name = "An AI app"
	}
	var uris []string
	switch v := p["redirect_uris"].(type) {
	case []any:
		for _, u := range v {
			uris = append(uris, str(u))
		}
	case string:
		if v != "" {
			uris = []string{v}
		}
	}
	if errs := redirectURIErrors(uris); len(errs) > 0 {
		writeJSON(w, 400, map[string]any{"error": "invalid_redirect_uri", "error_description": toSentence(errs)})
		return
	}
	uid := newUUID()
	now := store.Now().DB()
	if _, err := a.store.ExecContext(r.Context(), `INSERT INTO oauth_clients (uid, name, redirect_uris, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		uid, name, string(store.MustJSON(uris)), now, now); err != nil {
		a.log.Error("oauth client", "err", err)
		writeJSON(w, 500, map[string]any{"error": "server_error"})
		return
	}
	writeJSON(w, 201, O("client_id", uid, "client_name", name, "redirect_uris", uris, "token_endpoint_auth_method", "none",
		"grant_types", []string{"authorization_code", "refresh_token"}, "response_types", []string{"code"}))
}

// newUUID is a random (v4) UUID, as Rails' SecureRandom.uuid.
func newUUID() string {
	b := secret.RandomBytes(16)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	const hex = "0123456789abcdef"
	out := make([]byte, 0, 36)
	for i, x := range b {
		if i == 4 || i == 6 || i == 8 || i == 10 {
			out = append(out, '-')
		}
		out = append(out, hex[x>>4], hex[x&0x0f])
	}
	return string(out)
}

// ---- consent ------------------------------------------------------------------------

// authRequest is an authorization request, checked: a registered client,
// its own redirect URI, the code flow, and PKCE (S256).
type authRequest struct {
	client      *OauthClient
	redirectURI string
	params      map[string]string
	// answer is where to send the browser back when the request fails a
	// check the app can be told about (its redirect, with the error).
	answer string
	// invalid: a client or redirect we don't know, shown here instead of
	// redirecting anywhere.
	invalid bool
}

// authParamKeys are what the consent page carries through.
var authParamKeys = []string{"client_id", "redirect_uri", "response_type", "code_challenge", "code_challenge_method", "state", "scope", "resource"}

func (a *App) checkAuthRequest(ctx context.Context, get func(string) string) (*authRequest, error) {
	req := &authRequest{params: map[string]string{}}
	for _, k := range authParamKeys {
		req.params[k] = get(k)
	}
	client, err := a.oauthClientByUID(ctx, req.params["client_id"])
	if err != nil {
		return nil, err
	}
	req.client, req.redirectURI = client, req.params["redirect_uri"]
	// Never redirect to an address the client didn't register.
	if client == nil || !slices.Contains(client.RedirectURIs, req.redirectURI) {
		req.invalid = true
		return req, nil
	}
	switch {
	case req.params["response_type"] != "code":
		req.answer = req.redirectWith(map[string]string{"error": "unsupported_response_type"})
	case req.params["code_challenge"] == "" || req.params["code_challenge_method"] != "S256":
		req.answer = req.redirectWith(map[string]string{"error": "invalid_request", "error_description": "PKCE (S256) is required"})
	}
	return req, nil
}

// forPortal is whether a request is for a client's portal: it names the
// portal's MCP endpoint as its resource (RFC 8707), or asks for the
// portal scope.
func (req *authRequest) forPortal() bool {
	return strings.Contains(req.params["resource"], "/portal/mcp") || slices.Contains(strings.Fields(req.params["scope"]), "runwell:portal")
}

// redirectWith is the redirect URI with the answer and the state.
func (req *authRequest) redirectWith(answer map[string]string) string {
	u, err := url.Parse(req.redirectURI)
	if err != nil {
		return req.redirectURI
	}
	q := url.Values{}
	for k, v := range answer {
		if v != "" {
			q.Set(k, v)
		}
	}
	if st := req.params["state"]; st != "" {
		q.Set("state", st)
	}
	if u.RawQuery != "" {
		u.RawQuery += "&" + q.Encode()
	} else {
		u.RawQuery = q.Encode()
	}
	return u.String()
}

// authRequestJSON is what the consent screen shows: the app, where it goes
// back to, and whether it asked to be read-only.
func authRequestJSON(req *authRequest) Obj {
	if req.invalid {
		return O("status", "invalid")
	}
	if req.answer != "" {
		return O("status", "redirect", "redirect", req.answer)
	}
	returning := req.client.Name + " on this computer"
	if !isLoopback(req.redirectURI) {
		if u, err := url.Parse(req.redirectURI); err == nil {
			returning = u.Hostname()
		}
	}
	return O("status", "ask", "client_name", req.client.Name, "returning_to", returning, "loopback", isLoopback(req.redirectURI),
		"read_only", slices.Contains(strings.Fields(req.params["scope"]), "runwell:read"), "params", req.params)
}

func (a *App) showOauthAuthorization(c *Ctx) error {
	req, err := a.checkAuthRequest(c.R.Context(), c.Query)
	if err != nil {
		return err
	}
	out := authRequestJSON(req)
	if !req.invalid && req.answer == "" && req.forPortal() {
		return c.JSON(200, O("status", "portal", "redirect", "/portal/oauth/authorization?"+c.R.URL.RawQuery))
	}
	if c.Actor != nil && c.Actor.User != nil {
		u := c.Actor.User
		agent := u.DisplayName() + "'s agent"
		var name string
		if a.store.QueryRowContext(c.R.Context(), `SELECT name FROM users WHERE agent_owner_id = ?`, u.ID).Scan(&name) == nil && name != "" {
			agent = name
		}
		out = out.Set("agent_name", agent).Set("role", u.Role)
	}
	return c.JSON(200, out)
}

// approval reads the consent's answer: the request it carries, and yes or no.
func (a *App) approval(c *Ctx) (*authRequest, bool, bool, error) {
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return nil, false, false, err
	}
	req, err := a.checkAuthRequest(c.R.Context(), func(k string) string { return str(body[k]) })
	if err != nil {
		return nil, false, false, err
	}
	if req.invalid {
		return nil, false, false, apierr.BadRequest("The sign-in link it opened isn’t one Runwell recognizes: an unknown app, or a return address it didn’t register. Try connecting again from the app.")
	}
	return req, boolOf(body["approve"]), boolOf(body["read_only"]), nil
}

// createOauthAuthorization is a person's yes (or no) to an app acting as
// them: yes answers the redirect back with a single-use code bound to the
// redirect URI and the PKCE challenge.
func (a *App) createOauthAuthorization(c *Ctx) error {
	req, approve, readOnly, err := a.approval(c)
	if err != nil {
		return err
	}
	return a.answerAuthorization(c, req, approve, readOnly, c.Actor.User.Person().ID, 0)
}

func (a *App) answerAuthorization(c *Ctx, req *authRequest, approve, readOnly bool, userID, contactID int64) error {
	if req.answer != "" {
		return c.JSON(200, O("redirect", req.answer))
	}
	if !approve {
		return c.JSON(200, O("redirect", req.redirectWith(map[string]string{"error": "access_denied"})))
	}
	code := secret.Token(32)
	var user, contact any
	if userID != 0 {
		user = userID
	}
	if contactID != 0 {
		contact = contactID
	}
	now := store.Now()
	_, err := a.store.ExecContext(c.R.Context(), `INSERT INTO oauth_grants (oauth_client_id, user_id, contact_id, code_digest, redirect_uri, code_challenge, read_only, expires_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, req.client.ID, user, contact, secret.Digest(code), req.redirectURI, req.params["code_challenge"], readOnly,
		store.TimeOf(time.Now().Add(grantLifetime)).DB(), now.DB(), now.DB())
	if err != nil {
		return err
	}
	return c.JSON(200, O("redirect", req.redirectWith(map[string]string{"code": code})))
}

// showPortalOauthAuthorization is the consent for a client's contact,
// signed in to the portal by emailed link.
func (a *App) showPortalOauthAuthorization(c *Ctx) error {
	ctx := c.R.Context()
	req, err := a.checkAuthRequest(ctx, c.Query)
	if err != nil {
		return err
	}
	out := authRequestJSON(req)
	if ct := c.Actor; ct != nil && ct.Contact != nil {
		s, err := a.settings(ctx)
		if err != nil {
			return err
		}
		var client string
		_ = a.store.QueryRowContext(ctx, `SELECT name FROM clients WHERE id = ?`, ct.Contact.ClientID).Scan(&client)
		out = out.Set("contact_name", ct.Contact.Name).Set("client", client).Set("brand", a.BrandName(ctx)).
			Set("may_decide", ct.Contact.CanApprove && s.ClientAgentApprovals)
	}
	return c.JSON(200, out)
}

func (a *App) createPortalOauthAuthorization(c *Ctx) error {
	req, approve, readOnly, err := a.approval(c)
	if err != nil {
		return err
	}
	return a.answerAuthorization(c, req, approve, readOnly, 0, c.Actor.Contact.ID)
}

// ---- tokens --------------------------------------------------------------------------

// oauthToken trades a code (with its PKCE verifier) or a refresh token for
// an access token.
func (a *App) oauthToken(w http.ResponseWriter, r *http.Request) {
	c := &Ctx{App: a, W: w, R: r}
	fail := func(code string) { writeJSON(w, 400, map[string]any{"error": code}) }
	if !allowed(oauthTokenCalls, "ip:"+c.ip()) {
		writeJSON(w, 429, map[string]any{"error": "rate_limited"})
		return
	}
	p, err := oauthParams(r)
	if err != nil {
		fail("invalid_request")
		return
	}
	ctx := r.Context()
	client, err := a.oauthClientByUID(ctx, str(p["client_id"]))
	if err != nil || client == nil {
		fail("invalid_client")
		return
	}
	var tok *AccessToken
	switch str(p["grant_type"]) {
	case "authorization_code":
		tok, err = a.redeemGrant(ctx, client, str(p["code"]), str(p["redirect_uri"]), str(p["code_verifier"]))
	case "refresh_token":
		tok, err = a.refreshToken(ctx, str(p["refresh_token"]), client.ID)
	default:
		fail("unsupported_grant_type")
		return
	}
	if err != nil {
		a.log.Error("oauth token", "err", err)
		writeJSON(w, 500, map[string]any{"error": "server_error"})
		return
	}
	if tok == nil {
		fail("invalid_grant")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	writeJSON(w, 200, O("access_token", tok.Plaintext, "token_type", "Bearer", "expires_in", int(oauthLifetime.Seconds()),
		"refresh_token", tok.RefreshPlaintext, "scope", tokenScope(tok.ReadOnly, tok.ContactID != 0)))
}

// redeemGrant trades a code for a token, once: nil if anything doesn't
// match.
func (a *App) redeemGrant(ctx context.Context, client *OauthClient, code, redirectURI, verifier string) (*AccessToken, error) {
	var tok *AccessToken
	err := a.write(ctx, nil, func(t *Tx) error {
		var id int64
		var userID, contactID store.NullInt
		var redirect, challenge string
		var readOnly bool
		var used, expires store.Time
		err := t.QueryRow(`SELECT id, user_id, contact_id, redirect_uri, code_challenge, read_only, used_at, expires_at FROM oauth_grants WHERE code_digest = ? AND oauth_client_id = ?`,
			secret.Digest(code), client.ID).Scan(&id, &userID, &contactID, &redirect, &challenge, &readOnly, &used, &expires)
		if isNoRows(err) {
			return nil
		} else if err != nil {
			return err
		}
		if used.Valid || !expires.Valid || !expires.After(time.Now()) || redirect != redirectURI || !pkceVerifies(verifier, challenge) {
			return nil
		}
		if ok, err := a.holderActive(ctx, userID.Int64, contactID.Int64); err != nil || !ok {
			return err
		}
		if _, err := t.Exec(`UPDATE oauth_grants SET used_at = ?, updated_at = ? WHERE id = ?`, store.Now().DB(), store.Now().DB(), id); err != nil {
			return err
		}
		tok, err = t.issueToken(userID.Int64, contactID.Int64, client.Name, "oauth", client.ID, readOnly)
		return err
	})
	return tok, err
}

// pkceVerifies is whether verifier is the one the challenge was made from
// (S256).
func pkceVerifies(verifier, challenge string) bool {
	if verifier == "" {
		return false
	}
	sum := sha256.Sum256([]byte(verifier))
	return subtle.ConstantTimeCompare([]byte(base64.RawURLEncoding.EncodeToString(sum[:])), []byte(challenge)) == 1
}

// oauthRevocation is an app giving up its access: always 200, whether or
// not the token was known.
func (a *App) oauthRevocation(w http.ResponseWriter, r *http.Request) {
	p, _ := oauthParams(r)
	if plain := str(p["token"]); plain != "" {
		digest := secret.Digest(plain)
		now := store.Now().DB()
		if _, err := a.store.ExecContext(r.Context(), `UPDATE access_tokens SET revoked_at = ?, updated_at = ? WHERE revoked_at IS NULL AND (token_digest = ? OR refresh_token_digest = ?)`,
			now, now, digest, digest); err != nil {
			a.log.Warn("oauth revocation", "err", err)
		}
	}
	w.WriteHeader(http.StatusOK)
}
