package core

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Agreement versions: exactly what the client saw. A version is a draft
// until it's sent; sending freezes it (a snapshot of its items and price,
// hashed), so a decision always refers to fixed terms. Approving turns each
// scope item into a piece of work.

func init() { routeSets = append(routeSets, (*App).agreementRoutes) }

// AgreementKinds and cadences, as the Rails model has them.
var (
	AgreementKinds    = []string{"initial", "change_order", "revision", "add_on"}
	AgreementCadences = []string{"weekly", "monthly", "quarterly"}
	ApprovalDecisions = []string{"approved", "changes_requested"}
)

func (a *App) agreementRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/engagements/{ref}/agreement_versions", a.createAgreementVersion, tool("create_agreement_version", "Start a draft agreement version",
		note("A new draft on an engagement: the initial agreement, a change order, a revision or an add-on. Add scope with add_scope_item, then send_agreement."),
		bodyParam("kind", "string", strings.Join(AgreementKinds, ", ")+".", false),
		bodyParam("summary", "string", "What this version covers.", false),
		bodyParam("reason", "string", "Why it's changing.", false),
		next("add_scope_item", "send_agreement")))
	a.handle(mux, "PATCH /api/agreement_versions/{id}", a.updateAgreementVersion, tool("update_agreement_version", "Change a draft agreement version",
		body("agreement_version", `{summary, reason, cadence: weekly|monthly|quarterly, amount_cents: integer}`, true)))
	a.handle(mux, "DELETE /api/agreement_versions/{id}", a.discardDraft, tool("discard_draft", "Discard a draft agreement version",
		note("Only drafts: a sent version is frozen forever.")))
	a.handle(mux, "POST /api/agreement_versions/{id}/send_out", a.sendAgreement, need("send_agreements"), tool("send_agreement", "Send an agreement version to the client",
		note("Freezes the draft (snapshotted and hashed; it can never change again). With contact_id, emails that contact a signed approval link; without, mark it sent and record their decision later with record_decision."),
		bodyParam("contact_id", "integer", "The contact to email an approval link.", false),
		confirm("It freezes this version for good and, with a contact, emails them an approval link.")))
	a.handle(mux, "POST /api/agreement_versions/{id}/issue_link", a.issueApprovalLink, need("send_agreements"), tool("email_approval_link", "Email an approval link",
		bodyParam("contact_id", "integer", "The contact to email.", true),
		confirm("It emails the contact a signed link to approve or ask for changes.")))
	a.handle(mux, "POST /api/agreement_versions/{id}/record_decision", a.recordDecision, need("send_agreements"), tool("record_decision", "Record the client’s decision",
		note("For a decision that came by email, call or meeting. Say where it came from: the evidence is kept with the approval. Approving creates the work (one todo per scope item)."),
		bodyParam("decision", "string", "approved or changes_requested.", true),
		bodyParam("contact_id", "integer", "The contact who decided.", false),
		bodyParam("approver_name", "string", "Their name, if not a contact.", false),
		bodyParam("evidence", "string", "Where the decision came from: “Email from Ann, 3 March”.", true),
		bodyParam("comment", "string", "Their comment.", false)))
}

// ---- the records ---------------------------------------------------------------

// AgreementVersion is one version of an engagement's agreement.
type AgreementVersion struct {
	ID              int64
	EngagementID    int64
	Number          int
	Kind            string
	Summary         string
	Reason          string
	Cadence         string
	AmountCents     int64
	PriceDeltaCents int64
	Snapshot        store.JSON
	ContentHash     string
	SentAt          store.Time
	SentByID        int64
	SentByName      string
	SupersededByID  int64
	CreatedAt       store.Time
	Approval        *Approval
	Items           []*ScopeItem // by position, once loaded
}

// Approval is the client's decision on a sent version: immutable.
type Approval struct {
	ID             int64
	Decision       string
	ContactID      int64
	ContactName    string
	ApproverName   string
	ApproverEmail  string
	Comment        string
	Method         string // link, agent, recorded
	Evidence       string
	IPAddress      string
	UserAgent      string
	ContentHash    string
	DecidedAt      store.Time
	RecordedByID   int64
	RecordedByName string
}

// Draft is whether it's still a draft.
func (v *AgreementVersion) Draft() bool { return !v.SentAt.Valid }

// Sent is whether it was sent (and so is frozen).
func (v *AgreementVersion) Sent() bool { return v.SentAt.Valid }

// Approved is whether the client approved it.
func (v *AgreementVersion) Approved() bool {
	return v.Approval != nil && v.Approval.Decision == "approved"
}

// ChangesRequested is whether the client asked for changes.
func (v *AgreementVersion) ChangesRequested() bool {
	return v.Approval != nil && v.Approval.Decision == "changes_requested"
}

