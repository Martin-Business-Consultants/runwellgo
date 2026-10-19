package myday

import (
	"context"
	"testing"
)

// A removed person's priorities join their successor's, after the
// successor's own and without repeats, as far as five hold; theirs go.
func TestUserRemovedHandsPrioritiesOver(t *testing.T) {
	h, db := removedSetup(t)
	ctx := context.Background()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	now := railsTime()
	exec(`INSERT INTO my_day_bundles (id, user_id, name, position, created_at, updated_at) VALUES (1, 2, 'Priorities', 0, ?, ?), (2, 3, 'Priorities', 0, ?, ?)`, now, now, now, now)
	// Sarah (2): 10, 11, 12, 13. Marcus (3): 11, 20.
	for i, todo := range []int{10, 11, 12, 13} {
		exec(`INSERT INTO my_day_pins (bundle_id, todo_id, position, created_at, updated_at) VALUES (1, ?, ?, ?, ?)`, todo, i, now, now)
	}
	for i, todo := range []int{11, 20} {
		exec(`INSERT INTO my_day_pins (bundle_id, todo_id, position, created_at, updated_at) VALUES (2, ?, ?, ?, ?)`, todo, i, now, now)
	}
	emitRemoved(h, 2, 3, "Sarah")
	emitRemoved(h, 2, 3, "Sarah") // twice changes nothing

	rows, err := db.QueryContext(ctx, `SELECT p.todo_id FROM my_day_pins p JOIN my_day_bundles b ON b.id = p.bundle_id WHERE b.user_id = 3 ORDER BY p.position`)
	if err != nil {
		t.Fatal(err)
	}
	var got []int
	for rows.Next() {
		var id int
		_ = rows.Scan(&id)
		got = append(got, id)
	}
	rows.Close()
	want := []int{11, 20, 10, 12, 13}
	if len(got) != len(want) {
		t.Fatalf("Marcus's priorities: %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("Marcus's priorities: %v, want %v", got, want)
		}
	}
	var left int
	_ = db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM my_day_bundles WHERE user_id = 2) + (SELECT count(*) FROM my_day_pins WHERE bundle_id = 1)`).Scan(&left)
	if left != 0 {
		t.Fatalf("Sarah's priorities are still here: %d rows", left)
	}
}

// A successor without priorities gets a bundle made for them.
func TestUserRemovedMakesTheSuccessorsBundle(t *testing.T) {
	h, db := removedSetup(t)
	ctx := context.Background()
	now := railsTime()
	if _, err := db.ExecContext(ctx, `INSERT INTO my_day_bundles (id, user_id, name, position, created_at, updated_at) VALUES (1, 2, 'Priorities', 0, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO my_day_pins (bundle_id, todo_id, position, created_at, updated_at) VALUES (1, 7, 0, ?, ?)`, now, now); err != nil {
		t.Fatal(err)
	}
	emitRemoved(h, 2, 3, "Sarah")
	var n int
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM my_day_pins p JOIN my_day_bundles b ON b.id = p.bundle_id WHERE b.user_id = 3 AND p.todo_id = 7`).Scan(&n)
	if n != 1 {
		t.Fatal("Marcus didn't get Sarah's priority")
	}
}
