package core

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Engagements: what we agreed with a client (a project, a work order, a
// service). The engagement is the container; each agreement version is what
// the client saw. Its state is never stored: it's read off the versions and
// their decisions (Engagement#state). An internal client's engagements are
// internal projects, with no agreement to send.

func init() {
	routeSets = append(routeSets, (*App).engagementRoutes)
	searchable("Engagement", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		var ref, title, desc string
		err := db.QueryRowContext(ctx, `SELECT ref, title, COALESCE(description, ''), client_id, created_at FROM engagements WHERE id = ?`, id).
			Scan(&ref, &title, &desc, &d.ClientID, &d.CreatedAt)
		if isNoRows(err) {
			return d, false, nil
		}
		d.Title = ref + " " + title
		d.Content = strings.TrimSpace(plainText(desc) + " " + customSearchText(ctx, db, "Engagement", id))
		return d, err == nil, err
	})
	// A client's answer lists its engagements, newest first.
	addSection("Client", "engagements", func(a *App, ctx context.Context, id int64) (any, error) {
		list, err := a.loadEngagements(ctx, a.store, `e.client_id = ?`, id)
		if err != nil {
			return nil, err
		}
		return a.engagementList(ctx, list)
	})
	// Deleting an engagement takes what it has with it (Rails' dependent:
	// :destroy): its agreements (versions, their scope items, decisions and
	// approval links), its work and its commitments, each with what its own
	// area registered for it.
	onDelete("Engagement", func(t *Tx, id int64) error {
		if err := t.deleteAgreements(id); err != nil {
			return err
		}
		for _, k := range []struct{ typ, table, extra string }{
			{"Todo", "todos", `DELETE FROM assignments WHERE todo_id = ?`},
			{"Commitment", "commitments", ""},
		} {
			ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM `+k.table+` WHERE engagement_id = ?`, id)
			if err != nil {
				return err
			}
			for _, rid := range ids {
				if err := t.cascade(k.typ, rid); err != nil {
					return err
				}
				if k.extra != "" {
					if _, err := t.Exec(k.extra, rid); err != nil {
						return err
					}
				}
				if _, err := t.Exec(`DELETE FROM `+k.table+` WHERE id = ?`, rid); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// EngagementLabels, shapes, filters and close outcomes, as the Rails model
// has them.
var (
	EngagementShapes        = []string{"fixed", "recurring"}
	EngagementFilters       = []string{"approved", "open", "draft", "sent", "changes_requested", "internal", "closed", "all"}
	EngagementCloseOutcomes = []string{"completed", "cancelled"}
)

// engagementFilterLabels are the filter's words (Engagement.filter_options).
var engagementFilterLabels = map[string]string{
	"approved": "Active", "open": "All open", "draft": "Draft", "sent": "Sent",
	"changes_requested": "Changes requested", "internal": "Internal", "closed": "Closed", "all": "All",
}

const engagementCreateNote = "A project, work order or service for a client. Its agreement starts as a draft (create_agreement_version, add_scope_item), and it is sent to the client with send_agreement. For an internal client (yourself) it's an internal project: no agreement, add work straight away (create_work)."

func (a *App) engagementRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/engagements", a.listEngagements, tool("list_engagements", "List engagements",
		note("By derived state: approved (the default: agreed and not closed), open (every open one), draft, sent, changes_requested, closed or all."),
		query("state", "string", strings.Join(EngagementFilters, ", ")+"."),
		query("label", "string", strings.Join(append(slices.Clone(Labels), "all"), ", ")+".")))
	a.handle(mux, "GET /api/engagements/{ref}", a.showEngagement, tool("show_engagement", "Show an engagement",
		note("An engagement with its agreement versions (drafts and sent), scope, work, commitments, notes, documents and history."),
		next("create_agreement_version", "add_scope_item", "create_work")))
	a.handle(mux, "POST /api/engagements", a.createEngagement, tool("create_engagement", "Start an engagement", note(engagementCreateNote),
		body("engagement", `{client_id (required), label: project|work_order|service, shape: fixed|recurring, title (required), description, estimate_notes, custom_fields: {key: value}}`, true),
		next("add_scope_item", "send_agreement")))
	a.handle(mux, "POST /api/clients/{id}/engagements", a.createEngagement)
	a.handle(mux, "PATCH /api/engagements/{ref}", a.updateEngagement, tool("update_engagement", "Change an engagement",
		body("engagement", `{title, description, estimate_notes, label: project|work_order|service, shape: fixed|recurring, custom_fields: {key: value}}`, true)))
	a.handle(mux, "DELETE /api/engagements/{ref}", a.deleteEngagement, need("delete_records"), tool("delete_engagement", "Delete an engagement",
		note("Only while nothing was ever sent to the client: its draft, work and notes go with it.")))
	a.handle(mux, "POST /api/engagements/{ref}/close", a.closeEngagement, need("close_engagements"), tool("close_engagement", "Mark an engagement complete, or cancel it",
		note("outcome: completed (delivered; the default) or cancelled (stopped before then). Either way it's closed: off the open list, its agreement and history kept."),
		bodyParam("outcome", "string", "completed or cancelled.", false), bodyParam("reason", "string", "Why, in a few words.", false)))
	a.handle(mux, "POST /api/engagements/{ref}/reopen", a.reopenEngagement, need("close_engagements"), tool("reopen_engagement", "Reopen a closed engagement"))
	a.handle(mux, "POST /api/engagements/{ref}/erasure", a.eraseEngagement, need("erase_engagements"), tool("erase_engagement", "Delete a closed engagement permanently, with its agreements",
		note("Only a closed engagement. Removes it with its agreements (sent ones too), the client's decisions, work, commitments, notes, documents and history; the client keeps one event. confirm_ref must be the engagement's ref. Can't be undone: ask the person first."),
		bodyParam("confirm_ref", "string", "The engagement's ref, typed to mean it.", true),
		confirm("It deletes the engagement and the record of what the client agreed to, for good.")))
}

// ---- the record --------------------------------------------------------------

// Engagement is one engagement, with its client's name and whether it's
// internal, and its agreement versions once loaded.
type Engagement struct {
	ID             int64
	ClientID       int64
	Ref            string
	Label          string
	Shape          string
	Title          string
	Description    string
	EstimateNotes  string
	ClosedAt       store.Time
	CloseReason    string
	CloseOutcome   string
	CreatedByID    int64
	CreatedAt      store.Time
	UpdatedAt      store.Time
	ClientName     string
	ClientInternal bool
	ClientTimeZone string
	Versions       []*AgreementVersion // by number
}

const engagementSelect = `SELECT e.id, e.client_id, e.ref, e.label, e.shape, e.title, COALESCE(e.description, ''), COALESCE(e.estimate_notes, ''),
	e.closed_at, COALESCE(e.close_reason, ''), COALESCE(e.close_outcome, ''), COALESCE(e.created_by_id, 0), e.created_at, e.updated_at,
	c.name, c.internal, COALESCE(c.time_zone, '')
	FROM engagements e JOIN clients c ON c.id = e.client_id`

func scanEngagement(r interface{ Scan(...any) error }) (*Engagement, error) {
	var e Engagement
	err := r.Scan(&e.ID, &e.ClientID, &e.Ref, &e.Label, &e.Shape, &e.Title, &e.Description, &e.EstimateNotes,
		&e.ClosedAt, &e.CloseReason, &e.CloseOutcome, &e.CreatedByID, &e.CreatedAt, &e.UpdatedAt,
		&e.ClientName, &e.ClientInternal, &e.ClientTimeZone)
	return &e, err
}

// loadEngagements are the engagements matching where (on e, the engagement,
// and c, its client), newest first, each with its versions and decisions:
// one query for the engagements, one for their versions.
func (a *App) loadEngagements(ctx context.Context, db store.Querier, where string, args ...any) ([]*Engagement, error) {
	rows, err := db.QueryContext(ctx, engagementSelect+` WHERE `+where+` ORDER BY e.created_at DESC, e.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (*Engagement, error) { return scanEngagement(r) })
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(list))
	for i, e := range list {
		ids[i] = e.ID
	}
	versions, err := a.versionsOf(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	for _, e := range list {
		e.Versions = versions[e.ID]
	}
	return list, nil
}

// engagementByRef is the engagement with ref (WO-12, any case) and its
// versions; notFound when there's none. Other areas find an engagement
// from a path or an agent's argument with it.
func (a *App) engagementByRef(ctx context.Context, db store.Querier, ref string) (*Engagement, error) {
	list, err := a.loadEngagements(ctx, db, `e.ref = ?`, strings.ToUpper(strings.TrimSpace(ref)))
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, notFound("engagement")
	}
	return list[0], nil
}

// engagementByID is the same by id.
func (a *App) engagementByID(ctx context.Context, db store.Querier, id int64) (*Engagement, error) {
	list, err := a.loadEngagements(ctx, db, `e.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, notFound("engagement")
	}
	return list[0], nil
}

// Closed is whether it's closed (completed or cancelled).
func (e *Engagement) Closed() bool { return e.ClosedAt.Valid }

// Internal is whether it's your own project: no agreement, price or approval.
func (e *Engagement) Internal() bool { return e.ClientInternal }

// Recurring is whether it's a recurring service rather than fixed scope.
func (e *Engagement) Recurring() bool { return e.Shape == "recurring" }

// CurrentVersion is the latest version the client approved: the terms in force.
func (e *Engagement) CurrentVersion() *AgreementVersion {
	for _, v := range slices.Backward(e.Versions) {
		if v.Approved() {
			return v
		}
	}
	return nil
}

// DraftVersion is the open draft, if there is one.
func (e *Engagement) DraftVersion() *AgreementVersion {
	for _, v := range slices.Backward(e.Versions) {
		if v.Draft() {
			return v
		}
	}
	return nil
}

// PendingVersion is the version waiting on the client's decision.
func (e *Engagement) PendingVersion() *AgreementVersion {
	for _, v := range slices.Backward(e.Versions) {
		if v.AwaitingDecision() {
			return v
		}
	}
	return nil
}

// State is derived: closed, internal, draft, sent, changes_requested or
// approved, in that order of precedence.
func (e *Engagement) State() string {
	if e.Closed() {
		return "closed"
	}
	if e.Internal() {
		return "internal"
	}
	if len(e.Versions) == 0 {
		return "draft"
	}
	latest := e.Versions[len(e.Versions)-1]
	switch {
	case latest.Draft():
		return "draft"
	case latest.AwaitingDecision():
		return "sent"
	case latest.ChangesRequested():
		return "changes_requested"
	}
	return "approved"
}

// StateLabel is the state in words: "Completed" or "Cancelled" once closed.
func (e *Engagement) StateLabel() string {
	if e.Closed() {
		return humanize(strOr(e.CloseOutcome, "closed"))
	}
	return humanize(e.State())
}

// Active is whether it's underway: agreed with the client, or internal.
func (e *Engagement) Active() bool { s := e.State(); return s == "approved" || s == "internal" }

// Deletable is whether nothing was ever sent: anything sent is history.
func (e *Engagement) Deletable() bool {
	return !slices.ContainsFunc(e.Versions, func(v *AgreementVersion) bool { return v.Sent() })
}

// AgreedAmountCents is fixed scope's initial amount plus every approved
// change, or a recurring service's latest approved amount per period.
func (e *Engagement) AgreedAmountCents() int64 {
	var approved []*AgreementVersion
	for _, v := range e.Versions {
		if v.Approved() {
			approved = append(approved, v)
		}
	}
	if len(approved) == 0 {
		return 0
	}
	if e.Recurring() {
		return approved[len(approved)-1].AmountCents
	}
	var sum int64
	for _, v := range approved {
		if v.Kind == "initial" {
			sum += v.AmountCents
		} else {
			sum += v.PriceDeltaCents
		}
	}
	return sum
}

// zone is where the client is, for dates on its pages and mail.
func (a *App) clientZone(ctx context.Context, tz string) *time.Location {
	if tz == "" {
		return a.zone(ctx)
	}
	return location(tz)
}

// formatCents is money as the Rails core wrote it: "$1,500.00".
func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign, cents = "-", -cents
	}
	whole := strconv.FormatInt(cents/100, 10)
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	return fmt.Sprintf("%s$%s.%02d", sign, b.String(), cents%100)
}

// longDate is a date as the Rails core's :long format: "October 18, 2026".
func longDate(t store.Time, loc *time.Location) string {
	if !t.Valid {
		return ""
	}
	return t.Time.In(loc).Format("January 2, 2006")
}

// ---- JSON --------------------------------------------------------------------

// engagementJSON is an engagement as answers show it (engagements/_engagement.json),
// with what the app's lists and page add.
func (a *App) engagementJSON(e *Engagement, custom Obj, s *Settings) Obj {
	if custom == nil {
		custom = Obj{}
	}
	amount := e.AgreedAmountCents()
	o := a.agentRef("Engagement", e.ID, e.Ref, e.Ref+" "+e.Title, "/engagements/"+e.Ref).
		Set("title", e.Title).Set("label", e.Label).Set("shape", e.Shape).
		Set("type_name", s.LabelName(e.Label, 1)).Set("state", e.State()).Set("state_label", e.StateLabel())
	if e.Closed() {
		o = o.Set("close_outcome", nullable(e.CloseOutcome))
	}
	return o.Set("internal", e.Internal()).Set("agreed_amount", formatCents(amount)).Set("agreed_amount_cents", amount).
		Set("client", a.agentRef("Client", e.ClientID, "", e.ClientName, fmt.Sprintf("/clients/%d", e.ClientID))).
		Set("closed", e.Closed()).Set("custom_fields", custom).
		Set("active", e.Active()).Set("deletable", e.Deletable()).
		Set("closed_at", e.ClosedAt).Set("close_reason", nullable(e.CloseReason)).Set("created_at", e.CreatedAt)
}

// engagementList is engagements as a list answers them, custom values read
// in one query.
func (a *App) engagementList(ctx context.Context, list []*Engagement) ([]Obj, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]int64, len(list))
	for i, e := range list {
		ids[i] = e.ID
	}
	custom, err := a.customValues(ctx, a.store, "Engagement", ids)
	if err != nil {
		return nil, err
	}
	out := make([]Obj, len(list))
	for i, e := range list {
		out[i] = a.engagementJSON(e, custom[e.ID], s)
	}
	return out, nil
}

// engagementFilter is the list filter from the query: state=, or the old
// status=open|closed, else approved.
func engagementFilter(c *Ctx) string {
	if st := c.Query("state"); slices.Contains(EngagementFilters, st) {
		return st
	}
	if st := c.Query("status"); st == "open" || st == "closed" {
		return st
	}
	return "approved"
}

// engagementMatches is whether e shows under the filter.
func engagementMatches(e *Engagement, filter string) bool {
	switch filter {
	case "all":
		return true
	case "closed":
		return e.Closed()
	case "open":
		return !e.Closed()
	case "approved":
		return e.Active()
	}
	return e.State() == filter
}

// engagementCounts are how many each filter would show (Engagement.filter_counts).
func engagementCounts(list []*Engagement) map[string]int {
	tally := map[string]int{}
	for _, e := range list {
		tally[e.State()]++
	}
	open := len(list) - tally["closed"]
	tally["approved"] += tally["internal"]
	tally["open"] = open
	tally["all"] = len(list)
	return tally
}

// engagementSorts are the engagements table's sortable columns.
var engagementSorts = sortPaths{"ref": by("ref"), "title": by("title"), "type": by("label"), "client": by("client", "display_name")}

func (a *App) listEngagements(c *Ctx) error {
	ctx := c.R.Context()
	state := engagementFilter(c)
	label := c.Query("label")
	if !slices.Contains(Labels, label) {
		label = "all"
	}
	where, args := "1 = 1", []any{}
	if label != "all" {
		where, args = "e.label = ?", []any{label}
	}
	all, err := a.loadEngagements(ctx, a.store, where, args...)
	if err != nil {
		return err
	}
	counts := engagementCounts(all)
	var shown []*Engagement
	for _, e := range all {
		if engagementMatches(e, state) {
			shown = append(shown, e)
		}
	}
	list, err := a.engagementList(ctx, shown)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	filters := make([]Obj, len(EngagementFilters))
	for i, f := range EngagementFilters {
		filters[i] = O("value", f, "label", engagementFilterLabels[f], "count", counts[f])
	}
	labels := []Obj{O("value", "all", "label", "All types")}
	for _, l := range Labels {
		if s.LabelEnabled(l) || l == label {
			labels = append(labels, O("value", l, "label", s.LabelName(l, 2)))
		}
	}
	summary := pluralize(len(shown), strings.ToLower(s.Term("engagement", 1))) + " (" + strings.ToLower(humanize(state))
	if label != "all" {
		summary += ", " + label
	}
	c.Summary = summary + ")"
	return c.listAnswer(O("summary", c.Summary, "state", state, "label", label, "filters", filters, "labels", labels), "engagements", list, engagementSorts)
}

func (a *App) showEngagement(c *Ctx) error {
	ctx := c.R.Context()
	e, err := a.engagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	o, err := a.engagementShowJSON(ctx, e)
	if err != nil {
		return err
	}
	c.Summary = e.Ref + " " + e.Title + ": " + strings.ToLower(e.StateLabel())
	c.Crumbs = engagementCrumbs(e)
	return c.JSON(200, O("summary", c.Summary, "engagement", o))
}

// engagementCrumbs are the next steps an engagement's answer suggests.
func engagementCrumbs(e *Engagement) []Crumb {
	switch {
	case e.Closed():
		return []Crumb{step("Reopen it", "reopen_engagement", "ref", e.Ref)}
	case e.Internal():
		return []Crumb{step("Add work", "create_work", "ref", e.Ref)}
	case e.DraftVersion() != nil:
		d := e.DraftVersion()
		return []Crumb{step("Add a scope item", "add_scope_item", "agreement_version_id", d.ID), step("Send it to the client", "send_agreement", "id", d.ID)}
	case e.PendingVersion() != nil:
		return []Crumb{step("Record the client’s decision", "record_decision", "id", e.PendingVersion().ID)}
	}
	return []Crumb{step("Add work", "create_work", "ref", e.Ref), step("Start a new version", "create_agreement_version", "ref", e.Ref)}
}

// engagementShowJSON is the show answer (engagements/show.json): the
// engagement, its text, its agreement versions, what other areas add
// (work, commitments, notes, documents), and its history, with what the
// page shows beside them (the client's contacts, the scope column, the
// next step).
func (a *App) engagementShowJSON(ctx context.Context, e *Engagement) (Obj, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	if err := a.loadItems(ctx, a.store, e.Versions); err != nil {
		return nil, err
	}
	custom, err := a.customValues(ctx, a.store, "Engagement", []int64{e.ID})
	if err != nil {
		return nil, err
	}
	versions, err := a.versionsJSON(ctx, e, e.Versions)
	if err != nil {
		return nil, err
	}
	o := a.engagementJSON(e, custom[e.ID], s).
		Set("description", nullable(agentText(e.Description))).
		Set("estimate_notes", nullable(agentText(e.EstimateNotes))).
		Set("description_html", richTextHTML(e.Description)).
		Set("estimate_notes_html", richTextHTML(e.EstimateNotes)).
		Set("agreement_versions", versions)
	o, err = a.withSections(ctx, "Engagement", e.ID, o)
	if err != nil {
		return nil, err
	}
	history, err := a.eventsJSON(ctx, `e.subject_type = 'Engagement' AND e.subject_id = ?`, 20, e.ID)
	if err != nil {
		return nil, err
	}
	contacts, err := a.contactsJSON(ctx, `client_id = ? AND archived_at IS NULL`, e.ClientID)
	if err != nil {
		return nil, err
	}
	scope, err := a.engagementScope(ctx, e)
	if err != nil {
		return nil, err
	}
	next, err := a.engagementNextStep(ctx, e, s)
	if err != nil {
		return nil, err
	}
	agreed := []Obj{}
	for _, it := range e.agreedItems() {
		agreed = append(agreed, a.scopeItemJSON(it))
	}
	return o.Set("history", history).
		Set("recurring", e.Recurring()).
		Set("prices", s.Prices).
		Set("next_step", nullable(next.Text)).Set("next_step_link", next.Link).
		Set("agreed_items", agreed).
		Set("scope", scope).
		Set("new_version_kinds", newVersionKinds(e)).
		Set("client_contacts", contacts), nil
}

// agreedItems are the scope in force: every item of every approved version.
func (e *Engagement) agreedItems() []*ScopeItem {
	var out []*ScopeItem
	for _, v := range e.Versions {
		if v.Approved() {
			out = append(out, v.Items...)
		}
	}
	return out
}

// agentText is plain text from a rich text column, for a model to read.
func agentText(html string) string {
	if strings.TrimSpace(html) == "" {
		return ""
	}
	s := breakTags.ReplaceAllString(html, "\n")
	s = tags.ReplaceAllString(s, "")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
	lines := strings.Split(s, "\n")
	var kept []string
	for _, l := range lines {
		if l = strings.TrimSpace(l); l != "" {
			kept = append(kept, l)
		}
	}
	return strings.Join(kept, "\n")
}

// engagementScope is the scope down the page's side: all the work, each
// agreed scope item, items outside the scope that have work, and work
// outside the scope, each with how much is done. Read straight from the
// todos table, in one query.
func (a *App) engagementScope(ctx context.Context, e *Engagement) (Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT COALESCE(scope_item_id, 0), status FROM todos WHERE engagement_id = ?`, e.ID)
	if err != nil {
		return nil, err
	}
	type pair struct {
		item   int64
		status string
	}
	pairs, err := store.ScanAll(rows, func(r *sqlRows) (pair, error) {
		var p pair
		return p, r.Scan(&p.item, &p.status)
	})
	if err != nil {
		return nil, err
	}
	statuses := map[int64][]string{}
	var everything []string
	for _, p := range pairs {
		statuses[p.item] = append(statuses[p.item], p.status)
		everything = append(everything, p.status)
	}
	progress := func(list []string) (int, int) {
		done := 0
		for _, st := range list {
			if st == "done" {
				done++
			}
		}
		return done, len(list)
	}
	items := []Obj{}
	seen := map[int64]bool{}
	add := func(id int64, desc string) {
		seen[id] = true
		done, total := progress(statuses[id])
		items = append(items, O("id", id, "description", desc, "delivery_state", deliveryStateOf(statuses[id]), "done", done, "total", total))
	}
	for _, it := range e.agreedItems() {
		add(it.ID, it.Description)
	}
	var extra []int64
	for id := range statuses {
		if id != 0 && !seen[id] {
			extra = append(extra, id)
		}
	}
	slices.Sort(extra)
	if len(extra) > 0 {
		args := idArgs(extra)
		rows, err := a.store.QueryContext(ctx, `SELECT id, description FROM scope_items WHERE id IN (`+store.Placeholders(len(extra))+`) ORDER BY id`, args...)
		if err != nil {
			return nil, err
		}
		type named struct {
			id   int64
			desc string
		}
		list, err := store.ScanAll(rows, func(r *sqlRows) (named, error) {
			var n named
			return n, r.Scan(&n.id, &n.desc)
		})
		if err != nil {
			return nil, err
		}
		for _, n := range list {
			add(n.id, n.desc)
		}
	}
	var other any
	if list, ok := statuses[0]; ok {
		done, total := progress(list)
		other = O("done", done, "total", total)
	}
	done, total := progress(everything)
	return O("done", done, "total", total, "items", items, "other", other), nil
}

// nextStep is the line under an engagement's state saying what happens
// next, and a link it may carry.
type nextStep struct {
	Text string
	Link any
}

// engagementNextStep is engagement_next_step: build the draft, send it,
// wait on the client, revise, then do the work.
func (a *App) engagementNextStep(ctx context.Context, e *Engagement, s *Settings) (nextStep, error) {
	loc := a.zone(ctx)
	scopeItems := strings.ToLower(s.Term("scope_item", 2))
	work := s.Term("work", 1)
	switch e.State() {
	case "draft":
		draft := e.DraftVersion()
		approvers, err := store.Strings(ctx, a.store, `SELECT name FROM contacts WHERE client_id = ? AND archived_at IS NULL AND can_approve = 1 ORDER BY name COLLATE NOCASE`, e.ClientID)
		if err != nil {
			return nextStep{}, err
		}
		switch {
		case draft == nil:
			return nextStep{Text: "Next: start the agreement below, add its " + scopeItems + ", then send it to the client."}, nil
		case len(draft.Items) == 0:
			return nextStep{Text: "Next: add " + scopeItems + " to the draft below, then send it to the client."}, nil
		case len(approvers) == 0:
			return nextStep{Text: "Next: nobody at " + e.ClientName + " can approve yet. Add a contact who can approve, then send the draft.",
				Link: O("label", "Add a contact who can approve", "href", fmt.Sprintf("/clients/%d", e.ClientID))}, nil
		}
		return nextStep{Text: "Next: send the draft to " + toSentence(approvers) + " to approve."}, nil
	case "sent":
		v := e.PendingVersion()
		names, err := store.Strings(ctx, a.store, `SELECT c.name FROM approval_links l JOIN contacts c ON c.id = l.contact_id WHERE l.agreement_version_id = ? GROUP BY c.id ORDER BY MIN(l.id)`, v.ID)
		if err != nil {
			return nextStep{}, err
		}
		waiting := e.ClientName
		if len(names) > 0 {
			waiting = toSentence(names)
		}
		return nextStep{Text: "Waiting on " + waiting + " since " + longDate(v.SentAt, loc) + ". Email the link again or record their decision below."}, nil
	case "changes_requested":
		ap := e.Versions[len(e.Versions)-1].Approval
		who := ap.ContactName
		if who == "" {
			who = strOr(ap.ApproverName, e.ClientName)
		}
		fix := "a change order"
		if e.Recurring() {
			fix = "a revision"
		}
		return nextStep{Text: who + " asked for changes on " + longDate(ap.DecidedAt, loc) + ". Start " + fix + " below and send it again."}, nil
	case "approved", "internal":
		var done, open int
		if err := a.store.QueryRowContext(ctx, `SELECT COALESCE(SUM(status = 'done'), 0), COALESCE(SUM(status != 'done'), 0) FROM todos WHERE engagement_id = ?`, e.ID).Scan(&done, &open); err != nil {
			return nextStep{}, err
		}
		finish := ""
		if open == 0 && done > 0 && !e.Recurring() {
			finish = " Everything’s done: mark it complete in Details."
		}
		if e.State() == "internal" {
			w := fmt.Sprintf("%d open, %d done", open, done)
			switch {
			case done+open == 0:
				w = "nothing yet, so add it below"
			case open == 0:
				w = "all done"
			}
			return nextStep{Text: "Internal: no agreement to send. " + work + ": " + w + "." + finish}, nil
		}
		w := fmt.Sprintf("%d open, %d done", open, done)
		if open == 0 {
			w = "all done"
		}
		return nextStep{Text: "Approved " + longDate(e.CurrentVersion().Approval.DecidedAt, loc) + ". " + work + ": " + w + ". Changes go through a new version." + finish}, nil
	case "closed":
		text := e.StateLabel() + " " + longDate(e.ClosedAt, loc)
		if e.CloseReason != "" {
			text += ": " + e.CloseReason
		}
		return nextStep{Text: text + ". Its agreement and history stay; reopen it if work starts again."}, nil
	}
	return nextStep{}, nil
}

// toSentence is Rails' to_sentence: "A", "A and B", "A, B and C".

// ---- writes --------------------------------------------------------------------

// engagementParams reads and checks an engagement's fields; partial for an
// update (which never changes the client or the shape).
func (a *App) engagementParams(ctx context.Context, body map[string]any, partial bool) (map[string]any, error) {
	inv := apierr.Invalid{}
	out := map[string]any{}
	if !partial {
		id, _ := intOf(body["client_id"])
		if id == 0 {
			inv.Add("client", "must exist")
		} else if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM clients WHERE id = ?`, id); err != nil {
			return nil, err
		} else if n == 0 {
			inv.Add("client", "must exist")
		}
		out["client_id"] = id
	}
	if has(body, "title") || !partial {
		title := strings.TrimSpace(str(body["title"]))
		if title == "" {
			inv.Add("title", "can't be blank")
		}
		out["title"] = title
	}
	if has(body, "label") || !partial {
		label := str(body["label"])
		if label == "" && !partial {
			label = "work_order"
		}
		if !slices.Contains(Labels, label) {
			inv.Add("label", "is not included in the list")
		}
		out["label"] = label
	}
	if !partial {
		shape := str(body["shape"])
		if shape == "" {
			shape = "fixed"
		}
		if !slices.Contains(EngagementShapes, shape) {
			inv.Add("shape", "is not included in the list")
		}
		out["shape"] = shape
	}
	for _, k := range []string{"description", "estimate_notes"} {
		if has(body, k) {
			out[k] = nullable(sanitizeRichText(str(body[k])))
		}
	}
	return out, inv.Err()
}

// nextRef is the next ref for a label: its prefix (Settings › Names) and one
// past the highest number with that prefix.
func (t *Tx) nextRef(label string) (string, error) {
	s, err := t.App.settings(t.Ctx)
	if err != nil {
		return "", err
	}
	prefix := s.LabelPrefix(label)
	refs, err := store.Strings(t.Ctx, t, `SELECT ref FROM engagements WHERE ref LIKE ?`, prefix+"-%")
	if err != nil {
		return "", err
	}
	last := 0
	for _, r := range refs {
		n, _ := strconv.Atoi(r[strings.LastIndex(r, "-")+1:])
		last = max(last, n)
	}
	return fmt.Sprintf("%s-%d", prefix, last+1), nil
}

// insertEngagement starts an engagement from checked fields (engagementParams)
// and its custom field values: its ref, its history, and, for a client
// that isn't us, the draft agreement it starts with.
func (t *Tx) insertEngagement(fields, custom map[string]any) (id int64, ref string, internal bool, err error) {
	if ref, err = t.nextRef(str(fields["label"])); err != nil {
		return 0, "", false, err
	}
	if err := t.QueryRow(`SELECT internal FROM clients WHERE id = ?`, fields["client_id"]).Scan(&internal); err != nil {
		return 0, "", false, err
	}
	now := store.Now().DB()
	var createdBy any
	if uid := t.Actor.UserID(); uid != 0 {
		createdBy = uid
	}
	id, err = t.Insert(`INSERT INTO engagements (client_id, ref, label, shape, title, description, estimate_notes, created_by_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		fields["client_id"], ref, fields["label"], fields["shape"], fields["title"], fields["description"], fields["estimate_notes"], createdBy, now, now)
	if err != nil {
		return 0, "", false, err
	}
	if custom != nil {
		if err := t.setCustomFields("Engagement", id, custom); err != nil {
			return 0, "", false, err
		}
	}
	t.Reindex("Engagement", id)
	if err := t.Event("Engagement", id, "engagement.created", nil); err != nil {
		return 0, "", false, err
	}
	if err := t.recordMentions("Engagement", id, str(fields["description"])+" "+str(fields["estimate_notes"])); err != nil {
		return 0, "", false, err
	}
	if internal {
		return id, ref, true, nil
	}
	e, err := t.App.engagementByID(t.Ctx, t, id)
	if err != nil {
		return 0, "", false, err
	}
	_, err = t.draftVersion(e, "", "", "")
	return id, ref, false, err
}

func (a *App) createEngagement(c *Ctx) error {
	body, err := c.Wrapped("engagement")
	if err != nil {
		return err
	}
	if c.Param("id") != "" {
		id, err := c.IntParam("id")
		if err != nil {
			return err
		}
		body["client_id"] = id
	}
	ctx := c.R.Context()
	fields, err := a.engagementParams(ctx, body, false)
	if err != nil {
		return err
	}
	var id int64
	var ref string
	var internal bool
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		custom, _ := body["custom_fields"].(map[string]any)
		id, ref, internal, err = t.insertEngagement(fields, custom)
		return err
	})
	if err != nil {
		return err
	}
	e, err := a.engagementByID(ctx, a.store, id)
	if err != nil {
		return err
	}
	o, err := a.engagementShowJSON(ctx, e)
	if err != nil {
		return err
	}
	if internal {
		c.Summary = ref + " started. It's internal: add work as it comes up."
	} else {
		c.Summary = ref + " drafted."
	}
	c.Crumbs = engagementCrumbs(e)
	return c.JSON(201, O("summary", c.Summary, "engagement", o))
}

