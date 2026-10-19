package reports

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// overview is the page's whole report: the headline and totals, every
// client the filters keep (place, health, money, counts), home, and the
// snapshot's money, delivery and reach.
func overview(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, err := build(ctx, api, filterOf(req.Query), 0)
	if err != nil {
		return nil, err
	}
	kickIfStale(api)
	t := r.totals()
	return plugin.JSON(200, map[string]any{
		"summary": r.headline(t) + ".", "today": r.m.Today, "prices": r.m.Prices, "filter": r.filter, "filters": r.filterOptions(),
		"home": r.home, "totals": t, "clients": r.rows, "money": r.money(), "delivery": r.delivery(), "reach": r.reach(),
		"any_clients": len(r.all) > 0,
	}), nil
}

func healthReport(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, err := build(ctx, api, filterOf(req.Query), 0)
	if err != nil {
		return nil, err
	}
	t := r.totals()
	list := []map[string]any{}
	for _, c := range r.rows {
		list = append(list, map[string]any{"id": c.ID, "name": c.Name, "record": fmt.Sprintf("Client:%d", c.ID), "url": c.URL,
			"score": c.Health.Score, "band": c.Health.Band, "label": c.Health.Label, "reasons": c.Health.Reasons,
			"days_since_touch": c.DaysSince, "late_work": c.LateWork, "overdue_commitments": c.Overdue, "waiting_approvals": c.Waiting,
			"open_requests": c.OpenRequests, "activity_12_weeks": c.Activity})
	}
	summary := fmt.Sprintf("%d at risk, %d need attention, %d on track.", t["at_risk"], t["watch"], t["on_track"])
	return plugin.JSON(200, map[string]any{"summary": summary, "clients": list}), nil
}

func moneyReport(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, err := build(ctx, api, filterOf(req.Query), 0)
	if err != nil {
		return nil, err
	}
	m := r.money()
	if m["hidden"] == true {
		return plugin.JSON(200, m), nil
	}
	m["summary"] = fmt.Sprintf("%s agreed on open %s, %s approved this year.", dollars(m["agreed_cents"].(int64)), r.words.engagementN, dollars(m["this_year_cents"].(int64))) //nolint:forcetypeassert,errcheck // the report's own map, its types known
	return plugin.JSON(200, m), nil
}

func deliveryReport(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, err := build(ctx, api, filterOf(req.Query), 0)
	if err != nil {
		return nil, err
	}
	d := r.delivery()
	s := fmt.Sprintf("In 26 weeks: %d %s done, %d added.", d["done"], r.words.workN, d["added"])
	if rate := d["on_time_rate"]; rate != nil {
		s += fmt.Sprintf(" %v%% on time.", rate)
	}
	d["summary"] = s
	return plugin.JSON(200, d), nil
}

func reachReport(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, err := build(ctx, api, filterOf(req.Query), 0)
	if err != nil {
		return nil, err
	}
	t := r.totals()
	out := r.reach()
	out["summary"] = r.headline(t) + "."
	list := []map[string]any{}
	for _, c := range r.rows {
		list = append(list, map[string]any{"id": c.ID, "name": c.Name, "place": c.Place, "iana": c.IANA})
	}
	out["clients"] = list
	return plugin.JSON(200, out), nil
}

