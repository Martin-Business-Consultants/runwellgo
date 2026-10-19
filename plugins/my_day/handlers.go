package myday

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// The routes: the day (show_my_day), the priorities (add, move, remove, and
// several at once), and the picked todo changed in place (status, due date
// and details, who's on it) through the core's ChangeWork. Everything here
// is the caller's own: their work, their priorities (an agent's are its
// person's: req.Actor.ID).

// todoJSON is a todo as the day answers it (days/show.json.jbuilder): its
// agent ref, title, status, due date, whether it's late, its engagement's
// ref and its client's name; the engagement's title and the ids the screen
// links with beside them.
type todoJSON struct {
	Type            string  `json:"type"`
	ID              int64   `json:"id"`
	Record          string  `json:"record"`
	DisplayName     string  `json:"display_name"`
	URL             string  `json:"url,omitempty"`
	Title           string  `json:"title"`
	Status          string  `json:"status"`
	DueOn           *string `json:"due_on"`
	Late            bool    `json:"late"`
	Engagement      string  `json:"engagement"`
	Client          string  `json:"client"`
	EngagementTitle string  `json:"engagement_title"`
	ClientID        int64   `json:"client_id"`
}

// openJSON is the open todo: the same, with its details (agent_text) and
// its place in the priorities (1…5, null when it's not one).
type openJSON struct {
	todoJSON
	Description *string `json:"description"`
	Priority    *int    `json:"priority"`
}

func (d *day) json(t todo) todoJSON {
	j := todoJSON{Type: "Todo", ID: t.ID, Record: t.Record, DisplayName: t.DisplayName, URL: t.URL, Title: t.Title, Status: t.Status,
		Late: d.late(t), Engagement: t.EngagementRef, Client: t.Client, EngagementTitle: t.EngagementTitle, ClientID: t.ClientID}
	if t.DueOn != "" {
		due := t.DueOn
		j.DueOn = &due
	}
	return j
}

type groupJSON struct {
	Key   string     `json:"key"`
	Title string     `json:"title"`
	Todos []todoJSON `json:"todos"`
}

type dayJSON struct {
	Summary string  `json:"summary"`
	Show    string  `json:"show"`
	Q       *string `json:"q"`
	// The Clients and Engagements filters: what's chosen, and the choices.
	Client      *int64      `json:"client"`
	Engagement  *int64      `json:"engagement"`
	Clients     []choice    `json:"clients"`
	Engagements []choice    `json:"engagements"`
	Priorities  []todoJSON  `json:"priorities"`
	Groups      []groupJSON `json:"groups"`
	Todo        *openJSON   `json:"todo,omitempty"`
	// What the greeting says, whatever the filter: what's late and due today.
	Late     int    `json:"late_count"`
	DueToday int    `json:"due_today_count"`
	Limit    int    `json:"limit"`
	Today    string `json:"today"`
}

func showDay(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	clientID, _ := strconv.ParseInt(req.Query.Get("client"), 10, 64)
	engID, _ := strconv.ParseInt(req.Query.Get("engagement"), 10, 64)
	d, err := loadDay(ctx, api, req.Actor.ID, req.Query.Get("show"), req.Query.Get("q"), clientID, engID)
	if err != nil {
		return nil, err
	}
	out := dayJSON{Show: d.show, Priorities: []todoJSON{}, Groups: []groupJSON{}, Limit: limit, Today: d.today}
	out.Clients, out.Engagements = d.choices()
	if clientID > 0 {
		out.Client = &clientID
	}
	if engID > 0 {
		out.Engagement = &engID
	}
	if d.query != "" {
		out.Q = &d.query
	}
	for _, t := range d.priorities {
		out.Priorities = append(out.Priorities, d.json(t))
	}
	for _, g := range d.groups() {
		gj := groupJSON{Key: g.Key, Title: g.Title, Todos: []todoJSON{}}
		for _, t := range g.Todos {
			gj.Todos = append(gj.Todos, d.json(t))
		}
		out.Groups = append(out.Groups, gj)
	}
	out.Late, out.DueToday = d.counts()

	open, ok := todo{}, false
	if id, _ := strconv.ParseInt(req.Query.Get("todo"), 10, 64); id > 0 {
		open, ok = d.find(id)
	}
	if !ok {
		open, ok = d.first()
	}
	matching := ""
	if d.query != "" {
		matching = ", matching “" + d.query + "”"
	}
	for _, c := range out.Clients {
		if c.ID == clientID {
			matching += ", for " + c.Label
		}
	}
	for _, e := range out.Engagements {
		if e.ID == engID {
			matching += ", under " + e.Label
		}
	}
	out.Summary = fmt.Sprintf("%s of %d, %s of work (%s%s)", plural(len(d.priorities), "priority", "priorities"), limit,
		plural(len(d.todos), "piece", "pieces"), strings.ToLower(showLabels[d.show]), matching)
	if ok {
		j := openJSON{todoJSON: d.json(open), Description: agentText(open.Description)}
		if i := slices.IndexFunc(d.priorities, func(t todo) bool { return t.ID == open.ID }); i >= 0 {
			place := i + 1
			j.Priority = &place
		}
		out.Todo = &j
		out.Summary += "; open: " + open.Title
	}
	return plugin.JSON(http.StatusOK, out), nil
}

