package core

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Triage: a request becomes a new engagement with a draft agreement, an
// item on an engagement's draft, a piece of work, or a commitment (the
// Request#promote_to_* verbs). Each writes the other areas' rows directly,
// the way those Rails models' callbacks did (refs, positions, the lead's
// assignment), and records request.promoted.

// promoteBody reads a triage action's body (top-level params, as Rails'
// member actions took them).
func promoteBody(c *Ctx) (map[string]any, error) {
	body := map[string]any{}
	if err := c.OptionalBody(&body); err != nil {
		return nil, err
	}
	return body, nil
}

// promoteSetup reads the request and the body, and refuses a request
// already triaged.
func (a *App) promoteSetup(c *Ctx) (*Request, map[string]any, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, nil, err
	}
	body, err := promoteBody(c)
	if err != nil {
		return nil, nil, err
	}
	q, err := a.request(c.R.Context(), a.store, id)
	if err != nil {
		return nil, nil, err
	}
	return q, body, nil
}

// reqMissing is Rails' params.require failing: 400.
func reqMissing(name string) error {
	return apierr.BadRequest("param is missing or the value is empty: " + name)
}

// reqDate is a YYYY-MM-DD date as given, or nil when it isn't one (Rails
// casts a bad date to nil).
func reqDate(v any) any {
	s := strings.TrimSpace(str(v))
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return nil
	}
	return s
}

// promoteEngagementByRef is the engagement a triage names, by its ref.
func (a *App) promoteEngagementByRef(c *Ctx, ref string) (id int64, upper string, err error) {
	upper = strings.ToUpper(strings.TrimSpace(ref))
	err = a.store.QueryRowContext(c.R.Context(), `SELECT id FROM engagements WHERE ref = ?`, upper).Scan(&id)
	if isNoRows(err) {
		return 0, upper, notFound("engagement")
	}
	return id, upper, err
}

// reqAdoptClient gives a request with no client the one triage chose.
func (t *Tx) reqAdoptClient(q *Request, v any) error {
	cid, ok := intOf(v)
	if !ok || cid == 0 || q.ClientID != 0 {
		return nil
	}
	if n, err := store.Count(t.Ctx, t, `SELECT count(*) FROM clients WHERE id = ?`, cid); err != nil {
		return err
	} else if n == 0 {
		return notFound("client")
	}
	if _, err := t.Exec(`UPDATE requests SET client_id = ?, updated_at = ? WHERE id = ?`, cid, store.Now().DB(), q.ID); err != nil {
		return err
	}
	q.ClientID = cid
	return nil
}

// markPromoted records what the request became (Request#promote!).
func (t *Tx) markPromoted(q *Request, typ string, id int64) error {
	if !q.Open() {
		return reqRefused("already triaged")
	}
	now := store.Now().DB()
	if _, err := t.Exec(`UPDATE requests SET status = 'promoted', promoted_type = ?, promoted_id = ?, triaged_by_id = ?, triaged_at = ?, updated_at = ? WHERE id = ?`,
		typ, id, reqNullID(t.Actor.UserID()), now, now, q.ID); err != nil {
		return err
	}
	q.Status = "promoted"
	return t.requestEvent(q.ID, "request.promoted", map[string]any{"to": typ, "id": id}, q.Source, "")
}

// reqNextRef is the next ref for a label, exactly as Engagement::Refs
// makes it: the label's prefix (Settings › Names) and one more than the
// highest number any ref with that prefix has.
func (t *Tx) reqNextRef(label string) (string, error) {
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
		parts := strings.Split(r, "-")
		n, _ := strconv.Atoi(reqDigits(parts[len(parts)-1]))
		last = max(last, n)
	}
	return fmt.Sprintf("%s-%d", prefix, last+1), nil
}

// reqDigits is Ruby's to_i on a string: its leading digits.
func reqDigits(s string) string {
	i := 0
	for i < len(s) && s[i] >= '0' && s[i] <= '9' {
		i++
	}
	return s[:i]
}

// reqDraftVersion is the engagement's open draft; with none, a new one
// (Engagement#draft_version!): the first is the initial agreement, later
// ones change orders (fixed) or revisions (recurring), nothing copied.
func (t *Tx) reqDraftVersion(engagementID int64, summary, reason any) (int64, error) {
	var id int64
	err := t.QueryRow(`SELECT id FROM agreement_versions WHERE engagement_id = ? AND sent_at IS NULL ORDER BY number DESC LIMIT 1`, engagementID).Scan(&id)
	if err == nil || !isNoRows(err) {
		return id, err
	}
	var shape string
	if err := t.QueryRow(`SELECT shape FROM engagements WHERE id = ?`, engagementID).Scan(&shape); err != nil {
		return 0, err
	}
	var count, maxNumber int64
	if err := t.QueryRow(`SELECT count(*), COALESCE(MAX(number), 0) FROM agreement_versions WHERE engagement_id = ?`, engagementID).Scan(&count, &maxNumber); err != nil {
		return 0, err
	}
	recurring := shape == "recurring"
	kind := "initial"
	if count > 0 {
		kind = "change_order"
		if recurring {
			kind = "revision"
		}
	}
	// The terms in force: the latest version the client approved.
	var curCadence string
	var curAmount int64
	err = t.QueryRow(`SELECT COALESCE(v.cadence, ''), v.amount_cents FROM agreement_versions v JOIN approvals ap ON ap.agreement_version_id = v.id
		WHERE v.engagement_id = ? AND ap.decision = 'approved' ORDER BY v.number DESC LIMIT 1`, engagementID).Scan(&curCadence, &curAmount)
	if err != nil && !isNoRows(err) {
		return 0, err
	}
	var cadence any
	if recurring {
		cadence = "monthly"
		if curCadence != "" {
			cadence = curCadence
		}
	}
	var amount int64
	if kind == "revision" {
		amount = curAmount
	}
	now := store.Now().DB()
	return t.Insert(`INSERT INTO agreement_versions (engagement_id, number, kind, summary, reason, cadence, amount_cents, price_delta_cents, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, 0, ?, ?)`, engagementID, maxNumber+1, kind, summary, reason, cadence, amount, now, now)
}

