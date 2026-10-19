package ai

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Someone removed (Sarah, 2; Marcus, 3, takes over): the agent she made
// and the client she kept out of AI go to Marcus, an approval waiting on
// her waits on him (with the run paused on it), a run under way stops, and
// her finished run stays hers, named "Sarah (removed)". Hearing it twice
// changes nothing.
func TestUserRemoved(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	spawn := func(ctx context.Context, _ string, fn func(context.Context)) bool { fn(ctx); return true }
	h := plugin.New(&env{}, db, slog.New(slog.NewTextHandler(io.Discard, nil)), spawn, Module())
	if err := h.LoadAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Install(t.Context(), nil, Key); err != nil {
		t.Fatal(err)
	}
	grants := Module().Manifest.Permissions
	on := true
	if _, err := h.Configure(t.Context(), nil, Key, &grants, &on); err != nil {
		t.Fatal(err)
	}
	now := store.Now().DB().(string)
	for _, q := range []string{
		`INSERT INTO ai_agents (id, name, handle, created_by_id, created_at, updated_at) VALUES (1, 'Researcher', 'researcher', 2, '` + now + `', '` + now + `')`,
		`INSERT INTO ai_exclusions (client_id, user_id, created_at, updated_at) VALUES (8, 2, '` + now + `', '` + now + `')`,
		`INSERT INTO ai_chats (id, user_id, purpose, created_at, updated_at) VALUES (10, 2, 'agent', '` + now + `', '` + now + `'), (11, 2, 'agent', '` + now + `', '` + now + `'), (12, 2, 'agent', '` + now + `', '` + now + `'), (13, 2, 'ask', '` + now + `', '` + now + `')`,
		`INSERT INTO ai_runs (id, agent_id, user_id, ai_chat_id, status, source, created_at, updated_at) VALUES
			(1, 1, 2, 10, 'done', 'note', '` + now + `', '` + now + `'), (2, 1, 2, 11, 'working', 'note', '` + now + `', '` + now + `'), (3, 1, 2, 12, 'waiting', 'note', '` + now + `', '` + now + `')`,
		`INSERT INTO ai_approvals (id, source, source_id, title, requested_by, approvers, status, ai_chat_id, run_id, created_at, updated_at) VALUES
			(1, 'ai', 'chat:12:t1', 'Add a note', 2, '{"user_ids":[2]}', 'waiting', 12, 3, '` + now + `', '` + now + `'),
			(2, 'ai', 'chat:11:t1', 'Add a note', 2, '{"user_ids":[2]}', 'waiting', 11, 2, '` + now + `', '` + now + `'),
			(3, 'billing', 'inv-1', 'Send the invoice', 1, '{"user_ids":[2,3]}', 'waiting', NULL, NULL, '` + now + `', '` + now + `')`,
	} {
		if _, err := db.Exec(q); err != nil {
			t.Fatal(err)
		}
	}
	ev := map[string]any{"kind": "user.removed", "subject_type": "User", "subject_id": 2,
		"payload": map[string]any{"id": 2, "name": "Sarah", "successor_id": 3}}
	for range 2 {
		h.Emit(t.Context(), "user.removed", ev)
	}
	one := func(q string) string {
		t.Helper()
		var s string
		if err := db.QueryRow(q).Scan(&s); err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		return s
	}
	for q, want := range map[string]string{
		`SELECT created_by_id FROM ai_agents WHERE id = 1`:                 "3",
		`SELECT user_id FROM ai_exclusions WHERE client_id = 8`:            "3",
		`SELECT user_id || ' ' || status FROM ai_runs WHERE id = 1`:        "2 done",
		`SELECT user_id || ' ' || status FROM ai_runs WHERE id = 2`:        "2 cancelled",
		`SELECT user_id || ' ' || status FROM ai_runs WHERE id = 3`:        "3 waiting",
		`SELECT user_id FROM ai_chats WHERE id = 12`:                       "3",
		`SELECT user_id FROM ai_chats WHERE id = 13`:                       "2",
		`SELECT approvers || ' ' || status FROM ai_approvals WHERE id = 1`: `{"user_ids":[3]} waiting`,
		`SELECT status FROM ai_approvals WHERE id = 2`:                     "withdrawn",
		`SELECT approvers FROM ai_approvals WHERE id = 3`:                  `{"user_ids":[3]}`,
		`SELECT name FROM ai_removed_people WHERE user_id = 2`:             "Sarah",
	} {
		if got := one(q); got != want {
			t.Errorf("%s = %q, want %q", q, got, want)
		}
	}
	// Her finished run still names her.
	if raw := serve(t, h, "GET", "listRuns", nil, "", nil); !strings.Contains(raw, `"name":"Sarah (removed)"`) {
		t.Fatalf("runs: %s", raw)
	}
}
