package core

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Quick actions (the app's a key): add a note, a document, a commitment,
// work or a request to the record on screen, or to another one picked from
// a list. Notes, documents and plugin records are polymorphic, so each
// action names the record types it takes; the forms post to their areas'
// own routes (notes, documents, commitments, work, requests) with the
// record picked ("Client:12"). These routes only feed the menu and its
// picker, so agents have no tools for them (add_note and the rest cover it).

func init() { routeSets = append(routeSets, (*App).quickActionRoutes) }

func (a *App) quickActionRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/quick_actions", a.listQuickActions)
	a.handle(mux, "GET /api/quick_actions/new", a.newQuickAction)
	a.handle(mux, "GET /api/quick_actions/records", a.quickActionRecords)
}

// QuickActionRecordTypes are the records a quick action can go on.
var QuickActionRecordTypes = []string{"Client", "Engagement", "ScopeItem", "Todo", "Request"}

// quickActionStarting is how many records a picker shows before anything
// is typed: never the whole install.
const quickActionStarting = 12

// QuickAction is one entry in the menu.
type QuickAction struct {
	Key, Label, Title, Icon string
	Types                   []string
	// Context turns the record on screen into one this action takes (work
	// on a scope item goes to its engagement); "" for none.
	Context func(a *App, ctx context.Context, typ string, id int64) (string, int64)
}

// quickActions are the core's; plugins' areas append theirs in an init.
var quickActions = []*QuickAction{
	{Key: "note", Label: "Note", Title: "Add a note", Icon: "message-square", Types: QuickActionRecordTypes},
	{Key: "document", Label: "Document", Title: "Add a document", Icon: "paperclip", Types: QuickActionRecordTypes},
	// A promise with a date: on a client or engagement; work, a scope item
	// or a request take its engagement, or its client.
	{Key: "commitment", Label: "Commitment", Title: "Add a commitment", Icon: "bookmark", Types: []string{"Client", "Engagement"},
		Context: func(a *App, ctx context.Context, typ string, id int64) (string, int64) {
			if eng := a.quickEngagementOf(ctx, typ, id); eng != 0 {
				return "Engagement", eng
			}
			return a.quickClientOf(ctx, typ, id)
		}},
	// Work from anywhere: on an engagement; work or a scope item on screen
	// take theirs.
	{Key: "work", Label: "Work", Title: "Add work", Icon: "circle-check", Types: []string{"Engagement"},
		Context: func(a *App, ctx context.Context, typ string, id int64) (string, int64) {
			if eng := a.quickEngagementOf(ctx, typ, id); eng != 0 {
				return "Engagement", eng
			}
			return "", 0
		}},
	// Something a client asked for, logged from anywhere: on its client.
	{Key: "request", Label: "Request", Title: "Log a request", Icon: "inbox", Types: []string{"Client"},
		Context: func(a *App, ctx context.Context, typ string, id int64) (string, int64) {
			return a.quickClientOf(ctx, typ, id)
		}},
}

// quickEngagementOf is the engagement a record is on, 0 for none.
func (a *App) quickEngagementOf(ctx context.Context, typ string, id int64) int64 {
	q := map[string]string{
		"Todo":      `SELECT engagement_id FROM todos WHERE id = ?`,
		"ScopeItem": `SELECT v.engagement_id FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id WHERE s.id = ?`,
	}[typ]
	var eng int64
	if q != "" {
		_ = a.store.QueryRowContext(ctx, q, id).Scan(&eng)
	}
	return eng
}

// quickClientOf is the client a record belongs to.
func (a *App) quickClientOf(ctx context.Context, typ string, id int64) (string, int64) {
	q := map[string]string{
		"Client":     `SELECT id FROM clients WHERE id = ?`,
		"Engagement": `SELECT client_id FROM engagements WHERE id = ?`,
		"Todo":       `SELECT e.client_id FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.id = ?`,
		"ScopeItem":  `SELECT e.client_id FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id JOIN engagements e ON e.id = v.engagement_id WHERE s.id = ?`,
		"Request":    `SELECT client_id FROM requests WHERE id = ? AND client_id IS NOT NULL`,
	}[typ]
	var client int64
	if q == "" || a.store.QueryRowContext(ctx, q, id).Scan(&client) != nil || client == 0 {
		return "", 0
	}
	return "Client", client
}

