package core

import (
	"context"
	"encoding/csv"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Settings › Audit log, for owners: every event in the install, newest
// first, from sign-ins and security changes to agreements and work,
// filtered by area, person and period, and downloadable as CSV for whoever
// keeps the records. Nothing here can be edited or deleted.

func init() { routeSets = append(routeSets, (*App).auditRoutes) }

// AuditAreas and AuditPeriods are the log's filters, in order.
var (
	AuditAreas = [][2]string{{"all", "Everything"}, {"security", "Sign-in and settings"}, {"agreements", "Agreements"},
		{"work", "Work and notes"}, {"requests", "Requests and questions"}, {"clients", "Clients"}}
	AuditPeriods = [][2]string{{"7", "Last 7 days"}, {"30", "Last 30 days"}, {"90", "Last 90 days"}, {"all", "All time"}}
)

const auditCSVLimit = 50000

func (a *App) auditRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/settings/audit_log", a.showAuditLog, need("manage_settings"), tool("audit_log", "Read the audit log",
		note("Every event (sign-ins, security and settings changes, agreements, work, requests), newest first. area: all, security, agreements, work, requests, clients; person: a user id; days: 7, 30, 90 or all."),
		query("area", "string", "all, security, agreements, work, requests or clients."),
		query("person", "integer", "A user id."),
		query("days", "string", "7, 30, 90 or all.")))
}

func (a *App) showAuditLog(c *Ctx) error {
	ctx := c.R.Context()
	area := c.Query("area")
	if choiceName(AuditAreas, area) == area {
		area = "all"
	}
	days := c.Query("days")
	if choiceName(AuditPeriods, days) == days {
		days = "30"
	}
	where, args := []string{"1 = 1"}, []any{}
	if days != "all" {
		n, _ := strconv.Atoi(days)
		where, args = append(where, "e.occurred_at >= ?"), append(args, store.TimeOf(time.Now().AddDate(0, 0, -n)).DB())
	}
	var person *User
	if id, err := strconv.ParseInt(c.Query("person"), 10, 64); err == nil {
		if person, err = a.loadUser(ctx, id); err != nil {
			return err
		}
		if person != nil {
			where = append(where, "e.actor_user_id IN (SELECT ? UNION SELECT id FROM users WHERE agent_owner_id = ?)")
			args = append(args, person.ID, person.ID)
		}
	}
	switch area {
	case "security":
		where = append(where, "e.subject_type IN ('User', 'Setting')")
	case "agreements":
		where = append(where, "(e.kind LIKE 'agreement.%' OR e.kind LIKE 'engagement.%')")
	case "work":
		where = append(where, "(e.kind LIKE 'todo.%' OR e.kind LIKE 'commitment.%' OR e.kind LIKE 'note.%' OR e.kind LIKE 'document.%')")
	case "requests":
		where = append(where, "(e.kind LIKE 'request.%' OR e.kind LIKE 'question.%')")
	case "clients":
		where = append(where, "e.kind LIKE 'client.%'")
	}
	csvOut := c.Query("format") == "csv" || strings.HasSuffix(c.R.URL.Path, ".csv")
	limit := 500
	if csvOut {
		limit = auditCSVLimit
	} else if c.Query("per") != "" || c.Query("page") != "" {
		// The app pages through it, a table's worth at a time.
		page, per := c.paging(50, 200)
		limit = page * per
	}
	events, err := a.eventsJSON(ctx, strings.Join(where, " AND "), limit, args...)
	if err != nil {
		return err
	}
	refs := make([]subjectRef, len(events))
	for i, e := range events {
		id, _ := e.Get("subject_id").(int64) // events.subject_id is NOT NULL
		refs[i] = subjectRef{str(e.Get("subject_type")), id}
	}
	names := a.subjectNames(ctx, refs)
	for i, e := range events {
		events[i] = e.Set("on", names[refs[i]].label(refs[i]))
	}
	if csvOut {
		return a.auditCSV(c, events)
	}
	more := false
	if c.Query("per") != "" || c.Query("page") != "" {
		page, per := c.paging(50, 200)
		start := min((page-1)*per, len(events))
		more = len(events) == limit
		events = events[start:]
	}
	summary := fmt.Sprintf("%d events", len(events))
	if len(events) == 500 {
		summary += " (the newest 500)"
	}
	summary += ": " + strings.ToLower(choiceName(AuditAreas, area)) + ", " + strings.ToLower(choiceName(AuditPeriods, days))
	if person != nil {
		summary += ", by " + person.DisplayName()
	}
	c.Summary = summary
	return c.JSON(200, O("summary", summary, "area", area, "days", days, "person", userRefOf(person), "more", more,
		"areas", choicesObj(AuditAreas), "periods", choicesObj(AuditPeriods), "events", events))
}