// done answers a change: what happened, for the screen's toast and the
// agent's summary.
func done(msg string) *plugin.Response {
	return plugin.JSON(http.StatusOK, map[string]any{"summary": msg, "message": msg})
}

// refused answers a change that couldn't be made (the Rails alert).
func refused(msg string) *plugin.Response {
	return plugin.JSON(http.StatusUnprocessableEntity, map[string]any{"error": "refused", "message": msg, "summary": msg})
}

func badRequest(msg string) *plugin.Response {
	return plugin.JSON(http.StatusBadRequest, map[string]any{"error": "bad_request", "message": msg})
}

// body reads the request's JSON object (empty when there's none).
func body(req *plugin.Request) (map[string]any, error) {
	out := map[string]any{}
	if len(strings.TrimSpace(string(req.Body))) == 0 {
		return out, nil
	}
	err := json.Unmarshal(req.Body, &out)
	return out, err
}

// intParam is a number from the path, the query or the body ("12" or 12).
func intParam(req *plugin.Request, b map[string]any, name string) int64 {
	if v := req.Params[name]; v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	if v := req.Query.Get(name); v != "" {
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	switch v := b[name].(type) {
	case float64:
		return int64(v)
	case string:
		n, _ := strconv.ParseInt(v, 10, 64)
		return n
	}
	return 0
}

func strParam(req *plugin.Request, b map[string]any, name string) string {
	if v := req.Query.Get(name); v != "" {
		return v
	}
	if v, ok := b[name].(string); ok {
		return v
	}
	return ""
}

// ids are the picked todos: a list or "1,2,3", in the body or the query,
// each once, in the order given.
func ids(req *plugin.Request, b map[string]any) []int64 {
	var raw []string
	switch v := b["ids"].(type) {
	case []any:
		for _, x := range v {
			raw = append(raw, strings.Split(fmt.Sprint(x), ",")...)
		}
	case string:
		raw = strings.Split(v, ",")
	}
	raw = append(raw, req.Query["ids"]...)
	raw = append(raw, req.Query["ids[]"]...)
	var out []int64
	for _, s := range raw {
		if n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64); err == nil && n > 0 && !slices.Contains(out, n) {
			out = append(out, n)
		}
	}
	return out
}

// target is the request's todo (todo_id, or id), and its body.
func target(ctx context.Context, api *plugin.API, req *plugin.Request, param string) (todo, map[string]any, *plugin.Response, error) {
	b, err := body(req)
	if err != nil {
		return todo{}, nil, badRequest("Send a JSON object."), nil //nolint:nilerr // the caller's mistake, answered
	}
	t, err := findTodo(ctx, api, intParam(req, b, param))
	if err != nil {
		if res := plugin.Answer(err); res != nil {
			return todo{}, nil, res, nil
		}
		return todo{}, nil, nil, err
	}
	return t, b, nil, nil
}