func quickActionJSON(q *QuickAction) Obj {
	return O("key", q.Key, "label", q.Label, "title", q.Title, "icon", q.Icon, "types", q.Types)
}

func (a *App) listQuickActions(c *Ctx) error {
	list := make([]Obj, len(quickActions))
	for i, q := range quickActions {
		list[i] = quickActionJSON(q)
	}
	return c.JSON(200, O("actions", list, "record_types", QuickActionRecordTypes))
}

// quickRecord is a record a picker offers.
type quickRecord struct {
	Type      string
	ID        int64
	Label     string
	Kind      string
	Ref       string
	UpdatedAt string
}

func (r quickRecord) json() Obj {
	o := O("value", fmt.Sprintf("%s:%d", r.Type, r.ID), "type", r.Type, "id", r.ID, "label", r.Label, "kind", r.Kind)
	if r.Ref != "" {
		o = o.Set("ref", r.Ref)
	}
	return o
}

// quickLocate reads "Type:id", only of the given types.
func quickLocate(value string, types []string) (string, int64, error) {
	typ, raw, _ := strings.Cut(value, ":")
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil && typ == "Engagement" && quickRef.MatchString(raw) {
		return typ, 0, nil // by its ref: the caller looks it up
	}
	if !slices.Contains(types, typ) || err != nil {
		return "", 0, apierr.BadRequest("That isn't a record this takes: " + value)
	}
	return typ, id, nil
}

// quickRecordsSQL reads each type's records: id, label, the words for its
// kind (an engagement's label), and when it last changed.
var quickRecordsSQL = map[string]string{
	"Client":     `SELECT id, name, '', '', updated_at FROM clients`,
	"Engagement": `SELECT id, ref || ' ' || title, label, ref, updated_at FROM engagements`,
	"ScopeItem":  `SELECT id, description, '', '', updated_at FROM scope_items`,
	"Todo":       `SELECT id, title, '', '', updated_at FROM todos`,
	"Request":    `SELECT id, subject, '', '', updated_at FROM requests`,
}

// quickName is the column a type is found by.
var quickName = map[string]string{"Client": "name", "Engagement": "title", "ScopeItem": "description", "Todo": "title", "Request": "subject"}

func (a *App) quickRecordsWhere(ctx context.Context, typ, where, order string, limit int, args ...any) ([]quickRecord, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := a.store.QueryContext(ctx, quickRecordsSQL[typ]+` WHERE `+where+` ORDER BY `+order+` LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (quickRecord, error) {
		rec := quickRecord{Type: typ}
		var label string
		err := r.Scan(&rec.ID, &rec.Label, &label, &rec.Ref, &rec.UpdatedAt)
		switch typ {
		case "Engagement":
			rec.Kind = s.LabelName(label, 1)
		case "Client":
			rec.Kind = s.Term("client", 1)
		case "ScopeItem":
			rec.Kind = s.Term("scope_item", 1)
		case "Todo":
			rec.Kind = s.Term("work", 1)
		default:
			rec.Kind = humanize(strings.ToLower(typ))
		}
		return rec, err
	})
}

// quickStarting is what a picker offers before anything is typed: the
// record on screen, then the ones changed most recently, a dozen in all.
func (a *App) quickStarting(ctx context.Context, current *quickRecord, types []string) ([]Obj, error) {
	var all []quickRecord
	for _, typ := range types {
		list, err := a.quickRecordsWhere(ctx, typ, "1 = 1", "updated_at DESC", quickActionStarting)
		if err != nil {
			return nil, err
		}
		all = append(all, list...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].UpdatedAt > all[j].UpdatedAt })
	out := []Obj{}
	if current != nil {
		out = append(out, current.json())
	}
	for _, r := range all {
		if len(out) == quickActionStarting {
			break
		}
		if current != nil && r.Type == current.Type && r.ID == current.ID {
			continue
		}
		out = append(out, r.json())
	}
	return out, nil
}

// quickSearch is what a picker offers for what was typed: names, refs and
// titles that match (exactly, then starting with it, then containing it,
// then the search index), eight of each type, twenty in all.
func (a *App) quickSearch(ctx context.Context, text string, types []string) ([]Obj, error) {
	text = strings.TrimSpace(text)
	lowered := strings.ToLower(text)
	like := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(lowered)
	out := []Obj{}
	for _, typ := range types {
		var found []quickRecord
		if typ == "Engagement" && quickRef.MatchString(text) {
			list, err := a.quickRecordsWhere(ctx, typ, "ref = ?", "id", 1, strings.ToUpper(text))
			if err != nil {
				return nil, err
			}
			found = list
		} else {
			for _, try := range [][2]string{{"= ?", lowered}, {`LIKE ? ESCAPE '\'`, like + "%"}, {`LIKE ? ESCAPE '\'`, "%" + like + "%"}} {
				list, err := a.quickRecordsWhere(ctx, typ, "lower("+quickName[typ]+") "+try[0], "id", 8, try[1])
				if err != nil {
					return nil, err
				}
				if len(list) > 0 {
					found = list
					break
				}
			}
			if len(found) == 0 {
				hits, err := a.search(ctx, text)
				if err != nil {
					return nil, err
				}
				for _, h := range hits {
					if h.Type != typ || len(found) == 8 {
						continue
					}
					list, err := a.quickRecordsWhere(ctx, typ, "id = ?", "id", 1, h.ID)
					if err != nil {
						return nil, err
					}
					found = append(found, list...)
				}
			}
		}
		for _, r := range found {
			out = append(out, r.json())
		}
	}
	return out[:min(20, len(out))], nil
}

