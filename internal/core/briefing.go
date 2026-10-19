package core

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// The briefing: what needs a person right now. It is home, and the agent's
// first tool: questions for you, requests to triage, agreements waiting on
// a client, drafts ready to send, promises due (theirs and ours), work in
// review, blocked and overdue, and whatever plugins add. Each section shows
// its first few, with a count of all.

func init() { routeSets = append(routeSets, (*App).briefingRoutes) }

func (a *App) briefingRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/briefing", a.showBriefing, tool("briefing", "What needs a person now",
		note("Start here. The home page: questions for you, requests to triage, agreements awaiting the client, drafts ready to send, overdue and due commitments, blocked and overdue work, and anything plugins add. Each item names the tool that shows it.")))
}

// BriefingSection is one part of the briefing: its first items and a count
// of all of them.
type BriefingSection struct {
	Key, Title string
	Items      []Obj
	Count      int
}

// briefingSections are more sections, added by plugins' areas in an init:
// each answers its items for the person (all of them; the briefing keeps
// the first few).
var briefingSections []func(a *App, ctx context.Context, actor *Actor) (key, title string, items []Obj, err error)

// briefingShown is how many of each section home shows; the briefing tool
// lists 25.
const briefingShown = 8

func (a *App) showBriefing(c *Ctx) error {
	ctx := c.R.Context()
	home := c.truthy("home")
	limit := 25
	if home {
		limit = briefingShown
	}
	sections, err := a.briefing(ctx, c.Actor, limit)
	if err != nil {
		return err
	}
	list := make([]Obj, len(sections))
	parts := make([]string, len(sections))
	for i, s := range sections {
		list[i] = O("key", s.Key, "title", s.Title, "count", s.Count, "items", s.Items)
		parts[i] = fmt.Sprintf("%d %s", s.Count, strings.ToLower(s.Title))
	}
	c.Summary = "Nothing needs a person right now."
	if len(parts) > 0 {
		c.Summary = briefSentence(parts)
	}
	out := O("summary", c.Summary, "sections", list)
	// The app's home also has the personal half around it and, for an
	// owner, the first-run checklist.
	if home {
		h, err := a.briefingHome(ctx, c.Actor, c.Can("triage_requests"))
		if err != nil {
			return err
		}
		setup, err := a.setupJSON(ctx, c)
		if err != nil {
			return err
		}
		out = out.Set("home", h).Set("setup", setup)
	}
	return c.JSON(200, out)
}

// briefSentence is Rails' to_sentence: "a", "a and b", "a, b, and c".
func briefSentence(parts []string) string {
	switch len(parts) {
	case 0:
		return ""
	case 1:
		return parts[0]
	case 2:
		return parts[0] + " and " + parts[1]
	}
	return strings.Join(parts[:len(parts)-1], ", ") + ", and " + parts[len(parts)-1]
}

