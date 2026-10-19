package websitechecklist

import (
	"context"
	"testing"
)

// A removed person's scans are their successor's; what they and their agent
// did stays in the history under their names, with no id to a missing user.
func TestUserRemovedHandsScansOverAndKeepsHistory(t *testing.T) {
	h, db := removedSetup(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO website_checklist_sites (id, client_id, name, created_at, updated_at) VALUES (1, 1, 'Site', 'now', 'now')`)
	exec(`INSERT INTO website_checklist_scans (site_id, user_id, domain, created_at, updated_at) VALUES (1, 2, 'a.test', 'now', 'now'), (1, 4, 'b.test', 'now', 'now')`)
	exec(`INSERT INTO website_checklist_audits (record_type, action, actor_id, actor_name, created_at) VALUES
		('Site', 'created', 2, 'Sarah', 'now'), ('Site', 'updated', 2, NULL, 'now'), ('Site', 'updated', 5, NULL, 'now'), ('Site', 'updated', 4, 'Ann', 'now')`)
	h.Emit(ctx, "user.removed", map[string]any{"subject_type": "User", "subject_id": 2, "kind": "user.removed",
		"payload": map[string]any{"id": 2, "agent_id": 5, "name": "Sarah Lee", "successor_id": 3}})
	var scans int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM website_checklist_scans WHERE user_id = 3`).Scan(&scans)
	if scans != 1 {
		t.Fatalf("Marcus has %d of Sarah's scans, want 1", scans)
	}
	rows, err := db.QueryContext(ctx, `SELECT COALESCE(actor_id, 0), COALESCE(actor_name, '') FROM website_checklist_audits ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := [][2]any{{int64(0), "Sarah"}, {int64(0), "Sarah Lee"}, {int64(0), "Sarah Lee's agent"}, {int64(4), "Ann"}}
	i := 0
	for rows.Next() {
		var id int64
		var name string
		_ = rows.Scan(&id, &name)
		if want[i][0] != id || want[i][1] != name {
			t.Fatalf("audit %d: %d %q, want %v", i, id, name, want[i])
		}
		i++
	}
	// The history shows them by name.
	got := (&auditRow{ActorName: "Sarah Lee"}).who()
	if got != "Sarah Lee" {
		t.Fatalf("who: %q", got)
	}
}
