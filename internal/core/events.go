package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// ---- writes ----------------------------------------------------------------

// Tx is one write: the transaction, who is making it, and what to do once
// it commits (events to tell webhooks and plugins about, records to
// reindex). Every write of consequence goes through App.write, so it carries
// an actor and a source, as the Rails core's record_event! did.
type Tx struct {
	*sql.Tx
	App   *App
	Ctx   context.Context //nolint:containedctx // the write's own, for the helpers below
	Actor *Actor

	events  []Event
	reindex []searchRef
	after   []func(context.Context)
}

// write runs fn in one transaction as actor; once it commits, the events
// it recorded are published and the records it touched reindexed, in the
// background.
func (a *App) write(ctx context.Context, actor *Actor, fn func(*Tx) error) error {
	t := &Tx{App: a, Ctx: ctx, Actor: actor}
	err := a.store.Tx(ctx, func(tx *sql.Tx) error {
		t.Tx = tx
		return fn(t)
	})
	if err != nil {
		return err
	}
	if len(t.events) > 0 {
		events := t.events
		a.later("events", func(ctx context.Context) {
			for _, ev := range events {
				a.publish(ctx, ev)
			}
		})
	}
	if len(t.reindex) > 0 {
		refs := t.reindex
		a.later("search index", func(ctx context.Context) { a.reindex(ctx, refs...) })
	}
	for _, fn := range t.after {
		a.later("after commit", fn)
	}
	return nil
}

// Exec runs a statement in the write.
func (t *Tx) Exec(query string, args ...any) (sql.Result, error) {
	return t.ExecContext(t.Ctx, query, args...)
}

// Insert runs an INSERT and answers the new row's id.
func (t *Tx) Insert(query string, args ...any) (int64, error) {
	res, err := t.ExecContext(t.Ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// QueryRow reads one row in the write.
func (t *Tx) QueryRow(query string, args ...any) *sql.Row {
	return t.QueryRowContext(t.Ctx, query, args...)
}

// AfterCommit runs fn in the background once the write commits (mail,
// say): never if it doesn't.
func (t *Tx) AfterCommit(fn func(context.Context)) { t.after = append(t.after, fn) }

// Reindex queues a record for the search index once the write commits.
func (t *Tx) Reindex(subjectType string, id int64) {
	t.reindex = append(t.reindex, searchRef{subjectType, id})
}

// ---- events ----------------------------------------------------------------

// Event is one thing that happened to a record. Append-only: a correction is
// another event.
type Event struct {
	ID          int64          `json:"id"`
	SubjectType string         `json:"subject_type"`
	SubjectID   int64          `json:"subject_id"`
	Kind        string         `json:"kind"`
	Payload     map[string]any `json:"payload"`
	ActorUserID int64          `json:"actor_user_id,omitempty"`
	Actor       string         `json:"actor,omitempty"` // the app acting for someone ("Claude")
	Source      string         `json:"source"`
	OccurredAt  store.Time     `json:"occurred_at"`
}

// SecuritySubjects are the audit log's, not a record's history: kept out of
// changes (what agents catch up on).
var SecuritySubjects = []string{"User", "Setting"}

// Event records kind on the subject as the write's actor, from its source;
// webhooks and plugins hear of it once the write commits.
func (t *Tx) Event(subjectType string, subjectID int64, kind string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	ev := Event{SubjectType: subjectType, SubjectID: subjectID, Kind: kind, Payload: payload,
		ActorUserID: t.Actor.UserID(), Actor: t.Actor.AppLabel(), Source: t.Actor.Source(), OccurredAt: store.Now()}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var actorUser, actorLabel any
	if ev.ActorUserID != 0 {
		actorUser = ev.ActorUserID
	}
	if ev.Actor != "" {
		actorLabel = ev.Actor
	} else if t.Actor != nil && t.Actor.Contact != nil {
		actorLabel = t.Actor.Contact.Name
	}
	ev.ID, err = t.Insert(`INSERT INTO events (subject_type, subject_id, kind, payload, actor_user_id, actor, source, occurred_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		subjectType, subjectID, kind, string(raw), actorUser, actorLabel, ev.Source, ev.OccurredAt.DB(), ev.OccurredAt.DB())
	if err != nil {
		return err
	}
	t.events = append(t.events, ev)
	return nil
}

// listener hears every event, once its write has committed.
type listener func(ctx context.Context, ev Event)

// publish tells every listener, in the order they were added.
func (a *App) publish(ctx context.Context, ev Event) {
	for _, l := range a.listeners {
		l(ctx, ev)
	}
}

// listen wires who hears of events: plugins, then whatever else areas of
// the app add (webhooks, notifications) in an init with onEvent.
func (a *App) listen() {
	a.listeners = []listener{
		func(ctx context.Context, ev Event) { a.plugins.Emit(ctx, ev.Kind, ev) },
	}
	for _, l := range eventListeners {
		a.listeners = append(a.listeners, func(ctx context.Context, ev Event) { l(a, ctx, ev) })
	}
}

// eventListeners are added by areas of the app in an init.
var eventListeners []func(a *App, ctx context.Context, ev Event)

// onEvent adds a listener every App gets.
func onEvent(l func(a *App, ctx context.Context, ev Event)) {
	eventListeners = append(eventListeners, l)
}

// notFound is a lookup's "there's no such thing".
func notFound(what string) error { return apierr.NotFound("Couldn't find that " + what + ".") }

// isNoRows is whether err is sql.ErrNoRows.
func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
