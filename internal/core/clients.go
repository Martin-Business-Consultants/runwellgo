package core

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Clients: who we work for. A client marked internal is the business
// itself, whose engagements are internal projects with no agreement to send.

func init() {
	routeSets = append(routeSets, (*App).clientRoutes)
	searchable("Client", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		err := db.QueryRowContext(ctx, `SELECT name, created_at FROM clients WHERE id = ?`, id).Scan(&d.Title, &d.CreatedAt)
		if isNoRows(err) {
			return d, false, nil
		}
		d.Content, d.ClientID = customSearchText(ctx, db, "Client", id), id
		return d, err == nil, err
	})
	addSection("Client", "contacts", func(a *App, ctx context.Context, id int64) (any, error) {
		return a.contactsJSON(ctx, `client_id = ? AND archived_at IS NULL`, id)
	})
}

// ClientStatuses are a client's statuses.
var ClientStatuses = []string{"active", "dormant", "former"}

const clientSummary = "time_zone: where the client is, as a Rails name (\"Pacific Time (US & Canada)\") or IANA (\"America/Los_Angeles\"); blank means ours. Their portal, approval pages and emails show times in it. internal: yourself (the business or person running Runwell), whose engagements are internal projects with no agreement to send."

func (a *App) clientRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/clients", a.listClients, tool("list_clients", "List clients",
		note("Active clients unless status says otherwise: active (the default), dormant, former or all."),
		query("status", "string", "active (the default), dormant, former or all.")))
	a.handle(mux, "GET /api/clients/{id}", a.showClient, tool("show_client", "Show a client",
		note("A client with its contacts, engagements, commitments, notes and documents."),
		next("create_contact", "create_engagement", "add_note")))
	a.handle(mux, "POST /api/clients", a.createClient, tool("create_client", "Add a client",
		note(clientSummary+" contacts: people at the client to add with it, all or none (give portal access with create_contact afterwards, which previews it first)."),
		body("client", `{name (required), status: active|dormant|former, time_zone, internal: boolean, custom_fields: {key: value}, contacts: [{name (required), role, email, phone, can_approve: boolean}]}`, true),
		next("create_contact", "create_engagement")))
	a.handle(mux, "PATCH /api/clients/{id}", a.updateClient, tool("update_client", "Change a client", note(clientSummary),
		body("client", `{name, status: active|dormant|former, time_zone, internal: boolean, custom_fields: {key: value}}`, true)))
	a.handle(mux, "DELETE /api/clients/{id}", a.deleteClient, need("delete_records"), tool("delete_client", "Delete a client",
		note("Removes the client with its contacts, commitments and notes. A client with engagements can’t be deleted: set its status to former instead.")))

	a.handle(mux, "POST /api/clients/{id}/contacts", a.createContact, tool("create_contact", "Add a contact to a client",
		body("contact", `{name (required), role, email, phone, portal_access: boolean, can_approve: boolean}`, true),
		confirm("Giving a contact portal access lets them sign in and see this client’s shared work and documents.")))
	a.handle(mux, "PATCH /api/contacts/{id}", a.updateContact, tool("update_contact", "Change a contact",
		body("contact", `{name, role, email, phone, portal_access: boolean, can_approve: boolean}`, true),
		confirm("Portal access and approval rights decide what this person at the client can see and sign off.")))
	a.handle(mux, "DELETE /api/contacts/{id}", a.archiveContact, need("delete_records"), tool("archive_contact", "Archive a contact",
		note("They drop off the client’s contacts and lose portal access and approval rights. Past approvals and requests stay.")))
}

// clientSorts are the clients table's sortable columns.
var clientSorts = sortPaths{"name": by("name"), "status": by("status")}

// Client is one client.
type Client struct {
	ID        int64
	Name      string
	Status    string
	TimeZone  string
	Internal  bool
	CreatedAt store.Time
	UpdatedAt store.Time
}

