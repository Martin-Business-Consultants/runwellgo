package myday

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// A person's day (MyDay::Day): the open work they lead or are on, found by
// search and narrowed by when it's due, in groups to work down (late,
// today, this week, later, no date, then what's in review or blocked),
// oldest first in each, and their priorities (which the groups leave out).

var shows = []string{"open", "today", "week", "done", "all"}

// showLabels are the filters in words (Day::SHOWS).
var showLabels = map[string]string{"open": "Everything open", "today": "Due today or late", "week": "Due this week",
	"done": "Done this week", "all": "Everything"}

// waiting is work waiting on someone else, in groups of its own at the
// bottom.
var waiting = []string{"in_review", "blocked"}

var groupTitles = []struct{ key, title string }{
	{"late", "Late"}, {"today", "Today"}, {"week", "This week"}, {"later", "Later"}, {"undated", "No date"},
	{"in_review", "In review"}, {"blocked", "Blocked"}, {"done", "Done"},
}

// todo is a piece of work as the day shows it, from Records.
type todo struct {
	ID              int64
	Title, Status   string
	DueOn           string // YYYY-MM-DD or ""
	Record          string
	DisplayName     string
	URL             string
	EngagementID    int64
	EngagementRef   string
	EngagementTitle string
	ClientID        int64
	Client          string
	Description     string // rich text, as stored
	CreatedAt       time.Time
	CompletedAt     time.Time // zero while it's not done
}

func str(v any) string { s, _ := v.(string); return s }

func num(v any) int64 {
	switch n := v.(type) {
	case int64:
		return n
	case int:
		return int64(n)
	case float64:
		return int64(n)
	}
	return 0
}

func todoFrom(r map[string]any) todo {
	t := todo{ID: num(r["id"]), Title: str(r["title"]), Status: str(r["status"]), DueOn: str(r["due_on"]),
		Record: str(r["record"]), DisplayName: str(r["display_name"]), URL: str(r["url"])}
	if e, ok := r["engagement"].(map[string]any); ok {
		t.EngagementID, t.EngagementRef = num(e["id"]), str(e["ref"])
		t.EngagementTitle = strings.TrimPrefix(str(e["display_name"]), t.EngagementRef+" ")
	}
	if c, ok := r["client"].(map[string]any); ok {
		t.ClientID, t.Client = num(c["id"]), str(c["display_name"])
	}
	t.Description = str(r["description"])
	t.CreatedAt, t.CompletedAt = timeOf(r["created_at"]), timeOf(r["completed_at"])
	return t
}

// timeOf reads a record's timestamp, whatever type the host gave it (it
// answers as an ISO 8601 string in JSON; zero when there's none).
func timeOf(v any) time.Time {
	b, err := json.Marshal(v)
	if err != nil {
		return time.Time{}
	}
	var s string
	if json.Unmarshal(b, &s) != nil || s == "" {
		return time.Time{}
	}
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// todos reads work through Records, oldest first (created_at, id).
func todos(ctx context.Context, api *plugin.API, q plugin.Query) ([]todo, error) {
	q.Type = "Todo"
	if q.Limit == 0 {
		q.Limit = 500
	}
	list, err := api.Records(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]todo, len(list))
	for i, r := range list {
		out[i] = todoFrom(r)
	}
	slices.SortFunc(out, func(a, b todo) int {
		if c := a.CreatedAt.Compare(b.CreatedAt); c != 0 {
			return c
		}
		return cmp.Compare(a.ID, b.ID)
	})
	return out, nil
}

// findTodo is one piece of work by id; plugin.ErrNoRecord when there's none.
func findTodo(ctx context.Context, api *plugin.API, id int64) (todo, error) {
	if id <= 0 {
		return todo{}, plugin.ErrNoRecord
	}
	list, err := todos(ctx, api, plugin.Query{ID: id})
	if err != nil {
		return todo{}, err
	}
	if len(list) == 0 {
		return todo{}, plugin.ErrNoRecord
	}
	return list[0], nil
}

