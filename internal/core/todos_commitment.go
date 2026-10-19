package core

import (
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
)

// Make this a commitment: work that was really a promise ("Send the invoice
// by Friday") becomes one on the engagement's client (Todo#convert_to_commitment!).

func init() { routeSets = append(routeSets, (*App).todoCommitmentRoutes) }

func (a *App) todoCommitmentRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/work/{id}/commitment", a.convertTodoHandler, tool("convert_work_to_commitment", "Turn a todo into a commitment",
		note("For work that is really a dated promise. owner_kind: us (the todo's owner, else you) or client. due_on: blank keeps the todo's date (or a week from now). Its notes and documents move to the engagement; refused if a plugin keeps something on it (logged time, a QA gate) or it's done."),
		bodyParam("owner_kind", "string", "us (the default) or client.", false),
		bodyParam("due_on", "string", "YYYY-MM-DD; blank keeps the todo's date, else a week from now.", false)))
}

// convertTodo makes a todo a commitment: ours (its owner's, else the
// actor's) or the client's, due when it was due (or the date given, else in
// a week). Its notes and documents move to the engagement, the engagement's
// history records the change, and the work goes. Refused once it's done.
func (t *Tx) convertTodo(td *Todo, ownerKind, dueOn string) (int64, error) {
	if td.Status == "done" {
		return 0, &apierr.Error{Status: 422, Code: "refused", Message: "It’s done; a commitment is for what’s still to come."}
	}
	var clientID int64
	if err := t.QueryRow(`SELECT client_id FROM engagements WHERE id = ?`, td.EngagementID).Scan(&clientID); err != nil {
		return 0, err
	}
	kind := ownerKind
	if !slices.Contains(CommitmentOwnerKinds, kind) {
		kind = "us"
	}
	var user any
	if ownerKind != "client" {
		if td.OwnerID != 0 {
			user = td.OwnerID
		} else if t.Actor != nil && t.Actor.User != nil {
			user = t.Actor.User.Person().ID
		}
	}
	due := workDay(dueOn)
	if due == nil && td.DueOn != "" {
		due = td.DueOn
	}
	if due == nil {
		due = time.Now().In(t.App.zone(t.Ctx)).AddDate(0, 0, 7).Format("2006-01-02")
	}
	id, err := t.insertCommitment(clientID, td.EngagementID, map[string]any{
		"description": td.Title, "owner_kind": kind, "user_id": user, "contact_id": nil, "due_on": due, "source": "work",
	}, map[string]any{"from_work": td.ID})
	if err != nil {
		return 0, err
	}
	for _, q := range []string{
		`UPDATE notes SET subject_type = 'Engagement', subject_id = ? WHERE subject_type = 'Todo' AND subject_id = ?`,
		`UPDATE documents SET documentable_type = 'Engagement', documentable_id = ? WHERE documentable_type = 'Todo' AND documentable_id = ?`,
	} {
		if _, err := t.Exec(q, td.EngagementID, td.ID); err != nil {
			return 0, err
		}
	}
	if err := t.Event("Engagement", td.EngagementID, "todo.converted", map[string]any{"title": td.Title, "commitment": id}); err != nil {
		return 0, err
	}
	return id, t.deleteTodo(td.ID)
}

func (a *App) convertTodoHandler(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	td, err := a.todo(ctx, a.store, id)
	if err != nil {
		return err
	}
	var cid int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		cid, err = t.convertTodo(td, str(body["owner_kind"]), str(body["due_on"]))
		return err
	})
	if err != nil {
		return err
	}
	cm, err := a.commitment(ctx, a.store, cid, false)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	c.Summary = "“" + td.Title + "” is now a " + strings.ToLower(s.Term("commitment", 1)) + ": " + cm.OwnerName + " by " + commitmentDate(cm.DueOn) + "."
	return c.JSON(201, O("summary", c.Summary, "commitment", a.commitmentJSON(cm, a.today(ctx)), "path", "/engagements/"+cm.EngagementRef+"?tab=commitments"))
}
