package chat

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// When someone is removed from Runwell (the core's user.removed, heard
// once their user is gone), chat keeps what they said and hands on what
// they ran: their messages and reactions stay as written, under
// "Name (removed)" (chat_removed_people keeps the name, since Runwell no
// longer can); the channels they made go to whoever took over their work,
// who joins them if they weren't in; they leave every channel, but stay in
// their direct messages and groups so the others can still read them; and
// their chat settings and picture go.

// removal is user.removed's payload, as chat needs it.
type removal struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	SuccessorID int64  `json:"successor_id"`
}

func onEvent(ctx context.Context, api *plugin.API, kind string, data json.RawMessage) error {
	if kind != "user.removed" {
		return nil
	}
	var ev struct {
		Payload removal `json:"payload"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	return removePerson(ctx, api, ev.Payload)
}

func removePerson(ctx context.Context, api *plugin.API, r removal) error {
	if r.ID <= 0 {
		return nil
	}
	p, err := profileOf(ctx, api, r.ID)
	if err != nil {
		return err
	}
	name := strings.TrimSpace(r.Name)
	if name == "" {
		name = p.DisplayName
	}
	now := stamp()
	err = api.Tx(ctx, func(q plugin.Querier) error {
		if _, err := q.Exec(ctx, `INSERT INTO chat_removed_people (user_id, name, successor_id, removed_at) VALUES (?, ?, ?, ?)
			ON CONFLICT (user_id) DO UPDATE SET name = excluded.name, successor_id = excluded.successor_id`, r.ID, name, nullID(r.SuccessorID), now); err != nil {
			return err
		}
		if r.SuccessorID > 0 && r.SuccessorID != r.ID {
			// The channels they made: the successor manages them now, from
			// inside (a private one they couldn't otherwise see).
			rows, err := q.Query(ctx, `SELECT id FROM chat_conversations WHERE kind = 'channel' AND created_by_id = ?`, r.ID)
			if err != nil {
				return err
			}
			var channels []int64
			for rows.Next() {
				var id int64
				if err := rows.Scan(&id); err != nil {
					rows.Close()
					return err
				}
				channels = append(channels, id)
			}
			rows.Close()
			for _, id := range channels {
				if err := addTo(ctx, q, id, []int64{r.SuccessorID}, now); err != nil {
					return err
				}
			}
			if _, err := q.Exec(ctx, `UPDATE chat_conversations SET created_by_id = ?, updated_at = ? WHERE kind = 'channel' AND created_by_id = ?`, r.SuccessorID, now, r.ID); err != nil {
				return err
			}
		}
		if _, err := q.Exec(ctx, `DELETE FROM chat_members WHERE user_id = ? AND conversation_id IN (SELECT id FROM chat_conversations WHERE kind = 'channel')`, r.ID); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `DELETE FROM chat_profiles WHERE user_id = ?`, r.ID)
		return err
	})
	if err != nil {
		return err
	}
	if p.AvatarFileID > 0 {
		_ = api.Detach(ctx, "Chat::Profile", r.ID, p.AvatarFileID)
	}
	return nil
}

// removedPeople are the people removed from Runwell who once chatted, by
// id: the name chat calls them now.
func removedPeople(ctx context.Context, q plugin.Querier) (map[int64]string, error) {
	rows, err := q.Query(ctx, `SELECT user_id, name FROM chat_removed_people`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]string{}
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = removedName(name)
	}
	return out, rows.Err()
}

func removedName(name string) string {
	if name == "" {
		name = "Someone"
	}
	return name + " (removed)"
}

// formerJSON are the removed people, as the app names them where they
// still appear (an old direct message, a message's author).
func (d *directory) formerJSON() []map[string]any {
	out := []map[string]any{}
	for id, name := range d.removed {
		if _, still := d.people[id]; still {
			continue
		}
		out = append(out, map[string]any{"id": id, "name": name, "full_name": name, "avatar_url": avatarURL(id, profile{}),
			"presence": "offline", "available": false, "active": false, "removed": true})
	}
	return out
}
