package websitechecklist

import (
	"context"
	"encoding/json"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// onEvent hears a client deleted, and deletes their websites, as the Rails
// client's dependent: :destroy did: each one's deletion is in the history
// (kept), as whoever deleted the client.
func onEvent(ctx context.Context, api *plugin.API, kind string, data json.RawMessage) error {
	if kind == "user.removed" {
		return userRemoved(ctx, api, data)
	}
	if kind != "client.deleted" {
		return nil
	}
	var ev struct {
		SubjectID   int64          `json:"subject_id"`
		Payload     map[string]any `json:"payload"`
		ActorUserID int64          `json:"actor_user_id"`
		Actor       string         `json:"actor"`
		Source      string         `json:"source"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	sites, err := allSites(ctx, api, ev.SubjectID)
	if err != nil {
		return err
	}
	if len(sites) == 0 {
		return nil
	}
	// Who deleted the client: a person on staff, or an agent through an app.
	var actor *plugin.Caller
	if ev.ActorUserID != 0 {
		actor = &plugin.Caller{ID: ev.ActorUserID, Agent: ev.Source == "agent"}
		if users, err := api.Records(ctx, plugin.Query{Type: "User", ID: ev.ActorUserID}); err == nil && len(users) > 0 {
			actor.Name, _ = users[0]["name"].(string)
		}
		if actor.Agent {
			actor.AgentID, actor.Via = ev.ActorUserID, ev.Actor
		}
	}
	name, _ := ev.Payload["name"].(string)
	for _, s := range sites {
		if err := deleteSite(ctx, api, actor, s, name); err != nil {
			return err
		}
	}
	return nil
}

// userRemoved hands a removed person's scans to their successor (whose the
// recommendations now are to decide), and keeps the history true: what they
// (or their agent) did stays under their name, without an id that points
// at no one.
func userRemoved(ctx context.Context, api *plugin.API, data json.RawMessage) error {
	var ev struct {
		Payload struct {
			ID          int64  `json:"id"`
			AgentID     int64  `json:"agent_id"`
			Name        string `json:"name"`
			SuccessorID int64  `json:"successor_id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	p := ev.Payload
	if p.ID == 0 || p.SuccessorID == 0 || p.ID == p.SuccessorID {
		return nil
	}
	return api.Tx(ctx, func(q plugin.Querier) error {
		if _, err := q.Exec(ctx, `UPDATE website_checklist_scans SET user_id = ? WHERE user_id = ?`, p.SuccessorID, p.ID); err != nil {
			return err
		}
		for _, id := range []int64{p.ID, p.AgentID} {
			if id == 0 {
				continue
			}
			name := p.Name
			if id == p.AgentID && name != "" {
				name += "'s agent"
			}
			if _, err := q.Exec(ctx, `UPDATE website_checklist_audits SET actor_name = ? WHERE actor_id = ? AND COALESCE(actor_name, '') = ''`, nilIfBlank(name), id); err != nil {
				return err
			}
			if _, err := q.Exec(ctx, `UPDATE website_checklist_audits SET actor_id = NULL WHERE actor_id = ?`, id); err != nil {
				return err
			}
		}
		return nil
	})
}