const clientColumns = `id, name, status, COALESCE(time_zone, ''), internal, created_at, updated_at`

func scanClient(r interface{ Scan(...any) error }) (*Client, error) {
	var c Client
	err := r.Scan(&c.ID, &c.Name, &c.Status, &c.TimeZone, &c.Internal, &c.CreatedAt, &c.UpdatedAt)
	return &c, err
}

// client is the client with id; notFound when there's none.
func (a *App) client(ctx context.Context, db store.Querier, id int64) (*Client, error) {
	c, err := scanClient(db.QueryRowContext(ctx, `SELECT `+clientColumns+` FROM clients WHERE id = ?`, id))
	if isNoRows(err) {
		return nil, notFound("client")
	}
	return c, err
}

// clientJSON is a client as answers show it (clients/_client.json).
func (a *App) clientJSON(c *Client, custom Obj, tzDefault string) Obj {
	if custom == nil {
		custom = Obj{}
	}
	var tz any
	if c.TimeZone != "" {
		tz = c.TimeZone
	}
	used := c.TimeZone
	if used == "" {
		used = tzDefault
	}
	return a.agentRef("Client", c.ID, "", c.Name, fmt.Sprintf("/clients/%d", c.ID)).
		Set("name", c.Name).Set("status", c.Status).Set("time_zone", tz).Set("internal", c.Internal).
		Set("time_zone_used", used).Set("custom_fields", custom).Set("created_at", c.CreatedAt)
}

