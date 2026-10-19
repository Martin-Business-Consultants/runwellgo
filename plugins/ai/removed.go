package ai

import (
	"context"
	"encoding/json"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// When someone is removed from Runwell (the core's user.removed, heard
// once their user is gone), what they set up and what waits on them goes
// to whoever took over their work, and what they did stays theirs: the
// agents they made and the clients they kept out of AI go to the
// successor; approvals waiting on them (named as approver) wait on the
// successor instead, with the runs paused on those approvals, so the
// successor's decision carries them on as the successor; runs still queued
// or working stop (nobody is left to act as). Their Ask chats, finished
// runs, suggestions and feedback are kept, under "Name (removed)"
// (ai_removed_people keeps the name, since Runwell no longer can).

type removal struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	SuccessorID int64  `json:"successor_id"`
}

func removedPerson(ctx context.Context, api *plugin.API, data json.RawMessage) error {
	var ev struct {
		Payload removal `json:"payload"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	r := ev.Payload
	if r.ID <= 0 {
		return nil
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = "Someone"
	}
	succ := r.SuccessorID
	if succ == r.ID {
		succ = 0
	}
	now := store.Now().DB()
	var stopped []int64
	err := api.Tx(ctx, func(q plugin.Querier) error {
		if _, err := q.Exec(ctx, `INSERT INTO ai_removed_people (user_id, name, successor_id, removed_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET name = excluded.name, successor_id = excluded.successor_id`, r.ID, name, nullID(succ), now); err != nil {
			return err
		}
		// Runs still to do or under way: nobody is left to act as.
		rows, err := q.Query(ctx, `SELECT id FROM ai_runs WHERE user_id = ? AND status IN ('queued', 'working')`, r.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				rows.Close()
				return err
			}
			stopped = append(stopped, id)
		}
		rows.Close()
		for _, id := range stopped {
			if _, err := q.Exec(ctx, `UPDATE ai_runs SET status = 'cancelled', error = ?, finished_at = ?, updated_at = ? WHERE id = ?`,
				name+" was removed from Runwell before this finished.", now, now, id); err != nil {
				return err
			}
			if _, err := q.Exec(ctx, `UPDATE ai_approvals SET status = 'withdrawn', updated_at = ? WHERE run_id = ? AND status = 'waiting'`, now, id); err != nil {
				return err
			}
		}
		if succ == 0 {
			return nil
		}
		for _, stmt := range []string{
			`UPDATE ai_agents SET created_by_id = ? WHERE created_by_id = ?`,
			`UPDATE ai_exclusions SET user_id = ? WHERE user_id = ?`,
		} {
			if _, err := q.Exec(ctx, stmt, succ, r.ID); err != nil {
				return err
			}
		}
		// Approvals waiting on them wait on the successor.
		arows, err := q.Query(ctx, `SELECT id, COALESCE(approvers, '{}'), COALESCE(run_id, 0) FROM ai_approvals WHERE status = 'waiting'`)
		if err != nil {
			return err
		}
		type moved struct {
			id, run int64
			ap      approvers
		}
		var list []moved
		for arows.Next() {
			var m moved
			var raw string
			if err := arows.Scan(&m.id, &raw, &m.run); err != nil {
				arows.Close()
				return err
			}
			_ = json.Unmarshal([]byte(raw), &m.ap)
			if !slices.Contains(m.ap.UserIDs, r.ID) {
				continue
			}
			ids := []int64{}
			for _, id := range m.ap.UserIDs {
				if id == r.ID {
					id = succ
				}
				if !slices.Contains(ids, id) {
					ids = append(ids, id)
				}
			}
			m.ap.UserIDs = ids
			list = append(list, m)
		}
		arows.Close()
		for _, m := range list {
			raw, _ := json.Marshal(m.ap)
			if _, err := q.Exec(ctx, `UPDATE ai_approvals SET approvers = ?, updated_at = ? WHERE id = ?`, string(raw), now, m.id); err != nil {
				return err
			}
		}
		// A run paused on an approval goes on as whoever decides it.
		if _, err := q.Exec(ctx, `UPDATE ai_chats SET user_id = ?, updated_at = ? WHERE id IN (SELECT ai_chat_id FROM ai_runs WHERE user_id = ? AND status = 'waiting')`, succ, now, r.ID); err != nil {
			return err
		}
		_, err = q.Exec(ctx, `UPDATE ai_runs SET user_id = ?, updated_at = ? WHERE user_id = ? AND status = 'waiting'`, succ, now, r.ID)
		return err
	})
	if err != nil {
		return err
	}
	queue.Lock()
	for _, id := range stopped {
		if cancel := queue.cancels[id]; cancel != nil {
			cancel()
		}
		kept := queue.waiting[:0]
		for _, q := range queue.waiting {
			if q.id != id {
				kept = append(kept, q)
			}
		}
		queue.waiting = kept
	}
	queue.Unlock()
	return nil
}

// withRemoved names the people removed from Runwell that people (a
// lookup of those still on staff) lacks: "Name (removed)".
func withRemoved(ctx context.Context, api *plugin.API, people map[int64]string) map[int64]string {
	rows, err := api.Query(ctx, `SELECT user_id, name FROM ai_removed_people`)
	if err != nil {
		return people
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil && people[id] == "" {
			people[id] = name + " (removed)"
		}
	}
	return people
}
