package core

import (
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// Metrics counts each client's work, requests and notes, answers the
// dated facts, and leaves money out where the install hides prices.
func TestPluginMetrics(t *testing.T) {
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	a.Handler()
	ctx := t.Context()
	exec := func(q string, args ...any) int64 {
		res, err := a.store.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	c := exec(`INSERT INTO clients (name, status, created_at, updated_at) VALUES ('Acme', 'active', datetime('now'), datetime('now'))`)
	e := exec(`INSERT INTO engagements (client_id, ref, title, created_at, updated_at) VALUES (?, 'WO-1', 'Site', datetime('now'), datetime('now'))`, c)
	exec(`INSERT INTO todos (engagement_id, title, status, due_on, position, board_position, created_at, updated_at) VALUES (?, 'Late', 'planned', '2000-01-01', 1, 1, datetime('now'), datetime('now'))`, e)
	//nolint:dupword // three timestamps, all now
	exec(`INSERT INTO todos (engagement_id, title, status, position, board_position, completed_at, created_at, updated_at) VALUES (?, 'Done', 'done', 2, 2, datetime('now'), datetime('now'), datetime('now'))`, e)
	//nolint:dupword // three timestamps, all now
	exec(`INSERT INTO requests (client_id, subject, source, status, received_at, created_at, updated_at) VALUES (?, 'Help', 'email', 'open', datetime('now'), datetime('now'), datetime('now'))`, c)
	exec(`INSERT INTO notes (subject_type, subject_id, body, occurred_at, source, created_at, updated_at) VALUES ('Engagement', ?, 'hi', datetime('now'), 'app', datetime('now'), datetime('now'))`, e)

	m, err := pluginEnv{a}.Metrics(ctx, plugin.MetricsQuery{Since: time.Now().AddDate(0, -1, 0)})
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Clients) != 1 {
		t.Fatalf("clients %+v", m.Clients)
	}
	cm := m.Clients[0]
	if cm.OpenWork != 1 || cm.LateWork != 1 || cm.OpenRequests != 1 || cm.LastNoteAt == nil || cm.LastTouchAt == nil {
		t.Fatalf("client metrics %+v", cm)
	}
	kinds := map[string]int{}
	for _, f := range m.Facts {
		kinds[f.Kind]++
	}
	if kinds["todo.created"] != 2 || kinds["todo.done"] != 1 || kinds["request.received"] != 1 || kinds["note.added"] != 1 || kinds["client.created"] != 1 {
		t.Fatalf("facts %v", kinds)
	}
	if len(m.Engagements) != 1 || m.Engagements[0].WorkLate != 1 || m.Engagements[0].State != "draft" {
		t.Fatalf("engagements %+v", m.Engagements)
	}
	if m2, err := (pluginEnv{a}).Metrics(ctx, plugin.MetricsQuery{Since: time.Now().AddDate(0, -1, 0), ClientID: c + 1}); err != nil || len(m2.Clients) != 0 || len(m2.Facts) != 0 {
		t.Fatalf("another client's: %v %+v", err, m2)
	}
}