// AwaitingDecision is whether it's sent, undecided and not superseded.
func (v *AgreementVersion) AwaitingDecision() bool {
	return v.Sent() && v.Approval == nil && v.SupersededByID == 0
}

// Initial is whether it's the first agreement.
func (v *AgreementVersion) Initial() bool { return v.Kind == "initial" }

// Label is "Agreement", or "Change order #2".
func (v *AgreementVersion) Label() string {
	if v.Initial() {
		return "Agreement"
	}
	return fmt.Sprintf("%s #%d", humanize(v.Kind), v.Number)
}

// Status is draft, approved, changes_requested, awaiting_decision or superseded.
func (v *AgreementVersion) Status() string {
	switch {
	case v.Draft():
		return "draft"
	case v.Approved():
		return "approved"
	case v.ChangesRequested():
		return "changes_requested"
	case v.AwaitingDecision():
		return "awaiting_decision"
	}
	return "superseded"
}

// ItemsTotalCents is what its items add up to.
func (v *AgreementVersion) ItemsTotalCents() int64 {
	var sum int64
	for _, it := range v.Items {
		sum += it.PriceCents
	}
	return sum
}

// TotalCents is the total the client sees: the whole price for an initial
// version or a revision, the change for a change order or add-on.
func (v *AgreementVersion) TotalCents() int64 {
	if v.Kind == "initial" || v.Kind == "revision" {
		return v.AmountCents
	}
	return v.PriceDeltaCents
}