// addPriority puts work in the priorities (PrioritiesController#create), at
// the end or before another; the screen's drops land here too.
func addPriority(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	t, b, res, err := target(ctx, api, req, "todo_id")
	if res != nil || err != nil {
		return res, err
	}
	if t.Status == "done" {
		return refused("“" + t.Title + "” is done: priorities are for what's still to do."), nil
	}
	var placed bool
	err = withPriorities(ctx, api, req.Actor.ID, func(q plugin.Querier, pr *bundle) error {
		placed, err = pr.place(ctx, q, t.ID, intParam(req, b, "before"))
		return err
	})
	if err != nil {
		return nil, err
	}
	if !placed {
		return refused(fmt.Sprintf("Your priorities hold %d: take one off (or finish it) before adding “%s”.", limit, t.Title)), nil
	}
	return done("“" + t.Title + "” is in your priorities."), nil
}

// movePriority moves work one place up or down (PrioritiesController#update).
func movePriority(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	t, b, res, err := target(ctx, api, req, "todo_id")
	if res != nil || err != nil {
		return res, err
	}
	direction := strParam(req, b, "direction")
	if direction != "up" && direction != "down" {
		return plugin.Invalid(map[string][]string{"direction": {"is not included in the list"}}), nil
	}
	var moved bool
	err = withPriorities(ctx, api, req.Actor.ID, func(q plugin.Querier, pr *bundle) error {
		moved, err = pr.move(ctx, q, t.ID, direction)
		return err
	})
	if err != nil {
		return nil, err
	}
	if !moved {
		return refused("“" + t.Title + "” can't go further " + direction + "."), nil
	}
	return done("Moved “" + t.Title + "” " + direction + "."), nil
}

// removePriority takes work off the priorities (PrioritiesController#destroy).
func removePriority(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	t, _, res, err := target(ctx, api, req, "todo_id")
	if res != nil || err != nil {
		return res, err
	}
	if err := withPriorities(ctx, api, req.Actor.ID, func(q plugin.Querier, pr *bundle) error { return pr.unpin(ctx, q, t.ID) }); err != nil {
		return nil, err
	}
	return done("“" + t.Title + "” is off your priorities."), nil
}

// picked are the picked todos that exist, in the order given.
func picked(ctx context.Context, api *plugin.API, req *plugin.Request) ([]todo, *plugin.Response, error) {
	b, err := body(req)
	if err != nil {
		return nil, badRequest("Send a JSON object."), nil //nolint:nilerr // the caller's mistake, answered
	}
	want := ids(req, b)
	if len(want) == 0 {
		return nil, nil, nil
	}
	list, err := todos(ctx, api, plugin.Query{IDs: want})
	if err != nil {
		return nil, nil, err
	}
	var out []todo
	for _, id := range want {
		if i := slices.IndexFunc(list, func(t todo) bool { return t.ID == id }); i >= 0 {
			out = append(out, list[i])
		}
	}
	return out, nil, nil
}

