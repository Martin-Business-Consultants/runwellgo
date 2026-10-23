package core

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testInstall is an install served over HTTP, with an owner to call it as.
type testInstall struct {
	t     *testing.T
	a     *App
	srv   *httptest.Server
	token string // the owner's
}

// newTestInstall opens an install with no plugins installed (the
// agreements switch then reads as on, as on a first run).
func newTestInstall(t *testing.T) *testInstall {
	t.Helper()
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	srv := httptest.NewServer(a.Handler())
	t.Cleanup(srv.Close)
	ti := &testInstall{t: t, a: a, srv: srv}
	ti.token = ti.person("owner@example.com", "owner")
	return ti
}

// person adds someone on staff with role and answers a token of theirs.
func (ti *testInstall) person(email, role string) string {
	t, ctx := ti.t, ti.t.Context()
	t.Helper()
	res, err := ti.a.store.ExecContext(ctx, `INSERT INTO users (email_address, password_digest, name, role, created_at, updated_at) VALUES (?, 'x', ?, ?, datetime('now'), datetime('now'))`, email, email, role)
	if err != nil {
		t.Fatal(err)
	}
	id, _ := res.LastInsertId()
	var plain string
	err = ti.a.write(ctx, nil, func(tx *Tx) error {
		tok, err := tx.issueToken(id, 0, "Test", "personal", 0, false)
		if tok != nil {
			plain = tok.Plaintext
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return plain
}

// call sends body (JSON, or nothing) to path as the owner: the status and
// the answer.
func (ti *testInstall) call(method, path string, body any) (int, map[string]any) {
	t := ti.t
	t.Helper()
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		r = strings.NewReader(string(b))
	}
	req, _ := http.NewRequestWithContext(t.Context(), method, ti.srv.URL+path, r)
	req.Header.Set("Authorization", "Bearer "+ti.token)
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	var out map[string]any
	_ = json.NewDecoder(res.Body).Decode(&out)
	return res.StatusCode, out
}

// must is call that fails the test unless the status is want.
func (ti *testInstall) must(want int, method, path string, body any) map[string]any {
	ti.t.Helper()
	status, out := ti.call(method, path, body)
	if status != want {
		ti.t.Fatalf("%s %s: %d, want %d: %v", method, path, status, want, out)
	}
	return out
}

// client adds a client and answers its id.
func (ti *testInstall) client(name string) int64 {
	ti.t.Helper()
	out := ti.must(201, "POST", "/api/clients", map[string]any{"client": map[string]any{"name": name}})
	return int64(dig(out, "client", "id").(float64))
}

// toolNames are the tools the owner is offered.
func (ti *testInstall) toolNames() map[string]bool {
	ti.t.Helper()
	out := ti.must(200, "GET", "/api/agent/tools", nil)
	names := map[string]bool{}
	for _, tl := range out["tools"].([]any) {
		names[tl.(map[string]any)["name"].(string)] = true
	}
	return names
}

// me is what the session answers the owner as (meJSON): a session needs a
// cookie, which a token isn't.
func (ti *testInstall) me() Obj {
	t := ti.t
	t.Helper()
	var id int64
	if err := ti.a.store.QueryRowContext(t.Context(), `SELECT id FROM users WHERE email_address = 'owner@example.com'`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	u, err := ti.a.loadUser(t.Context(), id)
	if err != nil {
		t.Fatal(err)
	}
	me, err := ti.a.meJSON(t.Context(), &Ctx{App: ti.a, Actor: &Actor{User: u}})
	if err != nil {
		t.Fatal(err)
	}
	return me
}
