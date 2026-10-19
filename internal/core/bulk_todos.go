package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Work, many at once (a table's selection, or an agent): status, owner, due
// date, whether the client sees it, made into commitments, or removed. Each
// todo goes through the same rules as changing it alone, one at a time, so
// events are unchanged; one the action doesn't apply to is skipped and named
// in the answer rather than failing the rest (BulkAction).

func init() { routeSets = append(routeSets, (*App).bulkTodoRoutes) }

func (a *App) bulkTodoRoutes(mux *http.ServeMux) {
	a.handle(mux, "PATCH /api/bulk/work", a.bulkUpdateTodos, tool("bulk_update_work", "Change several todos at once",
		note("ids: the todos. Give any of status, owner_id (the lead: a person's id or name, keeping anyone else on it; blank takes everyone off), due_on (blank clears) or client_visible, and each todo changes as update_work would."),
		bodyParam("ids", "array", "The todos' ids.", true),
		body("todo", `{status: planned|in_progress|in_review|blocked|done, owner_id, due_on: YYYY-MM-DD, client_visible: boolean}`, false)))
	a.handle(mux, "DELETE /api/bulk/work", a.bulkDeleteTodos, need("delete_records"), tool("bulk_delete_work", "Remove several todos at once",
		bodyParam("ids", "array", "The todos' ids.", true)))
	a.handle(mux, "POST /api/bulk/work/commitments", a.bulkConvertTodos, tool("bulk_convert_work_to_commitments", "Turn several todos into commitments",
		bodyParam("ids", "array", "The todos' ids.", true),
		bodyParam("owner_kind", "string", "us (the default) or client.", false)))
}

// workBulkMax is the most one bulk action takes.
const workBulkMax = 500

// workBulkIDs are the selected ids: a list or "1,2,3", in the body or the
// query; positive, each once, at most 500.
func workBulkIDs(c *Ctx, body map[string]any) []int64 {
	var raw []string
	switch v := body["ids"].(type) {
	case []any:
		for _, x := range v {
			raw = append(raw, strings.Split(str(x), ",")...)
		}
	case string:
		raw = strings.Split(v, ",")
	}
	for _, q := range c.R.URL.Query()["ids"] {
		raw = append(raw, strings.Split(q, ",")...)
	}
	raw = append(raw, c.R.URL.Query()["ids[]"]...)
	var ids []int64
	for _, s := range raw {
		if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && n > 0 && !slices.Contains(ids, n) {
			ids = append(ids, n)
		}
	}
	if len(ids) > workBulkMax {
		ids = ids[:workBulkMax]
	}
	return ids
}

// workBulkSkip is a record a bulk action passed over, and why.
type workBulkSkip struct{ label, reason string }

// workBulkAnswer says what a bulk action did: "Updated 3 work. Skipped 1:
// “Logo” (it’s done)." Nothing done is a 422 with the reasons.
func workBulkAnswer(c *Ctx, done string, count int, skipped []workBulkSkip) error {
	var parts []string
	if count > 0 {
		parts = append(parts, fmt.Sprintf(done, count))
	}
	if len(skipped) > 0 {
		names := []string{}
		for i, s := range skipped {
			if i == 5 {
				break
			}
			reason := s.reason
			if reason == "" {
				reason = "it couldn’t be changed"
			}
			names = append(names, "“"+s.label+"” ("+reason+")")
		}
		more := ""
		if len(skipped) > 5 {
			more = fmt.Sprintf(" and %d more", len(skipped)-5)
		}
		parts = append(parts, fmt.Sprintf("Skipped %d: %s%s.", len(skipped), strings.Join(names, "; "), more))
	}
	msg := strings.Join(parts, ". ")
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	c.Summary = msg
	reasons := make([]Obj, len(skipped))
	for i, s := range skipped {
		reasons[i] = O("name", s.label, "reason", s.reason)
	}
	if count == 0 {
		return c.JSON(422, O("error", "refused", "message", msg, "summary", msg, "skipped", reasons))
	}
	return c.JSON(200, O("summary", msg, "count", count, "skipped", reasons))
}

// workBulkReason is why one record was skipped: the error's words.
func workBulkReason(err error) string {
	if e, ok := errors.AsType[*apierr.Error](err); ok {
		if len(e.Errors) > 0 {
			var parts []string
			for field, msgs := range e.Errors {
				for _, m := range msgs {
					if field == "base" {
						parts = append(parts, m)
					} else {
						parts = append(parts, humanize(field)+" "+m)
					}
				}
			}
			slices.Sort(parts)
			return strings.Join(parts, ", ")
		}
		msg := e.Message
		if strings.HasPrefix(msg, "It ") || strings.HasPrefix(msg, "It’s") {
			msg = "it" + strings.TrimPrefix(msg, "It")
		}
		return strings.TrimSuffix(msg, ".")
	}
	return ""
}

// selectedTodos are the selected todos that exist, in id order.
func (a *App) selectedTodos(ctx context.Context, ids []int64) ([]*Todo, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	return a.loadTodos(ctx, a.store, `t.id IN (`+store.Placeholders(len(ids))+`) ORDER BY t.id`, idArgs(ids)...)
}

func (a *App) workTerm(ctx context.Context) string {
	s, err := a.settings(ctx)
	if err != nil {
		return "work"
	}
	return strings.ToLower(s.Term("work", 2))
}

func (a *App) bulkUpdateTodos(c *Ctx) error {
	ctx := c.R.Context()
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	given, _ := body["todo"].(map[string]any)
	changes := map[string]any{}
	for _, k := range []string{"status", "owner_id", "due_on", "client_visible"} {
		if v, ok := given[k]; ok {
			changes[k] = v
		}
	}
	if len(changes) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Choose what to change."}
	}
	ch, err := a.todoParams(c, changes, true)
	if err != nil {
		return err
	}
	// Unassigned: nobody on it at all.
	if ch.ownerSet && ch.owner == 0 {
		ch.othersSet, ch.others = true, []int64{}
	}
	todos, err := a.selectedTodos(ctx, workBulkIDs(c, body))
	if err != nil {
		return err
	}
	if len(todos) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Select at least one first."}
	}
	count, skipped := 0, []workBulkSkip{}
	for _, td := range todos {
		if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.changeTodo(td, ch) }); err != nil {
			skipped = append(skipped, workBulkSkip{td.Title, workBulkReason(err)})
			continue
		}
		count++
	}
	return workBulkAnswer(c, "Updated %d "+a.workTerm(ctx), count, skipped)
}

