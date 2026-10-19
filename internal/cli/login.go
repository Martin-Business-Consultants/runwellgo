package cli

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Signing in to an install: through the browser (OAuth with PKCE, the CLI
// listening on this computer for the answer, RFC 8252), or by pasting a
// personal token made in Settings › Connected apps.

// oauthToken is the token endpoint's answer.
type oauthToken struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int    `json:"expires_in"`
	Error        string `json:"error"`
}

// tokenRequest posts to the install's token endpoint.
func tokenRequest(ctx context.Context, hc *http.Client, site string, form url.Values) (*oauthToken, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, site+"/oauth/token", strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	res, err := hc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("can't reach %s: %w", site, err)
	}
	defer res.Body.Close()
	var tok oauthToken
	if err := json.NewDecoder(io.LimitReader(res.Body, 1<<20)).Decode(&tok); err != nil {
		return nil, fmt.Errorf("the install's token endpoint answered %d", res.StatusCode)
	}
	if tok.Error != "" || tok.AccessToken == "" {
		return nil, fmt.Errorf("signing in failed: %s", strOr(tok.Error, res.Status))
	}
	return &tok, nil
}

func strOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

// browserLogin signs in through the browser: registers this CLI as an app
// (once per sign-in), opens the install's consent page, and waits for the
// answer on a loopback address.
func browserLogin(ctx context.Context, env Env, c *client, noBrowser, readOnly bool) (Profile, error) {
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", "127.0.0.1:0")
	if err != nil {
		return Profile{}, err
	}
	defer ln.Close()
	addr, ok := ln.Addr().(*net.TCPAddr)
	if !ok {
		return Profile{}, fmt.Errorf("listening on %s, not a TCP address", ln.Addr())
	}
	redirect := fmt.Sprintf("http://127.0.0.1:%d/callback", addr.Port)

	host, _ := os.Hostname()
	status, body, err := c.once(ctx, "POST", "/oauth/clients", map[string]any{"client_name": "runwell CLI on " + strOr(host, "this computer"), "redirect_uris": []string{redirect}})
	if err != nil {
		return Profile{}, err
	}
	var reg struct {
		ClientID string `json:"client_id"`
		Error    string `json:"error_description"`
	}
	if json.Unmarshal(body, &reg) != nil || status != http.StatusCreated || reg.ClientID == "" {
		return Profile{}, fmt.Errorf("%s couldn't register this CLI (%d): %s", c.URL, status, strOr(reg.Error, "is it a Runwell install?"))
	}

	verifier := randomString(48)
	sum := sha256.Sum256([]byte(verifier))
	state := randomString(16)
	scope := "runwell"
	if readOnly {
		scope = "runwell:read"
	}
	q := url.Values{"client_id": {reg.ClientID}, "redirect_uri": {redirect}, "response_type": {"code"}, "state": {state}, "scope": {scope},
		"code_challenge": {base64.RawURLEncoding.EncodeToString(sum[:])}, "code_challenge_method": {"S256"}, "resource": {c.URL + "/mcp"}}
	authURL := c.URL + "/oauth/authorize?" + q.Encode()

	type answer struct {
		code string
		err  error
	}
	answers := make(chan answer, 1)
	srv := &http.Server{ReadHeaderTimeout: 10 * time.Second, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/callback" {
			http.NotFound(w, r)
			return
		}
		v := r.URL.Query()
		var a answer
		switch {
		case v.Get("state") != state:
			a.err = errors.New("the sign-in came back for another request: run runwell login again")
		case v.Get("error") == "access_denied":
			a.err = errors.New("the sign-in was cancelled in the browser")
		case v.Get("error") != "":
			a.err = fmt.Errorf("signing in failed: %s %s", v.Get("error"), v.Get("error_description"))
		default:
			a.code = v.Get("code")
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if a.err != nil {
			fmt.Fprintf(w, "<!doctype html><title>runwell</title><p style=\"font-family:sans-serif\">%s. You can close this tab.</p>", html.EscapeString(a.err.Error()))
		} else {
			fmt.Fprint(w, "<!doctype html><title>runwell</title><p style=\"font-family:sans-serif\">Signed in. You can close this tab and go back to the terminal.</p>")
		}
		select {
		case answers <- a:
		default:
		}
	})}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	fmt.Fprintf(env.Stdout, "Approve runwell in your browser:\n\n    %s\n\nWaiting… (or press Ctrl-C and run runwell login %s --paste to paste a personal token instead)\n", authURL, c.URL)
	if !noBrowser {
		openBrowser(ctx, authURL)
	}
	var a answer
	select {
	case <-ctx.Done():
		return Profile{}, ctx.Err()
	case <-time.After(10 * time.Minute):
		return Profile{}, errors.New("nobody approved the sign-in: run runwell login again")
	case a = <-answers:
	}
	if a.err != nil {
		return Profile{}, a.err
	}
	tok, err := tokenRequest(ctx, c.http, c.URL, url.Values{"grant_type": {"authorization_code"}, "client_id": {reg.ClientID}, "code": {a.code},
		"redirect_uri": {redirect}, "code_verifier": {verifier}})
	if err != nil {
		return Profile{}, err
	}
	return Profile{URL: c.URL, Token: tok.AccessToken, ClientID: reg.ClientID, RefreshToken: tok.RefreshToken}, nil
}

// pasteToken reads a personal token from the terminal.
func pasteToken(env Env, site string) (string, error) {
	fmt.Fprintf(env.Stdout, "Make a personal token in %s/settings/connected_apps (Make a personal token), then paste it here: ", site)
	line, err := bufio.NewReader(env.Stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no token pasted")
	}
	tok := strings.TrimSpace(line)
	if tok == "" {
		return "", errors.New("no token pasted")
	}
	return tok, nil
}

func randomString(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)[:n]
}
