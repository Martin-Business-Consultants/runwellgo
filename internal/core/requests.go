package core

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Requests: something a client asked for that we have not dealt with yet.
// It is evidence, not work: triage turns it into scope, a change order, a
// todo or a commitment, or dismisses it (requests_promote.go).

func init() {
	routeSets = append(routeSets, (*App).requestRoutes)
	searchable("Request", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		var body, name, email string
		var client store.NullInt
		err := db.QueryRowContext(ctx, `SELECT subject, COALESCE(body, ''), COALESCE(sender_name, ''), COALESCE(sender_email, ''), client_id, created_at FROM requests WHERE id = ?`, id).
			Scan(&d.Title, &body, &name, &email, &client, &d.CreatedAt)
		if isNoRows(err) {
			return d, false, nil
		}
		d.Content = strings.Join(slices.DeleteFunc([]string{plainText(body), name, email}, func(s string) bool { return s == "" }), " ")
		d.ClientID = client.Int64
		return d, err == nil, err
	})
	// The sidebar's number beside Requests: open ones, waiting for triage.
	navCounts["requests"] = func(a *App, ctx context.Context, actor *Actor) (int, error) {
		return store.Count(ctx, a.store, `SELECT count(*) FROM requests WHERE status = 'open'`)
	}
}

// RequestStatuses are a request's statuses.
var RequestStatuses = []string{"open", "promoted", "dismissed"}

func (a *App) requestRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/requests", a.listRequests, tool("list_requests", "List requests",
		note("Open requests unless status says otherwise."),
		query("status", "string", "open (the default), promoted, dismissed or all."),
		query("source", "string", "Only requests from this source (email, portal, phone…); all by default.")))
	a.handle(mux, "GET /api/requests/{id}", a.showRequest, tool("show_request", "Show a request",
		next("promote_request_to_todo", "promote_request_to_change", "dismiss_request")))
	a.handle(mux, "POST /api/requests", a.createRequest, tool("create_request", "Log a request from a client",
		body("request", `{subject (required), body: text, client_id: integer, contact_id: integer, sender_name, sender_email, source, received_at}`, true)))
	a.handle(mux, "PATCH /api/requests/{id}", a.updateRequest, tool("update_request", "Change an open request",
		body("request", `{subject, body: text, client_id: integer, contact_id: integer, sender_name, sender_email, source}`, true)))
	a.handle(mux, "DELETE /api/requests/{id}", a.deleteRequest, need("delete_records"), tool("delete_request", "Delete an open request"))

	a.handle(mux, "POST /api/requests/{id}/promote_engagement", a.promoteRequestEngagement, need("triage_requests"),
		tool("promote_request_to_engagement", "Triage: make a request a new engagement",
			bodyParam("label", "string", "project, work_order or service.", false),
			bodyParam("title", "string", "The engagement's title; the request's subject when blank.", false),
			bodyParam("client_id", "integer", "The client, when the request has none.", false)))
	a.handle(mux, "POST /api/requests/{id}/promote_change", a.promoteRequestChange, need("triage_requests"),
		tool("promote_request_to_change", "Triage: add a request as a scope item on a draft",
			bodyParam("engagement_ref", "string", "The engagement, like WO-12.", true),
			bodyParam("price_cents", "integer", "The item's price in cents.", false)))
	a.handle(mux, "POST /api/requests/{id}/promote_todo", a.promoteRequestTodo, need("triage_requests"),
		tool("promote_request_to_todo", "Triage: make a request a todo",
			bodyParam("engagement_ref", "string", "The engagement, like WO-12.", true),
			bodyParam("owner_id", "integer", "Who leads it.", false),
			bodyParam("due_on", "string", "When it's due, YYYY-MM-DD.", false)))
	a.handle(mux, "POST /api/requests/{id}/promote_commitment", a.promoteRequestCommitment, need("triage_requests"),
		tool("promote_request_to_commitment", "Triage: make a request a commitment",
			bodyParam("due_on", "string", "When it's due, YYYY-MM-DD.", true),
			bodyParam("owner_kind", "string", "us or client.", false),
			bodyParam("user_id", "integer", "Who of us holds it.", false),
			bodyParam("engagement_ref", "string", "The engagement it's about, like WO-12.", false),
			bodyParam("client_id", "integer", "The client, when the request has none.", false)))
	a.handle(mux, "POST /api/requests/{id}/dismiss", a.dismissRequest, need("triage_requests"),
		tool("dismiss_request", "Triage: dismiss a request",
			bodyParam("reason", "string", "Why; \"Not needed\" when blank.", false)))

	a.handle(mux, "DELETE /api/bulk/requests", a.bulkDeleteRequests, need("delete_records"),
		tool("bulk_delete_requests", "Delete several open requests at once",
			bodyParam("ids", "array", "The requests' ids.", true)))
	a.handle(mux, "POST /api/bulk/requests/dismissal", a.bulkDismissRequests, need("triage_requests"),
		tool("bulk_dismiss_requests", "Dismiss several open requests at once",
			bodyParam("ids", "array", "The requests' ids.", true),
			bodyParam("reason", "string", "Why; \"Not needed\" when blank.", false)))
}

