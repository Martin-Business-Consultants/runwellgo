package core

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Questions: something only a person can answer, asked by an agent (or a
// plugin) about a record. The main human inbox: home lists a person's open
// ones, each answered with a choice or in words, or dismissed.

func init() { routeSets = append(routeSets, (*App).questionRoutes) }

func (a *App) questionRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/questions/{id}/answer", a.answerQuestion, tool("answer_question", "Answer a question",
		bodyParam("answer", "string", "The answer: one of its choices, or words of your own.", true)))
	a.handle(mux, "POST /api/questions/{id}/dismiss", a.dismissQuestion, tool("dismiss_question", "Dismiss a question"))
}

// Question is one question for a person.
type Question struct {
	ID          int64
	UserID      int64
	Text        string
	Choices     []any
	SubjectType string
	SubjectID   int64
	Context     string
	AskedBy     string
	Source      string
	Answer      string
	AnsweredAt  store.Time
	DismissedAt store.Time
	CreatedAt   store.Time
}

const questionColumns = `id, user_id, text, choices, COALESCE(subject_type, ''), COALESCE(subject_id, 0), COALESCE(context, ''), asked_by, source,
	COALESCE(answer, ''), answered_at, dismissed_at, created_at`

func scanQuestion(r interface{ Scan(...any) error }) (*Question, error) {
	var q Question
	var choices store.JSON
	err := r.Scan(&q.ID, &q.UserID, &q.Text, &choices, &q.SubjectType, &q.SubjectID, &q.Context, &q.AskedBy, &q.Source,
		&q.Answer, &q.AnsweredAt, &q.DismissedAt, &q.CreatedAt)
	q.Choices = choices.List()
	if q.Choices == nil {
		q.Choices = []any{}
	}
	return &q, err
}

// Open is whether it still waits for an answer.
func (q *Question) Open() bool { return !q.AnsweredAt.Valid && !q.DismissedAt.Valid }

// questionFor is one of the person's own questions; notFound for anyone
// else's (current_user.questions.find).
func (a *App) questionFor(c *Ctx) (*Question, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, err
	}
	q, err := scanQuestion(a.store.QueryRowContext(c.R.Context(), `SELECT `+questionColumns+` FROM questions WHERE id = ? AND user_id = ?`, id, questionUserID(c.Actor)))
	if isNoRows(err) {
		return nil, notFound("question")
	}
	return q, err
}

// questionUserID is whose questions the actor answers: the person, also
// when their agent asks on their behalf.
func questionUserID(actor *Actor) int64 {
	if actor == nil || actor.User == nil {
		return 0
	}
	return actor.User.Person().ID
}

// openQuestions are the person's unanswered questions, oldest first, and
// how many there are.
func (a *App) openQuestions(ctx context.Context, userID int64, limit int) ([]*Question, int, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT `+questionColumns+` FROM questions WHERE user_id = ? AND answered_at IS NULL AND dismissed_at IS NULL ORDER BY created_at ASC LIMIT ?`, userID, limit)
	if err != nil {
		return nil, 0, err
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (*Question, error) { return scanQuestion(r) })
	if err != nil {
		return nil, 0, err
	}
	n := len(list)
	if n >= limit {
		if n, err = store.Count(ctx, a.store, `SELECT count(*) FROM questions WHERE user_id = ? AND answered_at IS NULL AND dismissed_at IS NULL`, userID); err != nil {
			return nil, 0, err
		}
	}
	return list, n, nil
}

// questionsJSON are questions as the briefing lists them: the agent_ref,
// choices and context; and for the app, who asked, from where, and the
// record it's about.
func (a *App) questionsJSON(ctx context.Context, list []*Question) ([]Obj, error) {
	subjects := map[string]map[int64]Obj{}
	byType := map[string][]int64{}
	for _, q := range list {
		if q.SubjectType == "Engagement" || q.SubjectType == "Client" {
			byType[q.SubjectType] = append(byType[q.SubjectType], q.SubjectID)
		}
	}
	for typ, ids := range byType {
		subjects[typ] = map[int64]Obj{}
		query := `SELECT id, name, '' FROM clients WHERE id IN (%s)`
		if typ == "Engagement" {
			query = `SELECT id, title, ref FROM engagements WHERE id IN (%s)`
		}
		err := eachRow(ctx, a.store, fmt.Sprintf(query, store.Placeholders(len(ids))), idArgs(ids), func(rows *sql.Rows) error {
			var id int64
			var name, ref string
			if err := rows.Scan(&id, &name, &ref); err != nil {
				return err
			}
			if typ == "Engagement" {
				subjects[typ][id] = a.agentRef(typ, id, ref, ref+" "+name, "/engagements/"+ref)
			} else {
				subjects[typ][id] = a.agentRef(typ, id, "", name, fmt.Sprintf("/clients/%d", id))
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	out := make([]Obj, len(list))
	for i, q := range list {
		var subject any
		if s, ok := subjects[q.SubjectType][q.SubjectID]; ok {
			subject = s
		}
		out[i] = a.agentRef("Question", q.ID, "", q.Text, "/").
			Set("choices", q.Choices).Set("context", nullable(q.Context)).
			Set("text", q.Text).Set("asked_by", q.AskedBy).Set("source", q.Source).Set("subject", subject).Set("created_at", q.CreatedAt)
	}
	return out, nil
}

func (a *App) answerQuestion(c *Ctx) error {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	answer := strings.TrimSpace(str(body["answer"]))
	if answer == "" {
		return reqMissing("answer")
	}
	q, err := a.questionFor(c)
	if err != nil {
		return err
	}
	if !q.Open() {
		return reqRefused("already closed")
	}
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		if _, err := t.Exec(`UPDATE questions SET answer = ?, answered_at = ?, updated_at = ? WHERE id = ?`, answer, now, now, q.ID); err != nil {
			return err
		}
		// What it was about keeps the answer in its history.
		if questionEventful[q.SubjectType] && q.SubjectID != 0 {
			return t.Event(q.SubjectType, q.SubjectID, "question.answered", map[string]any{"question": q.Text, "answer": answer})
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.Summary = "Answered."
	return c.JSON(200, O("summary", c.Summary, "path", "/"))
}

// questionEventful are the records that keep a history (Eventful).
var questionEventful = map[string]bool{"Client": true, "AgreementVersion": true, "Commitment": true, "Engagement": true,
	"Todo": true, "Request": true, "User": true, "Setting": true}

func (a *App) dismissQuestion(c *Ctx) error {
	q, err := a.questionFor(c)
	if err != nil {
		return err
	}
	if !q.Open() {
		return reqRefused("already closed")
	}
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		_, err := t.Exec(`UPDATE questions SET dismissed_at = ?, updated_at = ? WHERE id = ?`, now, now, q.ID)
		return err
	})
	if err != nil {
		return err
	}
	c.Summary = "Dismissed."
	return c.JSON(200, O("summary", c.Summary, "path", "/"))
}