var quickRef = regexp.MustCompile(`\A[A-Za-z]+-\d+\z`)

func (a *App) quickTypes(c *Ctx) []string {
	var types []string
	for t := range strings.SplitSeq(c.Query("types"), ",") {
		if slices.Contains(QuickActionRecordTypes, t) {
			types = append(types, t)
		}
	}
	if len(types) == 0 {
		types = QuickActionRecordTypes
	}
	return types
}

func (a *App) quickActionRecords(c *Ctx) error {
	types := a.quickTypes(c)
	var list []Obj
	var err error
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		list, err = a.quickSearch(c.R.Context(), q, types)
	} else {
		list, err = a.quickStarting(c.R.Context(), nil, types)
	}
	if err != nil {
		return err
	}
	return c.JSON(200, O("records", list))
}

// newQuickAction is an action's form: the action, and the record it goes
// on (the one on screen, or the nearest one the action takes) with the
// picker's starting choices.
func (a *App) newQuickAction(c *Ctx) error {
	ctx := c.R.Context()
	var action *QuickAction
	for _, q := range quickActions {
		if q.Key == c.Query("kind") {
			action = q
		}
	}
	if action == nil {
		return notFound("quick action")
	}
	var current *quickRecord
	if v := c.Query("record"); v != "" {
		typ, id, err := quickLocate(v, QuickActionRecordTypes)
		if err != nil {
			return err
		}
		if typ == "Engagement" && id == 0 {
			_, ref, _ := strings.Cut(v, ":")
			if err := a.store.QueryRowContext(ctx, `SELECT id FROM engagements WHERE ref = ?`, strings.ToUpper(ref)).Scan(&id); err != nil {
				return notFound("engagement")
			}
			v = fmt.Sprintf("Engagement:%d", id)
		}
		if !slices.Contains(action.Types, typ) {
			typ, id = "", 0
			if action.Context != nil {
				typ, id = action.Context(a, ctx, quickTypeOf(v), quickIDOf(v))
			}
		}
		if typ != "" {
			list, err := a.quickRecordsWhere(ctx, typ, "id = ?", "id", 1, id)
			if err != nil {
				return err
			}
			if len(list) == 0 {
				return notFound(strings.ToLower(typ))
			}
			current = &list[0]
		}
	}
	records, err := a.quickStarting(ctx, current, action.Types)
	if err != nil {
		return err
	}
	var record any
	if current != nil {
		record = current.json()
	}
	return c.JSON(200, O("action", quickActionJSON(action), "record", record, "records", records))
}

func quickTypeOf(v string) string { t, _, _ := strings.Cut(v, ":"); return t }

func quickIDOf(v string) int64 {
	_, raw, _ := strings.Cut(v, ":")
	id, _ := strconv.ParseInt(raw, 10, 64)
	return id
}
