// Package myday is My day, ported from the Rails plugin
// (github.com/Martin-Business-Consultants/plugin-my-day) onto the Go plugin
// API alone: the work you lead or are on in one list, searched and filtered,
// with the piece you pick open beside it (its own Svelte screen,
// ui/src/plugins/my_day), and the work you drag to the top as your
// priorities, in the order you mean to do it. It keeps the Rails tables
// (my_day_bundles, my_day_pins), so an imported Rails database's priorities
// are its own; work is read with Records and changed with ChangeWork.
package myday

import (
	"context"
	"encoding/json"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Key is the plugin's key.
const Key = "my_day"

// schema is the Rails migration's final state (CreateMyDay), table for
// table and column for column, index for index.
const schema = `
CREATE TABLE IF NOT EXISTS "my_day_bundles" (
  "id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
  "user_id" integer NOT NULL,
  "name" varchar NOT NULL,
  "position" integer DEFAULT 0 NOT NULL,
  "created_at" datetime(6) NOT NULL,
  "updated_at" datetime(6) NOT NULL
);
CREATE INDEX IF NOT EXISTS "index_my_day_bundles_on_user_id" ON "my_day_bundles" ("user_id");
CREATE TABLE IF NOT EXISTS "my_day_pins" (
  "id" integer PRIMARY KEY AUTOINCREMENT NOT NULL,
  "bundle_id" integer NOT NULL,
  "todo_id" integer NOT NULL,
  "position" integer DEFAULT 0 NOT NULL,
  "created_at" datetime(6) NOT NULL,
  "updated_at" datetime(6) NOT NULL,
  CONSTRAINT "fk_rails_my_day_pins_bundle" FOREIGN KEY ("bundle_id") REFERENCES "my_day_bundles" ("id")
);
CREATE INDEX IF NOT EXISTS "index_my_day_pins_on_bundle_id" ON "my_day_pins" ("bundle_id");
CREATE INDEX IF NOT EXISTS "index_my_day_pins_on_todo_id" ON "my_day_pins" ("todo_id");
CREATE UNIQUE INDEX IF NOT EXISTS "index_my_day_pins_on_bundle_id_and_todo_id" ON "my_day_pins" ("bundle_id", "todo_id");
`

// Module is the plugin, for the core to register.
func Module() *plugin.Module {
	return &plugin.Module{
		Manifest: plugin.Manifest{
			Key:      Key,
			Name:     "My day",
			Version:  "0.1.0",
			Author:   "Martin Business Consultants",
			Homepage: "https://github.com/Martin-Business-Consultants/plugin-my-day",
			Description: "Your work in one list, searched and filtered, with the one you pick open beside it, and the work " +
				"you drag to the top as your priorities, in the order you mean to do it: a way to work through the day.",
			Permissions: plugin.Grants{RecordsRead: true, RecordsWrite: true, Tables: true},
			Schema:      schema,
			// Work marked done anywhere falls off everyone's priorities; a
			// removed person's priorities go to whoever takes their work.
			Hooks: []string{"todo.status", "user.removed"},
			Routes: []plugin.Route{
				{Method: "GET", Path: "/", Export: "day", Tool: &plugin.Tool{
					Name: "show_my_day", Title: "Show my day",
					Description: "Your priorities (the work you've put at the top of your day, in order), then the work you lead or are on in groups by when it's due (late, today, this week, later, no date, done). show: open (the default), today (due today or late), week, done (this week) or all. q: words in the title, its engagement or its client. client / engagement: only work under that client or engagement (ids; the answer lists the choices). todo: one to show in full.\n\nOften next: add_my_day_priority, set_my_day_status.",
					Query: []plugin.ToolParam{
						{Name: "show", Description: "open, today, week, done or all."},
						{Name: "q", Description: "Words in the title, its engagement or its client."},
						{Name: "client", Type: "integer", Description: "Only work for this client (its id)."},
						{Name: "engagement", Type: "integer", Description: "Only work under this engagement (its id)."},
						{Name: "todo", Type: "integer", Description: "One to show in full."},
					},
				}},
				{Method: "POST", Path: "/priorities/{todo_id}", Export: "addPriority", Tool: &plugin.Tool{
					Name: "add_my_day_priority", Title: "Add work to my priorities",
					Description: "Puts a piece of work in your priorities at the top of your day: at the end, or just before another (before: its todo id). Already there? It moves. They hold five; a sixth is refused.\n\nOften next: show_my_day.",
					Params:      []plugin.ToolParam{{Name: "before", Type: "integer", Description: "A todo id in your priorities to put it just before."}},
				}},
				{Method: "PATCH", Path: "/priorities/{todo_id}", Export: "movePriority", Tool: &plugin.Tool{
					Name: "move_my_day_priority", Title: "Move work up or down my priorities",
					Description: "One place up or down.",
					Params:      []plugin.ToolParam{{Name: "direction", Description: "up or down.", Required: true}},
				}},
				{Method: "DELETE", Path: "/priorities/{todo_id}", Export: "removePriority", Tool: &plugin.Tool{
					Name: "remove_my_day_priority", Title: "Remove work from my priorities",
					Description: "Takes it off the top of your day; the work itself stays as it is.",
				}},
				{Method: "POST", Path: "/bulk/priorities", Export: "bulkAdd", Tool: &plugin.Tool{
					Name: "bulk_add_my_day_priorities", Title: "Add several pieces of work to my priorities",
					Description: "At the end, in the order given, as far as the priorities hold (five).\n\nOften next: show_my_day.",
					Params:      []plugin.ToolParam{{Name: "ids", Type: "array", Description: "The todos' ids.", Required: true}},
				}},
				{Method: "DELETE", Path: "/bulk/priorities", Export: "bulkRemove", Tool: &plugin.Tool{
					Name: "bulk_remove_my_day_priorities", Title: "Remove several pieces of work from my priorities",
					Description: "Often next: show_my_day.",
					Params:      []plugin.ToolParam{{Name: "ids", Type: "array", Description: "The todos' ids.", Required: true}},
				}},
				{Method: "PATCH", Path: "/todos/{todo_id}/status", Export: "status", Tool: &plugin.Tool{
					Name: "set_my_day_status", Title: "Set a piece of work's status from my day",
					Description: "planned, in_progress, in_review, blocked or done. As an agent, finish into in_review for a person to mark done.\n\nOften next: show_my_day.",
					Params:      []plugin.ToolParam{{Name: "status", Description: "planned, in_progress, in_review, blocked or done.", Required: true}},
				}},
				{Method: "PATCH", Path: "/todos/{id}", Export: "update", Tool: &plugin.Tool{
					Name: "update_my_day_todo", Title: "Change a piece of work from my day",
					Description: "Its due date (blank to clear) or its details (rich text, @mentions welcome).\n\nOften next: show_my_day.",
					Wrap:        "todo", Body: `{due_on: YYYY-MM-DD (blank clears), description: rich text}`,
				}},
				// A drop onto the list takes work off (the drag-and-drop posts).
				{Method: "POST", Path: "/priorities/{todo_id}/removal", Export: "removePriority"},
				// Who's on it, from the card's people picker (the Rails card used the
				// core's owner picker, which has no agent tool of its own here).
				{Method: "PATCH", Path: "/todos/{todo_id}/people", Export: "people"},
			},
			Nav: &plugin.NavPage{Label: "My day", Path: "/my-day", Icon: "sun"},
		},
		Handlers: map[string]plugin.Handler{
			"day": showDay, "addPriority": addPriority, "movePriority": movePriority, "removePriority": removePriority,
			"bulkAdd": bulkAdd, "bulkRemove": bulkRemove, "status": setStatus, "update": updateTodo, "people": setPeople,
		},
		OnEvent: onEvent,
	}
}

// onEvent drops a todo from everyone's priorities once it's done, wherever
// it was marked (the Rails my_day.events subscriber), and hands a removed
// person's priorities to their successor.
func onEvent(ctx context.Context, api *plugin.API, kind string, data json.RawMessage) error {
	var ev struct {
		SubjectType string         `json:"subject_type"`
		SubjectID   int64          `json:"subject_id"`
		Payload     map[string]any `json:"payload"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	if kind == "user.removed" {
		var removed struct {
			Payload struct {
				ID          int64 `json:"id"`
				SuccessorID int64 `json:"successor_id"`
			} `json:"payload"`
		}
		if err := json.Unmarshal(data, &removed); err != nil {
			return err
		}
		return handOver(ctx, api, removed.Payload.ID, removed.Payload.SuccessorID)
	}
	if kind != "todo.status" || ev.SubjectType != "Todo" || ev.Payload["to"] != "done" {
		return nil
	}
	_, err := api.Exec(ctx, `DELETE FROM my_day_pins WHERE todo_id = ?`, ev.SubjectID)
	return err
}