func (a *App) bulkDeleteTodos(c *Ctx) error {
	ctx := c.R.Context()
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	todos, err := a.selectedTodos(ctx, workBulkIDs(c, body))
	if err != nil {
		return err
	}
	if len(todos) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Select at least one first."}
	}
	count, skipped := 0, []workBulkSkip{}
	for _, td := range todos {
		if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteTodo(td.ID) }); err != nil {
			skipped = append(skipped, workBulkSkip{td.Title, "it couldn’t be removed"})
			continue
		}
		count++
	}
	return workBulkAnswer(c, "Removed %d "+a.workTerm(ctx), count, skipped)
}

// bulkConvertTodos makes several todos commitments: each keeps its own date
// (a week from now without one); ones that can't be are skipped.
func (a *App) bulkConvertTodos(c *Ctx) error {
	ctx := c.R.Context()
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	kind := str(body["owner_kind"])
	if !slices.Contains(CommitmentOwnerKinds, kind) {
		kind = "us"
	}
	todos, err := a.selectedTodos(ctx, workBulkIDs(c, body))
	if err != nil {
		return err
	}
	if len(todos) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Select at least one first."}
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	count, skipped := 0, []workBulkSkip{}
	for _, td := range todos {
		err := a.write(ctx, c.Actor, func(t *Tx) error {
			_, err := t.convertTodo(td, kind, "")
			return err
		})
		if err != nil {
			skipped = append(skipped, workBulkSkip{td.Title, workBulkReason(err)})
			continue
		}
		count++
	}
	return workBulkAnswer(c, "Made %d into "+strings.ToLower(s.Term("commitment", 2)), count, skipped)
}