// briefing is every section with something in it, in order.
func (a *App) briefing(ctx context.Context, actor *Actor, limit int) ([]BriefingSection, error) {
	today := a.today(ctx)
	week := briefDateAdd(today, 7)
	type part struct {
		key, title string
		fn         func() ([]Obj, int, error)
	}
	parts := []part{
		{"questions", "Questions for you", func() ([]Obj, int, error) {
			list, n, err := a.openQuestions(ctx, questionUserID(actor), limit)
			if err != nil {
				return nil, 0, err
			}
			items, err := a.questionsJSON(ctx, list)
			return items, n, err
		}},
		{"requests", "Requests to triage", func() ([]Obj, int, error) {
			return a.briefRequests(ctx, limit)
		}},
		{"awaiting_client", "Waiting on a client", func() ([]Obj, int, error) {
			return a.briefVersions(ctx, `v.sent_at IS NOT NULL AND v.superseded_by_id IS NULL AND NOT EXISTS (SELECT 1 FROM approvals ap WHERE ap.agreement_version_id = v.id)`, `v.sent_at`, limit)
		}},
		{"drafts", "Drafts ready to send", func() ([]Obj, int, error) {
			// An internal project's draft is a plan, never sent.
			return a.briefVersions(ctx, `v.sent_at IS NULL AND EXISTS (SELECT 1 FROM scope_items si WHERE si.agreement_version_id = v.id)`, `v.id`, limit)
		}},
		{"waiting_on_them", "Waiting on them", func() ([]Obj, int, error) {
			return a.briefCommitments(ctx, `c.resolution IS NULL AND c.due_on <= ? AND c.owner_kind = 'client'`, `c.due_on, c.id`, limit, today, week)
		}},
		{"you_promised", "You promised", func() ([]Obj, int, error) {
			return a.briefCommitments(ctx, `c.resolution IS NULL AND c.due_on <= ? AND c.owner_kind = 'us'`, `c.due_on, c.id`, limit, today, week)
		}},
		{"review", "Ready for review", func() ([]Obj, int, error) {
			return a.briefTodos(ctx, `t.status = 'in_review'`, `t.updated_at, t.id`, limit, today)
		}},
		{"blocked", "Blocked work", func() ([]Obj, int, error) {
			return a.briefTodos(ctx, `t.status = 'blocked'`, `t.id`, limit, today)
		}},
		{"overdue_todos", "Overdue work", func() ([]Obj, int, error) {
			return a.briefTodos(ctx, `t.status != 'done' AND t.due_on < ?`, `t.id`, limit, today, today)
		}},
	}
	var out []BriefingSection
	for _, p := range parts {
		items, n, err := p.fn()
		if err != nil {
			return nil, fmt.Errorf("briefing %s: %w", p.key, err)
		}
		if n > 0 {
			out = append(out, BriefingSection{p.key, p.title, items, n})
		}
	}
	for _, fn := range briefingSections {
		key, title, items, err := fn(a, ctx, actor)
		if err != nil {
			return nil, err
		}
		if len(items) > 0 {
			out = append(out, BriefingSection{key, title, items[:min(limit, len(items))], len(items)})
		}
	}
	return out, nil
}

// briefDateAdd is a YYYY-MM-DD date days later.
func briefDateAdd(ymd string, days int) string {
	t, err := time.Parse("2006-01-02", ymd)
	if err != nil {
		return ymd
	}
	return t.AddDate(0, 0, days).Format("2006-01-02")
}

// briefCount is the whole count when the first page was full; the page's
// length otherwise (a large install has hundreds overdue).
func briefCount(ctx context.Context, db store.Querier, shown, limit int, query string, args ...any) (int, error) {
	if shown < limit {
		return shown, nil
	}
	return store.Count(ctx, db, query, args...)
}

// ---- requests -------------------------------------------------------------------

func (a *App) briefRequests(ctx context.Context, limit int) ([]Obj, int, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT `+requestColumns+requestFrom+` WHERE r.status = 'open' ORDER BY r.received_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, 0, err
	}
	reqs, err := store.ScanAll(rows, func(r *sqlRows) (*Request, error) { return scanRequest(r) })
	if err != nil {
		return nil, 0, err
	}
	items := make([]Obj, len(reqs))
	for i, q := range reqs {
		items[i] = a.requestJSON(q, nil).Set("sender_email", nullable(q.SenderEmail))
	}
	n, err := briefCount(ctx, a.store, len(items), limit, `SELECT count(*) FROM requests WHERE status = 'open'`)
	return items, n, err
}

// ---- agreement versions -----------------------------------------------------------

// briefMoney is Money.format: "$1,234.50".
func briefMoney(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := strconv.FormatInt(cents/100, 10)
	var b strings.Builder
	for i, ch := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(ch)
	}
	s := fmt.Sprintf("$%s.%02d", b.String(), cents%100)
	if neg {
		s = "-" + s
	}
	return s
}