func userRefOf(u *User) any {
	if u == nil {
		return nil
	}
	return O("id", u.ID, "name", u.DisplayName())
}

func (a *App) auditCSV(c *Ctx, events []Obj) error {
	h := c.W.Header()
	h.Set("Content-Type", "text/csv; charset=utf-8")
	h.Set("Content-Disposition", `attachment; filename="audit-log-`+a.today(c.R.Context())+`.csv"`)
	w := csv.NewWriter(c.W)
	_ = w.Write([]string{"when", "kind", "by", "via", "source", "subject_type", "subject_id", "subject", "details"})
	for _, e := range events {
		at, _ := e.Get("occurred_at").(store.Time)
		id, _ := e.Get("subject_id").(int64)
		_ = w.Write([]string{at.UTC().Format(time.RFC3339), str(e.Get("kind")), str(e.Get("by")), str(e.Get("via")), str(e.Get("source")),
			str(e.Get("subject_type")), strconv.FormatInt(id, 10), str(e.Get("on")), str(e.Get("details"))})
	}
	w.Flush()
	return w.Error()
}

// ---- what an event happened to, in words ----------------------------------------

type subjectRef struct {
	typ string
	id  int64
}

type subjectName struct {
	name  string
	path  string // its page in the app, "" for none
	found bool
}

// label is the subject in words (the Rails audit_subject_name).
func (n subjectName) label(r subjectRef) string {
	switch {
	case r.typ == "Setting":
		return "Settings"
	case !n.found:
		return fmt.Sprintf("%s %d (gone)", r.typ, r.id)
	case n.name == "":
		return fmt.Sprintf("%s %d", r.typ, r.id)
	}
	return n.name
}

// subjectNames names each subject, one query per type.
func (a *App) subjectNames(ctx context.Context, refs []subjectRef) map[subjectRef]subjectName {
	out := map[subjectRef]subjectName{}
	byType := map[string][]int64{}
	for _, r := range refs {
		byType[r.typ] = append(byType[r.typ], r.id)
	}
	queries := map[string]string{
		"Client":     `SELECT id, name, '/clients/' || id FROM clients`,
		"Contact":    `SELECT id, name, '/clients/' || client_id FROM contacts`,
		"Engagement": `SELECT id, ref || ' ' || title, '/engagements/' || ref FROM engagements`,
		"Todo":       `SELECT id, title, '/work/' || id FROM todos`,
		"Commitment": `SELECT id, description, '/commitments' FROM commitments`,
		"Request":    `SELECT id, subject, '/requests/' || id FROM requests`,
		"Question":   `SELECT id, text, '' FROM questions`,
		"ScopeItem":  `SELECT id, description, '' FROM scope_items`,
		"User":       `SELECT id, CASE WHEN name != '' THEN name ELSE email_address END, '' FROM users`,
		"Setting":    `SELECT id, 'Settings', '' FROM settings`,
		"Note":       `SELECT id, '', '' FROM notes`,
		"Document":   `SELECT id, '', '' FROM documents`,
		"AgreementVersion": `SELECT v.id, e.ref || ' v' || v.number, '/engagements/' || e.ref FROM agreement_versions v
			LEFT JOIN engagements e ON e.id = v.engagement_id`,
	}
	for typ, ids := range byType {
		q, ok := queries[typ]
		if !ok {
			continue
		}
		col := "id"
		if typ == "AgreementVersion" {
			col = "v.id"
		}
		if err := a.subjectNamesOf(ctx, typ, q+` WHERE `+col+` IN (`+store.Placeholders(len(ids))+`)`, ids, out); err != nil {
			a.log.Warn("subject names", "type", typ, "err", err)
		}
	}
	// Someone removed is still named, from their user.removed event.
	var gone []int64
	for _, id := range byType["User"] {
		if _, ok := out[subjectRef{"User", id}]; !ok {
			gone = append(gone, id)
		}
	}
	for id, name := range removedNames(ctx, a.store, gone) {
		out[subjectRef{"User", id}] = subjectName{name: name + " (removed)", found: true}
	}
	return out
}

// subjectNamesOf adds the names q finds for ids, of type typ, to out (as
// far as it got, on an error).
func (a *App) subjectNamesOf(ctx context.Context, typ, q string, ids []int64, out map[subjectRef]subjectName) error {
	rows, err := a.store.QueryContext(ctx, q, idArgs(ids)...)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name, path *string
		if rows.Scan(&id, &name, &path) != nil {
			continue
		}
		n := subjectName{found: true}
		if name != nil {
			n.name = *name
		}
		if path != nil {
			n.path = *path
		}
		out[subjectRef{typ, id}] = n
	}
	return rows.Err()
}
