package core

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Scope items: the lines of what we agreed, on an agreement version. They
// change only on a draft; once sent they're the client's record. Approval
// makes each one a piece of work, and its delivery state is read off that
// work.

func init() { routeSets = append(routeSets, (*App).scopeItemRoutes) }

func (a *App) scopeItemRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/scope_items/{id}", a.showScopeItem, tool("show_scope_item", "Show a scope item",
		note("A scope item with its work and how delivery stands.")))
	a.handle(mux, "POST /api/agreement_versions/{id}/scope_items", a.createScopeItem, tool("add_scope_item", "Add a scope item to a draft",
		note("A line of what we agreed, on a draft agreement version. The internal estimate never reaches the client."),
		body("scope_item", `{description (required), price_cents: integer, internal_estimate}`, true)))
	a.handle(mux, "PATCH /api/scope_items/{id}", a.updateScopeItem, tool("update_scope_item", "Change a scope item on a draft",
		body("scope_item", `{description, price_cents: integer, internal_estimate}`, true)))
	a.handle(mux, "DELETE /api/scope_items/{id}", a.removeScopeItem, tool("remove_scope_item", "Remove a scope item from a draft"))
}

// ScopeItem is one line of an agreement version.
type ScopeItem struct {
	ID               int64
	VersionID        int64
	Description      string
	InternalEstimate string
	PriceCents       int64
	Position         int
	CreatedAt        store.Time
	Statuses         []string // its work's statuses
}

// deliveryStateOf is delivery from the statuses of an item's work:
// not_started, done, blocked or in_progress.
func deliveryStateOf(statuses []string) string {
	if len(statuses) == 0 {
		return "not_started"
	}
	all, blocked := true, false
	for _, s := range statuses {
		all = all && s == "done"
		blocked = blocked || s == "blocked"
	}
	switch {
	case all:
		return "done"
	case blocked:
		return "blocked"
	}
	return "in_progress"
}

// DeliveryState is the item's delivery state.
func (it *ScopeItem) DeliveryState() string { return deliveryStateOf(it.Statuses) }

// Progress is how much of its work is done, of how much.
func (it *ScopeItem) Progress() (done, total int) {
	for _, s := range it.Statuses {
		if s == "done" {
			done++
		}
	}
	return done, len(it.Statuses)
}