// Request is one request.
type Request struct {
	ID              int64
	ClientID        int64
	ContactID       int64
	SenderName      string
	SenderEmail     string
	Subject         string
	Body            string
	Source          string
	Status          string
	PromotedType    string
	PromotedID      int64
	DismissedReason string
	ReceivedAt      store.Time
	TriagedByID     int64
	TriagedAt       store.Time
	CreatedAt       store.Time

	// Read with it: the client's and contact's names, and who triaged it.
	ClientName  string
	ContactName string
	TriagedBy   string
}

const requestColumns = `r.id, COALESCE(r.client_id, 0), COALESCE(r.contact_id, 0), COALESCE(r.sender_name, ''), COALESCE(r.sender_email, ''),
	r.subject, COALESCE(r.body, ''), r.source, r.status, COALESCE(r.promoted_type, ''), COALESCE(r.promoted_id, 0), COALESCE(r.dismissed_reason, ''),
	r.received_at, COALESCE(r.triaged_by_id, 0), r.triaged_at, r.created_at,
	COALESCE(cl.name, ''), COALESCE(co.name, ''), COALESCE(NULLIF(u.name, ''), u.email_address, '')`

const requestFrom = ` FROM requests r LEFT JOIN clients cl ON cl.id = r.client_id LEFT JOIN contacts co ON co.id = r.contact_id LEFT JOIN users u ON u.id = r.triaged_by_id`

func scanRequest(r interface{ Scan(...any) error }) (*Request, error) {
	var q Request
	err := r.Scan(&q.ID, &q.ClientID, &q.ContactID, &q.SenderName, &q.SenderEmail, &q.Subject, &q.Body, &q.Source, &q.Status,
		&q.PromotedType, &q.PromotedID, &q.DismissedReason, &q.ReceivedAt, &q.TriagedByID, &q.TriagedAt, &q.CreatedAt,
		&q.ClientName, &q.ContactName, &q.TriagedBy)
	return &q, err
}

// Open is whether it still waits for triage.
func (q *Request) Open() bool { return q.Status == "open" }

// RequesterName is who asked: the contact, else the sender's name, else
// their address.
func (q *Request) RequesterName() string {
	switch {
	case q.ContactName != "":
		return q.ContactName
	case q.SenderName != "":
		return q.SenderName
	}
	return q.SenderEmail
}

// request is the request with id; notFound when there's none.
func (a *App) request(ctx context.Context, db store.Querier, id int64) (*Request, error) {
	q, err := scanRequest(db.QueryRowContext(ctx, `SELECT `+requestColumns+requestFrom+` WHERE r.id = ?`, id))
	if isNoRows(err) {
		return nil, notFound("request")
	}
	return q, err
}

