package core

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Commitments: promises with a date between you and the people you work
// for, ours or the client's. Final once resolved: done, missed or dropped.

func init() {
	routeSets = append(routeSets, (*App).commitmentRoutes)
	searchable("Commitment", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		err := db.QueryRowContext(ctx, `SELECT description, COALESCE(resolution_note, ''), client_id, created_at FROM commitments WHERE id = ?`, id).
			Scan(&d.Title, &d.Content, &d.ClientID, &d.CreatedAt)
		if isNoRows(err) {
			return d, false, nil
		}
		return d, err == nil, err
	})
	addSection("Client", "open_commitments", func(a *App, ctx context.Context, id int64) (any, error) {
		return a.commitmentsJSON(ctx, a.store, `cm.client_id = ? AND cm.resolution IS NULL ORDER BY cm.due_on, cm.id`, id)
	})
	addSection("Engagement", "commitments", func(a *App, ctx context.Context, id int64) (any, error) {
		return a.commitmentsJSON(ctx, a.store, `cm.engagement_id = ? ORDER BY cm.due_on, cm.id`, id)
	})
	// A client's or an engagement's commitments go with it.
	onDelete("Client", func(t *Tx, id int64) error { return t.deleteCommitmentsWhere(`client_id = ?`, id) })
	onDelete("Engagement", func(t *Tx, id int64) error { return t.deleteCommitmentsWhere(`engagement_id = ?`, id) })
}

// CommitmentOwnerKinds are whose a commitment is: ours or the client's.
var CommitmentOwnerKinds = []string{"us", "client"}

// CommitmentResolutions are how a commitment ends.
var CommitmentResolutions = []string{"done", "missed", "dropped"}

const commitmentFields = `{description (required), due_on: YYYY-MM-DD (required), owner (required): us, user:<id>, client or contact:<id>, source: where it was promised}`

func (a *App) commitmentRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/commitments", a.listCommitments, tool("list_commitments", "List commitments",
		query("state", "string", "open (the default) or resolved."),
		query("owner_kind", "string", "us, client or all (the default).")))
	a.handle(mux, "GET /api/commitments/owners", a.commitmentOwners)
	a.handle(mux, "GET /api/commitments/{id}", a.showCommitment)
	a.handle(mux, "POST /api/engagements/{ref}/commitments", a.createCommitment, tool("create_commitment", "Add a commitment to an engagement",
		note("A promise with a date, by us or the client. owner: us, user:<id>, or client."),
		body("commitment", commitmentFields, true)))
	a.handle(mux, "POST /api/clients/{id}/commitments", a.createCommitment)
	a.handle(mux, "PATCH /api/commitments/{id}", a.updateCommitment, tool("update_commitment", "Change an open commitment",
		body("commitment", `{description, due_on: YYYY-MM-DD, owner: us, user:<id>, client or contact:<id>, source}`, true)))
	a.handle(mux, "POST /api/commitments/{id}/resolve", a.resolveCommitment, tool("resolve_commitment", "Resolve a commitment",
		note("Final once resolved."),
		bodyParam("resolution", "string", "done, missed or dropped.", false),
		bodyParam("note", "string", "How it ended (optional).", false)))
	a.handle(mux, "DELETE /api/commitments/{id}", a.deleteCommitment, need("delete_records"), tool("delete_commitment", "Delete an open commitment"))
}

// Commitment is one promise, with the names it's shown by.
type Commitment struct {
	ID             int64
	ClientID       int64
	EngagementID   int64
	Description    string
	OwnerKind      string
	UserID         int64
	ContactID      int64
	DueOn          string
	Resolution     string
	ResolvedAt     store.Time
	ResolutionNote string
	Source         string
	CreatedAt      store.Time

	ClientName, UserName, ContactName, EngagementRef, EngagementTitle string
	OwnerName                                                         string
}

// Open is whether it's still to come.
func (cm *Commitment) Open() bool { return cm.Resolution == "" }

// Overdue is whether it's open and past its date.
func (cm *Commitment) Overdue(today string) bool { return cm.Open() && cm.DueOn < today }

