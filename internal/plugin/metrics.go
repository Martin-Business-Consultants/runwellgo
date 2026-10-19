package plugin

import (
	"context"
	"errors"
	"time"
)

// "metrics": the numbers a report needs, read in one go, where reading every
// record with Records would be slow or capped: each client's counts today
// (open and late work, requests to triage, commitments due, approvals
// waiting, its contacts, when it was last touched), each engagement's state,
// agreed value and progress, and the dated facts behind trends (work added
// and done, agreements sent and decided, requests in and triaged, notes,
// commitments resolved, clients and engagements started). Read-only, and
// money is zero where the install hides prices.

// MetricsQuery narrows what Metrics reads: Facts from Since on (a year ago
// when zero, three years at most), and only ClientID's when it's set.
type MetricsQuery struct {
	Since    time.Time
	ClientID int64
}

// Metrics is the install's numbers, as of Today (YYYY-MM-DD, the install's
// zone). Prices is false when the install hides prices: every *Cents is 0.
type Metrics struct {
	Today       string              `json:"today"`
	Prices      bool                `json:"prices"`
	Clients     []ClientMetrics     `json:"clients"`
	Engagements []EngagementMetrics `json:"engagements"`
	Facts       []Fact              `json:"facts"`
	// Truncated says Facts stopped at the cap (50,000), the oldest left out.
	Truncated bool `json:"truncated"`
}

// ClientMetrics is one client today.
type ClientMetrics struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	TimeZone  string    `json:"time_zone"` // as stored (a Rails zone name or an IANA one); "" is the install's
	Internal  bool      `json:"internal"`
	CreatedAt time.Time `json:"created_at"`
	// Work under its engagements: not done, past its due date, due in the
	// next seven days, blocked, in review.
	OpenWork     int `json:"open_work"`
	LateWork     int `json:"late_work"`
	DueSoonWork  int `json:"due_soon_work"`
	BlockedWork  int `json:"blocked_work"`
	InReviewWork int `json:"in_review_work"`
	// Requests from it still to triage, and when the oldest came in.
	OpenRequests    int        `json:"open_requests"`
	OldestRequestAt *time.Time `json:"oldest_request_at"`
	// Commitments not resolved: all, past due (ours, theirs), due in the
	// next seven days.
	OpenCommitments    int `json:"open_commitments"`
	OverdueOurs        int `json:"overdue_ours"`
	OverdueTheirs      int `json:"overdue_theirs"`
	DueSoonCommitments int `json:"due_soon_commitments"`
	// Engagements whose latest agreement waits on the client (sent, or
	// changes requested), and since when the longest has waited.
	WaitingApprovals int        `json:"waiting_approvals"`
	WaitingSince     *time.Time `json:"waiting_since"`
	Contacts         int        `json:"contacts"`
	PortalContacts   int        `json:"portal_contacts"`
	// LastTouch is the latest of a note, work done, a decision or a request
	// on the client or anything under it; LastTouchKind says which (note,
	// work, approval, request) and LastNoteAt is the latest note's.
	LastTouchAt   *time.Time `json:"last_touch_at"`
	LastTouchKind string     `json:"last_touch_kind"`
	LastNoteAt    *time.Time `json:"last_note_at"`
}

// EngagementMetrics is one engagement today: its derived state (draft,
// sent, changes_requested, approved, internal, closed), what's agreed
// (fixed scope's total, or a recurring service's amount per Cadence), what
// waits on the client or is in draft, and its work.
type EngagementMetrics struct {
	ID           int64      `json:"id"`
	ClientID     int64      `json:"client_id"`
	Ref          string     `json:"ref"`
	Title        string     `json:"title"`
	Label        string     `json:"label"` // project, work_order, service
	Shape        string     `json:"shape"` // fixed, recurring
	State        string     `json:"state"`
	CloseOutcome string     `json:"close_outcome"`
	Cadence      string     `json:"cadence"`
	AgreedCents  int64      `json:"agreed_cents"`
	PendingCents int64      `json:"pending_cents"` // sent, waiting on the client
	DraftCents   int64      `json:"draft_cents"`
	WorkOpen     int        `json:"work_open"`
	WorkDone     int        `json:"work_done"`
	WorkLate     int        `json:"work_late"`
	CreatedAt    time.Time  `json:"created_at"`
	ApprovedAt   *time.Time `json:"approved_at"` // its first approval
	ClosedAt     *time.Time `json:"closed_at"`
}

// Fact is something dated that happened, for trends. Kinds: client.created,
// engagement.created, agreement.sent (Cents: its amount, or a change's
// difference), agreement.approved and agreement.changes_requested (Start:
// when it was sent), todo.created, todo.done (Due: its due date, if it had
// one), request.received, request.triaged (Start: when it came in),
// note.added, commitment.done / commitment.missed / commitment.dropped
// (Due: its date).
type Fact struct {
	Kind         string     `json:"kind"`
	At           time.Time  `json:"at"`
	ClientID     int64      `json:"client_id,omitempty"`
	EngagementID int64      `json:"engagement_id,omitempty"`
	ID           int64      `json:"id"` // the record's (the todo's, the request's…)
	Cents        int64      `json:"cents,omitempty"`
	Start        *time.Time `json:"start,omitempty"`
	Due          string     `json:"due,omitempty"`
}

// MetricsEnv is the core's side of Metrics, beside Env (an Env without it,
// a test's, answers that it can't).
type MetricsEnv interface {
	Metrics(ctx context.Context, q MetricsQuery) (*Metrics, error)
}

// Metrics is the install's numbers (metrics).
func (a *API) Metrics(ctx context.Context, q MetricsQuery) (*Metrics, error) {
	if err := a.need("metrics", a.grants.Metrics); err != nil {
		return nil, err
	}
	e, ok := a.h.env.(MetricsEnv)
	if !ok {
		return nil, errors.New("this Runwell can't read metrics")
	}
	now := time.Now()
	if q.Since.IsZero() {
		q.Since = now.AddDate(-1, 0, 0)
	}
	if q.Since.Before(now.AddDate(-3, 0, 0)) {
		q.Since = now.AddDate(-3, 0, 0)
	}
	return e.Metrics(ctx, q)
}