// briefVersions are agreement versions of clients' (not internal)
// engagements matching where, as agreement_versions/_agreement_version.
func (a *App) briefVersions(ctx context.Context, where, order string, limit int) ([]Obj, int, error) {
	from := ` FROM agreement_versions v JOIN engagements e ON e.id = v.engagement_id JOIN clients cl ON cl.id = e.client_id
		LEFT JOIN approvals ap ON ap.agreement_version_id = v.id WHERE cl.internal = 0 AND ` + where
	rows, err := a.store.QueryContext(ctx, `SELECT v.id, v.number, v.kind, COALESCE(v.summary, ''), COALESCE(v.reason, ''), COALESCE(v.cadence, ''), v.amount_cents,
		v.sent_at, COALESCE(v.superseded_by_id, 0), COALESCE(ap.decision, ''), e.id, e.ref, e.title, cl.id, cl.name`+from+` ORDER BY `+order+` LIMIT ?`, limit)
	if err != nil {
		return nil, 0, err
	}
	type version struct {
		id, number, amount, superseded, engID, clientID int64
		kind, summary, reason, cadence, decision        string
		ref, title, client                              string
		sentAt                                          store.Time
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (version, error) {
		var v version
		err := r.Scan(&v.id, &v.number, &v.kind, &v.summary, &v.reason, &v.cadence, &v.amount, &v.sentAt, &v.superseded, &v.decision,
			&v.engID, &v.ref, &v.title, &v.clientID, &v.client)
		return v, err
	})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, len(list))
	for i, v := range list {
		ids[i] = v.id
	}
	items, err := a.briefScopeItems(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Obj, len(list))
	for i, v := range list {
		draft := !v.sentAt.Valid
		status := "superseded"
		switch {
		case draft:
			status = "draft"
		case v.decision == "approved":
			status = "approved"
		case v.decision == "changes_requested":
			status = "changes_requested"
		case v.decision == "" && v.superseded == 0:
			status = "awaiting_decision"
		}
		amount := v.amount
		if draft {
			amount = 0
			for _, it := range items[v.id] {
				cents, _ := it.Get("price_cents").(int64) // NOT NULL
				amount += cents
			}
		}
		label := reqVersionLabel(v.kind, v.number)
		path := "/engagements/" + v.ref
		out[i] = a.agentRef("AgreementVersion", v.id, "", v.ref+" "+label, path).
			Set("number", v.number).Set("kind", v.kind).Set("summary", nullable(v.summary)).Set("reason", nullable(v.reason)).Set("cadence", nullable(v.cadence)).
			Set("label", label).Set("status", status).Set("draft", draft).Set("awaiting_decision", status == "awaiting_decision").
			Set("amount", briefMoney(amount)).Set("amount_cents", amount).Set("sent_at", v.sentAt).Set("decision", nullable(v.decision)).
			Set("scope_items", items[v.id]).
			Set("engagement", a.agentRef("Engagement", v.engID, v.ref, v.ref+" "+v.title, path).Set("title", v.title)).
			Set("client", a.agentRef("Client", v.clientID, "", v.client, fmt.Sprintf("/clients/%d", v.clientID)))
	}
	n, err := briefCount(ctx, a.store, len(out), limit, `SELECT count(*)`+from)
	return out, n, err
}