// loadItems reads the versions' items, by position, with their work's
// statuses straight from the todos table: two queries for the lot.
func (a *App) loadItems(ctx context.Context, db store.Querier, versions []*AgreementVersion) error {
	if len(versions) == 0 {
		return nil
	}
	byID := map[int64]*AgreementVersion{}
	ids := make([]int64, len(versions))
	for i, v := range versions {
		byID[v.ID], ids[i] = v, v.ID
		v.Items = nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id, agreement_version_id, description, COALESCE(internal_estimate, ''), price_cents, position, created_at
		FROM scope_items WHERE agreement_version_id IN (`+store.Placeholders(len(ids))+`) ORDER BY position, id`, idArgs(ids)...)
	if err != nil {
		return err
	}
	items, err := store.ScanAll(rows, func(r *sqlRows) (*ScopeItem, error) {
		var it ScopeItem
		return &it, r.Scan(&it.ID, &it.VersionID, &it.Description, &it.InternalEstimate, &it.PriceCents, &it.Position, &it.CreatedAt)
	})
	if err != nil || len(items) == 0 {
		return err
	}
	itemByID := map[int64]*ScopeItem{}
	itemIDs := make([]int64, len(items))
	for i, it := range items {
		byID[it.VersionID].Items = append(byID[it.VersionID].Items, it)
		itemByID[it.ID], itemIDs[i] = it, it.ID
	}
	rows, err = db.QueryContext(ctx, `SELECT scope_item_id, status FROM todos WHERE scope_item_id IN (`+store.Placeholders(len(itemIDs))+`)`, idArgs(itemIDs)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var status string
		if err := rows.Scan(&id, &status); err != nil {
			return err
		}
		itemByID[id].Statuses = append(itemByID[id].Statuses, status)
	}
	return rows.Err()
}

// scopeItemJSON is an item as answers show it (scope_items/_scope_item.json),
// with its position and progress for the page.
func (a *App) scopeItemJSON(it *ScopeItem) Obj {
	done, total := it.Progress()
	return a.agentRef("ScopeItem", it.ID, "", it.Description, fmt.Sprintf("/scope_items/%d", it.ID)).
		Set("description", it.Description).Set("internal_estimate", nullable(it.InternalEstimate)).
		Set("price", formatCents(it.PriceCents)).Set("price_cents", it.PriceCents).
		Set("delivery_state", it.DeliveryState()).
		Set("position", it.Position).Set("done", done).Set("total", total)
}

// scopeItemParams reads and checks an item's fields; partial for an update.
func scopeItemParams(body map[string]any, partial bool) (map[string]any, error) {
	inv := apierr.Invalid{}
	out := map[string]any{}
	if has(body, "description") || !partial {
		d := strings.TrimSpace(str(body["description"]))
		if d == "" {
			inv.Add("description", "can't be blank")
		}
		out["description"] = d
	}
	if has(body, "price_cents") || !partial {
		n, ok := roundCents(body["price_cents"])
		if !ok {
			inv.Add("price_cents", "must be an integer")
		}
		out["price_cents"] = n
	}
	if has(body, "internal_estimate") {
		out["internal_estimate"] = nullable(strings.TrimSpace(str(body["internal_estimate"])))
	}
	return out, inv.Err()
}

// scopeItemVersion is the version an item is on, and whether it's sent.
func (a *App) scopeItemVersion(ctx context.Context, id int64) (versionID int64, sent bool, err error) {
	var sentAt store.Time
	err = a.store.QueryRowContext(ctx, `SELECT v.id, v.sent_at FROM scope_items i JOIN agreement_versions v ON v.id = i.agreement_version_id WHERE i.id = ?`, id).Scan(&versionID, &sentAt)
	if isNoRows(err) {
		return 0, false, notFound("scope item")
	}
	return versionID, sentAt.Valid, err
}

func (a *App) showScopeItem(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	versionID, _, err := a.scopeItemVersion(ctx, id)
	if err != nil {
		return err
	}
	v, e, err := a.versionAndEngagement(ctx, a.store, versionID)
	if err != nil {
		return err
	}
	var it *ScopeItem
	for _, x := range v.Items {
		if x.ID == id {
			it = x
		}
	}
	if it == nil {
		return notFound("scope item")
	}
	work, err := a.scopeItemWork(ctx, e, it)
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	o := a.scopeItemJSON(it).
		Set("engagement", a.agentRef("Engagement", e.ID, e.Ref, e.Ref+" "+e.Title, "/engagements/"+e.Ref).Set("title", e.Title).Set("closed", e.Closed())).
		Set("agreement_version", a.agentRef("AgreementVersion", v.ID, "", e.Ref+" "+v.Label(), "/engagements/"+e.Ref).Set("label", v.Label()).Set("status", v.Status())).
		Set("client", a.agentRef("Client", e.ClientID, "", e.ClientName, fmt.Sprintf("/clients/%d", e.ClientID))).
		Set("work", work).Set("prices", s.Prices)
	// The work, notes and documents areas may answer their own keys.
	o, err = a.withSections(ctx, "ScopeItem", id, o)
	if err != nil {
		return err
	}
	c.Summary = it.Description + " (" + strings.ToLower(humanize(it.DeliveryState())) + ")"
	c.Crumbs = []Crumb{step("Show its engagement", "show_engagement", "ref", e.Ref)}
	return c.JSON(200, O("summary", c.Summary, "scope_item", o))
}

// scopeItemWork is the item's work (todos/_todo.json, the parts this area
// reads), by due date then position, straight from the todos table.
func (a *App) scopeItemWork(ctx context.Context, e *Engagement, it *ScopeItem) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT t.id, t.title, t.status, COALESCE(strftime('%Y-%m-%d', t.due_on), ''), t.client_visible, COALESCE(t.owner_id, 0), COALESCE(u.name, '')
		FROM todos t LEFT JOIN users u ON u.id = t.owner_id WHERE t.scope_item_id = ? ORDER BY t.due_on IS NULL, t.due_on, t.position`, it.ID)
	if err != nil {
		return nil, err
	}
	today := a.today(ctx)
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id, owner int64
		var title, status, ownerName string
		var due string
		var visible bool
		if err := r.Scan(&id, &title, &status, &due, &visible, &owner, &ownerName); err != nil {
			return nil, err
		}
		var dueOn any
		overdue := false
		if due != "" {
			dueOn, overdue = due, due < today && status != "done"
		}
		return a.agentRef("Todo", id, "", title, fmt.Sprintf("/work/%d", id)).
			Set("title", title).Set("status", status).Set("due_on", dueOn).Set("client_visible", visible).Set("overdue", overdue).
			Set("owner", userRef(owner, ownerName)).
			Set("engagement", a.agentRef("Engagement", e.ID, e.Ref, e.Ref+" "+e.Title, "/engagements/"+e.Ref)).
			Set("scope_item", a.agentRef("ScopeItem", it.ID, "", it.Description, fmt.Sprintf("/scope_items/%d", it.ID))), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

func (a *App) createScopeItem(c *Ctx) error {
	versionID, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("scope_item")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	v, err := a.versionByID(ctx, a.store, versionID)
	if err != nil {
		return err
	}
	fields, err := scopeItemParams(body, false)
	if err != nil {
		return refused(sentence(err))
	}
	if v.Sent() {
		return refused("items of a sent version are immutable")
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		var position int
		if err := t.QueryRow(`SELECT COALESCE(MAX(position), 0) + 1 FROM scope_items WHERE agreement_version_id = ?`, versionID).Scan(&position); err != nil {
			return err
		}
		now := store.Now().DB()
		id, err = t.Insert(`INSERT INTO scope_items (agreement_version_id, description, internal_estimate, price_cents, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			versionID, fields["description"], fields["internal_estimate"], fields["price_cents"], position, now, now)
		return err
	})
	if err != nil {
		return err
	}
	return a.scopeItemAnswer(c, 201, id, "Item added.")
}

// scopeItemAnswer answers a change to an item: the summary and the item.
func (a *App) scopeItemAnswer(c *Ctx, status int, id int64, summary string) error {
	ctx := c.R.Context()
	versionID, _, err := a.scopeItemVersion(ctx, id)
	if err != nil {
		return err
	}
	v, err := a.versionByID(ctx, a.store, versionID)
	if err != nil {
		return err
	}
	for _, it := range v.Items {
		if it.ID == id {
			c.Summary = summary
			return c.JSON(status, O("summary", summary, "scope_item", a.scopeItemJSON(it)))
		}
	}
	return notFound("scope item")
}

func (a *App) updateScopeItem(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("scope_item")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	_, sent, err := a.scopeItemVersion(ctx, id)
	if err != nil {
		return err
	}
	fields, err := scopeItemParams(body, true)
	if err != nil {
		return refused(sentence(err))
	}
	if sent {
		return refused("items of a sent version are immutable")
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("scope_items", id, fields) }); err != nil {
		return err
	}
	return a.scopeItemAnswer(c, 200, id, "Item updated.")
}

func (a *App) removeScopeItem(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	_, sent, err := a.scopeItemVersion(ctx, id)
	if err != nil {
		return err
	}
	if sent {
		return refused("items of a sent version are immutable")
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteScopeItem(id) }); err != nil {
		return err
	}
	c.Summary = "Item removed."
	return c.JSON(200, O("summary", c.Summary, "deleted", true))
}

// deleteScopeItem removes an item: its work stays, without it (Rails'
// dependent: :nullify); its notes and documents go with it (what areas
// registered for ScopeItem).
func (t *Tx) deleteScopeItem(id int64) error {
	if _, err := t.Exec(`UPDATE todos SET scope_item_id = NULL WHERE scope_item_id = ?`, id); err != nil {
		return err
	}
	if err := t.cascade("ScopeItem", id); err != nil {
		return err
	}
	_, err := t.Exec(`DELETE FROM scope_items WHERE id = ?`, id)
	return err
}