// OwnerKey is the one select's value for who owns it: us, user:3, client
// or contact:5.
func (cm *Commitment) OwnerKey() string {
	if cm.OwnerKind == "us" {
		if cm.UserID != 0 {
			return fmt.Sprintf("user:%d", cm.UserID)
		}
		return "us"
	}
	if cm.ContactID != 0 {
		return fmt.Sprintf("contact:%d", cm.ContactID)
	}
	return "client"
}

// Promiser is who will do it, as the sentence reads: "Rosa", "We".
func (cm *Commitment) Promiser() string {
	if cm.OwnerKind == "us" {
		return strOr(cm.UserName, "We")
	}
	return strOr(cm.ContactName, cm.ClientName)
}

const commitmentColumns = `cm.id, cm.client_id, COALESCE(cm.engagement_id, 0), cm.description, cm.owner_kind, COALESCE(cm.user_id, 0), COALESCE(cm.contact_id, 0),
	cm.due_on, COALESCE(cm.resolution, ''), cm.resolved_at, COALESCE(cm.resolution_note, ''), cm.source, cm.created_at,
	COALESCE(cl.name, ''), COALESCE(CASE WHEN u.name = '' THEN u.email_address ELSE u.name END, ''), COALESCE(ct.name, ''), COALESCE(e.ref, ''), COALESCE(e.title, '')`

const commitmentJoins = ` FROM commitments cm LEFT JOIN clients cl ON cl.id = cm.client_id LEFT JOIN users u ON u.id = cm.user_id
	LEFT JOIN contacts ct ON ct.id = cm.contact_id LEFT JOIN engagements e ON e.id = cm.engagement_id`

func scanCommitment(r interface{ Scan(...any) error }) (*Commitment, error) {
	var cm Commitment
	err := r.Scan(&cm.ID, &cm.ClientID, &cm.EngagementID, &cm.Description, &cm.OwnerKind, &cm.UserID, &cm.ContactID,
		&cm.DueOn, &cm.Resolution, &cm.ResolvedAt, &cm.ResolutionNote, &cm.Source, &cm.CreatedAt,
		&cm.ClientName, &cm.UserName, &cm.ContactName, &cm.EngagementRef, &cm.EngagementTitle)
	cm.DueOn = workDate(cm.DueOn)
	// Whose it is in words: one of us, else "Us"; a contact, else the client.
	if cm.OwnerKind == "us" {
		cm.OwnerName = strOr(cm.UserName, "Us")
	} else {
		cm.OwnerName = strOr(cm.ContactName, cm.ClientName)
	}
	return &cm, err
}