// reqAddScopeItem puts an item last on a draft version.
func (t *Tx) reqAddScopeItem(versionID int64, description string, priceCents int64) (int64, error) {
	var pos int64
	if err := t.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM scope_items WHERE agreement_version_id = ?`, versionID).Scan(&pos); err != nil {
		return 0, err
	}
	now := store.Now().DB()
	return t.Insert(`INSERT INTO scope_items (agreement_version_id, description, price_cents, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
		versionID, description, priceCents, pos+1, now, now)
}

// promoteEngagement drafts an engagement whose first scope item is the
// request (Request#promote_to_engagement!).
func (a *App) promoteRequestEngagement(c *Ctx) error {
	q, body, err := a.promoteSetup(c)
	if err != nil {
		return err
	}
	label := strings.TrimSpace(str(body["label"]))
	if label == "" {
		return reqMissing("label")
	}
	if !slices.Contains(Labels, label) {
		inv := apierr.Invalid{}
		inv.Add("label", "is not included in the list")
		return inv.Err()
	}
	title := strings.TrimSpace(str(body["title"]))
	if title == "" {
		title = q.Subject
	}
	var engID int64
	var ref string
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		if err := t.reqAdoptClient(q, body["client_id"]); err != nil {
			return err
		}
		if q.ClientID == 0 {
			return reqRefused("request has no client")
		}
		if !q.Open() {
			return reqRefused("already triaged")
		}
		if ref, err = t.reqNextRef(label); err != nil {
			return err
		}
		now := store.Now().DB()
		engID, err = t.Insert(`INSERT INTO engagements (client_id, ref, label, shape, title, description, created_by_id, created_at, updated_at) VALUES (?, ?, ?, 'fixed', ?, ?, ?, ?, ?)`,
			q.ClientID, ref, label, title, nullable(q.Body), reqNullID(t.Actor.UserID()), now, now)
		if err != nil {
			return err
		}
		version, err := t.reqDraftVersion(engID, nil, nil)
		if err != nil {
			return err
		}
		if _, err := t.reqAddScopeItem(version, q.Subject, 0); err != nil {
			return err
		}
		t.Reindex("Engagement", engID)
		return t.markPromoted(q, "Engagement", engID)
	})
	if err != nil {
		return err
	}
	c.Summary = "Drafted " + ref + " from the request."
	c.Crumbs = []Crumb{step("Send the agreement once it's ready", "show_engagement", "ref", ref)}
	return c.JSON(200, O("summary", c.Summary, "path", "/engagements/"+ref,
		"engagement", a.agentRef("Engagement", engID, ref, ref+" "+title, "/engagements/"+ref)))
}

// promoteRequestChange adds the request as an item on the engagement's open
// draft, opening a change order if there is none (Request#promote_to_change!).
func (a *App) promoteRequestChange(c *Ctx) error {
	q, body, err := a.promoteSetup(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(str(body["engagement_ref"])) == "" {
		return reqMissing("engagement_ref")
	}
	engID, ref, err := a.promoteEngagementByRef(c, str(body["engagement_ref"]))
	if err != nil {
		return err
	}
	price, _ := intOf(body["price_cents"])
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		if !q.Open() {
			return reqRefused("already triaged")
		}
		version, err := t.reqDraftVersion(engID, q.Subject, "Requested by "+q.RequesterName())
		if err != nil {
			return err
		}
		if _, err := t.reqAddScopeItem(version, q.Subject, price); err != nil {
			return err
		}
		return t.markPromoted(q, "AgreementVersion", version)
	})
	if err != nil {
		return err
	}
	c.Summary = "Added to " + ref + "'s draft."
	return c.JSON(200, O("summary", c.Summary, "path", "/engagements/"+ref))
}