// replyToken proves a reply by email is to this request: in its plus
// address and the acknowledgement's Message-ID. Derived from the install's
// key, so nothing is stored.
func (a *App) replyToken(id int64) string {
	m := hmac.New(sha256.New, []byte(a.secrets.Hex()))
	m.Write([]byte("request-reply:" + strconv.FormatInt(id, 10)))
	return hex.EncodeToString(m.Sum(nil))[:12]
}

// requestByReplyToken is the request a reply names, when the token is its.
func (a *App) requestByReplyToken(ctx context.Context, id int64, token string) *Request {
	q, err := a.request(ctx, a.store, id)
	if err != nil || subtle.ConstantTimeCompare([]byte(a.replyToken(id)), []byte(strings.ToLower(token))) != 1 {
		return nil
	}
	return q
}

// ---- JSON ---------------------------------------------------------------------

// promotedRef is what a request became, as an agent_ref, with the words the
// request's page shows for it (Request#promoted_label).
type promotedRef struct {
	Ref   Obj
	Label string
	Path  string
}

// promotedRefs reads what each request became, one query per kind.
func (a *App) promotedRefs(ctx context.Context, db store.Querier, reqs []*Request) (map[string]map[int64]promotedRef, error) {
	ids := map[string][]int64{}
	for _, q := range reqs {
		if q.PromotedType != "" && q.PromotedID != 0 {
			ids[q.PromotedType] = append(ids[q.PromotedType], q.PromotedID)
		}
	}
	out := map[string]map[int64]promotedRef{}
	read := func(typ, query string, fn func(r *sqlRows) (int64, promotedRef, error)) error {
		list := ids[typ]
		if len(list) == 0 {
			return nil
		}
		rows, err := db.QueryContext(ctx, fmt.Sprintf(query, store.Placeholders(len(list))), idArgs(list)...)
		if err != nil {
			return err
		}
		defer rows.Close()
		out[typ] = map[int64]promotedRef{}
		for rows.Next() {
			id, p, err := fn(rows)
			if err != nil {
				return err
			}
			out[typ][id] = p
		}
		return rows.Err()
	}
	err := read("Engagement", `SELECT id, ref, title FROM engagements WHERE id IN (%s)`, func(r *sqlRows) (int64, promotedRef, error) {
		var id int64
		var ref, title string
		err := r.Scan(&id, &ref, &title)
		path := "/engagements/" + ref
		return id, promotedRef{a.agentRef("Engagement", id, ref, ref+" "+title, path), ref + " " + title, path}, err
	})
	if err == nil {
		err = read("AgreementVersion", `SELECT v.id, v.kind, v.number, e.ref FROM agreement_versions v JOIN engagements e ON e.id = v.engagement_id WHERE v.id IN (%s)`, func(r *sqlRows) (int64, promotedRef, error) {
			var id, number int64
			var kind, ref string
			err := r.Scan(&id, &kind, &number, &ref)
			label := reqVersionLabel(kind, number)
			path := "/engagements/" + ref
			return id, promotedRef{a.agentRef("AgreementVersion", id, "", ref+" "+label, path), ref + " " + label, path}, err
		})
	}
	if err == nil {
		err = read("Todo", `SELECT id, title FROM todos WHERE id IN (%s)`, func(r *sqlRows) (int64, promotedRef, error) {
			var id int64
			var title string
			err := r.Scan(&id, &title)
			path := fmt.Sprintf("/work/%d", id)
			return id, promotedRef{a.agentRef("Todo", id, "", title, path), "Work: " + title, path}, err
		})
	}
	if err == nil {
		err = read("Commitment", `SELECT c.id, c.description, c.client_id, COALESCE(e.ref, '') FROM commitments c LEFT JOIN engagements e ON e.id = c.engagement_id WHERE c.id IN (%s)`, func(r *sqlRows) (int64, promotedRef, error) {
			var id, client int64
			var desc, ref string
			err := r.Scan(&id, &desc, &client, &ref)
			path := fmt.Sprintf("/clients/%d", client)
			if ref != "" {
				path = "/engagements/" + ref
			}
			return id, promotedRef{a.agentRef("Commitment", id, "", desc, path), "Commitment: " + desc, path}, err
		})
	}
	return out, err
}

