package core

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// A record's history (events/_timeline), and the custom fields a form shows.

func init() { routeSets = append(routeSets, (*App).eventRoutes) }

func (a *App) eventRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/events", a.listEvents)
	a.handle(mux, "GET /api/custom_fields", a.listModelCustomFields)
}

// eventsJSON are events as answers show them (events/_event.json), with
// the title and details the timeline prints.
func (a *App) eventsJSON(ctx context.Context, where string, limit int, args ...any) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT e.id, e.subject_type, e.subject_id, e.kind, e.payload, COALESCE(e.actor_user_id, 0), COALESCE(e.actor, ''), e.source, e.occurred_at,
			COALESCE(u.name, ''), COALESCE(u.email_address, '')
		FROM events e LEFT JOIN users u ON u.id = e.actor_user_id WHERE `+where+` ORDER BY e.occurred_at DESC, e.id DESC LIMIT ?`, append(args, limit)...)
	if err != nil {
		return nil, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id, subjectID, userID int64
		var typ, kind, actor, source, name, email string
		var payload store.JSON
		var at store.Time
		if err := r.Scan(&id, &typ, &subjectID, &kind, &payload, &userID, &actor, &source, &at, &name, &email); err != nil {
			return nil, err
		}
		by := "system"
		switch {
		case name != "":
			by = name
		case email != "":
			by = email
		case actor != "":
			by = actor
		}
		label := source
		if source == "agent" {
			label = "via " + strOr(actor, "an agent")
		}
		p := payload.Map()
		o := O("id", id, "kind", kind, "source", source, "occurred_at", at, "payload", p, "by", by)
		if source == "agent" {
			o = o.Set("via", actor)
		}
		return o.Set("subject_type", typ).Set("subject_id", subjectID).Set("actor", userRef(userID, by)).
			Set("title", eventTitle(kind)).Set("details", eventDetails(p)).Set("source_label", label), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

// eventTitle is "engagement.closed" → "Engagement closed".
func eventTitle(kind string) string {
	return humanize(strings.NewReplacer(".", " ", "_", " ").Replace(kind))
}

// eventDetails are the payload's scalar parts, in words: "from: planned ·
// to: in progress", "amount: $2,400.00".
func eventDetails(p map[string]any) string {
	keys := make([]string, 0, len(p))
	for k := range p {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var parts []string
	for _, k := range keys {
		var val string
		switch v := p[k].(type) {
		case nil, map[string]any, []any:
		case string:
			val = v
			if identWord.MatchString(v) {
				val = strings.ReplaceAll(v, "_", " ")
			}
		case json.Number:
			val = v.String()
		case float64:
			val = strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			val = map[bool]string{true: "yes", false: "no"}[v]
		default:
			val = fmt.Sprint(v)
		}
		if val == "" {
			continue
		}
		key := k
		if base, ok := strings.CutSuffix(k, "_cents"); ok {
			if n, err := strconv.ParseInt(val, 10, 64); err == nil {
				key, val = base, formatCents(n)
			}
		}
		parts = append(parts, strings.ReplaceAll(key, "_", " ")+": "+val)
	}
	return strings.Join(parts, " · ")
}

// identWord is a value written as a code ("in_progress"), shown with spaces.
var identWord = regexp.MustCompile(`^[a-z]+(_[a-z]+)+$`)

func strOr(s, fallback string) string {
	if s == "" {
		return fallback
	}
	return s
}

func (a *App) listEvents(c *Ctx) error {
	typ := c.Query("subject_type")
	id, _ := strconv.ParseInt(c.Query("subject_id"), 10, 64)
	if typ == "" || id == 0 {
		return c.JSON(200, O("events", []Obj{}))
	}
	limit, _ := strconv.Atoi(c.Query("limit"))
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	events, err := a.eventsJSON(c.R.Context(), `e.subject_type = ? AND e.subject_id = ?`, limit, typ, id)
	if err != nil {
		return err
	}
	return c.JSON(200, O("events", events))
}

func (a *App) listModelCustomFields(c *Ctx) error {
	model := c.Query("model")
	if !slices.Contains(CustomFieldModels, model) {
		return c.JSON(200, O("fields", []any{}))
	}
	fields, err := a.customFields(c.R.Context(), a.store, model)
	if err != nil {
		return err
	}
	if fields == nil {
		fields = []*CustomField{}
	}
	return c.JSON(200, O("fields", fields))
}