func (a *App) updateEngagement(c *Ctx) error {
	body, err := c.Wrapped("engagement")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	e, err := a.engagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	fields, err := a.engagementParams(ctx, body, true)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.update("engagements", e.ID, fields); err != nil {
			return err
		}
		if err := t.recordMentions("Engagement", e.ID, str(fields["description"])+" "+str(fields["estimate_notes"])); err != nil {
			return err
		}
		if custom, ok := body["custom_fields"].(map[string]any); ok {
			if err := t.setCustomFields("Engagement", e.ID, custom); err != nil {
				return err
			}
		}
		t.Reindex("Engagement", e.ID)
		return nil
	})
	if err != nil {
		return err
	}
	return a.answerEngagement(c, e.ID, e.Ref+" updated.")
}

// answerEngagement answers with the engagement as show does, after a change.
func (a *App) answerEngagement(c *Ctx, id int64, summary string) error {
	ctx := c.R.Context()
	e, err := a.engagementByID(ctx, a.store, id)
	if err != nil {
		return err
	}
	o, err := a.engagementShowJSON(ctx, e)
	if err != nil {
		return err
	}
	c.Summary = summary
	c.Crumbs = engagementCrumbs(e)
	return c.JSON(200, O("summary", summary, "engagement", o))
}

// refused is a model verb saying no (Rails' redirect with an alert): a 422
// in words.
func refused(msg string) error { return &apierr.Error{Status: 422, Code: "refused", Message: msg} }