// briefScopeItems are versions' items in order, as scope_items/_scope_item,
// with how their work is going.
func (a *App) briefScopeItems(ctx context.Context, versionIDs []int64) (map[int64][]Obj, error) {
	out := map[int64][]Obj{}
	if len(versionIDs) == 0 {
		return out, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT si.id, si.agreement_version_id, si.description, COALESCE(si.internal_estimate, ''), si.price_cents,
		(SELECT group_concat(t.status) FROM todos t WHERE t.scope_item_id = si.id)
		FROM scope_items si WHERE si.agreement_version_id IN (`+store.Placeholders(len(versionIDs))+`) ORDER BY si.position`, idArgs(versionIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id, version, price int64
		var desc, estimate string
		var statuses *string
		if err := rows.Scan(&id, &version, &desc, &estimate, &price, &statuses); err != nil {
			return nil, err
		}
		var list []string
		if statuses != nil {
			list = strings.Split(*statuses, ",")
		}
		out[version] = append(out[version], a.agentRef("ScopeItem", id, "", desc, fmt.Sprintf("/scope_items/%d", id)).
			Set("description", desc).Set("internal_estimate", nullable(estimate)).Set("price", briefMoney(price)).Set("price_cents", price).
			Set("delivery_state", briefDeliveryState(list)))
	}
	return out, rows.Err()
}

// briefDeliveryState is a scope item's, from its work's statuses
// (ScopeItem.delivery_state_of).
func briefDeliveryState(statuses []string) string {
	if len(statuses) == 0 {
		return "not_started"
	}
	done, blocked := true, false
	for _, s := range statuses {
		done = done && s == "done"
		blocked = blocked || s == "blocked"
	}
	switch {
	case done:
		return "done"
	case blocked:
		return "blocked"
	}
	return "in_progress"
}

// ---- commitments --------------------------------------------------------------------

// briefCommitments are commitments matching where, as
// commitments/_commitment.
func (a *App) briefCommitments(ctx context.Context, where, order string, limit int, today string, args ...any) ([]Obj, int, error) {
	from := ` FROM commitments c JOIN clients cl ON cl.id = c.client_id LEFT JOIN engagements e ON e.id = c.engagement_id
		LEFT JOIN users u ON u.id = c.user_id LEFT JOIN contacts co ON co.id = c.contact_id WHERE ` + where
	rows, err := a.store.QueryContext(ctx, `SELECT c.id, c.description, c.due_on, c.owner_kind, COALESCE(c.user_id, 0), COALESCE(c.contact_id, 0), c.source,
		COALESCE(c.resolution, ''), COALESCE(c.resolution_note, ''), cl.id, cl.name, COALESCE(e.id, 0), COALESCE(e.ref, ''), COALESCE(e.title, ''),
		COALESCE(NULLIF(u.name, ''), u.email_address, ''), COALESCE(co.name, '')`+from+` ORDER BY `+order+` LIMIT ?`, append(append([]any{}, args...), limit)...)
	if err != nil {
		return nil, 0, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id, user, contact, clientID, engID int64
		var desc, due, kind, source, resolution, resNote, client, ref, title, userName, contactName string
		if err := r.Scan(&id, &desc, &due, &kind, &user, &contact, &source, &resolution, &resNote, &clientID, &client, &engID, &ref, &title, &userName, &contactName); err != nil {
			return nil, err
		}
		due = due[:min(10, len(due))]
		return a.commitmentBriefJSON(id, desc, due, kind, user, contact, source, resolution, resNote, clientID, client, engID, ref, title, userName, contactName, today), nil
	})
	if err != nil {
		return nil, 0, err
	}
	if out == nil {
		out = []Obj{}
	}
	n, err := briefCount(ctx, a.store, len(out), limit, `SELECT count(*)`+from, args...)
	return out, n, err
}

// commitmentBriefJSON is one commitment as commitments/_commitment shows
// it, with the words home shows ("Ted will send the invoice").
func (a *App) commitmentBriefJSON(id int64, desc, due, kind string, user, contact int64, source, resolution, resNote string,
	clientID int64, client string, engID int64, ref, title, userName, contactName, today string) Obj {
	owner, ownerName := "us", "Us"
	promiser := "We"
	if kind == "us" {
		if user != 0 {
			owner = fmt.Sprintf("user:%d", user)
		}
		if userName != "" {
			ownerName, promiser = userName, userName
		}
	} else {
		owner, ownerName, promiser = "client", client, client
		if contact != 0 {
			owner = fmt.Sprintf("contact:%d", contact)
		}
		if contactName != "" {
			ownerName, promiser = contactName, contactName
		}
	}
	open := resolution == ""
	path := fmt.Sprintf("/clients/%d", clientID)
	var engagement any
	if engID != 0 {
		path = "/engagements/" + ref
		engagement = a.agentRef("Engagement", engID, ref, ref+" "+title, path)
	}
	return a.agentRef("Commitment", id, "", desc, path).
		Set("description", desc).Set("due_on", due).Set("owner", owner).Set("source", source).
		Set("resolution", nullable(resolution)).Set("resolution_note", nullable(resNote)).Set("owner_name", ownerName).
		Set("open", open).Set("overdue", open && due < today).
		Set("client", a.agentRef("Client", clientID, "", client, fmt.Sprintf("/clients/%d", clientID))).Set("engagement", engagement).
		Set("promiser", promiser)
}

// ---- work ---------------------------------------------------------------------------

// briefTodos are pieces of work matching where, as todos/_todo.
func (a *App) briefTodos(ctx context.Context, where, order string, limit int, today string, args ...any) ([]Obj, int, error) {
	from := ` FROM todos t JOIN engagements e ON e.id = t.engagement_id LEFT JOIN users u ON u.id = t.owner_id
		LEFT JOIN scope_items si ON si.id = t.scope_item_id WHERE ` + where
	rows, err := a.store.QueryContext(ctx, `SELECT t.id, t.title, t.status, COALESCE(t.due_on, ''), t.client_visible, COALESCE(t.owner_id, 0),
		COALESCE(NULLIF(u.name, ''), u.email_address, ''), e.id, e.ref, e.title, COALESCE(si.id, 0), COALESCE(si.description, '')`+from+` ORDER BY `+order+` LIMIT ?`,
		append(append([]any{}, args...), limit)...)
	if err != nil {
		return nil, 0, err
	}
	type todo struct {
		id, owner, engID, scopeID     int64
		title, status, due, ownerName string
		ref, engTitle, scopeDesc      string
		visible                       bool
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (todo, error) {
		var t todo
		err := r.Scan(&t.id, &t.title, &t.status, &t.due, &t.visible, &t.owner, &t.ownerName, &t.engID, &t.ref, &t.engTitle, &t.scopeID, &t.scopeDesc)
		t.due = t.due[:min(10, len(t.due))]
		return t, err
	})
	if err != nil {
		return nil, 0, err
	}
	ids := make([]int64, len(list))
	for i, t := range list {
		ids[i] = t.id
	}
	others, err := a.briefOtherOwners(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	custom, err := a.customValues(ctx, a.store, "Todo", ids)
	if err != nil {
		return nil, 0, err
	}
	out := make([]Obj, len(list))
	for i, t := range list {
		var scope any
		if t.scopeID != 0 {
			scope = a.agentRef("ScopeItem", t.scopeID, "", t.scopeDesc, fmt.Sprintf("/scope_items/%d", t.scopeID))
		}
		fields := custom[t.id]
		if fields == nil {
			fields = Obj{}
		}
		other := others[t.id]
		if other == nil {
			other = []any{}
		}
		out[i] = a.agentRef("Todo", t.id, "", t.title, fmt.Sprintf("/work/%d", t.id)).
			Set("title", t.title).Set("status", t.status).Set("due_on", nullable(t.due)).Set("client_visible", t.visible).
			Set("overdue", t.due != "" && t.due < today && t.status != "done").
			Set("owner", userRef(t.owner, t.ownerName)).Set("other_owners", other).
			Set("engagement", a.agentRef("Engagement", t.engID, t.ref, t.ref+" "+t.engTitle, "/engagements/"+t.ref)).
			Set("scope_item", scope).Set("custom_fields", fields)
	}
	n, err := briefCount(ctx, a.store, len(out), limit, `SELECT count(*)`+from, args...)
	return out, n, err
}

// briefOtherOwners are who else is on each piece of work, beside its lead.
func (a *App) briefOtherOwners(ctx context.Context, ids []int64) (map[int64][]any, error) {
	out := map[int64][]any{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT a.todo_id, u.id, COALESCE(NULLIF(u.name, ''), u.email_address) FROM assignments a JOIN todos t ON t.id = a.todo_id JOIN users u ON u.id = a.user_id
		WHERE a.todo_id IN (`+store.Placeholders(len(ids))+`) AND (t.owner_id IS NULL OR a.user_id != t.owner_id) ORDER BY a.created_at, a.id`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var todo, user int64
		var name string
		if err := rows.Scan(&todo, &user, &name); err != nil {
			return nil, err
		}
		out[todo] = append(out[todo], userRef(user, name))
	}
	return out, rows.Err()
}