// loadCommitments are the commitments matching where (cm is the table; it
// may end in ORDER BY).
func (a *App) loadCommitments(ctx context.Context, db store.Querier, where string, args ...any) ([]*Commitment, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+commitmentColumns+commitmentJoins+` WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (*Commitment, error) { return scanCommitment(r) })
}

func (a *App) commitment(ctx context.Context, db store.Querier, id int64, openOnly bool) (*Commitment, error) {
	where := `cm.id = ?`
	if openOnly {
		where += ` AND cm.resolution IS NULL`
	}
	list, err := a.loadCommitments(ctx, db, where, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, notFound("commitment")
	}
	return list[0], nil
}

// commitmentJSON is a commitment as answers show it
// (commitments/_commitment.json), with the words the app's lists use.
func (a *App) commitmentJSON(cm *Commitment, today string) Obj {
	path := fmt.Sprintf("/clients/%d", cm.ClientID)
	var eng any
	if cm.EngagementID != 0 {
		path = "/engagements/" + cm.EngagementRef
		eng = a.agentRef("Engagement", cm.EngagementID, cm.EngagementRef, cm.EngagementRef+" "+cm.EngagementTitle, path)
	}
	return a.agentRef("Commitment", cm.ID, "", cm.Description, path).
		Set("description", cm.Description).Set("due_on", cm.DueOn).Set("owner", cm.OwnerKey()).Set("source", cm.Source).
		Set("resolution", nullable(cm.Resolution)).Set("resolution_note", nullable(cm.ResolutionNote)).
		Set("owner_name", cm.OwnerName).Set("open", cm.Open()).Set("overdue", cm.Overdue(today)).
		Set("client", a.agentRef("Client", cm.ClientID, "", cm.ClientName, fmt.Sprintf("/clients/%d", cm.ClientID))).
		Set("engagement", eng).
		Set("owner_kind", cm.OwnerKind).Set("promiser", cm.Promiser()).Set("resolved_at", cm.ResolvedAt).Set("created_at", cm.CreatedAt)
}

func (a *App) commitmentsJSON(ctx context.Context, db store.Querier, where string, args ...any) ([]Obj, error) {
	list, err := a.loadCommitments(ctx, db, where, args...)
	if err != nil {
		return nil, err
	}
	today := a.today(ctx)
	out := make([]Obj, len(list))
	for i, cm := range list {
		out[i] = a.commitmentJSON(cm, today)
	}
	return out, nil
}

// commitmentDate is Rails' l(date, format: :short): "Oct 20".
func commitmentDate(ymd string) string {
	d, err := time.Parse("2006-01-02", ymd)
	if err != nil {
		return ymd
	}
	return d.Format("Jan 02")
}

// ---- reading -----------------------------------------------------------------

var commitmentSorts = map[string]string{
	"description": "cm.description",
	"due":         "cm.due_on",
	"client":      "cl.name",
	"status":      "cm.resolution",
}

func (a *App) listCommitments(c *Ctx) error {
	ctx := c.R.Context()
	state := c.Query("state")
	if state != "resolved" {
		state = "open"
	}
	kind := c.Query("owner_kind")
	if !slices.Contains(CommitmentOwnerKinds, kind) {
		kind = "all"
	}
	where, order, args := `cm.resolution IS NULL`, `cm.due_on, cm.id`, []any{}
	if state == "resolved" {
		where, order = `cm.resolution IS NOT NULL`, `cm.resolved_at DESC, cm.id`
	}
	if kind != "all" {
		where += ` AND cm.owner_kind = ?`
		args = append(args, kind)
	}
	if expr, ok := commitmentSorts[c.Query("sort")]; ok {
		dir := "ASC"
		if strings.EqualFold(c.Query("direction"), "desc") {
			dir = "DESC"
		}
		order = expr + " IS NULL, " + expr + " " + dir + ", cm.id"
	}
	list, err := a.commitmentsJSON(ctx, a.store, where+" ORDER BY "+order, args...)
	if err != nil {
		return err
	}
	counts, err := countByString(ctx, a.store, `SELECT CASE WHEN resolution IS NULL THEN 'open' ELSE 'resolved' END, count(*) FROM commitments GROUP BY 1`)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	filters := []Obj{O("value", "open", "label", "Open", "count", counts["open"]), O("value", "resolved", "label", "Resolved", "count", counts["resolved"])}
	c.Summary = fmt.Sprintf("%s (%s)", pluralize(len(list), "commitment"), state)
	return c.pagedAnswer(O("summary", c.Summary, "state", state, "owner_kind", kind, "filters", filters,
		"any_commitments", counts["open"]+counts["resolved"] > 0, "term", s.Term("commitment", 1)), "commitments", list, commitmentSorts)
}

func (a *App) showCommitment(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	cm, err := a.commitment(ctx, a.store, id, false)
	if err != nil {
		return err
	}
	groups, err := a.commitmentOwnerGroups(ctx, cm.ClientID)
	if err != nil {
		return err
	}
	c.Summary = cm.Promiser() + " will " + cm.Description
	return c.JSON(200, O("summary", c.Summary, "commitment", a.commitmentJSON(cm, a.today(ctx)).Set("owner_options", groups)))
}

// commitmentOwners are who can make a promise on either side, for a
// client: you and your people, the client and its contacts.
func (a *App) commitmentOwners(c *Ctx) error {
	id, _ := strconv.ParseInt(c.Query("client_id"), 10, 64)
	ctx := c.R.Context()
	if _, err := a.client(ctx, a.store, id); err != nil {
		return err
	}
	groups, err := a.commitmentOwnerGroups(ctx, id)
	if err != nil {
		return err
	}
	return c.JSON(200, O("owner_options", groups))
}

func (a *App) commitmentOwnerGroups(ctx context.Context, clientID int64) ([]Obj, error) {
	cl, err := a.client(ctx, a.store, clientID)
	if err != nil {
		return nil, err
	}
	team, err := a.team(ctx)
	if err != nil {
		return nil, err
	}
	ours := []Obj{O("value", "us", "label", "We")}
	for _, u := range team {
		ours = append(ours, O("value", fmt.Sprintf("user:%d", u.Get("id")), "label", u.Get("name")))
	}
	theirs := []Obj{O("value", "client", "label", cl.Name)}
	rows, err := a.store.QueryContext(ctx, `SELECT id, name FROM contacts WHERE client_id = ? AND archived_at IS NULL ORDER BY name COLLATE NOCASE`, clientID)
	if err != nil {
		return nil, err
	}
	contacts, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var name string
		err := r.Scan(&id, &name)
		return O("value", fmt.Sprintf("contact:%d", id), "label", name), err
	})
	if err != nil {
		return nil, err
	}
	return []Obj{O("label", "Your side", "options", ours), O("label", cl.Name, "options", append(theirs, contacts...))}, nil
}

// ---- writing -----------------------------------------------------------------

// commitmentParams reads and checks a commitment's fields against its
// client; partial for an update.
func (a *App) commitmentParams(ctx context.Context, body map[string]any, clientID int64, partial bool) (map[string]any, error) {
	inv := apierr.Invalid{}
	out := map[string]any{}
	if has(body, "description") || !partial {
		d := strings.TrimSpace(str(body["description"]))
		if d == "" {
			inv.Add("description", "can't be blank")
		}
		out["description"] = d
	}
	if has(body, "due_on") || !partial {
		d := workDay(body["due_on"])
		if d == nil {
			inv.Add("due_on", "can't be blank")
		}
		out["due_on"] = d
	}
	if has(body, "owner") || !partial {
		// One select picks who owns it: "us", "user:3", "client" or "contact:5".
		kind, idText, _ := strings.Cut(str(body["owner"]), ":")
		id, _ := strconv.ParseInt(idText, 10, 64)
		out["owner_kind"], out["user_id"], out["contact_id"] = "client", nil, nil
		if kind == "us" || kind == "user" || kind == "" {
			out["owner_kind"] = "us"
		}
		if kind == "user" && id > 0 {
			out["user_id"] = id
		}
		if kind == "contact" && id > 0 {
			out["contact_id"] = id
			if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM contacts WHERE id = ? AND client_id = ?`, id, clientID); err != nil {
				return nil, err
			} else if n == 0 {
				inv.Add("contact", "must belong to the client")
			}
		}
	}
	if has(body, "source") {
		if s := strings.TrimSpace(str(body["source"])); s != "" || !partial {
			out["source"] = s
		} else {
			inv.Add("source", "can't be blank")
		}
	}
	return out, inv.Err()
}