func (a *App) deleteEngagement(c *Ctx) error {
	ctx := c.R.Context()
	e, err := a.engagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	if !e.Deletable() {
		return refused(e.Ref + " was sent to the client, so it can’t be deleted. Close it instead.")
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.destroyEngagement(e.ID) }); err != nil {
		return err
	}
	c.Summary = e.Ref + " deleted."
	return c.JSON(200, O("summary", c.Summary, "deleted", true))
}

// destroyEngagement removes an engagement with everything that hangs off
// it: what areas registered (its agreements here; work, commitments, notes
// and documents there), its events and custom values.
func (t *Tx) destroyEngagement(id int64) error {
	if err := t.cascade("Engagement", id); err != nil {
		return err
	}
	_, err := t.Exec(`DELETE FROM engagements WHERE id = ?`, id)
	return err
}

// closeEngagement closes e as completed (the default) or cancelled, with
// an optional reason.
func (t *Tx) closeEngagement(e *Engagement, outcome, reason string) (string, error) {
	if !slices.Contains(EngagementCloseOutcomes, outcome) {
		outcome = "completed"
	}
	if _, err := t.Exec(`UPDATE engagements SET closed_at = ?, close_reason = ?, close_outcome = ?, updated_at = ? WHERE id = ?`,
		store.Now().DB(), nullable(reason), outcome, store.Now().DB(), e.ID); err != nil {
		return "", err
	}
	var r any
	if reason != "" {
		r = reason
	}
	return outcome, t.Event("Engagement", e.ID, "engagement.closed", map[string]any{"outcome": outcome, "reason": r})
}