// reqVersionLabel is an agreement version's name: "Agreement", "Change order #2".
func reqVersionLabel(kind string, number int64) string {
	if kind == "initial" {
		return "Agreement"
	}
	return fmt.Sprintf("%s #%d", humanize(kind), number)
}

// requestJSON is a request as lists show it (requests/_request.json).
func (a *App) requestJSON(q *Request, promoted map[string]map[int64]promotedRef) Obj {
	var client, became any
	if q.ClientID != 0 {
		client = a.agentRef("Client", q.ClientID, "", q.ClientName, fmt.Sprintf("/clients/%d", q.ClientID))
	}
	if p, ok := promoted[q.PromotedType][q.PromotedID]; ok {
		became = p.Ref
	}
	return a.agentRef("Request", q.ID, "", q.Subject, fmt.Sprintf("/requests/%d", q.ID)).
		Set("subject", q.Subject).Set("source", q.Source).Set("status", q.Status).Set("received_at", q.ReceivedAt).
		Set("dismissed_reason", nullable(q.DismissedReason)).Set("from", nullable(q.RequesterName())).Set("client", client).Set("promoted_to", became)
}

var reqTextBreaks = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`)

// reqAgentText is rich text as plain text, for a model to read (agent_text).
func reqAgentText(html string) any {
	if strings.TrimSpace(html) == "" {
		return nil
	}
	s := reqTextBreaks.ReplaceAllString(html, "\n")
	s = tags.ReplaceAllString(s, "")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
	for strings.Contains(s, "\n\n") {
		s = strings.ReplaceAll(s, "\n\n", "\n")
	}
	return strings.TrimSpace(s)
}

// ---- reading --------------------------------------------------------------------

// requestSorts are the table's sortable columns (?sort=&direction=).
var requestSorts = map[string]string{
	"subject":  "r.subject",
	"from":     "COALESCE(co.name, r.sender_name, r.sender_email)",
	"client":   "cl.name",
	"source":   "r.source",
	"received": "r.received_at",
	"status":   "r.status",
}

func (a *App) listRequests(c *Ctx) error {
	ctx := c.R.Context()
	status := c.Query("status")
	if status != "all" && !slices.Contains(RequestStatuses, status) {
		status = "open"
	}
	source := c.Query("source")
	if source == "" {
		source = "all"
	}
	where, args := []string{"1 = 1"}, []any{}
	if status != "all" {
		where, args = append(where, "r.status = ?"), append(args, status)
	}
	if source != "all" {
		where, args = append(where, "r.source = ?"), append(args, source)
	}
	order := "r.received_at DESC"
	if col, ok := requestSorts[c.Query("sort")]; ok {
		dir := "ASC"
		if c.Query("direction") == "desc" {
			dir = "DESC"
		}
		// Empty values sort last whichever way, then by id so pages are stable.
		order = fmt.Sprintf("(%s IS NULL OR %s = '') ASC, %s %s, r.id ASC", col, col, col, dir)
	}
	rows, err := a.store.QueryContext(ctx, `SELECT `+requestColumns+requestFrom+` WHERE `+strings.Join(where, " AND ")+` ORDER BY `+order, args...)
	if err != nil {
		return err
	}
	reqs, err := store.ScanAll(rows, func(r *sqlRows) (*Request, error) { return scanRequest(r) })
	if err != nil {
		return err
	}
	promoted, err := a.promotedRefs(ctx, a.store, reqs)
	if err != nil {
		return err
	}
	list := make([]Obj, len(reqs))
	for i, q := range reqs {
		list[i] = a.requestJSON(q, promoted)
	}
	// What the screen's filters offer: each status with its count, and the
	// sources there are.
	counts, err := countByString(ctx, a.store, `SELECT status, count(*) FROM requests GROUP BY status`)
	if err != nil {
		return err
	}
	labels := map[string]string{"open": "Open", "promoted": "Promoted", "dismissed": "Dismissed"}
	filters, total := []Obj{}, 0
	for _, st := range RequestStatuses {
		filters = append(filters, O("value", st, "label", labels[st], "count", counts[st]))
		total += counts[st]
	}
	filters = append(filters, O("value", "all", "label", "All requests", "count", total))
	sources, err := store.Strings(ctx, a.store, `SELECT DISTINCT source FROM requests ORDER BY source`)
	if err != nil {
		return err
	}
	if sources == nil {
		sources = []string{}
	}
	c.Summary = pluralize(len(reqs), "request") + " (" + status + ")"
	return c.pagedAnswer(O("summary", c.Summary, "status", status, "source", source, "filters", filters, "sources", sources), "requests", list, requestSorts)
}

func (a *App) showRequest(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	q, err := a.request(ctx, a.store, id)
	if err != nil {
		return err
	}
	o, err := a.requestShowJSON(ctx, q)
	if err != nil {
		return err
	}
	c.Summary = q.Subject + " (" + q.Status + ")"
	if q.Open() {
		c.Crumbs = []Crumb{
			step("Make it work on an engagement", "promote_request_to_todo", "id", id),
			step("Add it to a draft as scope", "promote_request_to_change", "id", id),
			step("Dismiss it", "dismiss_request", "id", id),
		}
	}
	return c.JSON(200, O("summary", c.Summary, "request", o))
}

// requestShowJSON is a request as its page and show_request answer it
// (requests/show.json): the list's keys, the sender, the contact, the body
// as text, then its notes and documents (their areas' sections). The app
// also reads the body's HTML, who triaged it, and the open engagements of
// its client, for the triage panel.
func (a *App) requestShowJSON(ctx context.Context, q *Request) (Obj, error) {
	promoted, err := a.promotedRefs(ctx, a.store, []*Request{q})
	if err != nil {
		return nil, err
	}
	var contact any
	if q.ContactID != 0 {
		contact = a.agentRef("Contact", q.ContactID, "", q.ContactName, fmt.Sprintf("/clients/%d", q.ClientID))
	}
	o := a.requestJSON(q, promoted).
		Set("sender_name", nullable(q.SenderName)).Set("sender_email", nullable(q.SenderEmail)).
		Set("contact", contact).Set("body", reqAgentText(q.Body))
	o = o.Set("notes", []Obj{}).Set("documents", []Obj{})
	o, err = a.withSections(ctx, "Request", q.ID, o)
	if err != nil {
		return nil, err
	}
	var label, path any
	if p, ok := promoted[q.PromotedType][q.PromotedID]; ok {
		label, path = p.Label, p.Path
	}
	options := []Obj{}
	if q.ClientID != 0 {
		rows, err := a.store.QueryContext(ctx, `SELECT ref, title FROM engagements WHERE client_id = ? AND closed_at IS NULL ORDER BY created_at DESC`, q.ClientID)
		if err != nil {
			return nil, err
		}
		options, err = store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
			var ref, title string
			err := r.Scan(&ref, &title)
			return O("ref", ref, "label", ref+" "+title), err
		})
		if err != nil {
			return nil, err
		}
		if options == nil {
			options = []Obj{}
		}
	}
	return o.Set("body_html", richTextHTML(q.Body)).Set("client_id", reqNullID(q.ClientID)).Set("contact_id", reqNullID(q.ContactID)).
		Set("triaged_by", userRef(q.TriagedByID, q.TriagedBy)).Set("triaged_at", q.TriagedAt).
		Set("promoted_label", label).Set("promoted_path", path).Set("engagement_options", options), nil
}

// reqNullID is an id, or null for none.
func reqNullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// ---- writing --------------------------------------------------------------------

// requestParams reads and checks a request's fields; partial for an update.
func (a *App) requestParams(ctx context.Context, body map[string]any, partial bool) (map[string]any, error) {
	inv := apierr.Invalid{}
	out := map[string]any{}
	for _, k := range []string{"subject", "source"} {
		if has(body, k) || !partial {
			v := strings.TrimSpace(str(body[k]))
			if v == "" {
				inv.Add(k, "can't be blank")
			}
			out[k] = v
		}
	}
	if has(body, "body") {
		out["body"] = nullable(sanitizeRichText(str(body["body"])))
	}
	for _, k := range []string{"sender_name", "sender_email"} {
		if has(body, k) {
			out[k] = nullable(strings.TrimSpace(str(body[k])))
		}
	}
	for k, table := range map[string]string{"client_id": "clients", "contact_id": "contacts"} {
		if !has(body, k) {
			continue
		}
		id, ok := intOf(body[k])
		if !ok || id == 0 {
			out[k] = nil
			continue
		}
		if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM `+table+` WHERE id = ?`, id); err != nil {
			return nil, err
		} else if n == 0 {
			inv.Add(strings.TrimSuffix(k, "_id"), "must exist")
		}
		out[k] = id
	}
	if has(body, "received_at") && str(body["received_at"]) != "" {
		t, err := store.ParseTime(str(body["received_at"]))
		if err != nil {
			inv.Add("received_at", "is invalid")
		} else {
			out["received_at"] = t.DB()
		}
	}
	return out, inv.Err()
}

