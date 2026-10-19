package plugin

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Runwell's records, as a plugin reads them (records:read) and the few
// things it may do to them (records:write). Reads answer each record as
// the agent tools show it (agent_ref first: type, id, record "Client:12",
// display_name, url). Writes go through the core's own rules and are
// recorded in history as whoever called the route (an event or a task: the
// system), with the plugin named.

// ReadTypes are the record types Records reads.
// CustomField is the install's fields (Settings › Fields), Status the
// model they're on (Client, Engagement, Todo).
var ReadTypes = []string{"Client", "Contact", "Engagement", "AgreementVersion", "ScopeItem", "Todo", "Commitment", "Request", "User", "CustomField"}

// Query asks for records of one Type: one by ID (or an engagement by Ref),
// or a list narrowed by the filters that apply to the type, newest or most
// pressing first, at most Limit (100 by default, 500 at most).
type Query struct {
	Type string
	ID   int64
	Ref  string // an engagement's ref, like WO-12
	// Filters: ClientID (anything under a client), EngagementID (contacts
	// excepted), AgreementVersionID (scope items), OwnerID (work led by or
	// commitments of a person), Status (a client's, a todo's or a request's),
	// Open (engagements and commitments not closed or resolved, work not
	// done, requests still open), IDs (exactly these).
	ClientID           int64
	EngagementID       int64
	AgreementVersionID int64
	OwnerID            int64
	Status             string
	Open               bool
	IDs                []int64
	Limit              int
}

// ParseRecord reads "Type:id" (Client:12), as agents and the app name a
// record.
func ParseRecord(s string) (typ string, id int64, err error) {
	typ, n, ok := strings.Cut(strings.TrimSpace(s), ":")
	id, perr := strconv.ParseInt(n, 10, 64)
	if !ok || perr != nil || id <= 0 || typ == "" {
		return "", 0, fmt.Errorf("a record is named Type:id, like Client:12, not %q", s)
	}
	return typ, id, nil
}

// Records answers the records q asks for (records:read).
func (a *API) Records(ctx context.Context, q Query) ([]map[string]any, error) {
	if err := a.need("records:read", a.grants.RecordsRead); err != nil {
		return nil, err
	}
	if !slices.Contains(ReadTypes, q.Type) {
		return nil, fmt.Errorf("records are one of %s, not %q", strings.Join(ReadTypes, ", "), q.Type)
	}
	if q.Limit <= 0 {
		q.Limit = 100
	}
	q.Limit = min(q.Limit, 500)
	return a.h.env.Records(ctx, q)
}

// ErrNoRecord is a record that isn't there.
var ErrNoRecord = errors.New("no such record")

// Record is one record by its name, "Type:id" (records:read); ErrNoRecord
// when there's none.
func (a *API) Record(ctx context.Context, record string) (map[string]any, error) {
	typ, id, err := ParseRecord(record)
	if err != nil {
		return nil, err
	}
	list, err := a.Records(ctx, Query{Type: typ, ID: id})
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, ErrNoRecord
	}
	return list[0], nil
}

// NoteKinds are a note's kinds.
var NoteKinds = []string{"call", "meeting", "email", "internal"}

// Note is a note to add to a record: a Client, Engagement, ScopeItem, Todo
// or Request ("Client:12"). Body is HTML or plain text; Kind is internal
// unless it says call, meeting or email.
type Note struct {
	Record string
	Body   string
	Kind   string
}

// Work is a piece of work to add to an engagement (by Ref, like WO-12),
// planned unless Status says otherwise, led by OwnerID when set.
type Work struct {
	Ref           string
	Title         string
	Description   string
	Status        string // planned, in_progress, in_review, blocked or done
	OwnerID       int64
	DueOn         string // YYYY-MM-DD
	ClientVisible *bool  // shown to the client unless false
}

// Commitment is a promise with an owner and a date, on a client or an
// engagement ("Client:12" or "Engagement:5"): ours (OwnerKind us, UserID
// for one of us) or the client's (client, ContactID for one of theirs).
type Commitment struct {
	Record      string
	Description string
	DueOn       string // YYYY-MM-DD
	OwnerKind   string // us (default) or client
	UserID      int64
	ContactID   int64
}

// WorkChange changes a piece of work (records:write), through the core's
// rules: its status (planned, in_progress, in_review, blocked, done), due
// date (YYYY-MM-DD, "" clears), description (HTML or text), lead (OwnerID,
// 0 for nobody) and who else is on it. Nil fields are left as they are.
// todo.status is recorded when the status moves, as the Work pages do.
type WorkChange struct {
	ID            int64
	Status        *string
	DueOn         *string
	Description   *string
	OwnerID       *int64
	OtherOwnerIDs *[]int64
}

// Event is something that happened to a record, in its history: Kind is
// "<plugin key>.<what>" (time_tracking.logged), so a plugin's events never
// pass for the core's.
type Event struct {
	Record  string
	Kind    string
	Payload map[string]any
}

// AddNote adds a note (records:write): note.added on its record. It
// answers the note.
func (a *API) AddNote(ctx context.Context, n Note) (map[string]any, error) {
	return a.write(ctx, n)
}

// AddWork adds a piece of work to an engagement (records:write):
// todo.created. It answers the work.
func (a *API) AddWork(ctx context.Context, w Work) (map[string]any, error) {
	return a.write(ctx, w)
}

// AddCommitment adds a commitment (records:write): commitment.added. It
// answers the commitment.
func (a *API) AddCommitment(ctx context.Context, c Commitment) (map[string]any, error) {
	return a.write(ctx, c)
}

// ChangeWork changes a piece of work (records:write) and answers it.
func (a *API) ChangeWork(ctx context.Context, w WorkChange) (map[string]any, error) {
	return a.write(ctx, w)
}

// RecordEvent records an event of the plugin's own on a record
// (records:write), which webhooks, listeners and other plugins hear.
func (a *API) RecordEvent(ctx context.Context, e Event) (map[string]any, error) {
	if !strings.HasPrefix(e.Kind, a.key+".") || !eventFormat.MatchString(e.Kind) {
		return nil, fmt.Errorf("a plugin's events are named %s.<what>, in lowercase letters and _", a.key)
	}
	return a.write(ctx, e)
}

func (a *API) write(ctx context.Context, w any) (map[string]any, error) {
	if err := a.need("records:write", a.grants.RecordsWrite); err != nil {
		return nil, err
	}
	return a.h.env.Write(ctx, a.actor, a.key, w)
}