func (a *App) closeEngagement(c *Ctx) error {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	e, err := a.engagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	var outcome string
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		outcome, err = t.closeEngagement(e, str(body["outcome"]), strings.TrimSpace(str(body["reason"])))
		return err
	})
	if err != nil {
		return err
	}
	return a.answerEngagement(c, e.ID, e.Ref+" "+outcome+".")
}

func (a *App) reopenEngagement(c *Ctx) error {
	ctx := c.R.Context()
	e, err := a.engagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	if !e.Closed() {
		return refused(e.Ref + " is open.")
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if _, err := t.Exec(`UPDATE engagements SET closed_at = NULL, close_reason = NULL, close_outcome = NULL, updated_at = ? WHERE id = ?`, store.Now().DB(), e.ID); err != nil {
			return err
		}
		return t.Event("Engagement", e.ID, "engagement.reopened", nil)
	})
	if err != nil {
		return err
	}
	return a.answerEngagement(c, e.ID, e.Ref+" is open again.")
}

// eraseEngagement deletes a closed engagement for good, its agreements
// (sent ones too) and the client's decisions included: the one exception to
// "sent is forever", for an owner who types the ref to mean it. The client
// keeps one event saying so.
func (a *App) eraseEngagement(c *Ctx) error {
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	e, err := a.engagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	if !e.Closed() {
		return refused("Close " + e.Ref + " first; only a closed one can be deleted permanently.")
	}
	if !strings.EqualFold(strings.TrimSpace(str(body["confirm_ref"])), e.Ref) {
		return refused("Type " + e.Ref + " to confirm deleting it.")
	}
	sent := 0
	for _, v := range e.Versions {
		if v.Sent() {
			sent++
		}
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		todos, err := store.Count(t.Ctx, t, `SELECT count(*) FROM todos WHERE engagement_id = ?`, e.ID)
		if err != nil {
			return err
		}
		if err := t.destroyEngagement(e.ID); err != nil {
			return err
		}
		return t.Event("Client", e.ClientID, "engagement.erased", map[string]any{"ref": e.Ref, "title": e.Title, "versions": sent, "todos": todos})
	})
	if err != nil {
		return err
	}
	c.Summary = e.Ref + " " + e.Title + " is deleted, with its agreements and history."
	return c.JSON(200, O("summary", c.Summary, "deleted", true, "client", a.agentRef("Client", e.ClientID, "", e.ClientName, fmt.Sprintf("/clients/%d", e.ClientID))))
}

// roundCents is a money amount sent as a number (12.5 dollars never: always
// cents) as a whole number of cents.
func roundCents(v any) (int64, bool) {
	switch x := v.(type) {
	case float64:
		if x != math.Trunc(x) {
			return 0, false
		}
		return int64(x), true
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return 0, true
		}
		n, err := strconv.ParseInt(s, 10, 64)
		return n, err == nil
	case nil:
		return 0, true
	}
	return intOf(v)
}