// reqMatchContact fills in the contact (and its client) from the sender's
// address, when there's no contact yet (Request#match_contact).
func (t *Tx) reqMatchContact(fields map[string]any, current *Request) error {
	contact, email := fields["contact_id"], fields["sender_email"]
	if _, ok := fields["contact_id"]; !ok && current != nil && current.ContactID != 0 {
		contact = current.ContactID
	}
	if _, ok := fields["sender_email"]; !ok && current != nil {
		email = nullable(current.SenderEmail)
	}
	if contact != nil || email == nil {
		return nil
	}
	var id, client int64
	err := t.QueryRow(`SELECT id, client_id FROM contacts WHERE email = ?`, strings.ToLower(strings.TrimSpace(str(email)))).Scan(&id, &client)
	if isNoRows(err) {
		return nil
	} else if err != nil {
		return err
	}
	fields["contact_id"] = id
	hasClient := fields["client_id"] != nil
	if _, ok := fields["client_id"]; !ok && current != nil && current.ClientID != 0 {
		hasClient = true
	}
	if !hasClient {
		fields["client_id"] = client
	}
	return nil
}

// insertRequest adds a request (received now unless said), its contact
// matched from the sender's address.
func (t *Tx) insertRequest(fields map[string]any) (int64, error) {
	if err := t.reqMatchContact(fields, nil); err != nil {
		return 0, err
	}
	now := store.Now().DB()
	received := fields["received_at"]
	if received == nil {
		received = now
	}
	id, err := t.Insert(`INSERT INTO requests (client_id, contact_id, sender_name, sender_email, subject, body, source, status, received_at, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)`,
		fields["client_id"], fields["contact_id"], fields["sender_name"], fields["sender_email"], fields["subject"], fields["body"], fields["source"], received, now, now)
	if err != nil {
		return 0, err
	}
	t.Reindex("Request", id)
	return id, t.recordMentions("Request", id, str(fields["body"]))
}

