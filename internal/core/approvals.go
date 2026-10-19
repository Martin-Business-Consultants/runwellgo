package core

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// The client's approval page, reached from the emailed link: it shows the
// version exactly as it was sent (its snapshot, never the live scope items
// or internal estimates), and the decision happens only on POST with a
// typed name. Open to anyone holding the link.

func init() { routeSets = append(routeSets, (*App).approvalRoutes) }

func (a *App) approvalRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/approve/{token}", a.showApproval, open)
	a.handle(mux, "POST /api/approve/{token}", a.decideApproval, open)
}

// ApprovalLink is a signed link a contact got to decide on a version.
type ApprovalLink struct {
	ID              int64
	VersionID       int64
	ContactID       int64
	ContactName     string
	ContactEmail    string
	ContactApprove  bool
	ContactTimeZone string
	Token           string
	ExpiresAt       store.Time
	OpenedAt        store.Time
}

// Expired is whether the link is past its date.
func (l *ApprovalLink) Expired() bool {
	return !l.ExpiresAt.Valid || !l.ExpiresAt.Time.After(time.Now())
}

// approvalLink is the link with token; notFound when there's none.
func (a *App) approvalLink(ctx context.Context, token string) (*ApprovalLink, error) {
	var l ApprovalLink
	err := a.store.QueryRowContext(ctx, `SELECT l.id, l.agreement_version_id, l.contact_id, c.name, COALESCE(c.email, ''), c.can_approve, COALESCE(cl.time_zone, ''),
		l.token, l.expires_at, l.opened_at
		FROM approval_links l JOIN contacts c ON c.id = l.contact_id JOIN clients cl ON cl.id = c.client_id WHERE l.token = ?`, token).
		Scan(&l.ID, &l.VersionID, &l.ContactID, &l.ContactName, &l.ContactEmail, &l.ContactApprove, &l.ContactTimeZone, &l.Token, &l.ExpiresAt, &l.OpenedAt)
	if isNoRows(err) || token == "" {
		return nil, apierr.NotFound("This link isn’t valid. Ask whoever sent it for a new one.")
	}
	return &l, err
}

func (a *App) showApproval(c *Ctx) error {
	ctx := c.R.Context()
	l, err := a.approvalLink(ctx, c.Param("token"))
	if err != nil {
		return err
	}
	if !l.OpenedAt.Valid {
		if _, err := a.store.ExecContext(ctx, `UPDATE approval_links SET opened_at = ? WHERE id = ? AND opened_at IS NULL`, store.Now().DB(), l.ID); err != nil {
			return err
		}
	}
	return a.approvalAnswer(c, l, "")
}

// approvalAnswer is what the page shows: the snapshot, the decision once
// there is one, and whether this link may still decide.
func (a *App) approvalAnswer(c *Ctx, l *ApprovalLink, notice string) error {
	ctx := c.R.Context()
	v, e, err := a.versionAndEngagement(ctx, a.store, l.VersionID)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	snap := v.Snapshot.Map()
	if len(snap) == 0 {
		items := []any{}
		for _, it := range v.Items {
			items = append(items, map[string]any{"description": it.Description, "price_cents": it.PriceCents})
		}
		snap = map[string]any{"engagement": map[string]any{"ref": e.Ref, "title": e.Title}, "client": e.ClientName, "items": items}
	}
	change := !v.Initial()
	totalLabel := "Total"
	if change && v.Kind != "revision" {
		totalLabel = "Change to price"
	}
	if v.Cadence != "" {
		totalLabel += " (" + strings.ToLower(humanize(v.Cadence)) + ")"
	}
	description := ""
	if !change {
		description = richTextHTML(str(snap["description"]))
	}
	usable := !l.Expired() && v.AwaitingDecision()
	tz := l.ContactTimeZone
	if tz == "" {
		tz = a.zoneName(ctx)
	}
	// Times as the client reads them, in their zone (Rails' :long).
	loc := location(tz)
	long := func(t store.Time) any {
		if !t.Valid {
			return nil
		}
		return t.Time.In(loc).Format("January 2, 2006, 3:04 PM")
	}
	var decision any
	if v.Approval != nil {
		ap := v.Approval
		via := ""
		if ap.Method == "agent" {
			parts := strings.Split(ap.UserAgent, " via ")
			via = parts[len(parts)-1]
		}
		decision = O("decision", ap.Decision, "approver_name", nullable(ap.ApproverName), "method", ap.Method, "via", nullable(via),
			"decided_at", ap.DecidedAt, "decided_at_text", long(ap.DecidedAt), "comment", nullable(richTextHTML(ap.Comment)))
	}
	hash := v.ContentHash
	if len(hash) > 16 {
		hash = hash[:16]
	}
	c.Summary = notice
	return c.JSON(200, O(
		"notice", nullable(notice),
		"brand", a.BrandName(ctx),
		"contact", O("name", l.ContactName, "can_approve", l.ContactApprove),
		"time_zone", tz,
		"usable", usable,
		"expires_at", l.ExpiresAt,
		"version", O("label", v.Label(), "kind", v.Kind, "sent_at", v.SentAt, "sent_at_text", long(v.SentAt), "summary", nullable(v.Summary),
			"reason", nullable(richTextHTML(v.Reason)), "cadence", nullable(v.Cadence), "content_hash", nullable(hash)),
		"snapshot", O("client", snap["client"], "engagement", snap["engagement"], "description", nullable(description), "items", asListOr(snap["items"])),
		"priced", v.Priced(s.Prices),
		"total_label", totalLabel,
		"total_cents", v.TotalCents(),
		"total", formatCents(v.TotalCents()),
		"decision", decision,
	))
}

func asListOr(v any) []any {
	if l := asList(v); l != nil {
		return l
	}
	return []any{}
}

// decideApproval records the contact's decision: their typed name, the
// time, where from, and the exact terms (the version's hash).
func (a *App) decideApproval(c *Ctx) error {
	ctx := c.R.Context()
	l, err := a.approvalLink(ctx, c.Param("token"))
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	v, e, err := a.versionAndEngagement(ctx, a.store, l.VersionID)
	if err != nil {
		return err
	}
	if l.Expired() || !v.AwaitingDecision() {
		return refused("This link is no longer valid.")
	}
	if !l.ContactApprove {
		return refused("This link was sent to you for review. Someone at your company who can approve agreements needs to decide.")
	}
	name := strings.TrimSpace(str(body["approver_name"]))
	if name == "" {
		return refused("Please type your name to confirm.")
	}
	if str(body["decision"]) == "" {
		return apierr.BadRequest("param is missing or the value is empty: decision")
	}
	err = a.write(ctx, nil, func(t *Tx) error {
		return t.decideVersion(e, v, Decision{Decision: str(body["decision"]), Method: "link", ContactID: l.ContactID,
			ApproverName: name, ApproverEmail: l.ContactEmail, Comment: str(body["comment"]),
			IPAddress: c.ip(), UserAgent: c.R.UserAgent(), Source: "approval link"})
	})
	if err != nil {
		return err
	}
	return a.approvalAnswer(c, l, "Thank you. Your decision has been recorded.")
}