// clientSnapshot is one client's card.
func clientSnapshot(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	id, err := pathID(req)
	if err != nil {
		return plugin.Answer(err), nil
	}
	r, err := build(ctx, api, filter{Status: "all", Health: "all"}, id)
	if err != nil {
		return nil, err
	}
	c := r.byID[id]
	if c == nil {
		return plugin.Answer(plugin.ErrNoRecord), nil
	}
	engs := []map[string]any{}
	sort.SliceStable(c.engagements, func(i, j int) bool {
		return (c.engagements[i].ClosedAt == nil) && (c.engagements[j].ClosedAt != nil)
	})
	var byLabel = map[string]int64{}
	for _, e := range c.engagements {
		total := e.WorkOpen + e.WorkDone
		progress := 0
		if total > 0 {
			progress = e.WorkDone * 100 / total
		}
		engs = append(engs, map[string]any{"id": e.ID, "ref": e.Ref, "title": e.Title, "label": e.Label, "shape": e.Shape, "state": e.State,
			"close_outcome": e.CloseOutcome, "closed": e.ClosedAt != nil, "cadence": e.Cadence, "agreed_cents": e.AgreedCents, "pending_cents": e.PendingCents,
			"work_open": e.WorkOpen, "work_done": e.WorkDone, "work_late": e.WorkLate, "progress": progress, "url": "/engagements/" + e.Ref})
		if e.ClosedAt == nil {
			byLabel[e.Label] += e.AgreedCents
		}
	}
	read := func(q plugin.Query) []map[string]any {
		q.ClientID, q.Limit = id, 8
		list, err := api.Records(ctx, q)
		if err != nil {
			api.Log("reports: " + err.Error())
			return []map[string]any{}
		}
		return list
	}
	pick := func(list []map[string]any, keys ...string) []map[string]any {
		out := []map[string]any{}
		for _, o := range list {
			p := map[string]any{}
			for _, k := range keys {
				p[k] = o[k]
			}
			if u, ok := o["url"].(string); ok {
				p["path"] = pathOf(u)
			}
			out = append(out, p)
		}
		return out
	}
	work := pick(read(plugin.Query{Type: "Todo", Open: true}), "id", "title", "status", "due_on", "overdue", "engagement")
	requests := pick(read(plugin.Query{Type: "Request", Open: true}), "id", "subject", "from", "received_at")
	commitments := pick(read(plugin.Query{Type: "Commitment", Open: true}), "id", "description", "due_on", "owner_name", "overdue")
	contactList := read(plugin.Query{Type: "Contact"})
	contacts := pick(contactList, "id", "name", "role", "email", "portal_access", "can_approve")

	return plugin.JSON(200, map[string]any{
		"summary": fmt.Sprintf("%s: %s (%d/100). %s.", c.Name, c.Health.Label, c.Health.Score, placeText(c.Place)),
		"client":  c, "health": c.Health, "engagements": engs, "money_by_label": byLabel, "prices": r.m.Prices,
		"work": work, "requests": requests, "commitments": commitments, "contacts": contacts,
		"checkin": checkin(c, contactList, engs, r.site.Name), "home": r.home,
	}), nil
}

func placeText(p Place) string {
	switch {
	case p.Label == "":
		return "Not on the map yet"
	case p.Approximate:
		return "Around " + p.Label + " (from their time zone)"
	case p.Country != "" && p.Country != p.Label:
		return "In " + p.Label + ", " + p.Country
	}
	return "In " + p.Label
}

// checkin is a short message to send them: hello, where things stand,
// what we need from them.
func checkin(c *clientRow, contacts []map[string]any, engs []map[string]any, us string) string {
	name := "there"
	for _, ct := range contacts {
		// The first name, past a title (Dr., Ms.).
		if n, _ := ct["name"].(string); n != "" {
			for w := range strings.FieldsSeq(n) {
				if !strings.HasSuffix(w, ".") {
					name = w
					break
				}
			}
			break
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Hi %s,\n\nA quick check-in from %s.", name, firstOf(us, "us"))
	var open []string
	for _, e := range engs {
		if e["closed"] == true {
			continue
		}
		open = append(open, fmt.Sprintf("%s (%d%% done)", e["title"], e["progress"]))
	}
	if len(open) > 0 {
		fmt.Fprintf(&b, " Where things stand: %s.", strings.Join(open, "; "))
	}
	if c.Waiting > 0 {
		b.WriteString(" There's an agreement waiting for your go-ahead whenever you're ready.")
	}
	b.WriteString("\n\nIs there anything you need from us this week?\n")
	return b.String()
}

// pathOf is a record's app path, from its url.
func pathOf(u string) string {
	if i := strings.Index(u, "://"); i >= 0 {
		if j := strings.Index(u[i+3:], "/"); j >= 0 {
			return u[i+3+j:]
		}
	}
	return u
}

// addNote adds a note to the client, from its card.
func addNote(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	id, err := pathID(req)
	if err != nil {
		return plugin.Answer(err), nil
	}
	var in struct {
		Body string `json:"body"`
		Kind string `json:"kind"`
	}
	if r := decodeBody(req, "note", &in); r != nil {
		return r, nil
	}
	note, err := api.AddNote(ctx, plugin.Note{Record: fmt.Sprintf("Client:%d", id), Body: in.Body, Kind: in.Kind})
	if err != nil {
		if r := plugin.Answer(err); r != nil {
			return r, nil
		}
		return nil, err
	}
	return plugin.JSON(201, map[string]any{"summary": "Note added.", "note": note, "at": time.Now().UTC()}), nil
}

// dollars is cents as money for an agent's summary.
func dollars(cents int64) string {
	whole := strconv.FormatInt(cents/100, 10)
	for i := len(whole) - 3; i > 0; i -= 3 {
		whole = whole[:i] + "," + whole[i:]
	}
	return "$" + whole
}