func (a *App) listClients(c *Ctx) error {
	ctx := c.R.Context()
	status := c.Query("status")
	if status != "all" && !slices.Contains(ClientStatuses, status) {
		status = "active"
	}
	where, args := "1 = 1", []any{}
	if status != "all" {
		where, args = "status = ?", []any{status}
	}
	rows, err := a.store.QueryContext(ctx, `SELECT `+clientColumns+` FROM clients WHERE `+where+` ORDER BY name COLLATE NOCASE`, args...)
	if err != nil {
		return err
	}
	clients, err := store.ScanAll(rows, func(r *sqlRows) (*Client, error) { return scanClient(r) })
	if err != nil {
		return err
	}
	ids := make([]int64, len(clients))
	for i, cl := range clients {
		ids[i] = cl.ID
	}
	custom, err := a.customValues(ctx, a.store, "Client", ids)
	if err != nil {
		return err
	}
	// What a card or row shows beside the name: contacts and open engagements.
	contacts, err := countBy(ctx, a.store, `SELECT client_id, count(*) FROM contacts WHERE archived_at IS NULL GROUP BY client_id`)
	if err != nil {
		return err
	}
	open, err := countBy(ctx, a.store, `SELECT client_id, count(*) FROM engagements WHERE closed_at IS NULL GROUP BY client_id`)
	if err != nil {
		return err
	}
	counts, err := countByString(ctx, a.store, `SELECT status, count(*) FROM clients GROUP BY status`)
	if err != nil {
		return err
	}
	tz := a.zoneName(ctx)
	list := make([]Obj, len(clients))
	for i, cl := range clients {
		list[i] = a.clientJSON(cl, custom[cl.ID], tz).Set("contacts_count", contacts[cl.ID]).Set("open_engagements_count", open[cl.ID])
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	filters := []Obj{}
	total := 0
	for _, st := range ClientStatuses {
		filters = append(filters, O("value", st, "label", humanize(st), "count", counts[st]))
		total += counts[st]
	}
	filters = append(filters, O("value", "all", "label", "All", "count", total))
	noun := strings.ToLower(s.Term("client", len(clients)))
	if status != "all" {
		noun = status + " " + noun
	}
	c.Summary = pluralize(len(clients), noun)
	return c.listAnswer(O("summary", c.Summary, "status", status, "filters", filters), "clients", list, clientSorts)
}

func (a *App) showClient(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	cl, err := a.client(ctx, a.store, id)
	if err != nil {
		return err
	}
	custom, err := a.customValues(ctx, a.store, "Client", []int64{id})
	if err != nil {
		return err
	}
	o, err := a.withSections(ctx, "Client", id, a.clientJSON(cl, custom[id], a.zoneName(ctx)))
	if err != nil {
		return err
	}
	c.Summary = cl.Name + " (" + cl.Status + ")"
	c.Crumbs = []Crumb{
		step("Add a contact", "create_contact", "id", id),
		step("Start an engagement", "create_engagement", "id", id),
	}
	return c.JSON(200, O("summary", c.Summary, "client", o))
}

// clientParams reads and checks a client's fields; partial for an update.
func (a *App) clientParams(ctx context.Context, body map[string]any, partial bool, selfID int64) (map[string]any, error) {
	inv := apierr.Invalid{}
	out := map[string]any{}
	if has(body, "name") || !partial {
		name := strings.TrimSpace(str(body["name"]))
		if name == "" {
			inv.Add("name", "can't be blank")
		} else if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM clients WHERE name = ? AND id != ?`, name, selfID); err != nil {
			return nil, err
		} else if n > 0 {
			inv.Add("name", "has already been taken")
		}
		out["name"] = name
	}
	if has(body, "status") || !partial {
		st := str(body["status"])
		if st == "" && !partial {
			st = "active"
		}
		if !slices.Contains(ClientStatuses, st) {
			inv.Add("status", "is not included in the list")
		}
		out["status"] = st
	}
	if has(body, "time_zone") {
		tz, ok := normalizeZone(str(body["time_zone"]))
		if !ok {
			inv.Add("time_zone", "isn’t a time zone we know")
		}
		if tz == "" {
			out["time_zone"] = nil
		} else {
			out["time_zone"] = tz
		}
	}
	if has(body, "internal") {
		out["internal"] = boolOf(body["internal"])
	}
	return out, inv.Err()
}

func (a *App) createClient(c *Ctx) error {
	body, err := c.Wrapped("client")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	fields, err := a.clientParams(ctx, body, false, 0)
	contacts, cerr := a.newClientContacts(ctx, c, body["contacts"])
	if err := joinInvalid(err, cerr); err != nil {
		return err
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		id, err = t.Insert(`INSERT INTO clients (name, status, time_zone, internal, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			fields["name"], fields["status"], fields["time_zone"], boolOf(fields["internal"]), now, now)
		if err != nil {
			return err
		}
		for _, ct := range contacts {
			cid, err := t.Insert(`INSERT INTO contacts (client_id, name, role, email, phone, portal_access, can_approve, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				id, ct["name"], ct["role"], ct["email"], ct["phone"], boolOf(ct["portal_access"]), boolOf(ct["can_approve"]), now, now)
			if err != nil {
				return err
			}
			t.Reindex("Contact", cid)
		}
		if custom, ok := body["custom_fields"].(map[string]any); ok {
			if err := t.setCustomFields("Client", id, custom); err != nil {
				return err
			}
		}
		t.Reindex("Client", id)
		return t.Event("Client", id, "client.created", nil)
	})
	if err != nil {
		return err
	}
	return a.showCreated(c, id)
}

// newClientContacts reads the contacts given with a new client (the new
// client form keeps them until it's saved), each checked as create_contact
// checks one, and against each other. Their problems are keyed by place:
// "contacts.1.email". Portal access is a person's call in the app: an agent
// gives it with create_contact, which previews it first.
func (a *App) newClientContacts(ctx context.Context, c *Ctx, v any) ([]map[string]any, error) {
	list, _ := v.([]any)
	inv := apierr.Invalid{}
	var out []map[string]any
	seen := map[string]int{}
	for i, raw := range list {
		body, _ := raw.(map[string]any)
		if body == nil {
			inv.Add(fmt.Sprintf("contacts.%d.base", i), "must be an object")
			continue
		}
		if c.Actor.Agent() && boolOf(body["portal_access"]) {
			inv.Add(fmt.Sprintf("contacts.%d.portal_access", i), "is given with create_contact, once the person has seen what it shares")
		}
		fields, err := a.contactParams(ctx, body, false, 0)
		if bad, ok := errors.AsType[*apierr.Error](err); ok {
			for field, msgs := range bad.Errors {
				for _, m := range msgs {
					inv.Add(fmt.Sprintf("contacts.%d.%s", i, field), m)
				}
			}
		} else if err != nil {
			return nil, err
		}
		if email := str(fields["email"]); email != "" {
			if j, dup := seen[email]; dup {
				inv.Add(fmt.Sprintf("contacts.%d.email", i), fmt.Sprintf("is the same as contact %d's", j+1))
			}
			seen[email] = i
		}
		out = append(out, fields)
	}
	return out, inv.Err()
}

// joinInvalid is one 422 holding every field problem in errs (the rest
// returned as they are).
func joinInvalid(errs ...error) error {
	all := apierr.Invalid{}
	for _, err := range errs {
		if err == nil {
			continue
		}
		bad, ok := errors.AsType[*apierr.Error](err)
		if !ok || bad.Errors == nil {
			return err
		}
		for field, msgs := range bad.Errors {
			for _, m := range msgs {
				all.Add(field, m)
			}
		}
	}
	return all.Err()
}

// showCreated answers a new client as show does, 201.
func (a *App) showCreated(c *Ctx, id int64) error {
	ctx := c.R.Context()
	cl, err := a.client(ctx, a.store, id)
	if err != nil {
		return err
	}
	custom, err := a.customValues(ctx, a.store, "Client", []int64{id})
	if err != nil {
		return err
	}
	c.Summary = "Added " + cl.Name + "."
	c.Crumbs = []Crumb{step("Add a contact", "create_contact", "id", id), step("Start an engagement", "create_engagement", "id", id)}
	return c.JSON(201, O("summary", c.Summary, "client", a.clientJSON(cl, custom[id], a.zoneName(ctx))))
}

func (a *App) updateClient(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("client")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	before, err := a.client(ctx, a.store, id)
	if err != nil {
		return err
	}
	fields, err := a.clientParams(ctx, body, true, id)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.update("clients", id, fields); err != nil {
			return err
		}
		changed := []string{}
		for k := range fields {
			changed = append(changed, k)
		}
		if custom, ok := body["custom_fields"].(map[string]any); ok {
			if err := t.setCustomFields("Client", id, custom); err != nil {
				return err
			}
			for k := range custom {
				changed = append(changed, "custom_fields."+k)
			}
		}
		t.Reindex("Client", id)
		// Heard by plugins (a place to look up again when the address
		// changed) once it commits; not a line in the client's history.
		slices.Sort(changed)
		t.events = append(t.events, Event{SubjectType: "Client", SubjectID: id, Kind: "client.updated", Payload: map[string]any{"changed": changed},
			ActorUserID: t.Actor.UserID(), Actor: t.Actor.AppLabel(), Source: t.Actor.Source(), OccurredAt: store.Now()})
		if st, ok := fields["status"].(string); ok && st != before.Status {
			return t.Event("Client", id, "client.status_changed", map[string]any{"from": before.Status, "to": st})
		}
		return nil
	})
	if err != nil {
		return err
	}
	return a.showClient(c)
}

func (a *App) deleteClient(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	cl, err := a.client(ctx, a.store, id)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteClient(cl) })
	if err != nil {
		return err
	}
	c.Summary = "Deleted " + cl.Name + "."
	return c.JSON(200, O("summary", c.Summary, "deleted", true))
}

// deleteClient removes a client with its contacts, commitments and notes;
// one with engagements is refused (set it to former instead).
func (t *Tx) deleteClient(cl *Client) error {
	n, err := store.Count(t.Ctx, t, `SELECT count(*) FROM engagements WHERE client_id = ?`, cl.ID)
	if err != nil {
		return err
	}
	if n > 0 {
		return &apierr.Error{Status: 422, Code: "has_engagements", Message: cl.Name + " has engagements, so it can’t be deleted: set its status to former instead."}
	}
	// Heard by plugins and webhooks once the delete commits (the client's own
	// history goes with it, so the event stays out of the database).
	t.events = append(t.events, Event{SubjectType: "Client", SubjectID: cl.ID, Kind: "client.deleted", Payload: map[string]any{"name": cl.Name},
		ActorUserID: t.Actor.UserID(), Actor: t.Actor.AppLabel(), Source: t.Actor.Source(), OccurredAt: store.Now()})
	if err := t.cascade("Client", cl.ID); err != nil {
		return err
	}
	ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM contacts WHERE client_id = ?`, cl.ID)
	if err != nil {
		return err
	}
	for _, cid := range ids {
		if err := t.deleteContact(cid); err != nil {
			return err
		}
	}
	if _, err := t.Exec(`UPDATE requests SET client_id = NULL WHERE client_id = ?`, cl.ID); err != nil {
		return err
	}
	_, err = t.Exec(`DELETE FROM clients WHERE id = ?`, cl.ID)
	return err
}

// ---- contacts --------------------------------------------------------------

// contactsJSON are the contacts matching where, by name (contacts/_contact.json).
func (a *App) contactsJSON(ctx context.Context, where string, args ...any) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT id, name, COALESCE(role, ''), COALESCE(email, ''), COALESCE(phone, ''), portal_access, can_approve, archived_at
		FROM contacts WHERE `+where+` ORDER BY name COLLATE NOCASE`, args...)
	if err != nil {
		return nil, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var name, role, email, phone string
		var portal, approve bool
		var archived store.Time
		if err := r.Scan(&id, &name, &role, &email, &phone, &portal, &approve, &archived); err != nil {
			return nil, err
		}
		return a.agentRef("Contact", id, "", name, "").Set("name", name).Set("role", nullable(role)).Set("email", nullable(email)).
			Set("phone", nullable(phone)).Set("portal_access", portal).Set("can_approve", approve).
			Set("can_sign_in", portal && !archived.Valid && email != ""), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

func (a *App) contactParams(ctx context.Context, body map[string]any, partial bool, selfID int64) (map[string]any, error) {
	inv := apierr.Invalid{}
	out := map[string]any{}
	if has(body, "name") || !partial {
		name := strings.TrimSpace(str(body["name"]))
		if name == "" {
			inv.Add("name", "can't be blank")
		}
		out["name"] = name
	}
	for _, k := range []string{"role", "phone"} {
		if has(body, k) {
			out[k] = nullable(strings.TrimSpace(str(body[k])))
		}
	}
	email := ""
	if has(body, "email") {
		email = strings.ToLower(strings.TrimSpace(str(body["email"])))
		switch {
		case email == "":
		case !validEmail(email):
			inv.Add("email", "is invalid")
		default:
			if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM contacts WHERE email = ? AND id != ?`, email, selfID); err != nil {
				return nil, err
			} else if n > 0 {
				inv.Add("email", "has already been taken")
			}
		}
		out["email"] = nullable(email)
	} else if selfID != 0 {
		_ = a.store.QueryRowContext(ctx, `SELECT COALESCE(email, '') FROM contacts WHERE id = ?`, selfID).Scan(&email)
	}
	for _, k := range []string{"portal_access", "can_approve"} {
		if has(body, k) {
			out[k] = boolOf(body[k])
		}
	}
	if out["portal_access"] == true && email == "" {
		inv.Add("email", "is needed for portal access")
	}
	return out, inv.Err()
}