// insertCommitment adds a commitment and records it (commitment.added).
func (t *Tx) insertCommitment(clientID int64, engagementID any, f map[string]any, payload map[string]any) (int64, error) {
	now := store.Now().DB()
	source := str(f["source"])
	if source == "" {
		source = t.Actor.Source()
	}
	id, err := t.Insert(`INSERT INTO commitments (client_id, engagement_id, description, owner_kind, user_id, contact_id, due_on, source, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, clientID, engagementID, f["description"], f["owner_kind"], f["user_id"], f["contact_id"], f["due_on"], source, now, now)
	if err != nil {
		return 0, err
	}
	t.Reindex("Commitment", id)
	return id, t.Event("Commitment", id, "commitment.added", payload)
}

func (a *App) createCommitment(c *Ctx) error {
	ctx := c.R.Context()
	var clientID int64
	var engagementID any
	var home string
	if ref := c.Param("ref"); ref != "" {
		e, err := a.workEngagementByRef(ctx, a.store, ref)
		if err != nil {
			return err
		}
		clientID, engagementID, home = e.ClientID, e.ID, "/engagements/"+e.Ref
	} else {
		id, err := c.IntParam("id")
		if err != nil {
			return err
		}
		if _, err := a.client(ctx, a.store, id); err != nil {
			return err
		}
		clientID, home = id, fmt.Sprintf("/clients/%d", id)
	}
	body, err := c.Wrapped("commitment")
	if err != nil {
		return err
	}
	f, err := a.commitmentParams(ctx, body, clientID, false)
	if err != nil {
		return err
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		id, err = t.insertCommitment(clientID, engagementID, f, nil)
		return err
	})
	if err != nil {
		return err
	}
	cm, err := a.commitment(ctx, a.store, id, false)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	c.Summary = fmt.Sprintf("%s added: %s by %s.", s.Term("commitment", 1), cm.OwnerName, commitmentDate(cm.DueOn))
	c.Crumbs = []Crumb{step("Resolve it when it's kept or missed", "resolve_commitment", "id", id)}
	return c.JSON(201, O("summary", c.Summary, "commitment", a.commitmentJSON(cm, a.today(ctx)), "path", home))
}

func (a *App) updateCommitment(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("commitment")
	if err != nil {
		return err
	}
	cm, err := a.commitment(ctx, a.store, id, true)
	if err != nil {
		return err
	}
	f, err := a.commitmentParams(ctx, body, cm.ClientID, true)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.update("commitments", id, f); err != nil {
			return err
		}
		t.Reindex("Commitment", id)
		return nil
	})
	if err != nil {
		return err
	}
	if cm, err = a.commitment(ctx, a.store, id, false); err != nil {
		return err
	}
	c.Summary = "Commitment updated."
	return c.JSON(200, O("summary", c.Summary, "commitment", a.commitmentJSON(cm, a.today(ctx))))
}

// resolve ends an open commitment: final from then on.
func (t *Tx) resolveCommitment(cm *Commitment, resolution, note string) error {
	if !cm.Open() {
		return &apierr.Error{Status: 422, Code: "refused", Message: "It’s already resolved: a resolved commitment is final."}
	}
	now := store.Now().DB()
	if _, err := t.Exec(`UPDATE commitments SET resolution = ?, resolved_at = ?, resolution_note = ?, updated_at = ? WHERE id = ? AND resolution IS NULL`,
		resolution, now, nullable(note), now, cm.ID); err != nil {
		return err
	}
	t.Reindex("Commitment", cm.ID)
	return t.Event("Commitment", cm.ID, "commitment."+resolution, map[string]any{"note": nullable(note)})
}

func (a *App) resolveCommitment(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	var body map[string]any
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	cm, err := a.commitment(ctx, a.store, id, false)
	if err != nil {
		return err
	}
	resolution := str(body["resolution"])
	if !slices.Contains(CommitmentResolutions, resolution) {
		resolution = "done"
	}
	note := strings.TrimSpace(str(body["note"]))
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.resolveCommitment(cm, resolution, note) }); err != nil {
		return err
	}
	if cm, err = a.commitment(ctx, a.store, id, false); err != nil {
		return err
	}
	c.Summary = "Marked " + resolution + "."
	return c.JSON(200, O("summary", c.Summary, "commitment", a.commitmentJSON(cm, a.today(ctx))))
}

// deleteCommitment removes one commitment with its history.
func (t *Tx) deleteCommitment(id int64) error {
	if err := t.cascade("Commitment", id); err != nil {
		return err
	}
	_, err := t.Exec(`DELETE FROM commitments WHERE id = ?`, id)
	return err
}

func (t *Tx) deleteCommitmentsWhere(where string, args ...any) error {
	ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM commitments WHERE `+where, args...)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if err := t.deleteCommitment(id); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) deleteCommitment(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	if _, err := a.commitment(ctx, a.store, id, true); err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteCommitment(id) }); err != nil {
		return err
	}
	c.Summary = "Commitment deleted."
	return c.JSON(200, O("summary", c.Summary, "deleted", true))
}
