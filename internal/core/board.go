package core

import (
	"net/http"
	"slices"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// The Work board: one column per status, the planned stream first, the
// doing columns, then done. Each column keeps its own order (board_position),
// set by dragging; a drop moves a todo to a column, before another card or
// to the end of it. Agents change status with update_work instead.

func init() { routeSets = append(routeSets, (*App).boardRoutes) }

func (a *App) boardRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/columns/work/{status}", a.showBoardColumn)
	a.handle(mux, "POST /api/columns/work/{id}/drops/stream", a.dropOnStream)
	a.handle(mux, "POST /api/columns/work/{id}/drops/column", a.dropOnColumn)
	a.handle(mux, "POST /api/columns/work/{id}/drops/closure", a.dropOnClosure)
}

// boardParam is a drop's or a column's param, from the body or the query.
func boardParam(c *Ctx, body map[string]any, key string) string {
	if v := str(body[key]); v != "" {
		return v
	}
	return c.Query(key)
}

// showBoardColumn is one column's cards: done ones by date, the rest in the
// board's order. The board's filters (owner, engagement) apply.
func (a *App) showBoardColumn(c *Ctx) error {
	ctx := c.R.Context()
	status := c.Param("status")
	if !slices.Contains(TodoStatuses, status) {
		return notFound("column")
	}
	where, args, err := a.workFilter(c, strOr(c.Query("owner"), "any"), strOr(c.Query("engagement"), "all"))
	if err != nil {
		return err
	}
	order := "t.board_position, t.id"
	if status == "done" {
		order = "t.due_on, t.position, t.id"
	}
	list, err := a.loadTodos(ctx, a.store, where+" AND t.status = ? ORDER BY "+order, append(args, status)...)
	if err != nil {
		return err
	}
	work, err := a.todosJSON(ctx, a.store, list)
	if err != nil {
		return err
	}
	return c.JSON(200, O("status", status, "count", len(list), "work", work))
}

// moveTodo moves a todo to a status (a board column), at the end of its
// engagement's list for it, and on the board just before `before` (a todo
// in that column) or, without one, at the end (Todo#move!).
func (t *Tx) moveTodo(td *Todo, status string, before int64) error {
	fields := map[string]any{"status": status}
	pos, err := t.nextTodoPosition("position", "engagement_id = ? AND status = ? AND id != ?", td.EngagementID, status, td.ID)
	if err != nil {
		return err
	}
	fields["position"] = pos
	var at int64
	if before != 0 {
		err := t.QueryRow(`SELECT board_position FROM todos WHERE id = ? AND status = ? AND id != ?`, before, status, td.ID).Scan(&at)
		if err != nil && !isNoRows(err) {
			return err
		}
	}
	if at != 0 {
		// Make room: everything from the neighbour on moves down one.
		if _, err := t.Exec(`UPDATE todos SET board_position = board_position + 1 WHERE status = ? AND board_position >= ? AND id != ?`, status, at, td.ID); err != nil {
			return err
		}
		fields["board_position"] = at
	} else {
		board, err := t.nextTodoPosition("board_position", "status = ? AND id != ?", status, td.ID)
		if err != nil {
			return err
		}
		fields["board_position"] = board
	}
	switch {
	case status == "done" && !td.CompletedAt.Valid:
		fields["completed_at"] = store.Now().DB()
	case status != "done" && td.CompletedAt.Valid:
		fields["completed_at"] = nil
	}
	if err := t.update("todos", td.ID, fields); err != nil {
		return err
	}
	t.Reindex("Todo", td.ID)
	if status != td.Status {
		return t.Event("Todo", td.ID, "todo.status", map[string]any{"from": td.Status, "to": status})
	}
	return nil
}

// drop moves the todo and answers it with how many cards its column now
// holds (under the board's filters).
func (a *App) drop(c *Ctx, status func(body map[string]any) (string, error)) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	to, err := status(body)
	if err != nil {
		return err
	}
	td, err := a.todo(ctx, a.store, id)
	if err != nil {
		return err
	}
	var before int64
	if to != "done" {
		before, _ = intOf(boardParam(c, body, "before"))
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.moveTodo(td, to, before) }); err != nil {
		return err
	}
	where, args, err := a.workFilter(c, strOr(boardParam(c, body, "owner"), "any"), strOr(boardParam(c, body, "engagement"), "all"))
	if err != nil {
		return err
	}
	n, err := store.Count(ctx, a.store, `SELECT count(*) FROM todos t WHERE `+where+` AND t.status = ?`, append(args, to)...)
	if err != nil {
		return err
	}
	moved, err := a.todo(ctx, a.store, id)
	if err != nil {
		return err
	}
	list, err := a.todosJSON(ctx, a.store, []*Todo{moved})
	if err != nil {
		return err
	}
	c.Summary = "Moved to " + humanize(to) + "."
	return c.JSON(200, O("summary", c.Summary, "status", to, "count", n, "todo", list[0]))
}

// dropOnStream: dragged to the planned stream.
func (a *App) dropOnStream(c *Ctx) error {
	return a.drop(c, func(map[string]any) (string, error) { return "planned", nil })
}

// dropOnColumn: dragged to a doing column (column_id).
func (a *App) dropOnColumn(c *Ctx) error {
	return a.drop(c, func(body map[string]any) (string, error) {
		col := boardParam(c, body, "column_id")
		if !slices.Contains(TodoBoardColumns, col) {
			return "", apierr.NotFound("Couldn't find that column.")
		}
		return col, nil
	})
}

// dropOnClosure: dragged to done.
func (a *App) dropOnClosure(c *Ctx) error {
	return a.drop(c, func(map[string]any) (string, error) { return "done", nil })
}
