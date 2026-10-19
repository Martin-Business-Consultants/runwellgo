package reports

import (
	"context"
	"testing"
)

// Who placed a client on the map stays a name; the id that would point at
// no one goes.
func TestUserRemovedKeepsWhoSetAPlace(t *testing.T) {
	h, db := removedSetup(t)
	ctx := context.Background()
	if _, err := db.ExecContext(ctx, `INSERT INTO reports_client_places (client_id, label, lat, lng, source, set_by_id, set_by, updated_at) VALUES
		(1, 'Leeds', 53.8, -1.5, 'set', 2, 'Sarah', 'now'), (2, 'York', 53.9, -1.1, 'set', 2, '', 'now'), (3, 'Hull', 53.7, -0.3, 'set', 3, 'Marcus', 'now')`); err != nil {
		t.Fatal(err)
	}
	emitRemoved(h, 2, 3, "Sarah Lee")
	rows, err := db.QueryContext(ctx, `SELECT client_id, COALESCE(set_by_id, 0), set_by FROM reports_client_places ORDER BY client_id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[int64][2]any{1: {int64(0), "Sarah"}, 2: {int64(0), "Sarah Lee"}, 3: {int64(3), "Marcus"}}
	for rows.Next() {
		var client, by int64
		var name string
		_ = rows.Scan(&client, &by, &name)
		if w := want[client]; w[0] != by || w[1] != name {
			t.Fatalf("client %d: set by %d %q, want %v", client, by, name, w)
		}
	}
}
