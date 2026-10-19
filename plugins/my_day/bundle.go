package myday

import (
	"context"
	"database/sql"
	"errors"
	"slices"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// A person's priorities (MyDay::Bundle): the work they've dragged to the top
// of their day, in the order they mean to do it. One each, made the first
// time it's needed; only its person sees it. Its pins point at the core's
// todos by id.

const (
	bundleName = "Priorities"
	// limit is how many it holds (Bundle::LIMIT).
	limit = 5
)

type pin struct {
	id       int64 // 0 until saved
	todoID   int64
	position int
}

type bundle struct {
	id   int64
	pins []pin // in order
}

// railsTime is a timestamp as Rails writes it into SQLite.
func railsTime() string { return time.Now().UTC().Format("2006-01-02 15:04:05.000000") }

// prioritiesFor is the person's bundle (Bundle.priorities_for), made when
// first needed, with its pins in order.
func prioritiesFor(ctx context.Context, q plugin.Querier, userID int64) (*bundle, error) {
	b := &bundle{}
	err := q.QueryRow(ctx, `SELECT id FROM my_day_bundles WHERE user_id = ? ORDER BY position, id LIMIT 1`, userID).Scan(&b.id)
	if errors.Is(err, sql.ErrNoRows) {
		now := railsTime()
		res, err := q.Exec(ctx, `INSERT INTO my_day_bundles (user_id, name, position, created_at, updated_at) VALUES (?, ?, 0, ?, ?)`, userID, bundleName, now, now)
		if err != nil {
			return nil, err
		}
		b.id, err = res.LastInsertId()
		return b, err
	}
	if err != nil {
		return nil, err
	}
	rows, err := q.Query(ctx, `SELECT id, todo_id, position FROM my_day_pins WHERE bundle_id = ? ORDER BY position, id`, b.id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p pin
		if err := rows.Scan(&p.id, &p.todoID, &p.position); err != nil {
			return nil, err
		}
		b.pins = append(b.pins, p)
	}
	return b, rows.Err()
}

func (b *bundle) index(todoID int64) int {
	return slices.IndexFunc(b.pins, func(p pin) bool { return p.todoID == todoID })
}

func (b *bundle) include(todoID int64) bool { return b.index(todoID) >= 0 }
func (b *bundle) full() bool                { return len(b.pins) >= limit }

func (b *bundle) todoIDs() []int64 {
	ids := make([]int64, len(b.pins))
	for i, p := range b.pins {
		ids[i] = p.todoID
	}
	return ids
}

// place puts a todo just before another (a todo id), or at the end: added
// if it wasn't here, moved if it was. Five at most: a sixth isn't added
// (false).
func (b *bundle) place(ctx context.Context, q plugin.Querier, todoID, before int64) (bool, error) {
	if b.full() && !b.include(todoID) {
		return false, nil
	}
	list := slices.Clone(b.pins)
	p := pin{todoID: todoID, position: -1}
	if i := b.index(todoID); i >= 0 {
		p = list[i]
		list = slices.Delete(list, i, i+1)
	}
	at := len(list)
	if before != 0 {
		if i := slices.IndexFunc(list, func(x pin) bool { return x.todoID == before }); i >= 0 {
			at = i
		}
	}
	list = slices.Insert(list, at, p)
	return true, b.renumber(ctx, q, list)
}

// add puts several at the end, in order, as far as they hold; it answers
// those that didn't fit.
func (b *bundle) add(ctx context.Context, q plugin.Querier, todoIDs []int64) ([]int64, error) {
	list := slices.Clone(b.pins)
	var skipped []int64
	for _, id := range todoIDs {
		in := slices.ContainsFunc(list, func(p pin) bool { return p.todoID == id })
		if len(list) >= limit && !in {
			skipped = append(skipped, id)
			continue
		}
		if !in {
			list = append(list, pin{todoID: id, position: -1})
		}
	}
	return skipped, b.renumber(ctx, q, list)
}

// move swaps a todo with its neighbour, up or down; false when it can't go
// further (or isn't here).
func (b *bundle) move(ctx context.Context, q plugin.Querier, todoID int64, direction string) (bool, error) {
	i := b.index(todoID)
	if i < 0 {
		return false, nil
	}
	other := i + 1
	if direction == "up" {
		other = i - 1
	}
	if other < 0 || other >= len(b.pins) {
		return false, nil
	}
	list := slices.Clone(b.pins)
	list[i], list[other] = list[other], list[i]
	return true, b.renumber(ctx, q, list)
}

func (b *bundle) unpin(ctx context.Context, q plugin.Querier, todoIDs ...int64) error {
	for _, id := range todoIDs {
		if _, err := q.Exec(ctx, `DELETE FROM my_day_pins WHERE bundle_id = ? AND todo_id = ?`, b.id, id); err != nil {
			return err
		}
	}
	b.pins = slices.DeleteFunc(b.pins, func(p pin) bool { return slices.Contains(todoIDs, p.todoID) })
	return nil
}

// renumber saves list's order: new pins inserted, moved ones updated.
func (b *bundle) renumber(ctx context.Context, q plugin.Querier, list []pin) error {
	now := railsTime()
	for i := range list {
		p := &list[i]
		switch {
		case p.id == 0:
			res, err := q.Exec(ctx, `INSERT INTO my_day_pins (bundle_id, todo_id, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, b.id, p.todoID, i, now, now)
			if err != nil {
				return err
			}
			if p.id, err = res.LastInsertId(); err != nil {
				return err
			}
		case p.position != i:
			if _, err := q.Exec(ctx, `UPDATE my_day_pins SET position = ?, updated_at = ? WHERE id = ?`, i, now, p.id); err != nil {
				return err
			}
		}
		p.position = i
	}
	b.pins = list
	return nil
}

// withPriorities runs fn on the person's bundle in one transaction.
func withPriorities(ctx context.Context, api *plugin.API, userID int64, fn func(q plugin.Querier, b *bundle) error) error {
	return api.Tx(ctx, func(q plugin.Querier) error {
		b, err := prioritiesFor(ctx, q, userID)
		if err != nil {
			return err
		}
		return fn(q, b)
	})
}

// handOver gives a removed person's priorities to their successor, who now
// leads that work: appended after the successor's own, in the removed
// person's order, as far as five hold (the rest are dropped; the work is
// still in the successor's list). Then the removed person's bundles go.
// Hearing it twice changes nothing.
func handOver(ctx context.Context, api *plugin.API, removedID, successorID int64) error {
	if removedID == 0 || successorID == 0 || removedID == successorID {
		return nil
	}
	return api.Tx(ctx, func(q plugin.Querier) error {
		rows, err := q.Query(ctx, `SELECT p.todo_id FROM my_day_pins p JOIN my_day_bundles b ON b.id = p.bundle_id
			WHERE b.user_id = ? ORDER BY b.position, b.id, p.position, p.id`, removedID)
		if err != nil {
			return err
		}
		var todoIDs []int64
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			todoIDs = append(todoIDs, id)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
		if len(todoIDs) > 0 {
			b, err := prioritiesFor(ctx, q, successorID)
			if err != nil {
				return err
			}
			if _, err := b.add(ctx, q, todoIDs); err != nil {
				return err
			}
		}
		if _, err := q.Exec(ctx, `DELETE FROM my_day_pins WHERE bundle_id IN (SELECT id FROM my_day_bundles WHERE user_id = ?)`, removedID); err != nil {
			return err
		}
		_, err = q.Exec(ctx, `DELETE FROM my_day_bundles WHERE user_id = ?`, removedID)
		return err
	})
}