// requestEvent records an event on a request from a source other than the
// write's own (a triage is recorded with the request's source; mail as
// "email" by its sender), as Request#record_event! did.
func (t *Tx) requestEvent(id int64, kind string, payload map[string]any, source string, actorLabel string) error {
	if payload == nil {
		payload = map[string]any{}
	}
	ev := Event{SubjectType: "Request", SubjectID: id, Kind: kind, Payload: payload,
		ActorUserID: t.Actor.UserID(), Actor: t.Actor.AppLabel(), Source: source, OccurredAt: store.Now()}
	if actorLabel != "" {
		ev.Actor = actorLabel
	}
	if ev.Source == "" {
		ev.Source = t.Actor.Source()
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var actorUser any
	if ev.ActorUserID != 0 {
		actorUser = ev.ActorUserID
	}
	ev.ID, err = t.Insert(`INSERT INTO events (subject_type, subject_id, kind, payload, actor_user_id, actor, source, occurred_at, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"Request", id, kind, string(raw), actorUser, nullable(ev.Actor), ev.Source, ev.OccurredAt.DB(), ev.OccurredAt.DB())
	if err != nil {
		return err
	}
	t.events = append(t.events, ev)
	return nil
}

func (a *App) createRequest(c *Ctx) error {
	body, err := c.Wrapped("request")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	fields, err := a.requestParams(ctx, body, false)
	if err != nil {
		return err
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if id, err = t.insertRequest(fields); err != nil {
			return err
		}
		return t.Event("Request", id, "request.received", nil)
	})
	if err != nil {
		return err
	}
	q, err := a.request(ctx, a.store, id)
	if err != nil {
		return err
	}
	o, err := a.requestShowJSON(ctx, q)
	if err != nil {
		return err
	}
	c.Summary = "Request logged."
	c.Crumbs = []Crumb{step("Make it work on an engagement", "promote_request_to_todo", "id", id), step("Dismiss it", "dismiss_request", "id", id)}
	return c.JSON(201, O("summary", c.Summary, "path", fmt.Sprintf("/requests/%d", id), "request", o))
}

// reqRefused is a write a record's state doesn't allow (a Rails redirect with
// an alert): 422, with what a person reads.
func reqRefused(message string) error {
	return &apierr.Error{Status: 422, Code: "refused", Message: message}
}

func (a *App) updateRequest(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("request")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	q, err := a.request(ctx, a.store, id)
	if err != nil {
		return err
	}
	if !q.Open() {
		return reqRefused("Only an open request can be edited.")
	}
	fields, err := a.requestParams(ctx, body, true)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.reqMatchContact(fields, q); err != nil {
			return err
		}
		if err := t.update("requests", id, fields); err != nil {
			return err
		}
		t.Reindex("Request", id)
		if b, ok := fields["body"]; ok {
			return t.recordMentions("Request", id, str(b))
		}
		return nil
	})
	if err != nil {
		return err
	}
	if q, err = a.request(ctx, a.store, id); err != nil {
		return err
	}
	o, err := a.requestShowJSON(ctx, q)
	if err != nil {
		return err
	}
	c.Summary = "Request updated."
	return c.JSON(200, O("summary", c.Summary, "path", fmt.Sprintf("/requests/%d", id), "request", o))
}

func (a *App) deleteRequest(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	q, err := a.request(ctx, a.store, id)
	if err != nil {
		return err
	}
	if !q.Open() {
		return reqRefused("A triaged request stays as history.")
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteRequest(id) }); err != nil {
		return err
	}
	c.Summary = "Request deleted."
	return c.JSON(200, O("summary", c.Summary, "path", "/requests", "deleted", true))
}

// deleteRequest removes a request with what hangs off it.
func (t *Tx) deleteRequest(id int64) error {
	if err := t.cascade("Request", id); err != nil {
		return err
	}
	_, err := t.Exec(`DELETE FROM requests WHERE id = ?`, id)
	return err
}

// dismiss closes an open request without work (Request#dismiss!).
func (t *Tx) dismissRequest(q *Request, reason string) error {
	if !q.Open() {
		return reqRefused("already triaged")
	}
	now := store.Now().DB()
	if _, err := t.Exec(`UPDATE requests SET status = 'dismissed', dismissed_reason = ?, triaged_by_id = ?, triaged_at = ?, updated_at = ? WHERE id = ?`,
		reason, reqNullID(t.Actor.UserID()), now, now, q.ID); err != nil {
		return err
	}
	return t.requestEvent(q.ID, "request.dismissed", map[string]any{"reason": reason}, q.Source, "")
}

// reqDismissReason is the reason given, "Not needed" when none.
func reqDismissReason(v any) string {
	if r := strings.TrimSpace(str(v)); r != "" {
		return r
	}
	return "Not needed"
}

func (a *App) dismissRequest(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	q, err := a.request(ctx, a.store, id)
	if err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.dismissRequest(q, reqDismissReason(body["reason"])) }); err != nil {
		return err
	}
	c.Summary = "Dismissed."
	return c.JSON(200, O("summary", c.Summary, "path", "/requests"))
}

// ---- many at once -------------------------------------------------------------

// reqBulkIDs are a bulk action's ids: a list, or comma-separated, at most 500.
func reqBulkIDs(v any) []int64 {
	var raw []string
	switch x := v.(type) {
	case []any:
		for _, e := range x {
			raw = append(raw, strings.Split(fmt.Sprint(e), ",")...)
		}
	case string:
		raw = strings.Split(x, ",")
	case float64:
		raw = []string{strconv.FormatInt(int64(x), 10)}
	}
	var ids []int64
	for _, s := range raw {
		id, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err == nil && id > 0 && !slices.Contains(ids, id) {
			ids = append(ids, id)
		}
		if len(ids) == 500 {
			break
		}
	}
	return ids
}

// eachRequest runs fn on each selected request, one write each, the way
// BulkAction does: a request it doesn't apply to is skipped and named.
func (a *App) eachRequest(c *Ctx, body map[string]any, done string, fn func(t *Tx, q *Request) string) error {
	ctx := c.R.Context()
	ids := reqBulkIDs(body["ids"])
	var reqs []*Request
	if len(ids) > 0 {
		rows, err := a.store.QueryContext(ctx, `SELECT `+requestColumns+requestFrom+` WHERE r.id IN (`+store.Placeholders(len(ids))+`) ORDER BY r.id`, idArgs(ids)...)
		if err != nil {
			return err
		}
		if reqs, err = store.ScanAll(rows, func(r *sqlRows) (*Request, error) { return scanRequest(r) }); err != nil {
			return err
		}
	}
	if len(reqs) == 0 {
		return reqRefused("Select at least one first.")
	}
	count, skipped := 0, []string{}
	for _, q := range reqs {
		reason := ""
		err := a.write(ctx, c.Actor, func(t *Tx) error {
			if reason = fn(t, q); reason != "" {
				return reqRefused(reason)
			}
			return nil
		})
		if err == nil {
			count++
			continue
		}
		if reason == "" {
			if e, ok := errors.AsType[*apierr.Error](err); ok && e.Status < 500 {
				reason = e.Message
			} else {
				return err
			}
		}
		skipped = append(skipped, "“"+q.Subject+"” ("+reason+")")
	}
	var parts []string
	if count > 0 {
		parts = append(parts, fmt.Sprintf(done, count))
	}
	if len(skipped) > 0 {
		more := ""
		if len(skipped) > 5 {
			more = fmt.Sprintf(" and %d more", len(skipped)-5)
		}
		parts = append(parts, fmt.Sprintf("Skipped %d: %s%s.", len(skipped), strings.Join(skipped[:min(5, len(skipped))], "; "), more))
	}
	msg := strings.Join(parts, ". ")
	if !strings.HasSuffix(msg, ".") {
		msg += "."
	}
	c.Summary = msg
	if count == 0 {
		return reqRefused(msg)
	}
	return c.JSON(200, O("summary", msg, "path", "/requests", "count", count, "skipped", skipped))
}

func (a *App) bulkDeleteRequests(c *Ctx) error {
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	return a.eachRequest(c, body, "Deleted %d requests", func(t *Tx, q *Request) string {
		if !q.Open() {
			return "it was triaged, so it stays as history"
		}
		if err := t.deleteRequest(q.ID); err != nil {
			return err.Error()
		}
		return ""
	})
}

func (a *App) bulkDismissRequests(c *Ctx) error {
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	reason := reqDismissReason(body["reason"])
	return a.eachRequest(c, body, "Dismissed %d requests", func(t *Tx, q *Request) string {
		if !q.Open() {
			return "it was already triaged"
		}
		if err := t.dismissRequest(q, reason); err != nil {
			return err.Error()
		}
		return ""
	})
}