func (a *App) createContact(c *Ctx) error {
	clientID, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("contact")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	if _, err := a.client(ctx, a.store, clientID); err != nil {
		return err
	}
	fields, err := a.contactParams(ctx, body, false, 0)
	if err != nil {
		return err
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		id, err = t.Insert(`INSERT INTO contacts (client_id, name, role, email, phone, portal_access, can_approve, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			clientID, fields["name"], fields["role"], fields["email"], fields["phone"], boolOf(fields["portal_access"]), boolOf(fields["can_approve"]), now, now)
		t.Reindex("Contact", id)
		return err
	})
	if err != nil {
		return err
	}
	list, err := a.contactsJSON(ctx, `id = ?`, id)
	if err != nil {
		return err
	}
	c.Summary = "Added " + str(fields["name"]) + "."
	return c.JSON(201, O("summary", c.Summary, "contact", list[0]))
}

func (a *App) updateContact(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("contact")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM contacts WHERE id = ? AND archived_at IS NULL`, id); err != nil {
		return err
	} else if n == 0 {
		return notFound("contact")
	}
	fields, err := a.contactParams(ctx, body, true, id)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.update("contacts", id, fields); err != nil {
			return err
		}
		t.Reindex("Contact", id)
		// Losing portal access signs them out of the portal at once.
		_, err := t.Exec(`DELETE FROM portal_sessions WHERE contact_id = ? AND contact_id IN (SELECT id FROM contacts WHERE id = ? AND (portal_access = 0 OR email IS NULL))`, id, id)
		return err
	})
	if err != nil {
		return err
	}
	list, err := a.contactsJSON(ctx, `id = ?`, id)
	if err != nil {
		return err
	}
	c.Summary = "Updated " + str(list[0].Get("name")) + "."
	return c.JSON(200, O("summary", c.Summary, "contact", list[0]))
}