// promoteRequestTodo makes the request work on an engagement
// (Request#promote_to_todo!): planned, last in its lists, its lead on it.
func (a *App) promoteRequestTodo(c *Ctx) error {
	q, body, err := a.promoteSetup(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(str(body["engagement_ref"])) == "" {
		return reqMissing("engagement_ref")
	}
	engID, ref, err := a.promoteEngagementByRef(c, str(body["engagement_ref"]))
	if err != nil {
		return err
	}
	var owner int64
	if id, ok := intOf(body["owner_id"]); ok && id != 0 {
		_ = a.store.QueryRowContext(c.R.Context(), `SELECT id FROM users WHERE id = ?`, id).Scan(&owner)
	}
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		if !q.Open() {
			return reqRefused("already triaged")
		}
		todo, err := t.reqInsertTodo(engID, q.Subject, nullable(q.Body), owner, reqDate(body["due_on"]))
		if err != nil {
			return err
		}
		return t.markPromoted(q, "Todo", todo)
	})
	if err != nil {
		return err
	}
	c.Summary = "Added as work on " + ref + "."
	return c.JSON(200, O("summary", c.Summary, "path", "/engagements/"+ref))
}

// reqInsertTodo adds planned work to an engagement, last on its list and on
// the board, with its lead's assignment (Todo's positioned columns and
// Todo::Assignable).
func (t *Tx) reqInsertTodo(engagementID int64, title string, description any, owner int64, due any) (int64, error) {
	var pos, board int64
	if err := t.QueryRow(`SELECT COALESCE(MAX(position), 0) FROM todos WHERE engagement_id = ? AND status = 'planned'`, engagementID).Scan(&pos); err != nil {
		return 0, err
	}
	if err := t.QueryRow(`SELECT COALESCE(MAX(board_position), 0) FROM todos WHERE status = 'planned'`).Scan(&board); err != nil {
		return 0, err
	}
	now := store.Now().DB()
	id, err := t.Insert(`INSERT INTO todos (engagement_id, title, description, status, owner_id, due_on, position, board_position, client_visible, created_by_id, created_at, updated_at)
		VALUES (?, ?, ?, 'planned', ?, ?, ?, ?, 1, ?, ?, ?)`, engagementID, title, description, reqNullID(owner), due, pos+1, board+1, reqNullID(t.Actor.UserID()), now, now)
	if err != nil {
		return 0, err
	}
	if owner != 0 {
		var assigner any
		if t.Actor != nil && t.Actor.User != nil {
			assigner = t.Actor.User.Person().ID
		}
		if _, err := t.Exec(`INSERT INTO assignments (todo_id, user_id, assigner_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, id, owner, assigner, now, now); err != nil {
			return 0, err
		}
	}
	t.Reindex("Todo", id)
	return id, nil
}

// promoteRequestCommitment makes the request a promise with a date
// (Request#promote_to_commitment!): the client's, made by its contact, or
// ours.
func (a *App) promoteRequestCommitment(c *Ctx) error {
	q, body, err := a.promoteSetup(c)
	if err != nil {
		return err
	}
	if strings.TrimSpace(str(body["due_on"])) == "" {
		return reqMissing("due_on")
	}
	var engID int64
	var ref string
	if strings.TrimSpace(str(body["engagement_ref"])) != "" {
		if engID, ref, err = a.promoteEngagementByRef(c, str(body["engagement_ref"])); err != nil {
			return err
		}
	}
	ownerKind := str(body["owner_kind"])
	if ownerKind == "" {
		ownerKind = "us"
	}
	var user int64
	if id, ok := intOf(body["user_id"]); ok && id != 0 {
		_ = a.store.QueryRowContext(c.R.Context(), `SELECT id FROM users WHERE id = ?`, id).Scan(&user)
	}
	var commitment int64
	err = a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		if err := t.reqAdoptClient(q, body["client_id"]); err != nil {
			return err
		}
		if q.ClientID == 0 {
			return reqRefused("request has no client")
		}
		if !q.Open() {
			return reqRefused("already triaged")
		}
		var problems []string
		due := reqDate(body["due_on"])
		if due == nil {
			problems = append(problems, "Give it a due date")
		}
		if ownerKind != "us" && ownerKind != "client" {
			problems = append(problems, "Say who made the promise: us or the client")
		}
		var contact int64
		if ownerKind == "client" && q.ContactID != 0 {
			contact = q.ContactID
			var contactClient int64
			if err := t.QueryRow(`SELECT client_id FROM contacts WHERE id = ?`, contact).Scan(&contactClient); err == nil && contactClient != q.ClientID {
				problems = append(problems, "Choose a contact at this client")
			}
		}
		if len(problems) > 0 {
			return reqRefused(strings.Join(problems, ". ") + ".")
		}
		now := store.Now().DB()
		commitment, err = t.Insert(`INSERT INTO commitments (client_id, engagement_id, description, owner_kind, user_id, contact_id, due_on, source, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, q.ClientID, reqNullID(engID), q.Subject, ownerKind, reqNullID(user), reqNullID(contact), due, q.Source, now, now)
		if err != nil {
			return err
		}
		t.Reindex("Commitment", commitment)
		return t.markPromoted(q, "Commitment", commitment)
	})
	if err != nil {
		return err
	}
	path := fmt.Sprintf("/clients/%d", q.ClientID)
	if ref != "" {
		path = "/engagements/" + ref
	}
	c.Summary = "Commitment added."
	return c.JSON(200, O("summary", c.Summary, "path", path))
}