// today is the date in the install's time zone (Date.current).
func today(ctx context.Context, api *plugin.API) time.Time {
	loc := time.UTC
	if s, err := api.Site(ctx); err == nil {
		if l, err := time.LoadLocation(s.TimeZone); err == nil {
			loc = l
		}
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
}

type group struct {
	Key   string
	Title string
	Todos []todo
}

type day struct {
	userID     int64
	show       string
	query      string
	clientID   int64 // 0: every client
	engID      int64 // 0: every engagement
	today      string
	week       string
	mine       []todo // all the person's work, unfiltered
	todos      []todo // filtered and searched
	priorities []todo // in order, without what's done or gone
}

// loadDay reads the person's work and priorities. Pins whose todo is done
// or gone are dropped on the way (Day#priorities): the event usually has
// already.
func loadDay(ctx context.Context, api *plugin.API, userID int64, show, query string, clientID, engID int64) (*day, error) {
	if !slices.Contains(shows, show) {
		show = "open"
	}
	query = strings.TrimSpace(query)
	if r := []rune(query); len(r) > 100 {
		query = string(r[:100])
	}
	t := today(ctx, api)
	d := &day{userID: userID, show: show, query: query, clientID: clientID, engID: engID, today: t.Format(time.DateOnly), week: t.AddDate(0, 0, 7).Format(time.DateOnly)}
	var err error
	if d.mine, err = todos(ctx, api, plugin.Query{OwnerID: userID}); err != nil {
		return nil, err
	}
	for _, td := range d.mine {
		if d.keep(td) && d.matches(td) && d.under(td) {
			d.todos = append(d.todos, td)
		}
	}

	b, err := prioritiesFor(ctx, api, userID)
	if err != nil {
		return nil, err
	}
	if len(b.pins) > 0 {
		pinned, err := todos(ctx, api, plugin.Query{IDs: b.todoIDs()})
		if err != nil {
			return nil, err
		}
		byID := map[int64]todo{}
		for _, td := range pinned {
			byID[td.ID] = td
		}
		var stale []int64
		for _, p := range b.pins {
			td, ok := byID[p.todoID]
			if !ok || td.Status == "done" {
				stale = append(stale, p.todoID)
				continue
			}
			d.priorities = append(d.priorities, td)
		}
		if len(stale) > 0 {
			if err := b.unpin(ctx, api, stale...); err != nil {
				return nil, err
			}
		}
	}
	return d, nil
}

// keep is the tab's filter (Day#filtered): done is done in the last seven
// days.
func (d *day) keep(t todo) bool {
	open := t.Status != "done"
	switch d.show {
	case "open":
		return open
	case "today":
		return open && t.DueOn != "" && t.DueOn <= d.today
	case "week":
		return open && t.DueOn != "" && t.DueOn <= d.week
	case "done":
		return !open && !t.CompletedAt.IsZero() && !t.CompletedAt.Before(time.Now().AddDate(0, 0, -7))
	}
	return true
}

// matches is the search: the title, its engagement's or its client's name.
func (d *day) matches(t todo) bool {
	if d.query == "" {
		return true
	}
	q := strings.ToLower(d.query)
	return strings.Contains(strings.ToLower(t.Title), q) || strings.Contains(strings.ToLower(t.EngagementTitle), q) ||
		strings.Contains(strings.ToLower(t.Client), q)
}

// under is the Clients and Engagements filters.
func (d *day) under(t todo) bool {
	return (d.clientID == 0 || t.ClientID == d.clientID) && (d.engID == 0 || t.EngagementID == d.engID)
}

// choice is one of a filter's options: a client or engagement the person's
// work is under, with how much of it the tab shows.
type choice struct {
	ID    int64  `json:"id"`
	Label string `json:"label"`
	Count int    `json:"count"`
}

// choices are the clients and engagements of the person's work in this
// tab, by name; the engagements only the chosen client's.
func (d *day) choices() (clients, engagements []choice) {
	clients, engagements = []choice{}, []choice{}
	ci, ei := map[int64]int{}, map[int64]int{}
	for _, t := range d.mine {
		if !d.keep(t) {
			continue
		}
		if t.ClientID != 0 {
			if i, ok := ci[t.ClientID]; ok {
				clients[i].Count++
			} else {
				ci[t.ClientID] = len(clients)
				clients = append(clients, choice{t.ClientID, t.Client, 1})
			}
		}
		if t.EngagementID != 0 && (d.clientID == 0 || t.ClientID == d.clientID) {
			if i, ok := ei[t.EngagementID]; ok {
				engagements[i].Count++
			} else {
				ei[t.EngagementID] = len(engagements)
				label := t.EngagementTitle
				if t.EngagementRef != "" {
					label = t.EngagementRef + " " + label
				}
				engagements = append(engagements, choice{t.EngagementID, label, 1})
			}
		}
	}
	byLabel := func(a, b choice) int { return strings.Compare(strings.ToLower(a.Label), strings.ToLower(b.Label)) }
	slices.SortFunc(clients, byLabel)
	slices.SortFunc(engagements, byLabel)
	return
}

func (d *day) pinned(id int64) bool {
	return slices.ContainsFunc(d.priorities, func(t todo) bool { return t.ID == id })
}

func (d *day) late(t todo) bool { return t.Status != "done" && t.DueOn != "" && t.DueOn < d.today }

// groups are the listed work, without the priorities, by when it's due.
func (d *day) groups() []group {
	buckets := map[string][]todo{}
	for _, t := range d.todos {
		if d.pinned(t.ID) {
			continue
		}
		var key string
		switch {
		case t.Status == "done":
			key = "done"
		case slices.Contains(waiting, t.Status):
			key = t.Status
		case t.DueOn == "":
			key = "undated"
		case t.DueOn < d.today:
			key = "late"
		case t.DueOn == d.today:
			key = "today"
		case t.DueOn <= d.week:
			key = "week"
		default:
			key = "later"
		}
		buckets[key] = append(buckets[key], t)
	}
	var out []group
	for _, g := range groupTitles {
		if len(buckets[g.key]) > 0 {
			out = append(out, group{g.key, g.title, buckets[g.key]})
		}
	}
	return out
}

// counts are what's late and what's due today of the person's open work,
// whatever the filter or search.
func (d *day) counts() (late, dueToday int) {
	for _, t := range d.mine {
		if t.Status == "done" || t.DueOn == "" {
			continue
		}
		if t.DueOn < d.today {
			late++
		} else if t.DueOn == d.today {
			dueToday++
		}
	}
	return
}

// find is the todo asked for, if it's one of the priorities or the
// person's; ok false when not.
func (d *day) find(id int64) (todo, bool) {
	for _, list := range [][]todo{d.priorities, d.todos, d.mine} {
		if i := slices.IndexFunc(list, func(t todo) bool { return t.ID == id }); i >= 0 {
			return list[i], true
		}
	}
	return todo{}, false
}

// first is the one to open when none was asked for: the priorities' first
// not done, then the list's first still to do, then the list's first.
func (d *day) first() (todo, bool) {
	for _, t := range d.priorities {
		if t.Status != "done" {
			return t, true
		}
	}
	var listed []todo
	for _, g := range d.groups() {
		listed = append(listed, g.Todos...)
	}
	for _, t := range listed {
		if t.Status != "done" && !slices.Contains(waiting, t.Status) {
			return t, true
		}
	}
	if len(listed) > 0 {
		return listed[0], true
	}
	return todo{}, false
}

func plural(n int, one, many string) string {
	if n == 1 {
		return fmt.Sprintf("%d %s", n, one)
	}
	return fmt.Sprintf("%d %s", n, many)
}