func (a *App) archiveContact(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	var name string
	if err := a.store.QueryRowContext(ctx, `SELECT name FROM contacts WHERE id = ? AND archived_at IS NULL`, id).Scan(&name); err != nil {
		return notFound("contact")
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if _, err := t.Exec(`UPDATE contacts SET archived_at = ?, updated_at = ? WHERE id = ?`, store.Now().DB(), store.Now().DB(), id); err != nil {
			return err
		}
		_, err := t.Exec(`DELETE FROM portal_sessions WHERE contact_id = ?`, id)
		return err
	})
	if err != nil {
		return err
	}
	c.Summary = name + " archived."
	return c.JSON(200, O("summary", c.Summary, "archived", true))
}

// deleteContact removes a contact for good (its client is being deleted):
// its sessions, tokens and approval links go; approvals and requests keep
// their record without it.
func (t *Tx) deleteContact(id int64) error {
	for _, q := range []string{
		`DELETE FROM portal_sessions WHERE contact_id = ?`,
		`DELETE FROM access_tokens WHERE contact_id = ?`,
		`DELETE FROM approval_links WHERE contact_id = ?`,
		`UPDATE approvals SET contact_id = NULL WHERE contact_id = ?`,
		`UPDATE requests SET contact_id = NULL WHERE contact_id = ?`,
		`UPDATE commitments SET contact_id = NULL WHERE contact_id = ?`,
		`DELETE FROM contacts WHERE id = ?`,
	} {
		if _, err := t.Exec(q, id); err != nil {
			return err
		}
	}
	t.Reindex("Contact", id)
	return nil
}

func init() {
	searchable("Contact", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		var role, email, phone string
		var archived store.Time
		err := db.QueryRowContext(ctx, `SELECT name, COALESCE(role, ''), COALESCE(email, ''), COALESCE(phone, ''), client_id, created_at, archived_at FROM contacts WHERE id = ?`, id).
			Scan(&d.Title, &role, &email, &phone, &d.ClientID, &d.CreatedAt, &archived)
		if isNoRows(err) {
			return d, false, nil
		}
		d.Content = strings.Join(slices.DeleteFunc([]string{role, email, phone}, func(s string) bool { return s == "" }), " ")
		return d, err == nil, err
	})
}
