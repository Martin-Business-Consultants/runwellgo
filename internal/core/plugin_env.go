package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// pluginEnv is what plugins reach of Runwell (plugin.Env): the core's own
// small state, the install, mail, records to read, and the few writes a
// plugin may make, through the core's rules.
type pluginEnv struct{ a *App }

// coreKV is where the core keeps its own small state in plugin_kv (which
// default plugins it has installed, when tasks last ran): a key no plugin
// can have.
const coreKV = "_core"

// Setting is the core's state under key, as JSON.
func (e pluginEnv) Setting(ctx context.Context, key string) (map[string]any, error) {
	var raw []byte
	err := e.a.store.QueryRowContext(ctx, `SELECT value FROM plugin_kv WHERE plugin_key = ? AND key = ?`, coreKV, key).Scan(&raw)
	if isNoRows(err) {
		return map[string]any{}, nil
	} else if err != nil {
		return nil, err
	}
	out := map[string]any{}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &out); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// ReplaceSetting writes data as key's, whole.
func (e pluginEnv) ReplaceSetting(ctx context.Context, key string, data map[string]any) error {
	raw, err := json.Marshal(data)
	if err != nil {
		return err
	}
	_, err = e.a.store.ExecContext(ctx, `INSERT INTO plugin_kv (plugin_key, key, value, updated_at) VALUES (?, ?, ?, ?)
		ON CONFLICT (plugin_key, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, coreKV, key, raw, store.Now().DB())
	return err
}

// Site is the install plugins run on.
func (e pluginEnv) Site(ctx context.Context) (plugin.Site, error) {
	s, err := e.a.settings(ctx)
	if err != nil {
		return plugin.Site{}, err
	}
	_, email := e.a.mailSender(s)
	terms := map[string]map[string]string{}
	for _, k := range TermKeys {
		terms[k] = map[string]string{"one": s.Term(k, 1), "other": s.Term(k, 2)}
	}
	return plugin.Site{Name: e.a.BrandName(ctx), URL: e.a.BaseURL(), Locale: "en", TimeZone: e.a.zoneName(ctx), Location: e.a.zone(ctx), ContactEmail: email, Terms: terms, MaxUpload: MaxUpload()}, nil
}

// SendMail sends m through the install's mail.
func (e pluginEnv) SendMail(ctx context.Context, m mail.Message) error { return e.a.sendMail(ctx, m) }

// Audit records what someone did to a plugin in the audit log: an event on
// the settings, {plugin: key, …} (settings.plugin {plugin, on} is the one
// the Rails core recorded for the switch).
func (e pluginEnv) Audit(ctx context.Context, actor plugin.Actor, action string, _ int64, label string, meta map[string]any) {
	who, _ := actor.(*Actor)
	payload := map[string]any{"plugin": label}
	for k, v := range meta {
		payload[k] = v
	}
	err := e.a.write(ctx, who, func(t *Tx) error {
		s, err := e.a.settings(ctx)
		if err != nil {
			return err
		}
		return t.Event("Setting", s.ID, action, payload)
	})
	if err != nil {
		e.a.log.Warn("plugin audit", "action", action, "err", err)
	}
}

// ---- records:read ----------------------------------------------------------

// Records answers q with plain SQL, each record shaped as the agent tools'
// partials shape it (clients/_client, todos/_todo, …): agent_ref first.
func (e pluginEnv) Records(ctx context.Context, q plugin.Query) ([]map[string]any, error) {
	a := e.a
	var where []string
	var args []any
	filter := func(cond string, v ...any) {
		where = append(where, cond)
		args = append(args, v...)
	}
	byID := func(col string) {
		if q.ID != 0 {
			filter(col+" = ?", q.ID)
		}
		if len(q.IDs) > 0 {
			filter(col+" IN ("+store.Placeholders(len(q.IDs))+")", idArgs(q.IDs)...)
		}
	}
	sqlWhere := func() string {
		if len(where) == 0 {
			return "1 = 1"
		}
		return strings.Join(where, " AND ")
	}
	limit := fmt.Sprintf(" LIMIT %d", q.Limit)
	var out []map[string]any
	scan := func(query string, fn func(r *sql.Rows) (map[string]any, error)) error {
		rows, err := a.store.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		out, err = store.ScanAll(rows, fn)
		return err
	}
	ref := func(typ string, id int64, ref, display, path string) map[string]any {
		o := map[string]any{"type": typ, "id": id, "record": fmt.Sprintf("%s:%d", typ, id), "display_name": display}
		if ref != "" {
			o["ref"] = ref
		}
		if path != "" {
			o["url"] = a.BaseURL() + path
		}
		return o
	}
	clientRef := func(id int64, name string) map[string]any {
		return ref("Client", id, "", name, fmt.Sprintf("/clients/%d", id))
	}
	engagementRef := func(id int64, r, title string) map[string]any {
		if id == 0 {
			return nil
		}
		return ref("Engagement", id, r, r+" "+title, "/engagements/"+r)
	}
	user := func(id sql.NullInt64, name sql.NullString) any {
		if !id.Valid {
			return nil
		}
		return map[string]any{"id": id.Int64, "name": name.String}
	}
	today := a.today(ctx)

	var err error
	switch q.Type {
	case "Client":
		byID("id")
		if q.ClientID != 0 {
			filter("id = ?", q.ClientID)
		}
		if q.Status != "" {
			filter("status = ?", q.Status)
		}
		err = scan(`SELECT id, name, status, COALESCE(time_zone, ''), internal FROM clients WHERE `+sqlWhere()+` ORDER BY name COLLATE NOCASE`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id int64
			var name, status, tz string
			var internal bool
			err := r.Scan(&id, &name, &status, &tz, &internal)
			o := clientRef(id, name)
			o["name"], o["status"], o["time_zone"], o["internal"] = name, status, nullable(tz), internal
			return o, err
		})
		if err == nil && len(out) > 0 {
			// Their custom fields, by key, as clients/_client has them.
			ids := make([]int64, len(out))
			for i, o := range out {
				ids[i] = o["id"].(int64)
			}
			var custom map[int64]Obj
			if custom, err = a.customValues(ctx, a.store, "Client", ids); err == nil {
				for _, o := range out {
					fields := map[string]any{}
					for _, f := range custom[o["id"].(int64)] {
						fields[f.K] = f.V
					}
					o["custom_fields"] = fields
				}
			}
		}
	case "CustomField":
		// The fields an install records (Settings › Fields), active ones in
		// order; Status narrows them to a model's (Client, Engagement, Todo).
		byID("id")
		filter("archived_at IS NULL")
		if q.Status != "" {
			filter("model_type = ?", q.Status)
		}
		err = scan(`SELECT `+customFieldColumns+` FROM custom_fields WHERE `+sqlWhere()+` ORDER BY model_type, position`+limit, func(r *sql.Rows) (map[string]any, error) {
			f, err := scanCustomField(r)
			if err != nil {
				return nil, err
			}
			o := ref("CustomField", f.ID, "", f.Label, "/settings/fields")
			o["model_type"], o["label"], o["key"], o["kind"], o["choices"], o["client_visible"] = f.ModelType, f.Label, f.Key, f.Kind, f.Choices, f.ClientVisible
			return o, nil
		})
	case "Contact":
		filter("ct.archived_at IS NULL")
		byID("ct.id")
		if q.ClientID != 0 {
			filter("ct.client_id = ?", q.ClientID)
		}
		err = scan(`SELECT ct.id, ct.client_id, c.name, ct.name, COALESCE(ct.role, ''), COALESCE(ct.email, ''), COALESCE(ct.phone, ''), ct.portal_access, ct.can_approve
			FROM contacts ct JOIN clients c ON c.id = ct.client_id WHERE `+sqlWhere()+` ORDER BY ct.name COLLATE NOCASE`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id, clientID int64
			var client, name, role, email, phone string
			var portal, approve bool
			err := r.Scan(&id, &clientID, &client, &name, &role, &email, &phone, &portal, &approve)
			o := ref("Contact", id, "", name, fmt.Sprintf("/clients/%d", clientID))
			o["name"], o["role"], o["email"], o["phone"] = name, nullable(role), nullable(email), nullable(phone)
			o["portal_access"], o["can_approve"], o["can_sign_in"] = portal, approve, portal && email != ""
			o["client"] = clientRef(clientID, client)
			return o, err
		})
	case "Engagement":
		byID("e.id")
		if q.Ref != "" {
			filter("e.ref = ?", q.Ref)
		}
		if q.ClientID != 0 {
			filter("e.client_id = ?", q.ClientID)
		}
		if q.EngagementID != 0 {
			filter("e.id = ?", q.EngagementID)
		}
		if q.Open {
			filter("e.closed_at IS NULL")
		}
		err = scan(`SELECT e.id, e.ref, e.title, e.label, e.shape, e.closed_at, COALESCE(e.close_outcome, ''), c.id, c.name, c.internal
			FROM engagements e JOIN clients c ON c.id = e.client_id WHERE `+sqlWhere()+` ORDER BY e.created_at DESC`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id, clientID int64
			var eref, title, label, shape, outcome, client string
			var closed store.Time
			var internal bool
			err := r.Scan(&id, &eref, &title, &label, &shape, &closed, &outcome, &clientID, &client, &internal)
			o := engagementRef(id, eref, title)
			o["title"], o["label"], o["shape"] = title, label, shape
			o["closed"], o["internal"], o["client"] = closed.Valid, internal, clientRef(clientID, client)
			if closed.Valid {
				o["close_outcome"], o["closed_at"] = nullable(outcome), closed
			}
			return o, err
		})
	case "AgreementVersion":
		byID("av.id")
		if q.EngagementID != 0 {
			filter("av.engagement_id = ?", q.EngagementID)
		}
		if q.ClientID != 0 {
			filter("e.client_id = ?", q.ClientID)
		}
		err = scan(`SELECT av.id, av.number, av.kind, COALESCE(av.summary, ''), COALESCE(av.reason, ''), COALESCE(av.cadence, ''), av.amount_cents, av.sent_at, av.superseded_by_id,
			(SELECT decision FROM approvals ap WHERE ap.agreement_version_id = av.id ORDER BY ap.decided_at DESC LIMIT 1),
			(SELECT COALESCE(sum(price_cents), 0) FROM scope_items si WHERE si.agreement_version_id = av.id),
			e.id, e.ref, e.title
			FROM agreement_versions av JOIN engagements e ON e.id = av.engagement_id WHERE `+sqlWhere()+` ORDER BY av.engagement_id, av.number DESC`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id, number, amount, items, engID int64
			var kind, summary, reason, cadence, eref, title string
			var sent store.Time
			var superseded sql.NullInt64
			var decision sql.NullString
			err := r.Scan(&id, &number, &kind, &summary, &reason, &cadence, &amount, &sent, &superseded, &decision, &items, &engID, &eref, &title)
			label := "Agreement"
			if kind != "initial" {
				label = fmt.Sprintf("%s #%d", humanize(kind), number)
			}
			o := ref("AgreementVersion", id, "", eref+" "+label, "/engagements/"+eref)
			o["number"], o["kind"], o["summary"], o["reason"], o["cadence"], o["label"] = number, kind, nullable(summary), nullable(reason), nullable(cadence), label
			draft := !sent.Valid
			if draft {
				amount = items
			}
			o["draft"], o["superseded"], o["amount_cents"], o["amount"] = draft, superseded.Valid, amount, pluginMoney(amount)
			o["sent_at"], o["decision"] = sent, nil
			if decision.Valid {
				o["decision"] = decision.String
			}
			o["engagement"] = engagementRef(engID, eref, title)
			return o, err
		})
	case "ScopeItem":
		byID("si.id")
		if q.AgreementVersionID != 0 {
			filter("si.agreement_version_id = ?", q.AgreementVersionID)
		}
		if q.EngagementID != 0 {
			filter("av.engagement_id = ?", q.EngagementID)
		}
		if q.ClientID != 0 {
			filter("e.client_id = ?", q.ClientID)
		}
		err = scan(`SELECT si.id, si.description, COALESCE(si.internal_estimate, ''), si.price_cents, si.agreement_version_id, e.id, e.ref, e.title
			FROM scope_items si JOIN agreement_versions av ON av.id = si.agreement_version_id JOIN engagements e ON e.id = av.engagement_id
			WHERE `+sqlWhere()+` ORDER BY si.agreement_version_id, si.position`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id, price, avID, engID int64
			var desc, estimate, eref, title string
			err := r.Scan(&id, &desc, &estimate, &price, &avID, &engID, &eref, &title)
			o := ref("ScopeItem", id, "", desc, fmt.Sprintf("/scope_items/%d", id))
			o["description"], o["internal_estimate"], o["price_cents"], o["price"] = desc, nullable(estimate), price, pluginMoney(price)
			o["agreement_version_id"], o["engagement"] = avID, engagementRef(engID, eref, title)
			return o, err
		})
	case "Todo":
		byID("t.id")
		if q.EngagementID != 0 {
			filter("t.engagement_id = ?", q.EngagementID)
		}
		if q.ClientID != 0 {
			filter("e.client_id = ?", q.ClientID)
		}
		if q.OwnerID != 0 {
			filter("(t.owner_id = ? OR t.id IN (SELECT todo_id FROM assignments WHERE user_id = ?))", q.OwnerID, q.OwnerID)
		}
		if q.Status != "" {
			filter("t.status = ?", q.Status)
		}
		if q.Open {
			filter("t.status <> 'done'")
		}
		err = scan(`SELECT t.id, t.title, t.status, COALESCE(t.due_on, ''), t.client_visible, t.owner_id, u.name, t.scope_item_id, COALESCE(si.description, ''), e.id, e.ref, e.title, c.id, c.name,
				COALESCE(t.description, ''), t.created_at, t.completed_at,
				(SELECT json_group_array(json_object('id', ou.id, 'name', ou.name)) FROM assignments a JOIN users ou ON ou.id = a.user_id WHERE a.todo_id = t.id AND a.user_id IS NOT t.owner_id)
			FROM todos t JOIN engagements e ON e.id = t.engagement_id JOIN clients c ON c.id = e.client_id
			LEFT JOIN users u ON u.id = t.owner_id LEFT JOIN scope_items si ON si.id = t.scope_item_id
			WHERE `+sqlWhere()+` ORDER BY t.due_on IS NULL, t.due_on, t.id`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id, engID, clientID int64
			var title, status, due, siDesc, eref, etitle, client string
			var visible bool
			var ownerID, siID sql.NullInt64
			var owner sql.NullString
			var description string
			var created, completed store.Time
			var others store.JSON
			err := r.Scan(&id, &title, &status, &due, &visible, &ownerID, &owner, &siID, &siDesc, &engID, &eref, &etitle, &clientID, &client,
				&description, &created, &completed, &others)
			o := ref("Todo", id, "", title, fmt.Sprintf("/work/%d", id))
			o["title"], o["status"], o["due_on"], o["client_visible"] = title, status, nullable(due), visible
			o["overdue"] = due != "" && due < today && status != "done"
			o["owner"], o["engagement"], o["client"] = user(ownerID, owner), engagementRef(engID, eref, etitle), clientRef(clientID, client)
			o["description"], o["created_at"], o["completed_at"] = nullable(description), created, completed
			o["other_owners"] = others.List()
			o["scope_item"] = nil
			if siID.Valid {
				o["scope_item"] = ref("ScopeItem", siID.Int64, "", siDesc, fmt.Sprintf("/scope_items/%d", siID.Int64))
			}
			return o, err
		})
	case "Commitment":
		byID("cm.id")
		if q.ClientID != 0 {
			filter("cm.client_id = ?", q.ClientID)
		}
		if q.EngagementID != 0 {
			filter("cm.engagement_id = ?", q.EngagementID)
		}
		if q.OwnerID != 0 {
			filter("cm.user_id = ?", q.OwnerID)
		}
		if q.Open {
			filter("cm.resolution IS NULL")
		}
		err = scan(`SELECT cm.id, cm.description, COALESCE(cm.due_on, ''), cm.owner_kind, cm.user_id, u.name, cm.contact_id, ct.name, cm.source, cm.resolution, cm.resolution_note,
			c.id, c.name, e.id, e.ref, e.title
			FROM commitments cm JOIN clients c ON c.id = cm.client_id LEFT JOIN engagements e ON e.id = cm.engagement_id
			LEFT JOIN users u ON u.id = cm.user_id LEFT JOIN contacts ct ON ct.id = cm.contact_id
			WHERE `+sqlWhere()+` ORDER BY cm.due_on, cm.id`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id, clientID int64
			var desc, due, kind, source, client string
			var userID, contactID, engID sql.NullInt64
			var userName, contactName, resolution, note, eref, etitle sql.NullString
			err := r.Scan(&id, &desc, &due, &kind, &userID, &userName, &contactID, &contactName, &source, &resolution, &note, &clientID, &client, &engID, &eref, &etitle)
			path := fmt.Sprintf("/clients/%d", clientID)
			if engID.Valid {
				path = "/engagements/" + eref.String
			}
			o := ref("Commitment", id, "", desc, path)
			owner, ownerName := "us", "Us"
			switch {
			case kind == "us" && userID.Valid:
				owner, ownerName = fmt.Sprintf("user:%d", userID.Int64), userName.String
			case kind != "us" && contactID.Valid:
				owner, ownerName = fmt.Sprintf("contact:%d", contactID.Int64), contactName.String
			case kind != "us":
				owner, ownerName = "client", client
			}
			o["description"], o["due_on"], o["owner"], o["source"] = desc, due, owner, source
			o["resolution"], o["resolution_note"] = nullable(resolution.String), nullable(note.String)
			o["owner_name"], o["open"], o["overdue"] = ownerName, !resolution.Valid, !resolution.Valid && due < today
			o["client"], o["engagement"] = clientRef(clientID, client), engagementRef(engID.Int64, eref.String, etitle.String)
			return o, err
		})
	case "User":
		// People on staff (agents aren't), with whether they can act now.
		byID("u.id")
		filter("u.agent_owner_id IS NULL")
		if q.Open {
			filter("u.deactivated_at IS NULL")
		}
		err = scan(`SELECT u.id, u.name, u.email_address, u.role, u.deactivated_at IS NULL FROM users u WHERE `+sqlWhere()+` ORDER BY u.name COLLATE NOCASE`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id int64
			var name, email, role string
			var active bool
			err := r.Scan(&id, &name, &email, &role, &active)
			o := ref("User", id, "", strOr(name, email), "")
			o["name"], o["email_address"], o["role"], o["active"] = strOr(name, email), email, role, active
			return o, err
		})
	case "Request":
		byID("rq.id")
		if q.ClientID != 0 {
			filter("rq.client_id = ?", q.ClientID)
		}
		if q.Status != "" {
			filter("rq.status = ?", q.Status)
		}
		if q.Open {
			filter("rq.status = 'open'")
		}
		err = scan(`SELECT rq.id, rq.subject, rq.source, rq.status, rq.received_at, COALESCE(rq.dismissed_reason, ''),
			COALESCE(ct.name, rq.sender_name, rq.sender_email, ''), c.id, COALESCE(c.name, ''), COALESCE(rq.promoted_type, ''), COALESCE(rq.promoted_id, 0)
			FROM requests rq LEFT JOIN clients c ON c.id = rq.client_id LEFT JOIN contacts ct ON ct.id = rq.contact_id
			WHERE `+sqlWhere()+` ORDER BY rq.received_at DESC`+limit, func(r *sql.Rows) (map[string]any, error) {
			var id, promotedID int64
			var subject, source, status, reason, from, client, promotedType string
			var received store.Time
			var clientID sql.NullInt64
			err := r.Scan(&id, &subject, &source, &status, &received, &reason, &from, &clientID, &client, &promotedType, &promotedID)
			o := ref("Request", id, "", subject, fmt.Sprintf("/requests/%d", id))
			o["subject"], o["source"], o["status"], o["received_at"], o["dismissed_reason"] = subject, source, status, received, nullable(reason)
			o["from"], o["client"], o["promoted_to"] = from, nil, nil
			if clientID.Valid {
				o["client"] = clientRef(clientID.Int64, client)
			}
			if promotedType != "" {
				o["promoted_to"] = map[string]any{"type": promotedType, "id": promotedID, "record": fmt.Sprintf("%s:%d", promotedType, promotedID)}
			}
			return o, err
		})
	}
	if err != nil {
		return nil, err
	}
	if out == nil {
		out = []map[string]any{}
	}
	return out, nil
}

// pluginMoney is cents as plugins' records show money: $1,234.50.
func pluginMoney(cents int64) string {
	neg := cents < 0
	if neg {
		cents = -cents
	}
	whole := fmt.Sprint(cents / 100)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	s := fmt.Sprintf("$%s.%02d", whole, cents%100)
	if neg {
		s = "-" + s
	}
	return s
}

// ---- records:write -----------------------------------------------------------

// noteSubjects are what a note can be on (NotesController::SUBJECTS).
var noteSubjects = map[string]string{"Client": "clients", "Engagement": "engagements", "ScopeItem": "scope_items", "Todo": "todos", "Request": "requests"}

// Write runs one of the write verbs a plugin may use, as actor (the caller
// of its route, or the system for an event or a task), in one write with
// the event the core records for it. The plugin is named in the event's
// payload.
func (e pluginEnv) Write(ctx context.Context, actor plugin.Actor, key string, w any) (map[string]any, error) {
	a := e.a
	who, _ := actor.(*Actor)
	var out map[string]any
	err := a.write(ctx, who, func(t *Tx) error {
		var err error
		switch w := w.(type) {
		case plugin.Note:
			out, err = e.addNote(t, key, w)
		case plugin.Work:
			out, err = e.addWork(t, key, w)
		case plugin.Commitment:
			out, err = e.addCommitment(t, key, w)
		case plugin.Event:
			out, err = e.recordEvent(t, key, w)
		case plugin.WorkChange:
			out, err = e.changeWork(t, key, w)
		default:
			err = fmt.Errorf("plugin %s: no write %T", key, w)
		}
		return err
	})
	return out, err
}

// subject finds a record named "Type:id" of one of types, in the write.
func pluginSubject(t *Tx, record string, tables map[string]string) (string, int64, error) {
	typ, id, err := plugin.ParseRecord(record)
	if err != nil {
		inv := apierr.Invalid{}
		inv.Add("record", "Choose where this goes.")
		return "", 0, inv.Err()
	}
	table := tables[typ]
	if table == "" {
		inv := apierr.Invalid{}
		inv.Add("record", "can't be a "+typ)
		return "", 0, inv.Err()
	}
	var found int64
	if err := t.QueryRow(`SELECT id FROM `+table+` WHERE id = ?`, id).Scan(&found); isNoRows(err) {
		return "", 0, notFound(strings.ToLower(humanize(typ)))
	} else if err != nil {
		return "", 0, err
	}
	return typ, id, nil
}

func (e pluginEnv) addNote(t *Tx, key string, n plugin.Note) (map[string]any, error) {
	typ, id, err := pluginSubject(t, n.Record, noteSubjects)
	if err != nil {
		return nil, err
	}
	if n.Kind == "" {
		n.Kind = "internal"
	}
	body := richTextHTML(n.Body)
	inv := apierr.Invalid{}
	if plainText(body) == "" {
		inv.Add("body", "can't be blank")
	}
	if !slices.Contains(plugin.NoteKinds, n.Kind) {
		inv.Add("kind", "is not included in the list")
	}
	if len(inv) > 0 {
		return nil, inv.Err()
	}
	now := store.Now()
	var author any
	if uid := t.Actor.UserID(); uid != 0 {
		author = uid
	}
	noteID, err := t.Insert(`INSERT INTO notes (subject_type, subject_id, body, kind, occurred_at, author_id, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		typ, id, body, n.Kind, now.DB(), author, t.Actor.Source(), now.DB(), now.DB())
	if err != nil {
		return nil, err
	}
	if err := t.Event(typ, id, "note.added", map[string]any{"kind": n.Kind, "plugin": key}); err != nil {
		return nil, err
	}
	t.Reindex("Note", noteID)
	return map[string]any{"type": "Note", "id": noteID, "record": fmt.Sprintf("Note:%d", noteID), "kind": n.Kind, "subject": fmt.Sprintf("%s:%d", typ, id), "body": body}, nil
}

// todoStatuses are a piece of work's statuses (Todo::STATUSES).
var todoStatuses = []string{"planned", "in_progress", "in_review", "blocked", "done"}

func (e pluginEnv) addWork(t *Tx, key string, w plugin.Work) (map[string]any, error) {
	s, err := e.a.settings(t.Ctx)
	if err != nil {
		return nil, err
	}
	var engID int64
	if err := t.QueryRow(`SELECT id FROM engagements WHERE ref = ?`, strings.TrimSpace(w.Ref)).Scan(&engID); isNoRows(err) {
		inv := apierr.Invalid{}
		inv.Add("engagement", "Choose the "+strings.ToLower(s.Term("engagement", 1))+" it’s for.")
		return nil, inv.Err()
	} else if err != nil {
		return nil, err
	}
	if w.Status == "" {
		w.Status = "planned"
	}
	inv := apierr.Invalid{}
	w.Title = strings.TrimSpace(w.Title)
	if w.Title == "" {
		inv.Add("title", "can't be blank")
	}
	if !slices.Contains(todoStatuses, w.Status) {
		inv.Add("status", "is not included in the list")
	}
	if w.DueOn != "" {
		if _, err := time.Parse(time.DateOnly, w.DueOn); err != nil {
			inv.Add("due_on", "is not a date")
		}
	}
	if w.OwnerID != 0 {
		var n int
		if err := t.QueryRow(`SELECT count(*) FROM users WHERE id = ? AND deactivated_at IS NULL AND agent_owner_id IS NULL`, w.OwnerID).Scan(&n); err != nil {
			return nil, err
		} else if n == 0 {
			inv.Add("owner", "must exist")
		}
	}
	if len(inv) > 0 {
		return nil, inv.Err()
	}
	visible := w.ClientVisible == nil || *w.ClientVisible
	var position, board int64
	if err := t.QueryRow(`SELECT COALESCE(max(position), 0) + 1 FROM todos WHERE engagement_id = ? AND status = ?`, engID, w.Status).Scan(&position); err != nil {
		return nil, err
	}
	if err := t.QueryRow(`SELECT COALESCE(max(board_position), 0) + 1 FROM todos WHERE status = ?`, w.Status).Scan(&board); err != nil {
		return nil, err
	}
	now := store.Now()
	var completed any
	if w.Status == "done" {
		completed = now.DB()
	}
	var owner, creator any
	if w.OwnerID != 0 {
		owner = w.OwnerID
	}
	if uid := t.Actor.UserID(); uid != 0 {
		creator = uid
	}
	id, err := t.Insert(`INSERT INTO todos (engagement_id, title, description, status, owner_id, due_on, position, client_visible, completed_at, created_by_id, created_at, updated_at, board_position)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		engID, w.Title, nullable(richTextHTML(w.Description)), w.Status, owner, nullable(w.DueOn), position, visible, completed, creator, now.DB(), now.DB(), board)
	if err != nil {
		return nil, err
	}
	if w.OwnerID != 0 {
		// Everyone on a piece of work has an assignment, its lead too.
		if _, err := t.Exec(`INSERT INTO assignments (todo_id, user_id, assigner_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, id, w.OwnerID, creator, now.DB(), now.DB()); err != nil {
			return nil, err
		}
	}
	if err := t.Event("Todo", id, "todo.created", map[string]any{"plugin": key}); err != nil {
		return nil, err
	}
	t.Reindex("Todo", id)
	return map[string]any{"type": "Todo", "id": id, "record": fmt.Sprintf("Todo:%d", id), "title": w.Title, "status": w.Status, "url": e.a.BaseURL() + fmt.Sprintf("/work/%d", id)}, nil
}

func (e pluginEnv) addCommitment(t *Tx, key string, c plugin.Commitment) (map[string]any, error) {
	typ, id, err := pluginSubject(t, c.Record, map[string]string{"Client": "clients", "Engagement": "engagements"})
	if err != nil {
		return nil, err
	}
	clientID, engagement := id, any(nil)
	if typ == "Engagement" {
		engagement = id
		if err := t.QueryRow(`SELECT client_id FROM engagements WHERE id = ?`, id).Scan(&clientID); err != nil {
			return nil, err
		}
	}
	if c.OwnerKind == "" {
		c.OwnerKind = "us"
	}
	inv := apierr.Invalid{}
	c.Description = strings.TrimSpace(c.Description)
	if c.Description == "" {
		inv.Add("description", "can't be blank")
	}
	if c.DueOn == "" {
		inv.Add("due_on", "can't be blank")
	} else if _, err := time.Parse(time.DateOnly, c.DueOn); err != nil {
		inv.Add("due_on", "is not a date")
	}
	if c.OwnerKind != "us" && c.OwnerKind != "client" {
		inv.Add("owner_kind", "is not included in the list")
	}
	// The owner matches the kind: one of us for ours, one of the client's
	// contacts for theirs (Commitment#owner_matches_kind).
	var user, contact any
	if c.OwnerKind == "us" && c.UserID != 0 {
		user = c.UserID
	}
	if c.OwnerKind == "client" && c.ContactID != 0 {
		var n int
		if err := t.QueryRow(`SELECT count(*) FROM contacts WHERE id = ? AND client_id = ?`, c.ContactID, clientID).Scan(&n); err != nil {
			return nil, err
		} else if n == 0 {
			inv.Add("contact", "must be one of the client’s contacts")
		}
		contact = c.ContactID
	}
	if len(inv) > 0 {
		return nil, inv.Err()
	}
	now := store.Now()
	cid, err := t.Insert(`INSERT INTO commitments (client_id, engagement_id, description, owner_kind, user_id, contact_id, due_on, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		clientID, engagement, c.Description, c.OwnerKind, user, contact, c.DueOn, t.Actor.Source(), now.DB(), now.DB())
	if err != nil {
		return nil, err
	}
	if err := t.Event("Commitment", cid, "commitment.added", map[string]any{"plugin": key}); err != nil {
		return nil, err
	}
	t.Reindex("Commitment", cid)
	return map[string]any{"type": "Commitment", "id": cid, "record": fmt.Sprintf("Commitment:%d", cid), "description": c.Description, "due_on": c.DueOn, "owner_kind": c.OwnerKind}, nil
}

// eventSubjects are the records a plugin can record its own events on.
var eventSubjects = map[string]string{"Client": "clients", "Contact": "contacts", "Engagement": "engagements", "AgreementVersion": "agreement_versions",
	"ScopeItem": "scope_items", "Todo": "todos", "Commitment": "commitments", "Request": "requests"}

func (e pluginEnv) recordEvent(t *Tx, key string, ev plugin.Event) (map[string]any, error) {
	typ, id, err := pluginSubject(t, ev.Record, eventSubjects)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	for k, v := range ev.Payload {
		payload[k] = v
	}
	payload["plugin"] = key
	if err := t.Event(typ, id, ev.Kind, payload); err != nil {
		return nil, err
	}
	return map[string]any{"kind": ev.Kind, "record": fmt.Sprintf("%s:%d", typ, id)}, nil
}

// changeWork changes a piece of work as the Work pages do (changeTodo), the
// change checked the same way: a status from the list, a lead who's an
// active person.
func (e pluginEnv) changeWork(t *Tx, key string, w plugin.WorkChange) (map[string]any, error) {
	td, err := t.App.todo(t.Ctx, t, w.ID)
	if err != nil {
		return nil, err
	}
	ch := &todoChange{fields: map[string]any{}}
	inv := apierr.Invalid{}
	if w.Status != nil {
		if !slices.Contains(TodoStatuses, *w.Status) {
			inv.Add("status", "is not included in the list")
		}
		ch.status = *w.Status
	}
	if w.DueOn != nil {
		ch.fields["due_on"] = workDay(*w.DueOn)
	}
	if w.Description != nil {
		ch.fields["description"] = nullable(sanitizeRichText(*w.Description))
	}
	people := func(ids ...int64) error {
		for _, id := range ids {
			n, err := store.Count(t.Ctx, t, `SELECT count(*) FROM users WHERE id = ? AND deactivated_at IS NULL AND agent_owner_id IS NULL`, id)
			if err != nil {
				return err
			}
			if n == 0 {
				inv.Add("owner_id", fmt.Sprintf("%d isn't someone on the team", id))
			}
		}
		return nil
	}
	if w.OwnerID != nil {
		ch.ownerSet, ch.owner = true, *w.OwnerID
		if *w.OwnerID != 0 {
			if err := people(*w.OwnerID); err != nil {
				return nil, err
			}
		}
	}
	if w.OtherOwnerIDs != nil {
		ch.othersSet, ch.others = true, *w.OtherOwnerIDs
		if err := people(*w.OtherOwnerIDs...); err != nil {
			return nil, err
		}
	}
	if err := inv.Err(); err != nil {
		return nil, err
	}
	if err := t.changeTodo(td, ch); err != nil {
		return nil, err
	}
	list, err := e.Records(t.Ctx, plugin.Query{Type: "Todo", ID: w.ID})
	if err != nil || len(list) == 0 {
		return map[string]any{"id": w.ID}, err
	}
	return list[0], nil
}

// ---- files -------------------------------------------------------------

func pluginFile(b *Blob) *plugin.File {
	return &plugin.File{ID: b.ID, Filename: b.Filename, ContentType: b.ContentType, ByteSize: b.ByteSize, CreatedAt: b.CreatedAt.Time.UTC().Format("2006-01-02T15:04:05Z")}
}

// AttachFile stores a file and attaches it to a plugin's record (the plugin
// API checked the record type is the plugin's own).
func (e pluginEnv) AttachFile(ctx context.Context, recordType string, recordID int64, name, filename, contentType string, r io.Reader) (*plugin.File, error) {
	var out *plugin.File
	err := e.a.write(ctx, nil, func(t *Tx) error {
		b, err := t.putBlob(filename, contentType, r)
		if err != nil {
			return err
		}
		if _, err := t.Exec(`INSERT INTO active_storage_attachments (name, record_type, record_id, blob_id, created_at) VALUES (?, ?, ?, ?, ?)`,
			name, recordType, recordID, b.ID, store.Now().DB()); err != nil {
			return err
		}
		out = pluginFile(b)
		return nil
	})
	return out, err
}

// Attachments are the files on a plugin's record under name, oldest first.
func (e pluginEnv) Attachments(ctx context.Context, recordType string, recordID int64, name string) ([]plugin.File, error) {
	rows, err := e.a.store.QueryContext(ctx, `SELECT b.id, b.key, b.filename, COALESCE(b.content_type, ''), b.byte_size, COALESCE(b.checksum, ''), b.created_at
		FROM active_storage_attachments x JOIN active_storage_blobs b ON b.id = x.blob_id
		WHERE x.record_type = ? AND x.record_id = ? AND x.name = ? ORDER BY x.id`, recordType, recordID, name)
	if err != nil {
		return nil, err
	}
	blobs, err := store.ScanAll(rows, func(r *sqlRows) (Blob, error) {
		var b Blob
		return b, r.Scan(&b.ID, &b.Key, &b.Filename, &b.ContentType, &b.ByteSize, &b.Checksum, &b.CreatedAt)
	})
	out := []plugin.File{}
	for i := range blobs {
		out = append(out, *pluginFile(&blobs[i]))
	}
	return out, err
}

// attachedTo is a blob attached to the record, nil when it isn't.
func (e pluginEnv) attachedTo(ctx context.Context, recordType string, recordID, fileID int64) (*Blob, error) {
	var b Blob
	err := e.a.store.QueryRowContext(ctx, `SELECT b.id, b.key, b.filename, COALESCE(b.content_type, ''), b.byte_size, COALESCE(b.checksum, ''), b.created_at
		FROM active_storage_attachments x JOIN active_storage_blobs b ON b.id = x.blob_id
		WHERE x.record_type = ? AND x.record_id = ? AND b.id = ?`, recordType, recordID, fileID).
		Scan(&b.ID, &b.Key, &b.Filename, &b.ContentType, &b.ByteSize, &b.Checksum, &b.CreatedAt)
	if isNoRows(err) {
		return nil, plugin.ErrNoRecord
	}
	return &b, err
}

// OpenFile is a file on a plugin's record, to read.
func (e pluginEnv) OpenFile(ctx context.Context, recordType string, recordID, fileID int64) (io.ReadCloser, *plugin.File, error) {
	b, err := e.attachedTo(ctx, recordType, recordID, fileID)
	if err != nil {
		return nil, nil, err
	}
	f, err := os.Open(e.a.blobPath(b.Key))
	if err != nil {
		return nil, nil, err
	}
	return f, pluginFile(b), nil
}

// DetachFile removes a file from a plugin's record, and the file.
func (e pluginEnv) DetachFile(ctx context.Context, recordType string, recordID, fileID int64) error {
	b, err := e.attachedTo(ctx, recordType, recordID, fileID)
	if err != nil {
		return err
	}
	return e.a.write(ctx, nil, func(t *Tx) error {
		if _, err := t.Exec(`DELETE FROM active_storage_attachments WHERE record_type = ? AND record_id = ? AND blob_id = ?`, recordType, recordID, b.ID); err != nil {
			return err
		}
		if _, err := t.Exec(`DELETE FROM active_storage_blobs WHERE id = ?`, b.ID); err != nil {
			return err
		}
		path := t.App.blobPath(b.Key)
		t.AfterCommit(func(context.Context) { _ = os.Remove(path) })
		return nil
	})
}
