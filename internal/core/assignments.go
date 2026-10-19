package core

import (
	"net/http"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Who's on a piece of work: its owner (the lead) and anyone else, Fizzy's
// assignments. Everyone on it, the lead too, has an assignment, so "my work"
// is work anyone gave me a part in. Setting the owner directly (a form, a
// bulk change, an agent) replaces the lead and keeps the rest.

func init() { routeSets = append(routeSets, (*App).assignmentRoutes) }

func (a *App) assignmentRoutes(mux *http.ServeMux) {
	// The owner picker's toggle. Agents set the lead with update_work's
	// owner_id and everyone else with other_owner_ids, so there's no tool.
	a.handle(mux, "POST /api/work/{id}/assignments", a.toggleAssignment)
}

// assign puts someone on a todo (assigned by whoever's acting); the first on
// leads.
func (t *Tx) assign(todoID, userID int64) error {
	if err := t.addAssignment(todoID, userID); err != nil {
		return err
	}
	var owner store.NullInt
	if err := t.QueryRow(`SELECT owner_id FROM todos WHERE id = ?`, todoID).Scan(&owner); err != nil {
		return err
	}
	if !owner.Valid {
		return t.lead(todoID, userID)
	}
	return nil
}

// unassign takes someone off a todo; when that was the lead, whoever came
// on next leads, or nobody.
func (t *Tx) unassign(todoID, userID int64) error {
	if _, err := t.Exec(`DELETE FROM assignments WHERE todo_id = ? AND user_id = ?`, todoID, userID); err != nil {
		return err
	}
	var owner store.NullInt
	if err := t.QueryRow(`SELECT owner_id FROM todos WHERE id = ?`, todoID).Scan(&owner); err != nil {
		return err
	}
	if owner.Valid && owner.Int64 == userID {
		return t.lead(todoID, t.firstAssignee(todoID))
	}
	return nil
}

// assignedTo is whether someone leads a todo or is on it.
func (t *Tx) assignedTo(todoID, userID int64) (bool, error) {
	n, err := store.Count(t.Ctx, t, `SELECT (SELECT count(*) FROM todos WHERE id = ? AND owner_id = ?) + (SELECT count(*) FROM assignments WHERE todo_id = ? AND user_id = ?)`,
		todoID, userID, todoID, userID)
	return n > 0, err
}

// lead makes someone the lead (0: nobody) without touching who else is on
// it (Rails' lead!, which skips follow_lead).
func (t *Tx) lead(todoID, userID int64) error {
	var owner any
	if userID != 0 {
		owner = userID
	}
	_, err := t.Exec(`UPDATE todos SET owner_id = ?, updated_at = ? WHERE id = ?`, owner, store.Now().DB(), todoID)
	return err
}

func (t *Tx) addAssignment(todoID, userID int64) error {
	now := store.Now().DB()
	_, err := t.Exec(`INSERT INTO assignments (todo_id, user_id, assigner_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (todo_id, user_id) DO NOTHING`, todoID, userID, t.actorPersonID(), now, now)
	return err
}

// firstAssignee is whoever came on first, 0 for nobody.
func (t *Tx) firstAssignee(todoID int64) int64 {
	var id int64
	_ = t.QueryRow(`SELECT user_id FROM assignments WHERE todo_id = ? ORDER BY created_at, id LIMIT 1`, todoID).Scan(&id)
	return id
}

// followLead keeps assignments in step when the owner was set directly:
// the old lead comes off, the new one goes on. With the lead cleared (and no
// other_owner_ids given), whoever else is on it leads.
func (t *Tx) followLead(todoID, before, now int64, othersGiven bool) error {
	if before != 0 {
		if _, err := t.Exec(`DELETE FROM assignments WHERE todo_id = ? AND user_id = ?`, todoID, before); err != nil {
			return err
		}
	}
	if now != 0 {
		return t.addAssignment(todoID, now)
	}
	if !othersGiven {
		if first := t.firstAssignee(todoID); first != 0 {
			return t.lead(todoID, first)
		}
	}
	return nil
}

// applyOtherOwners makes everyone else on a todo exactly these (active
// people only); with no lead, the first leads.
func (t *Tx) applyOtherOwners(todoID int64, ids []int64) error {
	var people []int64
	if ids = workIDs(ids); len(ids) > 0 {
		active, err := store.Int64s(t.Ctx, t, `SELECT id FROM users WHERE deactivated_at IS NULL AND agent_owner_id IS NULL AND id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...)
		if err != nil {
			return err
		}
		for _, id := range ids { // in the order given
			for _, ok := range active {
				if ok == id {
					people = append(people, id)
				}
			}
		}
	}
	var owner store.NullInt
	if err := t.QueryRow(`SELECT owner_id FROM todos WHERE id = ?`, todoID).Scan(&owner); err != nil {
		return err
	}
	if !owner.Valid && len(people) > 0 {
		if err := t.lead(todoID, people[0]); err != nil {
			return err
		}
		owner.Int64, owner.Valid = people[0], true
	}
	wanted := people
	if owner.Valid {
		wanted = workIDs(append([]int64{owner.Int64}, people...))
	}
	if len(wanted) == 0 {
		_, err := t.Exec(`DELETE FROM assignments WHERE todo_id = ?`, todoID)
		return err
	}
	if _, err := t.Exec(`DELETE FROM assignments WHERE todo_id = ? AND user_id NOT IN (`+store.Placeholders(len(wanted))+`)`, append([]any{todoID}, idArgs(wanted)...)...); err != nil {
		return err
	}
	for _, id := range wanted {
		if err := t.addAssignment(todoID, id); err != nil {
			return err
		}
	}
	return nil
}

// toggleAssignment is the picker's click: on if they're off it, off if
// they're on. The answer is the todo as it now stands.
func (a *App) toggleAssignment(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	if _, err := a.todo(ctx, a.store, id); err != nil {
		return err
	}
	personID, _ := intOf(body["assignee_id"])
	var name string
	if err := a.store.QueryRowContext(ctx, `SELECT CASE WHEN name = '' THEN email_address ELSE name END FROM users WHERE id = ? AND deactivated_at IS NULL AND agent_owner_id IS NULL`, personID).Scan(&name); err != nil {
		return notFound("person")
	}
	on := false
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		was, err := t.assignedTo(id, personID)
		if err != nil {
			return err
		}
		if was {
			err = t.unassign(id, personID)
		} else {
			err = t.assign(id, personID)
		}
		on = !was
		return err
	})
	if err != nil {
		return err
	}
	o, _, err := a.todoShowJSON(ctx, id)
	if err != nil {
		return err
	}
	c.Summary = name + " is off it."
	if on {
		c.Summary = name + " is on it."
	}
	return c.JSON(200, O("summary", c.Summary, "todo", o))
}