// snapshotItems are the items the client saw: the snapshot's once sent,
// the live ones before.
func (v *AgreementVersion) snapshotItems() []map[string]any {
	var out []map[string]any
	if v.Sent() {
		for _, it := range asList(v.Snapshot.Map()["items"]) {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	for _, it := range v.Items {
		out = append(out, map[string]any{"description": it.Description, "price_cents": float64(it.PriceCents)})
	}
	return out
}

// Priced is whether there's money to show on it (priced_version?): prices
// are on, and something on it costs something.
func (v *AgreementVersion) Priced(prices bool) bool {
	if !prices {
		return false
	}
	for _, it := range v.snapshotItems() {
		if n, _ := intOf(it["price_cents"]); n != 0 {
			return true
		}
	}
	return v.AmountCents != 0 || v.TotalCents() != 0
}

// AmountText is what a version puts in front of the client: the whole
// price, a change, or a new recurring price (version_amount).
func (v *AgreementVersion) AmountText() string {
	cents := v.AmountCents
	if v.Draft() {
		cents = v.ItemsTotalCents()
	}
	switch {
	case v.Kind == "revision":
		if v.Cadence != "" {
			return formatCents(v.AmountCents) + " / " + v.Cadence
		}
		return formatCents(v.AmountCents)
	case v.Initial():
		return formatCents(cents)
	case v.Draft():
		return "+" + formatCents(cents)
	}
	return "+" + formatCents(v.PriceDeltaCents)
}

const versionSelect = `SELECT v.id, v.engagement_id, v.number, v.kind, COALESCE(v.summary, ''), COALESCE(v.reason, ''), COALESCE(v.cadence, ''),
	v.amount_cents, v.price_delta_cents, v.snapshot, COALESCE(v.content_hash, ''), v.sent_at, COALESCE(v.sent_by_id, 0), COALESCE(su.name, ''),
	COALESCE(v.superseded_by_id, 0), v.created_at,
	COALESCE(ap.id, 0), COALESCE(ap.decision, ''), COALESCE(ap.contact_id, 0), COALESCE(ac.name, ''), COALESCE(ap.approver_name, ''), COALESCE(ap.approver_email, ''),
	COALESCE(ap.comment, ''), COALESCE(ap.method, ''), COALESCE(ap.evidence, ''), COALESCE(ap.ip_address, ''), COALESCE(ap.user_agent, ''),
	COALESCE(ap.content_hash, ''), ap.decided_at, COALESCE(ap.recorded_by_id, 0), COALESCE(ru.name, '')
	FROM agreement_versions v
	LEFT JOIN users su ON su.id = v.sent_by_id
	LEFT JOIN approvals ap ON ap.agreement_version_id = v.id
	LEFT JOIN contacts ac ON ac.id = ap.contact_id
	LEFT JOIN users ru ON ru.id = ap.recorded_by_id`

func scanVersion(r interface{ Scan(...any) error }) (*AgreementVersion, error) {
	var v AgreementVersion
	var ap Approval
	err := r.Scan(&v.ID, &v.EngagementID, &v.Number, &v.Kind, &v.Summary, &v.Reason, &v.Cadence,
		&v.AmountCents, &v.PriceDeltaCents, &v.Snapshot, &v.ContentHash, &v.SentAt, &v.SentByID, &v.SentByName,
		&v.SupersededByID, &v.CreatedAt,
		&ap.ID, &ap.Decision, &ap.ContactID, &ap.ContactName, &ap.ApproverName, &ap.ApproverEmail,
		&ap.Comment, &ap.Method, &ap.Evidence, &ap.IPAddress, &ap.UserAgent,
		&ap.ContentHash, &ap.DecidedAt, &ap.RecordedByID, &ap.RecordedByName)
	if ap.ID != 0 {
		v.Approval = &ap
	}
	return &v, err
}

// versionsOf are the engagements' versions with their decisions, by
// number: engagement id → versions. One query for the lot.
func (a *App) versionsOf(ctx context.Context, db store.Querier, engagementIDs []int64) (map[int64][]*AgreementVersion, error) {
	out := map[int64][]*AgreementVersion{}
	if len(engagementIDs) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, versionSelect+` WHERE v.engagement_id IN (`+store.Placeholders(len(engagementIDs))+`) ORDER BY v.engagement_id, v.number`, idArgs(engagementIDs)...)
	if err != nil {
		return nil, err
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (*AgreementVersion, error) { return scanVersion(r) })
	if err != nil {
		return nil, err
	}
	for _, v := range list {
		out[v.EngagementID] = append(out[v.EngagementID], v)
	}
	return out, nil
}

// versionByID is one version with its decision and items.
func (a *App) versionByID(ctx context.Context, db store.Querier, id int64) (*AgreementVersion, error) {
	v, err := scanVersion(db.QueryRowContext(ctx, versionSelect+` WHERE v.id = ?`, id))
	if isNoRows(err) {
		return nil, notFound("agreement version")
	}
	if err != nil {
		return nil, err
	}
	return v, a.loadItems(ctx, db, []*AgreementVersion{v})
}

// versionAndEngagement is a version and its engagement (with every
// version, this one among them, items loaded).
func (a *App) versionAndEngagement(ctx context.Context, db store.Querier, id int64) (*AgreementVersion, *Engagement, error) {
	var engagementID int64
	if err := db.QueryRowContext(ctx, `SELECT engagement_id FROM agreement_versions WHERE id = ?`, id).Scan(&engagementID); err != nil {
		if isNoRows(err) {
			return nil, nil, notFound("agreement version")
		}
		return nil, nil, err
	}
	e, err := a.engagementByID(ctx, db, engagementID)
	if err != nil {
		return nil, nil, err
	}
	if err := a.loadItems(ctx, db, e.Versions); err != nil {
		return nil, nil, err
	}
	for _, v := range e.Versions {
		if v.ID == id {
			return v, e, nil
		}
	}
	return nil, nil, notFound("agreement version")
}

// ---- JSON ----------------------------------------------------------------------

// versionJSON is a version as answers show it (agreement_versions/_agreement_version.json),
// with what the engagement page shows beside it: who sent it, its hash, the
// decision in full and the approval links out.
func (a *App) versionJSON(e *Engagement, v *AgreementVersion, links []Obj, prices bool) Obj {
	amount := v.AmountCents
	if v.Draft() {
		amount = v.ItemsTotalCents()
	}
	items := make([]Obj, len(v.Items))
	for i, it := range v.Items {
		items[i] = a.scopeItemJSON(it)
	}
	var decision any
	var approval any
	if v.Approval != nil {
		decision = v.Approval.Decision
		approval = a.approvalJSON(v.Approval)
	}
	if links == nil {
		links = []Obj{}
	}
	return a.agentRef("AgreementVersion", v.ID, "", e.Ref+" "+v.Label(), "/engagements/"+e.Ref).
		Set("number", v.Number).Set("kind", v.Kind).Set("summary", nullable(v.Summary)).Set("reason", nullable(v.Reason)).Set("cadence", nullable(v.Cadence)).
		Set("label", v.Label()).Set("status", v.Status()).Set("draft", v.Draft()).Set("awaiting_decision", v.AwaitingDecision()).
		Set("amount", formatCents(amount)).Set("amount_cents", amount).Set("sent_at", v.SentAt).Set("decision", decision).
		Set("scope_items", items).
		Set("amount_cents_set", v.AmountCents).Set("price_delta_cents", v.PriceDeltaCents).Set("amount_text", v.AmountText()).
		Set("priced", v.Priced(prices)).Set("content_hash", nullable(v.ContentHash)).
		Set("sent_by", userRef(v.SentByID, v.SentByName)).Set("approval", approval).Set("approval_links", links)
}

// approvalJSON is a decision as the page shows it.
func (a *App) approvalJSON(ap *Approval) Obj {
	var contact any
	if ap.ContactID != 0 {
		contact = O("id", ap.ContactID, "name", ap.ContactName)
	}
	return O("id", ap.ID, "decision", ap.Decision, "method", ap.Method, "contact", contact,
		"approver_name", nullable(ap.ApproverName), "approver_email", nullable(ap.ApproverEmail),
		"comment", nullable(richTextHTML(ap.Comment)), "evidence", nullable(ap.Evidence), "content_hash", nullable(ap.ContentHash),
		"decided_at", ap.DecidedAt, "recorded_by", userRef(ap.RecordedByID, ap.RecordedByName))
}

// versionsJSON are an engagement's versions (items loaded), with their
// approval links: one query for the links.
func (a *App) versionsJSON(ctx context.Context, e *Engagement, versions []*AgreementVersion) ([]Obj, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	links := map[int64][]Obj{}
	if len(versions) > 0 {
		ids := make([]int64, len(versions))
		for i, v := range versions {
			ids[i] = v.ID
		}
		rows, err := a.store.QueryContext(ctx, `SELECT l.id, l.agreement_version_id, l.contact_id, COALESCE(c.name, ''), l.expires_at, l.opened_at
			FROM approval_links l LEFT JOIN contacts c ON c.id = l.contact_id WHERE l.agreement_version_id IN (`+store.Placeholders(len(ids))+`) ORDER BY l.id`, idArgs(ids)...)
		if err != nil {
			return nil, err
		}
		type link struct {
			id, version, contact int64
			name                 string
			expires, opened      store.Time
		}
		list, err := store.ScanAll(rows, func(r *sqlRows) (link, error) {
			var l link
			return l, r.Scan(&l.id, &l.version, &l.contact, &l.name, &l.expires, &l.opened)
		})
		if err != nil {
			return nil, err
		}
		for _, l := range list {
			links[l.version] = append(links[l.version], O("id", l.id, "contact", O("id", l.contact, "name", l.name), "expires_at", l.expires, "opened_at", l.opened))
		}
	}
	out := make([]Obj, len(versions))
	for i, v := range versions {
		out[i] = a.versionJSON(e, v, links[v.ID], s.Prices)
	}
	return out, nil
}

// newVersionKinds are what the next version may be (new_version_kinds).
func newVersionKinds(e *Engagement) []Obj {
	switch {
	case len(e.Versions) == 0:
		return []Obj{O("value", "initial", "label", "Agreement")}
	case e.Recurring():
		return []Obj{O("value", "revision", "label", "Revision (new recurring price)"), O("value", "add_on", "label", "Add-on")}
	}
	return []Obj{O("value", "change_order", "label", "Change order")}
}

// ---- lifecycle -----------------------------------------------------------------

// draftVersion opens a new draft on e. The first is initial; later ones are
// change orders (fixed) or revisions (recurring). Nothing is copied: a
// change order lists only what changes.
func (t *Tx) draftVersion(e *Engagement, kind, summary, reason string) (*AgreementVersion, error) {
	if e.DraftVersion() != nil {
		return nil, refused("a draft is already open")
	}
	if kind == "" {
		switch {
		case len(e.Versions) == 0:
			kind = "initial"
		case e.Recurring():
			kind = "revision"
		default:
			kind = "change_order"
		}
	}
	if !slices.Contains(AgreementKinds, kind) {
		inv := apierr.Invalid{}
		inv.Add("kind", "is not included in the list")
		return nil, inv.Err()
	}
	number := 1
	for _, v := range e.Versions {
		number = max(number, v.Number+1)
	}
	var cadence any
	var amount int64
	current := e.CurrentVersion()
	if e.Recurring() {
		cadence = "monthly"
		if current != nil && current.Cadence != "" {
			cadence = current.Cadence
		}
	}
	if kind == "revision" && current != nil {
		amount = current.AmountCents
	}
	now := store.Now().DB()
	id, err := t.Insert(`INSERT INTO agreement_versions (engagement_id, number, kind, summary, reason, cadence, amount_cents, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, number, kind, nullable(summary), nullable(sanitizeRichText(reason)), cadence, amount, now, now)
	if err != nil {
		return nil, err
	}
	v := &AgreementVersion{ID: id, EngagementID: e.ID, Number: number, Kind: kind, Summary: summary, Reason: reason, Cadence: str(cadence), AmountCents: amount}
	e.Versions = append(e.Versions, v)
	return v, nil
}

// sendVersion freezes v: snapshots its items and price, hashes the
// snapshot and stamps who sent it when. Versions still waiting on a
// decision are superseded by it.
func (t *Tx) sendVersion(e *Engagement, v *AgreementVersion) error {
	switch {
	case v.Sent():
		return refused("already sent")
	case e.Internal():
		return refused(e.ClientName + " is internal: its projects have no agreement to send")
	case len(v.Items) == 0 && v.Kind != "revision":
		return refused("nothing to send: add at least one item")
	}
	for _, other := range e.Versions {
		if other.ID != v.ID && other.AwaitingDecision() {
			if _, err := t.Exec(`UPDATE agreement_versions SET superseded_by_id = ?, updated_at = ? WHERE id = ?`, v.ID, store.Now().DB(), other.ID); err != nil {
				return err
			}
			other.SupersededByID = v.ID
		}
	}
	if v.Kind != "revision" {
		v.AmountCents = v.ItemsTotalCents()
	}
	switch {
	case v.Initial():
		v.PriceDeltaCents = 0
	case v.Kind == "revision":
		var current int64
		if cv := e.CurrentVersion(); cv != nil {
			current = cv.AmountCents
		}
		v.PriceDeltaCents = v.AmountCents - current
	default:
		v.PriceDeltaCents = v.ItemsTotalCents()
	}
	items := make([]Obj, len(v.Items))
	for i, it := range v.Items {
		items[i] = O("description", it.Description, "price_cents", it.PriceCents)
	}
	var description any
	if e.Description != "" {
		description = e.Description
	}
	snapshot := O(
		"engagement", O("ref", e.Ref, "title", e.Title, "label", e.Label, "shape", e.Shape),
		"client", e.ClientName,
		"version", v.Number, "kind", v.Kind, "summary", nullable(v.Summary), "reason", nullable(v.Reason), "cadence", nullable(v.Cadence),
		"items", items,
		"amount_cents", v.AmountCents, "price_delta_cents", v.PriceDeltaCents,
		"description", description,
	)
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	sum := sha256.Sum256(raw)
	v.Snapshot, v.ContentHash, v.SentAt = store.JSON(raw), hex.EncodeToString(sum[:]), store.Now()
	var sentBy any
	if uid := t.Actor.UserID(); uid != 0 {
		sentBy, v.SentByID = uid, uid
	}
	if _, err := t.Exec(`UPDATE agreement_versions SET amount_cents = ?, price_delta_cents = ?, snapshot = ?, content_hash = ?, sent_at = ?, sent_by_id = ?, updated_at = ? WHERE id = ?`,
		v.AmountCents, v.PriceDeltaCents, string(raw), v.ContentHash, v.SentAt.DB(), sentBy, store.Now().DB(), v.ID); err != nil {
		return err
	}
	if err := t.Event("AgreementVersion", v.ID, "agreement.sent", map[string]any{"version": v.Number, "kind": v.Kind, "amount_cents": v.AmountCents}); err != nil {
		return err
	}
	return t.Event("Engagement", e.ID, "agreement.sent", map[string]any{"version": v.Number})
}

// Decision is what the client decided and how it reached us.
type Decision struct {
	Decision      string
	Method        string // link, agent, recorded
	ContactID     int64
	ApproverName  string
	ApproverEmail string
	Comment       string
	Evidence      string
	IPAddress     string
	UserAgent     string
	RecordedByID  int64
	// Source overrides the write's own (the approval page's "approval link").
	Source string
}

// decideVersion records the client's decision on a sent version (by link,
// their agent, or a person with evidence). Approving turns each scope item
// into a piece of work. The portal calls it too.
func (t *Tx) decideVersion(e *Engagement, v *AgreementVersion, d Decision) error {
	switch {
	case v.Draft():
		return refused("not sent")
	case v.Approval != nil:
		return refused("already decided")
	case v.SupersededByID != 0:
		return refused("superseded")
	}
	inv := apierr.Invalid{}
	if !slices.Contains(ApprovalDecisions, d.Decision) {
		inv.Add("decision", "is not included in the list")
	}
	if d.Method == "recorded" && strings.TrimSpace(d.Evidence) == "" {
		inv.Add("evidence", "can't be blank")
	}
	if err := inv.Err(); err != nil {
		return err
	}
	var contactName, contactEmail string
	if d.ContactID != 0 {
		if err := t.QueryRow(`SELECT name, COALESCE(email, '') FROM contacts WHERE id = ?`, d.ContactID).Scan(&contactName, &contactEmail); err != nil {
			if !isNoRows(err) {
				return err
			}
			d.ContactID = 0
		}
	}
	name, email := strOr(d.ApproverName, contactName), strOr(d.ApproverEmail, contactEmail)
	var contact, recordedBy any
	if d.ContactID != 0 {
		contact = d.ContactID
	}
	if d.RecordedByID != 0 {
		recordedBy = d.RecordedByID
	}
	now := store.Now()
	id, err := t.Insert(`INSERT INTO approvals (agreement_version_id, decision, contact_id, approver_name, approver_email, comment, method, evidence, ip_address, user_agent, content_hash, decided_at, recorded_by_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		v.ID, d.Decision, contact, nullable(name), nullable(email), nullable(sanitizeRichText(d.Comment)), d.Method, nullable(d.Evidence),
		nullable(d.IPAddress), nullable(d.UserAgent), nullable(v.ContentHash), now.DB(), recordedBy, now.DB(), now.DB())
	if err != nil {
		return err
	}
	v.Approval = &Approval{ID: id, Decision: d.Decision, ContactID: d.ContactID, ApproverName: name, ApproverEmail: email, Method: d.Method, DecidedAt: now, RecordedByID: d.RecordedByID}
	var by any
	if name != "" {
		by = name
	}
	if err := t.eventFrom(d.Source, "AgreementVersion", v.ID, "agreement."+d.Decision, map[string]any{"version": v.Number, "by": by, "method": d.Method}); err != nil {
		return err
	}
	if err := t.eventFrom(d.Source, "Engagement", e.ID, "agreement."+d.Decision, map[string]any{"version": v.Number}); err != nil {
		return err
	}
	if d.Decision == "approved" {
		return t.spawnTodos(e, v, d.RecordedByID)
	}
	return nil
}

// eventFrom is Event with the source named (an approval link's), when
// there is one.
func (t *Tx) eventFrom(source, subjectType string, id int64, kind string, payload map[string]any) error {
	if err := t.Event(subjectType, id, kind, payload); err != nil {
		return err
	}
	if source == "" {
		return nil
	}
	ev := &t.events[len(t.events)-1]
	ev.Source = source
	_, err := t.Exec(`UPDATE events SET source = ? WHERE id = ?`, source, ev.ID)
	return err
}

// spawnTodos makes every approved scope item a piece of work to track: one
// todo per item that has none, planned, at the end of its engagement's
// planned list and of the board's.
func (t *Tx) spawnTodos(e *Engagement, v *AgreementVersion, createdBy int64) error {
	var by any
	if createdBy != 0 {
		by = createdBy
	}
	for _, it := range v.Items {
		n, err := store.Count(t.Ctx, t, `SELECT count(*) FROM todos WHERE scope_item_id = ?`, it.ID)
		if err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		var position, board int64
		if err := t.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM todos WHERE engagement_id = ? AND status = 'planned'`, e.ID).Scan(&position); err != nil {
			return err
		}
		if err := t.QueryRow(`SELECT COALESCE(MAX(board_position), 0) + 1 FROM todos WHERE status = 'planned'`).Scan(&board); err != nil {
			return err
		}
		now := store.Now().DB()
		id, err := t.Insert(`INSERT INTO todos (engagement_id, scope_item_id, title, status, position, board_position, client_visible, created_by_id, created_at, updated_at) VALUES (?, ?, ?, 'planned', ?, ?, 1, ?, ?, ?)`,
			e.ID, it.ID, it.Description, position, board, by, now, now)
		if err != nil {
			return err
		}
		t.Reindex("Todo", id)
	}
	return nil
}

// issueLink makes a signed approval link for a contact, good for 30 days,
// emails it, and records that it went.
func (t *Tx) issueLink(e *Engagement, v *AgreementVersion, contactID int64) error {
	var name, email string
	if err := t.QueryRow(`SELECT name, COALESCE(email, '') FROM contacts WHERE id = ?`, contactID).Scan(&name, &email); err != nil {
		return err
	}
	if email == "" {
		return refused(name + " has no email address.")
	}
	token := base64.RawURLEncoding.EncodeToString(secret.RandomBytes(32))
	expires := store.TimeOf(time.Now().Add(30 * 24 * time.Hour))
	now := store.Now().DB()
	if _, err := t.Insert(`INSERT INTO approval_links (agreement_version_id, contact_id, token, expires_at, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		v.ID, contactID, token, expires.DB(), now, now); err != nil {
		return err
	}
	a := t.App
	t.AfterCommit(func(ctx context.Context) { a.mailLater(a.agreementMail(ctx, e, v, name, email, token, expires)) })
	return t.Event("AgreementVersion", v.ID, "agreement.emailed", map[string]any{"to": email})
}

// agreementMail is AgreementMailer#sent: the approval link, for the
// contact's decision.
func (a *App) agreementMail(ctx context.Context, e *Engagement, v *AgreementVersion, name, email, token string, expires store.Time) mail.Message {
	brand := a.BrandName(ctx)
	first := strings.Fields(name)
	hi := name
	if len(first) > 0 {
		hi = first[0]
	}
	url := a.BaseURL() + "/approve/" + token
	until := longDate(expires, a.clientZone(ctx, e.ClientTimeZone))
	reason := "You're getting this because you can approve work for " + e.ClientName + "."
	text := "Hi " + hi + ",\n\n" + e.Title + " (" + e.Ref + ") is ready for your decision. Read it through, then approve it or ask for changes:\n\n" +
		url + "\n\nThe link is yours alone and works until " + until + ".\n\n--\n" + reason + "\n"
	who := brand + " has"
	if brand == "Runwell" {
		who = "We’ve"
	}
	color := SchemeColors["fizzy"]
	if s, err := a.settings(ctx); err == nil {
		color = s.BrandColor()
	}
	h := html.EscapeString
	body := `<!doctype html><html><body style="margin:0;padding:24px;background:#f6f6f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#1f2023">` +
		`<span style="display:none">` + h(e.Title+" ("+e.Ref+") is ready for your decision.") + `</span>` +
		`<div style="max-width:560px;margin:0 auto;background:#fff;border-radius:12px;padding:32px">` +
		`<h1 style="font-size:20px;margin:0 0 16px">` + h("Please review "+e.Title) + `</h1>` +
		`<p>Hi ` + h(hi) + `,</p>` +
		`<p>` + h(who) + ` sent <strong>` + h(e.Title) + `</strong> (` + h(e.Ref) + `) for your decision. Read it through, then approve it or ask for changes.</p>` +
		`<p style="margin:24px 0"><a href="` + h(url) + `" style="background:` + color + `;color:#fff;text-decoration:none;padding:10px 18px;border-radius:8px;display:inline-block">Review and decide</a></p>` +
		`<p style="font-size:13px;color:#6b6f76">The link is yours alone and works until ` + h(until) + `.</p>` +
		`</div><p style="max-width:560px;margin:16px auto 0;font-size:12px;color:#6b6f76">` + h(reason) + `</p></body></html>`
	return mail.Message{To: []string{email}, Subject: e.Title + ": " + strings.ToLower(v.Label()) + " for your approval", Text: text, HTML: body}
}

// deleteAgreements removes an engagement's versions with their items,
// decisions and links (it's being deleted or erased).
func (t *Tx) deleteAgreements(engagementID int64) error {
	ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM agreement_versions WHERE engagement_id = ?`, engagementID)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := t.deleteVersion(id); err != nil {
			return err
		}
	}
	return nil
}

// deleteVersion removes one version with its items, decision and links:
// the version's own rules (a sent one stays) are its callers'.
func (t *Tx) deleteVersion(id int64) error {
	items, err := store.Int64s(t.Ctx, t, `SELECT id FROM scope_items WHERE agreement_version_id = ?`, id)
	if err != nil {
		return err
	}
	for _, it := range items {
		if err := t.deleteScopeItem(it); err != nil {
			return err
		}
	}
	for _, q := range []string{
		`DELETE FROM approvals WHERE agreement_version_id = ?`,
		`DELETE FROM approval_links WHERE agreement_version_id = ?`,
		`UPDATE agreement_versions SET superseded_by_id = NULL WHERE superseded_by_id = ?`,
	} {
		if _, err := t.Exec(q, id); err != nil {
			return err
		}
	}
	if err := t.cascade("AgreementVersion", id); err != nil {
		return err
	}
	_, err = t.Exec(`DELETE FROM agreement_versions WHERE id = ?`, id)
	return err
}

// ---- handlers ------------------------------------------------------------------

// versionAnswer answers a version change: the summary and the version.
func (a *App) versionAnswer(c *Ctx, id int64, summary string) error {
	ctx := c.R.Context()
	v, e, err := a.versionAndEngagement(ctx, a.store, id)
	if err != nil {
		return err
	}
	list, err := a.versionsJSON(ctx, e, []*AgreementVersion{v})
	if err != nil {
		return err
	}
	c.Summary = summary
	c.Crumbs = engagementCrumbs(e)
	return c.JSON(200, O("summary", summary, "agreement_version", list[0], "engagement", a.agentRef("Engagement", e.ID, e.Ref, e.Ref+" "+e.Title, "/engagements/"+e.Ref)))
}

func (a *App) createAgreementVersion(c *Ctx) error {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	e, err := a.engagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	var v *AgreementVersion
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		v, err = t.draftVersion(e, str(body["kind"]), strings.TrimSpace(str(body["summary"])), strings.TrimSpace(str(body["reason"])))
		return err
	})
	if err != nil {
		return err
	}
	return a.versionAnswer(c, v.ID, v.Label()+" drafted.")
}

func (a *App) updateAgreementVersion(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("agreement_version")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	v, err := a.versionByID(ctx, a.store, id)
	if err != nil {
		return err
	}
	if v.Sent() {
		return refused(v.Label() + " has been sent and can no longer change.")
	}
	inv := apierr.Invalid{}
	fields := map[string]any{}
	if has(body, "summary") {
		fields["summary"] = nullable(strings.TrimSpace(str(body["summary"])))
	}
	if has(body, "reason") {
		fields["reason"] = nullable(sanitizeRichText(str(body["reason"])))
	}
	if has(body, "cadence") {
		cadence := str(body["cadence"])
		if cadence != "" && !slices.Contains(AgreementCadences, cadence) {
			inv.Add("cadence", "is not included in the list")
		}
		fields["cadence"] = nullable(cadence)
	}
	if has(body, "amount_cents") {
		n, ok := roundCents(body["amount_cents"])
		if !ok {
			inv.Add("amount_cents", "is not a number")
		}
		fields["amount_cents"] = n
	}
	if err := inv.Err(); err != nil {
		return refused(sentence(err))
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("agreement_versions", id, fields) }); err != nil {
		return err
	}
	return a.versionAnswer(c, id, "Draft updated.")
}

// sentence is a validation error's messages in one line ("Cadence is not
// included in the list"), as Rails' full_messages.to_sentence.
func sentence(err error) string {
	e, ok := errors.AsType[*apierr.Error](err)
	if !ok || e.Errors == nil {
		return err.Error()
	}
	keys := make([]string, 0, len(e.Errors))
	for k := range e.Errors {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	var parts []string
	for _, k := range keys {
		for _, m := range e.Errors[k] {
			if k == "base" {
				parts = append(parts, m)
			} else {
				parts = append(parts, humanize(k)+" "+m)
			}
		}
	}
	return toSentence(parts)
}

func (a *App) discardDraft(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	v, err := a.versionByID(ctx, a.store, id)
	if err != nil {
		return err
	}
	if v.Sent() {
		return refused("sent agreement versions cannot be deleted")
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteVersion(id) }); err != nil {
		return err
	}
	c.Summary = "Draft discarded."
	return c.JSON(200, O("summary", c.Summary, "deleted", true))
}

// activeContact is a client's contact who isn't archived, 0 when the id
// isn't one.
func (a *App) activeContact(ctx context.Context, clientID int64, v any) (int64, string, error) {
	id, _ := intOf(v)
	if id == 0 {
		return 0, "", nil
	}
	var name string
	err := a.store.QueryRowContext(ctx, `SELECT name FROM contacts WHERE id = ? AND client_id = ? AND archived_at IS NULL`, id, clientID).Scan(&name)
	if isNoRows(err) {
		return 0, "", nil
	}
	return id, name, err
}

// sendAgreement freezes the draft and, with a contact, emails them an
// approval link.
func (a *App) sendAgreement(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	v, e, err := a.versionAndEngagement(ctx, a.store, id)
	if err != nil {
		return err
	}
	contactID, contactName, err := a.activeContact(ctx, e.ClientID, body["contact_id"])
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.sendVersion(e, v); err != nil {
			return err
		}
		if contactID != 0 {
			return t.issueLink(e, v, contactID)
		}
		return nil
	})
	if err != nil {
		return err
	}
	if contactID != 0 {
		return a.versionAnswer(c, id, v.Label()+" sent to "+contactName+".")
	}
	return a.versionAnswer(c, id, v.Label()+" marked as sent.")
}

func (a *App) issueApprovalLink(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	v, e, err := a.versionAndEngagement(ctx, a.store, id)
	if err != nil {
		return err
	}
	contactID, contactName, err := a.activeContact(ctx, e.ClientID, body["contact_id"])
	if err != nil {
		return err
	}
	if contactID == 0 {
		return notFound("contact")
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.issueLink(e, v, contactID) }); err != nil {
		return err
	}
	return a.versionAnswer(c, id, "Approval link emailed to "+contactName+".")
}

// recordDecision is a person recording what the client decided, with
// evidence.
func (a *App) recordDecision(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	if str(body["decision"]) == "" {
		return apierr.BadRequest("param is missing or the value is empty: decision")
	}
	ctx := c.R.Context()
	v, e, err := a.versionAndEngagement(ctx, a.store, id)
	if err != nil {
		return err
	}
	contactID, _ := intOf(body["contact_id"])
	if contactID != 0 {
		if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM contacts WHERE id = ? AND client_id = ?`, contactID, e.ClientID); err != nil {
			return err
		} else if n == 0 {
			contactID = 0
		}
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		return t.decideVersion(e, v, Decision{Decision: str(body["decision"]), Method: "recorded", ContactID: contactID,
			ApproverName: strings.TrimSpace(str(body["approver_name"])), Comment: str(body["comment"]),
			Evidence: strings.TrimSpace(str(body["evidence"])), RecordedByID: c.Actor.UserID()})
	})
	if err != nil {
		return err
	}
	return a.versionAnswer(c, id, "Decision recorded.")
}

var breakTags = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`)