// bulkAdd puts the picked work at the end of the priorities, as far as they
// hold (BulkPrioritiesController#create); done work is passed over.
func bulkAdd(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	list, res, err := picked(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	var want []int64
	for _, t := range list {
		if t.Status != "done" {
			want = append(want, t.ID)
		}
	}
	var skipped []int64
	err = withPriorities(ctx, api, req.Actor.ID, func(q plugin.Querier, pr *bundle) error {
		skipped, err = pr.add(ctx, q, want)
		return err
	})
	if err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("%d added to your priorities", len(want)-len(skipped))
	if len(skipped) > 0 {
		msg += fmt.Sprintf("; %d didn't fit (they hold %d)", len(skipped), limit)
	}
	return plugin.JSON(http.StatusOK, map[string]any{"summary": msg + ".", "message": msg + ".", "alert": len(skipped) > 0}), nil
}

// bulkRemove takes the picked work off the priorities
// (BulkPrioritiesController#destroy).
func bulkRemove(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	list, res, err := picked(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	gone := make([]int64, len(list))
	for i, t := range list {
		gone[i] = t.ID
	}
	if err := withPriorities(ctx, api, req.Actor.ID, func(q plugin.Querier, pr *bundle) error { return pr.unpin(ctx, q, gone...) }); err != nil {
		return nil, err
	}
	return done(fmt.Sprintf("%d taken off your priorities.", len(list))), nil
}

var (
	blockEnd = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`)
	tags     = regexp.MustCompile(`<[^>]*>`)
	blanks   = regexp.MustCompile(`\n+`)
)

// agentText is rich text as an agent reads it (agent_text): plain text, a
// line per paragraph, nil when there's none.
func agentText(html string) *string {
	if strings.TrimSpace(html) == "" {
		return nil
	}
	s := blockEnd.ReplaceAllString(html, "\n")
	s = tags.ReplaceAllString(s, "")
	s = strings.NewReplacer("&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&nbsp;", " ").Replace(s)
	s = strings.TrimSpace(blanks.ReplaceAllString(s, "\n"))
	return &s
}

var statuses = []string{"planned", "in_progress", "in_review", "blocked", "done"}

func humanize(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// change runs a ChangeWork, answering the core's refusal (a 422) as is.
func change(ctx context.Context, api *plugin.API, w plugin.WorkChange) (map[string]any, *plugin.Response, error) {
	rec, err := api.ChangeWork(ctx, w)
	if err != nil {
		if res := plugin.Answer(err); res != nil {
			return nil, res, nil
		}
		return nil, nil, err
	}
	return rec, nil, nil
}

// setStatus moves a piece of work on (StatusesController#update), through
// the core's rules (its history records it); done takes it off everyone's
// priorities at once.
func setStatus(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	t, b, res, err := target(ctx, api, req, "todo_id")
	if res != nil || err != nil {
		return res, err
	}
	status := strParam(req, b, "status")
	if !slices.Contains(statuses, status) {
		return plugin.Invalid(map[string][]string{"status": {"is not included in the list"}}), nil
	}
	rec, res, err := change(ctx, api, plugin.WorkChange{ID: t.ID, Status: &status})
	if res != nil || err != nil {
		return res, err
	}
	now := str(rec["status"])
	if now == "" {
		now = status
	}
	if now == "done" {
		r, err := api.Exec(ctx, `DELETE FROM my_day_pins WHERE todo_id = ?`, t.ID)
		if err != nil {
			return nil, err
		}
		if n, _ := r.RowsAffected(); n > 0 {
			return done("“" + t.Title + "” is done, and off your priorities."), nil
		}
	}
	return done("“" + t.Title + "” is " + strings.ToLower(humanize(now)) + "."), nil
}

// updateTodo changes the due date or details (TodosController#update).
func updateTodo(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	t, b, res, err := target(ctx, api, req, "id")
	if res != nil || err != nil {
		return res, err
	}
	fields, _ := b["todo"].(map[string]any)
	w := plugin.WorkChange{ID: t.ID}
	if v, ok := fields["due_on"]; ok {
		s, _ := v.(string)
		w.DueOn = &s
	}
	if v, ok := fields["description"]; ok {
		s, _ := v.(string)
		w.Description = &s
	}
	if w.DueOn == nil && w.Description == nil {
		return badRequest(`Send {"todo": {"due_on": "YYYY-MM-DD", "description": "…"}}.`), nil
	}
	if _, res, err := change(ctx, api, w); res != nil || err != nil {
		return res, err
	}
	return done("“" + t.Title + "” saved."), nil
}

// setPeople is who's on it, from the card: owner_id leads (0 for nobody),
// other_owner_ids are on it too.
func setPeople(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	t, b, res, err := target(ctx, api, req, "todo_id")
	if res != nil || err != nil {
		return res, err
	}
	owner := intParam(req, b, "owner_id")
	others := []int64{}
	if list, ok := b["other_owner_ids"].([]any); ok {
		for _, v := range list {
			if n, ok := v.(float64); ok && int64(n) != owner && !slices.Contains(others, int64(n)) {
				others = append(others, int64(n))
			}
		}
	}
	if _, res, err := change(ctx, api, plugin.WorkChange{ID: t.ID, OwnerID: &owner, OtherOwnerIDs: &others}); res != nil || err != nil {
		return res, err
	}
	return done("“" + t.Title + "” saved."), nil
}
