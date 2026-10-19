package core

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// danglingUserRefs counts rows whose user column names nobody: every
// column that points at a user (a Rails database enforces them all).
func danglingUserRefs(ctx context.Context, t *testing.T, a *App) map[string]int {
	t.Helper()
	cols := append(append([]removalColumn{}, removalMoves...), removalGone...)
	cols = append(cols, removalColumn{"", "assignments", "user_id"}, removalColumn{"", "invitations", "user_id"},
		removalColumn{"", "plugin_notifications", "from_id"}, removalColumn{"", "users", "agent_owner_id"}, removalColumn{"", "events", "actor_user_id"})
	out := map[string]int{}
	for _, c := range cols {
		var n int
		q := `SELECT count(*) FROM ` + c.Table + ` WHERE ` + c.Column + ` IS NOT NULL AND ` + c.Column + ` NOT IN (SELECT id FROM users)`
		if err := a.store.QueryRowContext(ctx, q).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			out[c.Table+"."+c.Column] = n
		}
	}
	return out
}

func TestRemovePersonHandsTheirWorkOver(t *testing.T) {
	heard := make(chan map[string]any, 2)
	listener := &plugin.Module{
		Manifest: plugin.Manifest{Key: "listener", Name: "Listener", Version: "1", Hooks: []string{"user.removed"}},
		OnEvent: func(_ context.Context, _ *plugin.API, kind string, data json.RawMessage) error {
			var ev struct {
				Payload map[string]any `json:"payload"`
			}
			if err := json.Unmarshal(data, &ev); err != nil {
				return err
			}
			heard <- ev.Payload
			return nil
		},
	}
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Plugins: []*plugin.Module{listener}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	ctx := t.Context()
	if _, err := a.plugins.Install(ctx, nil, "listener"); err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := a.plugins.Configure(ctx, nil, "listener", nil, &on); err != nil {
		t.Fatal(err)
	}
	exec := func(q string, args ...any) int64 {
		t.Helper()
		res, err := a.store.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	user := func(email, name, role string) int64 {
		return exec(`INSERT INTO users (email_address, password_digest, name, role, created_at, updated_at) VALUES (?, 'x', ?, ?, datetime('now'), datetime('now'))`, email, name, role)
	}
	ted, sarah, marcus := user("ted@example.com", "Ted", "owner"), user("sarah@example.com", "Sarah Lee", "manager"), user("marcus@example.com", "Marcus", "member")
	gone := user("gone@example.com", "Gone", "member")
	exec(`UPDATE users SET deactivated_at = datetime('now') WHERE id = ?`, gone)
	sarahAgent := exec(`INSERT INTO users (email_address, password_digest, name, role, agent_owner_id, created_at, updated_at) VALUES ('sarah-agent@agents', 'x', 'Sarah’s agent', 'manager', ?, datetime('now'), datetime('now'))`, sarah)

	token := func(userID int64) string {
		var plain string
		err := a.write(ctx, nil, func(tx *Tx) error {
			tok, err := tx.issueToken(userID, 0, "Test", "personal", 0, false)
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
	tedToken, sarahToken := token(ted), token(sarah)
	call := func(tok, method, path, body string) (int, map[string]any) {
		t.Helper()
		req, _ := http.NewRequest(method, srv.URL+path, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+tok)
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

	// Sarah's work: she leads two, is on one Marcus is on too, wrote a note,
	// owns a commitment, triaged a request, and did things history recalls.
	client := exec(`INSERT INTO clients (name, created_at, updated_at) VALUES ('Harbor', datetime('now'), datetime('now'))`)
	eng := exec(`INSERT INTO engagements (client_id, ref, title, created_by_id, created_at, updated_at) VALUES (?, 'WO-1', 'Site', ?, datetime('now'), datetime('now'))`, client, sarah)
	todo := func(title string, owner int64) int64 {
		return exec(`INSERT INTO todos (engagement_id, title, owner_id, position, board_position, created_by_id, created_at, updated_at) VALUES (?, ?, ?, 1, 1, ?, datetime('now'), datetime('now'))`, eng, title, owner, sarah)
	}
	t1, t2, t3 := todo("Homepage", sarah), todo("Footer", sarah), todo("Logo", marcus)
	assign := func(todoID, userID int64) {
		exec(`INSERT INTO assignments (todo_id, user_id, assigner_id, created_at, updated_at) VALUES (?, ?, ?, datetime('now'), datetime('now'))`, todoID, userID, ted)
	}
	assign(t1, sarah)
	assign(t2, sarah)
	assign(t2, marcus)
	assign(t3, marcus)
	assign(t3, sarah)
	exec(`INSERT INTO notes (subject_type, subject_id, body, kind, occurred_at, author_id, source, created_at, updated_at) VALUES ('Client', ?, '<p>Hi</p>', 'internal', datetime('now'), ?, 'app', datetime('now'), datetime('now'))`, client, sarah)
	exec(`INSERT INTO notes (subject_type, subject_id, body, kind, occurred_at, author_id, source, created_at, updated_at) VALUES ('Client', ?, '<p>From an app</p>', 'internal', datetime('now'), ?, 'agent', datetime('now'), datetime('now'))`, client, sarahAgent)
	exec(`INSERT INTO commitments (client_id, description, user_id, due_on, source, created_at, updated_at) VALUES (?, 'Send the plan', ?, '2026-11-01', 'manual', datetime('now'), datetime('now'))`, client, sarah)
	exec(`INSERT INTO requests (subject, source, received_at, triaged_by_id, created_at, updated_at) VALUES ('Help', 'email', datetime('now'), ?, datetime('now'), datetime('now'))`, sarah)
	exec(`INSERT INTO sessions (user_id, created_at, updated_at) VALUES (?, datetime('now'), datetime('now'))`, sarah)
	exec(`INSERT INTO notifications (user_id, creator_id, source_type, source_id, created_at, updated_at) VALUES (?, ?, 'Mention', 1, datetime('now'), datetime('now'))`, sarah, ted)
	exec(`INSERT INTO events (subject_type, subject_id, kind, payload, actor_user_id, source, occurred_at, created_at) VALUES ('Todo', ?, 'todo.status', '{}', ?, 'app', datetime('now'), datetime('now'))`, t1, sarah)
	exec(`INSERT INTO events (subject_type, subject_id, kind, payload, actor_user_id, actor, source, occurred_at, created_at) VALUES ('Todo', ?, 'todo.created', '{}', ?, 'Claude', 'agent', datetime('now'), datetime('now'))`, t2, sarahAgent)

	// The preview counts it all.
	status, preview := call(tedToken, "GET", "/api/settings/people/"+itoa(sarah)+"/removal", "")
	if status != 200 || preview["allowed"] != true {
		t.Fatalf("preview: %d %v", status, preview)
	}
	counts := map[string]float64{}
	for _, list := range []string{"moves", "removes"} {
		for _, g := range preview[list].([]any) {
			m := g.(map[string]any)
			counts[m["key"].(string)] = m["count"].(float64)
		}
	}
	want := map[string]float64{"work_lead": 2, "work_on": 1, "work_added": 3, "commitments": 1, "notes": 2, "engagements": 1, "requests": 1, "sign_ins": 2, "agent": 1, "notifications": 1}
	for k, v := range want {
		if counts[k] != v {
			t.Errorf("preview %s = %v, want %v (all: %v)", k, counts[k], v, counts)
		}
	}

	// Refusals: not yourself, an owner only by an owner, a real active
	// successor, and their name typed.
	body := func(successor int64, confirm string) string {
		b, _ := json.Marshal(map[string]any{"successor_id": successor, "confirm": confirm})
		return string(b)
	}
	if s, _ := call(tedToken, "POST", "/api/settings/people/"+itoa(ted)+"/removal", body(marcus, "Ted")); s != 422 {
		t.Errorf("removing yourself: %d", s)
	}
	if s, _ := call(sarahToken, "POST", "/api/settings/people/"+itoa(ted)+"/removal", body(marcus, "Ted")); s != 403 {
		t.Errorf("a manager removing an owner: %d", s)
	}
	for _, c := range []struct {
		successor int64
		confirm   string
		field     string
	}{{0, "Sarah Lee", "successor_id"}, {sarah, "Sarah Lee", "successor_id"}, {sarahAgent, "Sarah Lee", "successor_id"},
		{gone, "Sarah Lee", "successor_id"}, {9999, "Sarah Lee", "successor_id"}, {marcus, "Sarah", "confirm"}} {
		s, out := call(tedToken, "POST", "/api/settings/people/"+itoa(sarah)+"/removal", body(c.successor, c.confirm))
		errs, _ := out["errors"].(map[string]any)
		if s != 422 || errs[c.field] == nil {
			t.Errorf("successor %d, confirm %q: %d %v", c.successor, c.confirm, s, out)
		}
	}
	if s, _ := call(tedToken, "POST", "/api/settings/people/"+itoa(sarahAgent)+"/removal", body(marcus, "x")); s != 404 {
		t.Errorf("removing an agent: %d", s)
	}

	// Done, by email address in any case.
	status, out := call(tedToken, "POST", "/api/settings/people/"+itoa(sarah)+"/removal", body(marcus, " SARAH@example.com "))
	if status != 200 || !strings.Contains(out["summary"].(string), "Removed Sarah Lee. Their work is now Marcus’s.") {
		t.Fatalf("remove: %d %v", status, out)
	}
	one := func(q string, args ...any) int {
		var n int
		if err := a.store.QueryRowContext(ctx, q, args...).Scan(&n); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return n
	}
	if n := one(`SELECT count(*) FROM users WHERE id IN (?, ?)`, sarah, sarahAgent); n != 0 {
		t.Errorf("%d of Sarah's users left", n)
	}
	if n := one(`SELECT count(*) FROM todos WHERE owner_id = ?`, marcus); n != 3 {
		t.Errorf("Marcus leads %d, want 3", n)
	}
	if n := one(`SELECT count(*) FROM assignments WHERE user_id = ?`, marcus); n != 3 {
		t.Errorf("Marcus is on %d, want 3 (no duplicates)", n)
	}
	if n := one(`SELECT count(*) FROM notes WHERE author_id = ?`, marcus); n != 2 {
		t.Errorf("Marcus has %d notes, want 2", n)
	}
	if n := one(`SELECT count(*) FROM commitments WHERE user_id = ?`, marcus); n != 1 {
		t.Errorf("Marcus has %d commitments", n)
	}
	if n := one(`SELECT count(*) FROM access_tokens WHERE user_id = ?`, sarah); n != 0 {
		t.Errorf("%d of Sarah's tokens left", n)
	}
	if d := danglingUserRefs(ctx, t, a); len(d) != 0 {
		t.Errorf("columns pointing at nobody: %v", d)
	}
	if n := one(`SELECT count(*) FROM events WHERE actor_user_id = ?`, marcus); n != 0 {
		t.Errorf("history was rewritten to Marcus: %d events", n)
	}

	// History still names her, and the audit log says who went.
	events, err := a.eventsJSON(ctx, `e.subject_type = 'Todo'`, 10)
	if err != nil {
		t.Fatal(err)
	}
	bys := []string{}
	for _, e := range events {
		bys = append(bys, str(e.Get("by")))
	}
	if !strings.Contains(strings.Join(bys, ","), "Sarah Lee") || !strings.Contains(strings.Join(bys, ","), "Claude for Sarah Lee") {
		t.Errorf("history reads %v", bys)
	}
	names := a.subjectNames(ctx, []subjectRef{{"User", sarah}})
	if got := names[subjectRef{"User", sarah}].label(subjectRef{"User", sarah}); got != "Sarah Lee (removed)" {
		t.Errorf("the audit log names her %q", got)
	}
	select {
	case p := <-heard:
		if p["name"] != "Sarah Lee" || p["successor_id"].(float64) != float64(marcus) || p["id"].(float64) != float64(sarah) {
			t.Errorf("plugins heard %v", p)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the plugin never heard user.removed")
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }
