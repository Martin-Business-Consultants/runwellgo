package core

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// The client portal: a contact signed in by emailed link sees their own
// client's engagements (only what's shared: the agreements exactly as they
// were sent, the work marked visible to the client, documents shared with
// them, never internal estimates), decides an agreement awaiting them,
// sends a request, and connects their own AI. Their own agent can do the
// same with a token of theirs, through the same routes (the portal_ tools),
// and reaches only this contact's client. Times are in the client's zone.

func init() { routeSets = append(routeSets, (*App).portalRoutes) }

func (a *App) portalRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/portal/session", a.createPortalSession, open, portal)
	a.handle(mux, "POST /api/portal/session/{token}", a.showPortalSessionLink, open, portal)
	a.handle(mux, "DELETE /api/portal/session", a.destroyPortalSession, portal)

	a.handle(mux, "GET /api/portal/me", a.showPortalMe, portal, tool("portal_me", "Who am I acting for, and what can I do",
		note("The contact and company you act for, whether they can decide agreements (and whether the business lets an agent do it), and what's waiting on them. Call this first.")))
	a.handle(mux, "GET /api/portal/engagements", a.listPortalEngagements, portal, tool("portal_engagements", "Your engagements",
		note("Every engagement with you: its ref (like WO-12), title, state, and whether an agreement is waiting for your decision.")))
	a.handle(mux, "GET /api/portal/engagements/{ref}", a.showPortalEngagement, portal, tool("portal_engagement", "One engagement: what was agreed, the work, and anything awaiting you",
		note("Give its ref (WO-12) or title. The scope in force, the agreement waiting for your decision (exactly as it was sent), past agreements and decisions, the work shared with you, and its documents."),
		next("portal_decide", "portal_send_request")))
	a.handle(mux, "POST /api/portal/engagements/{engagement_ref}/approvals", a.createPortalApproval, portal, tool("portal_decide", "Decide on the agreement awaiting you",
		note("Approve, or ask for changes to, the agreement portal_engagement shows as awaiting your decision. Only a contact who can approve may; approver_name is the person's own typed name, which is recorded with the time and that their agent did it. Show them the agreement first."),
		bodyParam("decision", "string", "approved or changes_requested.", true),
		bodyParam("approver_name", "string", "The person's own typed name.", true),
		bodyParam("comment", "string", "Rich text: HTML (<p>, <strong>, lists) or plain text.", false),
		confirm("This records the client's decision on the agreement, binding as if they clicked it themselves.")))
	a.handle(mux, "GET /api/portal/work", a.listPortalWork, portal, tool("portal_work", "The work being done for you",
		note("Everything shared with you across your engagements, with its status and due date.")))
	a.handle(mux, "GET /api/portal/requests", a.listPortalRequests, portal, tool("portal_requests", "What you've asked for",
		note("Your requests, newest first, with whether each is still open, became work, or was closed.")))
	a.handle(mux, "POST /api/portal/requests", a.createPortalRequest, portal, tool("portal_send_request", "Ask for something",
		note("A request they triage: a short subject and what you need (and by when) in body. Show the person what you'll send first."),
		body("request", `{subject (required), body: text}`, true)))
	a.handle(mux, "GET /api/portal/documents/{id}", a.downloadPortalDocument, portal)
}

// approvalDecisions are what a client may decide.
var approvalDecisions = []string{"approved", "changes_requested"}

// ---- signing in --------------------------------------------------------------------

const portalLinkLifetime = time.Hour

// portalLoginToken is the emailed link's token: the contact and when it
// stops working, sealed with the install's key, so nothing is stored.
func (a *App) portalLoginToken(contactID int64) string {
	sealed := a.secrets.Seal(fmt.Sprintf("portal_login:%d:%d", contactID, time.Now().Add(portalLinkLifetime).Unix()))
	return strings.NewReplacer("+", "-", "/", "_").Replace(strings.TrimPrefix(sealed, secret.SealedPrefix))
}

// portalLoginContact is the contact a link's token is for; 0 when it
// isn't one, or it has expired.
func (a *App) portalLoginContact(token string) int64 {
	plain, ok := a.secrets.Open(secret.SealedPrefix + strings.NewReplacer("-", "+", "_", "/").Replace(token))
	if !ok {
		return 0
	}
	parts := strings.Split(plain, ":")
	if len(parts) != 3 || parts[0] != "portal_login" {
		return 0
	}
	id, _ := strconv.ParseInt(parts[1], 10, 64)
	exp, _ := strconv.ParseInt(parts[2], 10, 64)
	if time.Now().Unix() > exp {
		return 0
	}
	return id
}

// createPortalSession emails a sign-in link. It always answers the same
// way, so the form doesn't reveal which addresses exist.
func (a *App) createPortalSession(c *Ctx) error {
	if !allowed(a.mailed, "portal:"+c.ip()) {
		return &apierr.Error{Status: 429, Code: "rate_limited", Message: "Try again later."}
	}
	var body struct {
		Email string `json:"email"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	email := strings.ToLower(strings.TrimSpace(body.Email))
	notice := "If that address belongs to a client contact, a sign-in link is on its way."
	if email == "" {
		return c.JSON(200, O("notice", notice))
	}
	var id, clientID int64
	var name, client string
	err := a.store.QueryRowContext(ctx, `SELECT c.id, c.name, c.client_id, cl.name FROM contacts c JOIN clients cl ON cl.id = c.client_id
		WHERE lower(c.email) = ? AND c.portal_access = 1 AND c.archived_at IS NULL ORDER BY c.id LIMIT 1`, email).Scan(&id, &name, &clientID, &client)
	if err != nil && !isNoRows(err) {
		return err
	}
	if id != 0 {
		a.mailLater(portalLinkMail(email, name, client, a.BaseURL()+"/portal/session/"+a.portalLoginToken(id)))
	}
	return c.JSON(200, O("notice", notice))
}

// portalLinkMail is PortalMailer#magic_link.
func portalLinkMail(to, name, client, link string) mail.Message {
	first := strings.Fields(name)
	hi := "there"
	if len(first) > 0 {
		hi = first[0]
	}
	reason := "Someone asked to sign in with this address. If it wasn't you, ignore this email."
	text := "Hi " + hi + ",\n\nSign in: " + link + "\nThis link works for one hour.\n\n--\n" + reason + "\n"
	esc := func(s string) string {
		return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;").Replace(s)
	}
	html := `<!doctype html><html><body style="font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',sans-serif;color:#1f2937;line-height:1.5">` +
		`<div style="display:none">Your link to sign in. It works for one hour.</div>` +
		`<h1 style="font-size:20px">Sign in to ` + esc(client) + `’s portal</h1>` +
		`<p>Hi ` + esc(hi) + `, here’s your link to sign in. No password needed.</p>` +
		`<p><a href="` + esc(link) + `" style="display:inline-block;padding:10px 18px;border-radius:6px;background:#111827;color:#fff;text-decoration:none">Sign in</a></p>` +
		`<p style="font-size:13px;color:#6b7280">It works for one hour.</p>` +
		`<p style="font-size:12px;color:#9ca3af">Someone asked to sign in with this address. If it wasn’t you, ignore this email.</p></body></html>`
	return mail.Message{To: []string{to}, Subject: "Your sign-in link", Text: text, HTML: html}
}

// showPortalSessionLink signs a contact in from the emailed link.
func (a *App) showPortalSessionLink(c *Ctx) error {
	ctx := c.R.Context()
	id := a.portalLoginContact(c.Param("token"))
	var contact *Contact
	if id != 0 {
		var err error
		if contact, err = a.loadContact(ctx, id); err != nil {
			return err
		}
	}
	if contact == nil || !contact.PortalAccess || contact.Email == "" {
		return &apierr.Error{Status: 422, Code: "expired", Message: "That link has expired. Request a new one."}
	}
	csrf, err := a.signInContact(c, contact.ID)
	if err != nil {
		return err
	}
	return c.JSON(200, O("csrf", csrf, "contact", O("id", contact.ID, "name", contact.Name)))
}

func (a *App) destroyPortalSession(c *Ctx) error {
	if err := a.signOut(c); err != nil {
		return err
	}
	return c.NoContent()
}

// ---- who ---------------------------------------------------------------------------

// portalClient is the contact's client: its name, and the zone its times
// are shown in (its own, else ours).
func (a *App) portalClient(ctx context.Context, clientID int64) (name, zone string, err error) {
	var tz string
	err = a.store.QueryRowContext(ctx, `SELECT name, COALESCE(time_zone, '') FROM clients WHERE id = ?`, clientID).Scan(&name, &tz)
	if tz == "" {
		tz = a.zoneName(ctx)
	}
	return name, tz, err
}

func (a *App) showPortalMe(c *Ctx) error {
	ctx := c.R.Context()
	ct := c.Actor.Contact
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	client, zone, err := a.portalClient(ctx, ct.ClientID)
	if err != nil {
		return err
	}
	engagements, err := a.portalEngagements(ctx, ct.ClientID, "")
	if err != nil {
		return err
	}
	pending := []Obj{}
	for _, e := range engagements {
		if e.pending() != nil {
			pending = append(pending, a.portalRef(e))
		}
	}
	c.Summary = "You act for " + ct.Name + " at " + client
	if len(pending) > 0 {
		c.Summary += "; " + pluralize(len(pending), "agreement") + " awaiting a decision"
	}
	c.Summary += "."
	readOnly := c.Actor.Token != nil && c.Actor.Token.ReadOnly
	out := O("summary", c.Summary,
		"me", O("name", ct.Name, "email", ct.Email, "company", client, "can_decide_agreements", ct.CanApprove,
			"agents_may_decide", s.ClientAgentApprovals, "time_zone", zone),
		"business", a.BrandName(ctx), "awaiting_decision", pending, "read_only", readOnly)
	// What the portal's screens need besides: the zone to show times in,
	// the install's words, and the session's CSRF token.
	out = out.Set("time_zone_iana", location(zone).String()).Set("terms", s.TermsJSON()).Set("prices", s.Prices)
	if c.Actor.Session != nil {
		out = out.Set("csrf", c.Actor.Session.CSRF)
	}
	return c.JSON(200, out)
}

// ---- engagements, as the client sees them --------------------------------------------

type portalApproval struct {
	Decision, ApproverName, Method, UserAgent, Comment string
	DecidedAt                                          store.Time
}

type portalVersion struct {
	ID                           int64
	Number                       int
	Kind, Summary, Reason        string
	Cadence, ContentHash         string
	AmountCents, PriceDeltaCents int64
	Snapshot                     store.JSON
	SentAt                       store.Time
	SupersededByID               int64
	Approval                     *portalApproval
}

func (v *portalVersion) sent() bool { return v.SentAt.Valid }
func (v *portalVersion) approved() bool {
	return v.Approval != nil && v.Approval.Decision == "approved"
}
func (v *portalVersion) awaitingDecision() bool {
	return v.sent() && v.Approval == nil && v.SupersededByID == 0
}
func (v *portalVersion) changesRequested() bool {
	return v.Approval != nil && v.Approval.Decision == "changes_requested"
}
func (v *portalVersion) label() string {
	if v.Kind == "initial" {
		return "Agreement"
	}
	return fmt.Sprintf("%s #%d", humanize(v.Kind), v.Number)
}

// totalCents is what the version puts in front of the client: the whole
// price, or a change order's change to it.
func (v *portalVersion) totalCents() int64 {
	if v.Kind == "initial" || v.Kind == "revision" {
		return v.AmountCents
	}
	return v.PriceDeltaCents
}

type portalEngagement struct {
	ID                                    int64
	ClientID                              int64
	Ref, Label, Shape, Title, Description string
	ClosedAt                              store.Time
	CloseOutcome                          string
	Internal                              bool
	CreatedAt                             store.Time
	Versions                              []*portalVersion
}

func (e *portalEngagement) pending() *portalVersion {
	for _, v := range slices.Backward(e.Versions) {
		if v.awaitingDecision() {
			return v
		}
	}
	return nil
}

// state is derived, never stored: Engagement#state.
func (e *portalEngagement) state() string {
	switch {
	case e.ClosedAt.Valid:
		return "closed"
	case e.Internal:
		return "internal"
	}
	if len(e.Versions) == 0 {
		return "draft"
	}
	latest := e.Versions[len(e.Versions)-1]
	switch {
	case !latest.sent():
		return "draft"
	case latest.awaitingDecision():
		return "sent"
	case latest.changesRequested():
		return "changes_requested"
	}
	return "approved"
}

func (e *portalEngagement) stateLabel() string {
	if e.ClosedAt.Valid {
		return humanize(strOr(e.CloseOutcome, "closed"))
	}
	return humanize(e.state())
}

func (e *portalEngagement) active() bool {
	s := e.state()
	return s == "approved" || s == "internal"
}

func (e *portalEngagement) recurring() bool { return e.Shape == "recurring" }

func (e *portalEngagement) approvedVersions() []*portalVersion {
	var out []*portalVersion
	for _, v := range e.Versions {
		if v.approved() {
			out = append(out, v)
		}
	}
	return out
}

// agreedCents: fixed scope is the initial amount plus every approved
// change; recurring, the latest approved amount per period.
func (e *portalEngagement) agreedCents() int64 {
	versions := e.approvedVersions()
	if len(versions) == 0 {
		return 0
	}
	if e.recurring() {
		return versions[len(versions)-1].AmountCents
	}
	var sum int64
	for _, v := range versions {
		if v.Kind == "initial" {
			sum += v.AmountCents
		} else {
			sum += v.PriceDeltaCents
		}
	}
	return sum
}

// portalEngagements are a client's engagements, newest first, with their
// versions and decisions; ref narrows to one.
func (a *App) portalEngagements(ctx context.Context, clientID int64, ref string) ([]*portalEngagement, error) {
	where, args := `e.client_id = ?`, []any{clientID}
	if ref != "" {
		where += ` AND e.ref = ?`
		args = append(args, strings.ToUpper(ref))
	}
	rows, err := a.store.QueryContext(ctx, `SELECT e.id, e.client_id, e.ref, e.label, e.shape, e.title, COALESCE(e.description, ''), e.closed_at, COALESCE(e.close_outcome, ''), cl.internal, e.created_at
		FROM engagements e JOIN clients cl ON cl.id = e.client_id WHERE `+where+` ORDER BY e.created_at DESC, e.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (*portalEngagement, error) {
		var e portalEngagement
		err := r.Scan(&e.ID, &e.ClientID, &e.Ref, &e.Label, &e.Shape, &e.Title, &e.Description, &e.ClosedAt, &e.CloseOutcome, &e.Internal, &e.CreatedAt)
		return &e, err
	})
	if err != nil || len(list) == 0 {
		return list, err
	}
	byID := map[int64]*portalEngagement{}
	ids := make([]int64, len(list))
	for i, e := range list {
		byID[e.ID], ids[i] = e, e.ID
	}
	rows, err = a.store.QueryContext(ctx, `SELECT id, engagement_id, number, kind, COALESCE(summary, ''), COALESCE(reason, ''), COALESCE(cadence, ''), amount_cents, price_delta_cents,
			snapshot, COALESCE(content_hash, ''), sent_at, COALESCE(superseded_by_id, 0)
		FROM agreement_versions WHERE engagement_id IN (`+store.Placeholders(len(ids))+`) ORDER BY number, id`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	versions := map[int64]*portalVersion{}
	var versionIDs []int64
	defer rows.Close()
	for rows.Next() {
		var v portalVersion
		var engagementID int64
		if err := rows.Scan(&v.ID, &engagementID, &v.Number, &v.Kind, &v.Summary, &v.Reason, &v.Cadence, &v.AmountCents, &v.PriceDeltaCents,
			&v.Snapshot, &v.ContentHash, &v.SentAt, &v.SupersededByID); err != nil {
			return nil, err
		}
		e := byID[engagementID]
		e.Versions = append(e.Versions, &v)
		versions[v.ID] = &v
		versionIDs = append(versionIDs, v.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(versionIDs) == 0 {
		return list, nil
	}
	arows, err := a.store.QueryContext(ctx, `SELECT agreement_version_id, decision, COALESCE(approver_name, ''), method, COALESCE(user_agent, ''), COALESCE(comment, ''), decided_at
		FROM approvals WHERE agreement_version_id IN (`+store.Placeholders(len(versionIDs))+`)`, idArgs(versionIDs)...)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var ap portalApproval
		var versionID int64
		if err := arows.Scan(&versionID, &ap.Decision, &ap.ApproverName, &ap.Method, &ap.UserAgent, &ap.Comment, &ap.DecidedAt); err != nil {
			return nil, err
		}
		versions[versionID].Approval = &ap
	}
	return list, arows.Err()
}

// portalEngagement is one of the contact's client's engagements, by ref,
// or by title when an agent named it so.
func (a *App) portalEngagement(ctx context.Context, clientID int64, ref string) (*portalEngagement, error) {
	list, err := a.portalEngagements(ctx, clientID, ref)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, notFound("engagement")
	}
	return list[0], nil
}

// portalRef is an engagement as a client's agent sees it: its ref and
// title, and its portal page.
func (a *App) portalRef(e *portalEngagement) Obj {
	return O("type", "Engagement", "ref", e.Ref, "display_name", e.Ref+" "+e.Title, "url", a.BaseURL()+"/portal/engagements/"+e.Ref)
}

// portalSnapshot is a sent version exactly as the client saw it (or, for
// an old row without one, its items as they stand).
func (a *App) portalSnapshot(ctx context.Context, e *portalEngagement, v *portalVersion, client string) (map[string]any, error) {
	if snap := v.Snapshot.Map(); len(snap) > 0 {
		return snap, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT description, price_cents FROM scope_items WHERE agreement_version_id = ? ORDER BY position`, v.ID)
	if err != nil {
		return nil, err
	}
	items, err := store.ScanAll(rows, func(r *sqlRows) (map[string]any, error) {
		var d string
		var p int64
		err := r.Scan(&d, &p)
		return map[string]any{"description": d, "price_cents": p}, err
	})
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []map[string]any{}
	}
	return map[string]any{"engagement": map[string]any{"ref": e.Ref, "title": e.Title}, "client": client, "items": items}, nil
}

// pricedVersion is whether a version shows prices.
func pricedVersion(s *Settings, v *portalVersion, snap map[string]any) bool {
	if !s.Prices {
		return false
	}
	for _, it := range asList(snap["items"]) {
		if m, ok := it.(map[string]any); ok {
			if n, _ := intOf(m["price_cents"]); n != 0 {
				return true
			}
		}
	}
	return v.AmountCents != 0 || v.totalCents() != 0
}

// portalMoney is Rails' Money.format: "$1,200.00".
func portalMoney(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := strconv.FormatInt(cents/100, 10)
	var b strings.Builder
	for i, r := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(r)
	}
	s := fmt.Sprintf("%s.%02d", b.String(), cents%100)
	if neg {
		return "$-" + s
	}
	return "$" + s
}

func (a *App) listPortalEngagements(c *Ctx) error {
	ctx := c.R.Context()
	ct := c.Actor.Contact
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	client, _, err := a.portalClient(ctx, ct.ClientID)
	if err != nil {
		return err
	}
	list, err := a.portalEngagements(ctx, ct.ClientID, "")
	if err != nil {
		return err
	}
	out := []Obj{}
	awaiting := 0
	for _, e := range list {
		pending := e.pending() != nil
		if pending {
			awaiting++
		}
		agreed := e.agreedCents()
		out = append(out, a.portalRef(e).Set("title", e.Title).Set("label", s.LabelName(e.Label, 1)).Set("state", e.state()).
			Set("awaiting_your_decision", pending).
			Set("state_label", e.stateLabel()).Set("active", e.active()).Set("priced", s.Prices && agreed != 0).
			Set("agreed_cents", agreed).Set("agreed", portalMoney(agreed)).Set("per_period", e.recurring()))
	}
	c.Summary = pluralize(len(list), strings.ToLower(s.Term("engagement", len(list)))) + " with " + client
	if awaiting > 0 {
		c.Summary += fmt.Sprintf("; %d awaiting your decision", awaiting)
	}
	return c.JSON(200, O("summary", c.Summary, "engagements", out))
}

func (a *App) showPortalEngagement(c *Ctx) error {
	ctx := c.R.Context()
	ct := c.Actor.Contact
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	client, zone, err := a.portalClient(ctx, ct.ClientID)
	if err != nil {
		return err
	}
	e, err := a.portalEngagement(ctx, ct.ClientID, c.Param("ref"))
	if err != nil {
		return err
	}
	agreed := e.agreedCents()
	o := a.portalRef(e).Set("title", e.Title).Set("label", s.LabelName(e.Label, 1)).Set("state", e.state()).
		Set("agreed_cents", agreed).Set("agreed", portalMoney(agreed)).Set("per_period", e.recurring()).
		Set("description", nullable(plainText(e.Description)))

	versionJSON := func(v *portalVersion) (Obj, error) {
		snap, err := a.portalSnapshot(ctx, e, v, client)
		if err != nil {
			return nil, err
		}
		return O("label", v.label(), "sent_at", v.SentAt, "agreement", snap), nil
	}
	pending := e.pending()
	if pending != nil {
		p, err := versionJSON(pending)
		if err != nil {
			return err
		}
		how := "Someone at " + client + " who can approve agreements needs to decide."
		if ct.CanApprove {
			how = "Show the person the agreement above, then call portal_decide with their decision and their typed name."
		}
		agreement, _ := p.Get("agreement").(map[string]any) // versionJSON's
		o = o.Set("awaiting_your_decision", O("agreement", p.Get("agreement"), "label", p.Get("label"), "sent_at", p.Get("sent_at"),
			"you_can_decide", ct.CanApprove, "how", how).Set("version", a.portalVersionDetail(s, pending, agreement)))
	} else {
		o = o.Set("awaiting_your_decision", nil)
	}

	items, err := a.portalAgreedItems(ctx, e)
	if err != nil {
		return err
	}
	o = o.Set("scope_in_force", items)

	agreements := []Obj{}
	for _, v := range slices.Backward(e.Versions) {
		if !v.sent() {
			continue
		}
		vj, err := versionJSON(v)
		if err != nil {
			return err
		}
		if ap := v.Approval; ap != nil {
			vj = vj.Set("decision", ap.Decision).Set("decided_by", ap.ApproverName).Set("decided_at", ap.DecidedAt)
		}
		agreement, _ := vj.Get("agreement").(map[string]any) // versionJSON's
		agreements = append(agreements, vj.Set("version", a.portalVersionDetail(s, v, agreement)))
	}
	o = o.Set("agreements", agreements)

	work, err := a.portalWorkJSON(ctx, `t.engagement_id = ?`, e.ID)
	if err != nil {
		return err
	}
	o = o.Set("work", work)

	docs, err := a.portalDocuments(ctx, e)
	if err != nil {
		return err
	}
	o = o.Set("documents", docs)

	// What the portal's page shows besides: the description as rich text,
	// the state in words, the client-visible custom fields and the open
	// commitments.
	priced := s.Prices && agreed != 0
	o = o.Set("description_html", nullable(e.Description)).Set("state_label", e.stateLabel()).Set("active", e.active()).Set("priced", priced)
	fields, err := a.portalCustomFields(ctx, e.ID)
	if err != nil {
		return err
	}
	o = o.Set("custom_fields", fields)
	commitments, err := a.portalCommitments(ctx, e.ID, client, zone)
	if err != nil {
		return err
	}
	o = o.Set("commitments", commitments)

	c.Summary = e.Ref + " " + e.Title + ": " + strings.ToLower(e.stateLabel())
	if pending != nil {
		c.Summary += "; an agreement awaits your decision"
	}
	if pending != nil && ct.CanApprove {
		c.Crumbs = []Crumb{step("Decide on the agreement, once the person has seen it", "portal_decide", "engagement_ref", e.Ref)}
	}
	return c.JSON(200, O("summary", c.Summary, "engagement", o))
}

// portalVersionDetail is what the page prints of a version beside its
// snapshot: its words, its total, how it was decided.
func (a *App) portalVersionDetail(s *Settings, v *portalVersion, snap map[string]any) Obj {
	o := O("kind", v.Kind, "change", v.Kind != "initial", "summary", nullable(v.Summary), "reason", nullable(v.Reason), "cadence", nullable(v.Cadence),
		"total_cents", v.totalCents(), "total", portalMoney(v.totalCents()), "priced", pricedVersion(s, v, snap), "content_hash", nullable(v.ContentHash))
	if ap := v.Approval; ap != nil {
		via := ""
		if ap.Method == "agent" {
			parts := strings.Split(ap.UserAgent, " via ")
			via = parts[len(parts)-1]
		}
		o = o.Set("approval", O("decision", ap.Decision, "approver_name", ap.ApproverName, "method", ap.Method, "via", nullable(via),
			"comment", nullable(ap.Comment), "decided_at", ap.DecidedAt))
	}
	return o
}

// portalAgreedItems are the scope in force: every item of every approved
// version, with its delivery state from its work.
func (a *App) portalAgreedItems(ctx context.Context, e *portalEngagement) ([]Obj, error) {
	out := []Obj{}
	var ids []int64
	for _, v := range e.approvedVersions() {
		ids = append(ids, v.ID)
	}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT s.id, s.description, s.price_cents, (SELECT group_concat(status) FROM todos WHERE scope_item_id = s.id)
		FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id WHERE s.agreement_version_id IN (`+store.Placeholders(len(ids))+`) ORDER BY v.number, s.position`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id, price int64
		var desc string
		var statuses *string
		if err := r.Scan(&id, &desc, &price, &statuses); err != nil {
			return nil, err
		}
		var list []string
		if statuses != nil && *statuses != "" {
			list = strings.Split(*statuses, ",")
		}
		return O("description", desc, "price_cents", price, "price", portalMoney(price), "delivery", deliveryState(list)), nil
	})
}

// deliveryState is a scope item's, from the statuses of its work.
func deliveryState(statuses []string) string {
	switch {
	case len(statuses) == 0:
		return "not_started"
	case !slices.ContainsFunc(statuses, func(s string) bool { return s != "done" }):
		return "done"
	case slices.Contains(statuses, "blocked"):
		return "blocked"
	}
	return "in_progress"
}

// portalWorkJSON is the work shared with the client: client-visible todos
// by due date.
func (a *App) portalWorkJSON(ctx context.Context, where string, args ...any) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT t.title, t.status, t.due_on, e.ref, e.title FROM todos t JOIN engagements e ON e.id = t.engagement_id
		WHERE t.client_visible = 1 AND `+where+` ORDER BY t.due_on, t.position, t.id`, args...)
	if err != nil {
		return nil, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var title, status, ref, etitle string
		var due *string
		if err := r.Scan(&title, &status, &due, &ref, &etitle); err != nil {
			return nil, err
		}
		var dueOn any
		if due != nil && *due != "" {
			dueOn = (*due)[:min(len(*due), 10)]
		}
		return O("title", title, "status", status, "due_on", dueOn,
			"engagement", O("type", "Engagement", "ref", ref, "display_name", ref+" "+etitle, "url", a.BaseURL()+"/portal/engagements/"+ref)), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

// portalDocuments are every file the client may see for an engagement: its
// own, its scope items', its work's, and the client's.
func (a *App) portalDocuments(ctx context.Context, e *portalEngagement) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT d.id, d.created_at FROM documents d WHERE d.client_visible = 1 AND (
			(d.documentable_type = 'Engagement' AND d.documentable_id = ?)
			OR (d.documentable_type = 'ScopeItem' AND d.documentable_id IN (SELECT s.id FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id WHERE v.engagement_id = ?))
			OR (d.documentable_type = 'Todo' AND d.documentable_id IN (SELECT id FROM todos WHERE engagement_id = ? AND client_visible = 1))
			OR (d.documentable_type = 'Client' AND d.documentable_id = ?))
		ORDER BY d.created_at DESC, d.id DESC`, e.ID, e.ID, e.ID, e.ClientID)
	if err != nil {
		return nil, err
	}
	type doc struct {
		id int64
		at store.Time
	}
	docs, err := store.ScanAll(rows, func(r *sqlRows) (doc, error) {
		var d doc
		return d, r.Scan(&d.id, &d.at)
	})
	if err != nil {
		return nil, err
	}
	out := []Obj{}
	for _, d := range docs {
		b, err := a.attachedBlob(ctx, a.store, "Document", d.id, "file")
		if err != nil {
			return nil, err
		}
		if b == nil {
			continue
		}
		out = append(out, O("filename", b.Filename, "byte_size", b.ByteSize, "content_type", b.ContentType,
			"url", fmt.Sprintf("%s/api/portal/documents/%d", a.BaseURL(), d.id), "created_at", d.at))
	}
	return out, nil
}

// portalCustomFields are the engagement's fields marked visible to the
// client, with their values.
func (a *App) portalCustomFields(ctx context.Context, id int64) ([]Obj, error) {
	fields, err := a.customFields(ctx, a.store, "Engagement")
	if err != nil {
		return nil, err
	}
	values, err := a.customValues(ctx, a.store, "Engagement", []int64{id})
	if err != nil {
		return nil, err
	}
	out := []Obj{}
	for _, f := range fields {
		if !f.ClientVisible || f.ArchivedAt.Valid {
			continue
		}
		if v := values[id].Get(f.Key); v != nil && v != "" {
			out = append(out, O("key", f.Key, "label", f.Label, "kind", f.Kind, "value", v))
		}
	}
	return out, nil
}

// portalCommitments are the engagement's open commitments: who owes what,
// by when, and whether it's overdue in the client's zone.
func (a *App) portalCommitments(ctx context.Context, engagementID int64, client, zone string) ([]Obj, error) {
	today := time.Now().In(location(zone)).Format("2006-01-02")
	rows, err := a.store.QueryContext(ctx, `SELECT c.description, c.owner_kind, c.due_on, COALESCE(u.name, ''), COALESCE(u.email_address, ''), COALESCE(ct.name, '')
		FROM commitments c LEFT JOIN users u ON u.id = c.user_id LEFT JOIN contacts ct ON ct.id = c.contact_id
		WHERE c.engagement_id = ? AND c.resolved_at IS NULL ORDER BY c.due_on, c.id`, engagementID)
	if err != nil {
		return nil, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var desc, kind, due, user, email, contact string
		if err := r.Scan(&desc, &kind, &due, &user, &email, &contact); err != nil {
			return nil, err
		}
		due = due[:min(len(due), 10)]
		owner := strOr(contact, client)
		if kind == "us" {
			owner = strOr(strOr(user, email), "Us")
		}
		return O("description", desc, "owner_name", owner, "due_on", due, "overdue", due < today), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

// downloadPortalDocument is a shared file, for its client's contact only.
func (a *App) downloadPortalDocument(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	clientID := c.Actor.Contact.ClientID
	n, err := store.Count(ctx, a.store, `SELECT count(*) FROM documents d WHERE d.id = ? AND d.client_visible = 1 AND (
			(d.documentable_type = 'Client' AND d.documentable_id = ?)
			OR (d.documentable_type = 'Engagement' AND d.documentable_id IN (SELECT id FROM engagements WHERE client_id = ?))
			OR (d.documentable_type = 'ScopeItem' AND d.documentable_id IN (SELECT s.id FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id JOIN engagements e ON e.id = v.engagement_id WHERE e.client_id = ?))
			OR (d.documentable_type = 'Todo' AND d.documentable_id IN (SELECT t.id FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE e.client_id = ? AND t.client_visible = 1)))`,
		id, clientID, clientID, clientID, clientID)
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound("document")
	}
	b, err := a.attachedBlob(ctx, a.store, "Document", id, "file")
	if err != nil {
		return err
	}
	if b == nil {
		return notFound("document")
	}
	a.serveBlob(c.W, c.R, b, true)
	return nil
}

// ---- deciding -------------------------------------------------------------------------

// createPortalApproval records the client's decision on the agreement
// awaiting them, from the portal or by their own agent (with their typed
// name, when the owner lets agents decide).
func (a *App) createPortalApproval(c *Ctx) error {
	ctx := c.R.Context()
	ct := c.Actor.Contact
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	e, err := a.portalEngagement(ctx, ct.ClientID, c.Param("engagement_ref"))
	if err != nil {
		return err
	}
	v := e.pending()
	if v == nil || !ct.CanApprove {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Nothing is awaiting your approval."}
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	agent := c.Actor.Token != nil
	if agent && !s.ClientAgentApprovals {
		return &apierr.Error{Status: 422, Code: "refused", Message: a.BrandName(ctx) + " asks for agreements to be decided in the portal itself, not by an agent."}
	}
	name := strings.TrimSpace(str(body["approver_name"]))
	if name == "" {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Please type your name to confirm."}
	}
	decision := str(body["decision"])
	if decision == "" {
		return apierr.BadRequest("param is missing or the value is empty: decision")
	}
	if !slices.Contains(approvalDecisions, decision) {
		inv := apierr.Invalid{}
		inv.Add("decision", "is not included in the list")
		return inv.Err()
	}
	method, ua := "link", c.R.UserAgent()
	if agent {
		method, ua = "agent", ct.Name+"'s agent via "+c.Actor.Token.Name
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		return t.portalDecide(e, v, decision, method, ct, name, sanitizeRichText(str(body["comment"])), c.ip(), ua)
	})
	if err != nil {
		return err
	}
	c.Summary = "Thank you. Your decision has been recorded."
	return c.JSON(201, O("summary", c.Summary, "notice", c.Summary, "decision", decision, "engagement", a.portalRef(e)))
}

// portalDecide is AgreementVersion#decide! for a client's decision:
// recorded once, against the version's content hash; approving turns each
// of its scope items into work.
func (t *Tx) portalDecide(e *portalEngagement, v *portalVersion, decision, method string, ct *Contact, name, comment, ip, ua string) error {
	if !v.sent() || v.SupersededByID != 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Nothing is awaiting your approval."}
	}
	version, engagement, err := t.App.versionAndEngagement(t.Ctx, t, v.ID)
	if err != nil {
		return err
	}
	if version.Approval != nil {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Nothing is awaiting your approval."}
	}
	// One rule for every way a client decides: the engagements area's.
	return t.decideVersion(engagement, version, Decision{Decision: decision, Method: method, ContactID: ct.ID,
		ApproverName: name, ApproverEmail: ct.Email, Comment: comment, IPAddress: ip, UserAgent: ua})
}

// ---- work and requests ------------------------------------------------------------------

func (a *App) listPortalWork(c *Ctx) error {
	work, err := a.portalWorkJSON(c.R.Context(), `e.client_id = ?`, c.Actor.Contact.ClientID)
	if err != nil {
		return err
	}
	open := 0
	for _, w := range work {
		if w.Get("status") != "done" {
			open++
		}
	}
	c.Summary = fmt.Sprintf("%s of work shared with you, %d not done yet", pluralize(len(work), "piece"), open)
	return c.JSON(200, O("summary", c.Summary, "work", work))
}

func (a *App) listPortalRequests(c *Ctx) error {
	rows, err := a.store.QueryContext(c.R.Context(), `SELECT id, subject, received_at, status, COALESCE(dismissed_reason, '') FROM requests WHERE client_id = ? ORDER BY received_at DESC, id DESC LIMIT 100`,
		c.Actor.Contact.ClientID)
	if err != nil {
		return err
	}
	open := 0
	list, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var subject, status, reason string
		var at store.Time
		if err := r.Scan(&id, &subject, &at, &status, &reason); err != nil {
			return nil, err
		}
		words := "closed"
		switch status {
		case "open":
			words = "open: not dealt with yet"
			open++
		case "promoted":
			words = "taken on"
		default:
			if reason != "" {
				words += ": " + reason
			}
		}
		return O("id", id, "subject", subject, "received_at", at, "status", words, "state", status), nil
	})
	if err != nil {
		return err
	}
	if list == nil {
		list = []Obj{}
	}
	c.Summary = fmt.Sprintf("%s, %d still open", pluralize(len(list), "request"), open)
	return c.JSON(200, O("summary", c.Summary, "requests", list))
}

// createPortalRequest is a request from the client, for staff to triage.
func (a *App) createPortalRequest(c *Ctx) error {
	ctx := c.R.Context()
	ct := c.Actor.Contact
	body, err := c.Wrapped("request")
	if err != nil {
		return err
	}
	subject := strings.TrimSpace(str(body["subject"]))
	if subject == "" {
		inv := apierr.Invalid{}
		inv.Add("subject", "can't be blank")
		return inv.Err()
	}
	source, label := "portal", ct.Name
	if c.Actor.Token != nil {
		source, label = "agent", ct.Name+" via "+c.Actor.Token.Name
	}
	// The history says who asked, and through which app: no one on staff.
	asker := &Actor{Contact: ct}
	if c.Actor.Token != nil {
		asker.Token = &Token{ID: c.Actor.Token.ID, Name: label, Kind: c.Actor.Token.Kind}
	}
	var id int64
	now := store.Now()
	err = a.write(ctx, asker, func(t *Tx) error {
		id, err = t.Insert(`INSERT INTO requests (client_id, contact_id, sender_name, sender_email, subject, body, source, status, received_at, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'open', ?, ?, ?)`, ct.ClientID, ct.ID, ct.Name, nullable(ct.Email), subject, nullable(sanitizeRichText(str(body["body"]))), source, now.DB(), now.DB(), now.DB())
		if err != nil {
			return err
		}
		t.Reindex("Request", id)
		return t.Event("Request", id, "request.received", nil)
	})
	if err != nil {
		return err
	}
	c.Summary = "Sent. We'll get back to you."
	return c.JSON(201, O("summary", c.Summary, "notice", c.Summary, "request", O("id", id, "subject", subject, "received_at", now, "status", "open: not dealt with yet", "state", "open")))
}
