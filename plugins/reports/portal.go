package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// A client's progress report: what they see on their portal's "Your
// progress" page, and what the owner previews from the client's page. Only
// what's theirs and meant for them: engagements that aren't drafts, the
// work shared with them (client-visible), their requests, the commitments
// we made them; never the health score, notes, internal work or anyone
// else. The owner chooses the sections (money off unless turned on), and
// can switch it off for one client.

// todoRec is a piece of work as Records answers it (the fields used here).
type todoRec struct {
	ID            int64      `json:"id"`
	Title         string     `json:"title"`
	Status        string     `json:"status"`
	DueOn         string     `json:"due_on"`
	ClientVisible bool       `json:"client_visible"`
	CompletedAt   *time.Time `json:"completed_at"`
	Engagement    struct {
		Ref string `json:"ref"`
	} `json:"engagement"`
}

type commitmentRec struct {
	ID          int64  `json:"id"`
	Description string `json:"description"`
	DueOn       string `json:"due_on"`
	Owner       string `json:"owner"`
	OwnerName   string `json:"owner_name"`
	Open        bool   `json:"open"`
	Overdue     bool   `json:"overdue"`
	Engagement  *struct {
		Ref string `json:"ref"`
	} `json:"engagement"`
}

// decodeRecords reads Records' answer into typed records.
func decodeRecords(list []map[string]any, into any) error {
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, into)
}

// hiddenFor is whether a client's report is switched off.
func hiddenFor(ctx context.Context, api *plugin.API, clientID int64) (bool, error) {
	var id int64
	err := api.QueryRow(ctx, `SELECT client_id FROM reports_portal_hidden WHERE client_id = ?`, clientID).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// stateWords is an engagement's state as the client reads it.
func stateWords(e plugin.EngagementMetrics) string {
	if e.ClosedAt != nil {
		return "Finished"
	}
	switch e.State {
	case "sent":
		return "Waiting for your go-ahead"
	case "changes_requested":
		return "Being revised"
	case "approved":
		return "Under way"
	}
	return "Under way"
}

const reportMonths = 12

// clientReport builds the report a client sees, with only the sections
// given.
func buildClientReport(ctx context.Context, api *plugin.API, clientID int64, sections []string) (map[string]any, error) {
	r, err := build(ctx, api, filter{Status: "all", Health: "all"}, clientID)
	if err != nil {
		return nil, err
	}
	c := r.byID[clientID]
	if c == nil {
		return nil, plugin.ErrNoRecord
	}
	on := func(s string) bool { return slices.Contains(sections, s) }
	yearAgo := r.now.AddDate(0, -reportMonths, 0)

	// The engagements they see (not drafts), newest work first, and the
	// work shared with them under each.
	engs := portalEngagements(c, yearAgo)
	work := map[int64][]todoRec{}
	var shared []todoRec
	for _, e := range engs {
		list, err := api.Records(ctx, plugin.Query{Type: "Todo", EngagementID: e.ID, Limit: 500})
		if err != nil {
			return nil, err
		}
		var todos []todoRec
		if err := decodeRecords(list, &todos); err != nil {
			return nil, err
		}
		for _, t := range todos {
			if t.ClientVisible {
				work[e.ID] = append(work[e.ID], t)
				shared = append(shared, t)
			}
		}
	}

	out := map[string]any{
		"client": map[string]any{"id": c.ID, "name": c.Name}, "from": firstOf(r.site.Name, "us"), "today": r.m.Today,
		"generated_at": time.Now().UTC(), "sections": sections, "prices": r.m.Prices,
	}
	var summary []string

	if on("engagements") || on("money") {
		list := engagementItems(r, engs, work, on)
		if on("engagements") {
			out["engagements"] = list
			active := 0
			for _, e := range engs {
				if e.ClosedAt == nil {
					active++
				}
			}
			summary = append(summary, plural(active, r.words.engagement1, r.words.engagementN)+" under way")
		}
		if on("money") && r.m.Prices {
			out["money"] = agreedMoney(engs)
		}
	}

	if on("work_done") {
		o, total := workDone(r, shared)
		out["work_done"] = o
		summary = append(summary, plural(total, r.words.work1, r.words.workN)+" done in a year")
	}

	if on("on_time") {
		o, line := onTimeStats(shared, r, yearAgo)
		if line != "" {
			summary = append(summary, line)
		}
		out["on_time"] = o
	}

	if on("requests") {
		out["requests"] = requestStats(r, c, clientID, yearAgo)
	}

	var commitments []commitmentRec
	if on("commitments") || on("next") {
		list, err := api.Records(ctx, plugin.Query{Type: "Commitment", ClientID: clientID, Limit: 500})
		if err != nil {
			return nil, err
		}
		if err := decodeRecords(list, &commitments); err != nil {
			return nil, err
		}
	}
	if on("commitments") {
		out["commitments"] = commitmentStats(r, commitments, yearAgo)
	}

	if on("next") {
		out["next"] = nextSteps(r, shared, commitments, engs)
	}

	s := c.Name + "'s progress"
	if len(summary) > 0 {
		s += ": " + strings.Join(summary, ", ")
	}
	out["summary"] = s + "."
	return out, nil
}

// portalEngagements are the engagements a client sees (not drafts, nor
// closed over a year ago), open ones first, then newest first.
func portalEngagements(c *clientRow, yearAgo time.Time) []plugin.EngagementMetrics {
	var engs []plugin.EngagementMetrics
	for _, e := range c.engagements {
		if e.State == "draft" || e.State == "internal" {
			continue
		}
		if e.ClosedAt != nil && e.ClosedAt.Before(yearAgo) {
			continue
		}
		engs = append(engs, e)
	}
	sort.SliceStable(engs, func(i, j int) bool {
		if (engs[i].ClosedAt == nil) != (engs[j].ClosedAt == nil) {
			return engs[i].ClosedAt == nil
		}
		return engs[i].CreatedAt.After(engs[j].CreatedAt)
	})
	return engs
}

// engagementItems are the engagements the report shows, each with its
// shared work's progress.
func engagementItems(r *report, engs []plugin.EngagementMetrics, work map[int64][]todoRec, on func(string) bool) []map[string]any {
	list := []map[string]any{}
	for _, e := range engs {
		done, open, late := 0, 0, 0
		for _, t := range work[e.ID] {
			switch t.Status {
			case "done":
				done++
			default:
				open++
				if t.DueOn != "" && t.DueOn < r.m.Today {
					late++
				}
			}
		}
		progress := 0
		if done+open > 0 {
			progress = done * 100 / (done + open)
		} else if e.ClosedAt != nil {
			progress = 100
		}
		item := map[string]any{"ref": e.Ref, "title": e.Title, "label": e.Label, "state": e.State, "state_words": stateWords(e),
			"closed": e.ClosedAt != nil, "done": done, "open": open, "late": late, "progress": progress, "path": "/portal/engagements/" + e.Ref}
		if on("money") && r.m.Prices && e.State != "sent" {
			item["agreed_cents"], item["shape"], item["cadence"] = e.AgreedCents, e.Shape, e.Cadence
		}
		list = append(list, item)
	}
	return list
}

// agreedMoney is what's agreed on the open engagements, and monthly.
func agreedMoney(engs []plugin.EngagementMetrics) map[string]any {
	var total, monthly int64
	by := []map[string]any{}
	for _, e := range engs {
		if e.ClosedAt != nil || e.State == "sent" || e.AgreedCents == 0 {
			continue
		}
		total += e.AgreedCents
		if e.Shape == "recurring" {
			monthly += perMonth(e.AgreedCents, e.Cadence)
		}
		by = append(by, map[string]any{"ref": e.Ref, "title": e.Title, "agreed_cents": e.AgreedCents, "shape": e.Shape, "cadence": e.Cadence})
	}
	return map[string]any{"agreed_cents": total, "monthly_recurring_cents": monthly, "by_engagement": by}
}

// workDone is the shared work done each month of the year, and in all.
func workDone(r *report, shared []todoRec) (map[string]any, int) {
	months := r.months(reportMonths)
	idx := map[string]int{}
	by := make([]map[string]any, len(months))
	for i, m := range months {
		idx[m] = i
		by[i] = map[string]any{"month": m, "done": 0}
	}
	total := 0
	for _, t := range shared {
		if t.Status != "done" || t.CompletedAt == nil {
			continue
		}
		if i, ok := idx[t.CompletedAt.In(r.loc).Format("2006-01")]; ok {
			by[i]["done"] = by[i]["done"].(int) + 1 //nolint:forcetypeassert,errcheck // the report's own map, its types known
			total++
		}
	}
	return map[string]any{"months": by, "total": total}, total
}

// onTimeStats is how much shared work was done by its due date, and
// that as a line of the summary ("" when nothing had one).
func onTimeStats(shared []todoRec, r *report, yearAgo time.Time) (o map[string]any, line string) {
	withDue, onTime := 0, 0
	for _, t := range shared {
		if t.Status != "done" || t.CompletedAt == nil || t.DueOn == "" || t.CompletedAt.Before(yearAgo) {
			continue
		}
		withDue++
		if t.CompletedAt.In(r.loc).Format(time.DateOnly) <= t.DueOn {
			onTime++
		}
	}
	o = map[string]any{"with_due": withDue, "on_time": onTime, "rate": nil}
	if withDue > 0 {
		rate := math.Round(float64(onTime) / float64(withDue) * 100)
		o["rate"] = rate
		line = fmt.Sprintf("%v%% on time", rate)
	}
	return o, line
}

// requestStats is the client's requests this year, and how fast they're
// answered.
func requestStats(r *report, c *clientRow, clientID int64, yearAgo time.Time) map[string]any {
	received, answered := 0, 0
	var hours []float64
	for _, f := range r.m.Facts {
		if f.ClientID != clientID || f.At.Before(yearAgo) {
			continue
		}
		switch f.Kind {
		case "request.received":
			received++
		case "request.triaged":
			answered++
			if f.Start != nil {
				hours = append(hours, f.At.Sub(*f.Start).Hours())
			}
		}
	}
	o := map[string]any{"received": received, "answered": answered, "open": c.OpenRequests, "typical_hours": nil}
	if len(hours) > 0 {
		sort.Float64s(hours)
		o["typical_hours"] = math.Round(hours[len(hours)/2]*10) / 10
	}
	return o
}

// ours is whether a commitment is the team's (not the client's).
func ours(cm commitmentRec) bool { return cm.Owner == "us" || strings.HasPrefix(cm.Owner, "user:") }

// commitmentStats is how the team's commitments have been kept this year.
func commitmentStats(r *report, commitments []commitmentRec, yearAgo time.Time) map[string]any {
	mine := map[int64]bool{}
	open := 0
	for _, cm := range commitments {
		if ours(cm) {
			mine[cm.ID] = true
			if cm.Open {
				open++
			}
		}
	}
	kept, onTime, missed := 0, 0, 0
	for _, f := range r.m.Facts {
		if !mine[f.ID] || f.At.Before(yearAgo) {
			continue
		}
		switch f.Kind {
		case "commitment.done":
			kept++
			if f.Due == "" || f.At.In(r.loc).Format(time.DateOnly) <= f.Due {
				onTime++
			}
		case "commitment.missed":
			missed++
		}
	}
	o := map[string]any{"kept": kept, "kept_on_time": onTime, "missed": missed, "open": open, "rate": nil}
	if kept+missed > 0 {
		o["rate"] = math.Round(float64(kept) / float64(kept+missed) * 100)
	}
	return o
}

// nextSteps is what's coming: the next shared work, what each side has
// promised, and what the client has to decide.
func nextSteps(r *report, shared []todoRec, commitments []commitmentRec, engs []plugin.EngagementMetrics) map[string]any {
	var upcoming []todoRec
	for _, t := range shared {
		if t.Status != "done" {
			upcoming = append(upcoming, t)
		}
	}
	sort.SliceStable(upcoming, func(i, j int) bool {
		a, b := upcoming[i].DueOn, upcoming[j].DueOn
		if (a == "") != (b == "") {
			return b == ""
		}
		return a < b
	})
	workList := []map[string]any{}
	for _, t := range upcoming[:min(8, len(upcoming))] {
		workList = append(workList, map[string]any{"title": t.Title, "status": t.Status, "due_on": nilIf(t.DueOn), "late": t.DueOn != "" && t.DueOn < r.m.Today,
			"engagement": t.Engagement.Ref, "path": "/portal/engagements/" + t.Engagement.Ref})
	}
	promised, waiting := []map[string]any{}, []map[string]any{}
	for _, cm := range commitments {
		if !cm.Open {
			continue
		}
		item := map[string]any{"description": cm.Description, "due_on": nilIf(cm.DueOn), "late": cm.Overdue}
		if ours(cm) {
			promised = append(promised, item)
		} else {
			waiting = append(waiting, item)
		}
	}
	decide := []map[string]any{}
	for _, e := range engs {
		if e.ClosedAt == nil && e.State == "sent" {
			decide = append(decide, map[string]any{"ref": e.Ref, "title": e.Title, "path": "/portal/engagements/" + e.Ref})
		}
	}
	return map[string]any{"work": workList, "more_work": max(0, len(upcoming)-len(workList)), "promised": promised[:min(6, len(promised))],
		"waiting_on_you": waiting[:min(6, len(waiting))], "to_decide": decide}
}

func nilIf(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// portalStatus says whether the contact's portal shows the report (its
// link in the portal's nav).
func portalStatus(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	cfg := portalConfig(ctx, api)
	hidden, err := hiddenFor(ctx, api, req.Contact.ClientID)
	if err != nil {
		return nil, err
	}
	return plugin.JSON(200, map[string]any{"available": cfg.On && !hidden && len(cfg.Sections) > 0, "label": "Your progress"}), nil
}

// portalReport is the contact's own client's report.
func portalReport(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	cfg := portalConfig(ctx, api)
	hidden, err := hiddenFor(ctx, api, req.Contact.ClientID)
	if err != nil {
		return nil, err
	}
	if !cfg.On || hidden || len(cfg.Sections) == 0 {
		return plugin.JSON(404, map[string]any{"error": "not_found", "message": "There's no progress report here."}), nil
	}
	out, err := buildClientReport(ctx, api, req.Contact.ClientID, cfg.Sections)
	if err != nil {
		if r := plugin.Answer(err); r != nil {
			return r, nil
		}
		return nil, err
	}
	return plugin.JSON(200, out), nil
}

// clientReport is the owner's preview (and the agent's show_client_report):
// the report exactly as the client sees it, and whether they do.
func clientReport(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	id, err := pathID(req)
	if err != nil {
		return plugin.Answer(err), nil
	}
	cfg := portalConfig(ctx, api)
	hidden, err := hiddenFor(ctx, api, id)
	if err != nil {
		return nil, err
	}
	out, err := buildClientReport(ctx, api, id, cfg.Sections)
	if err != nil {
		if r := plugin.Answer(err); r != nil {
			return r, nil
		}
		return nil, err
	}
	portal := api.Grants().Portal
	seen := cfg.On && !hidden && portal && len(cfg.Sections) > 0
	why := ""
	switch {
	case !portal:
		why = "Reports may not serve clients in their portal: tick “Serve clients in their portal” for Reports in Settings › Plugins."
	case !cfg.On || len(cfg.Sections) == 0:
		why = "Clients don't see their progress report: it's off in Settings › Reports."
	case hidden:
		why = "Switched off for this client."
	}
	out["visible"] = map[string]any{"seen": seen, "page_on": cfg.On, "hidden": hidden, "portal_granted": portal, "why": nilIf(why)}
	if !seen {
		out["summary"] = out["summary"].(string) + " They don't see it now: " + strings.TrimSuffix(why, ".") + "." //nolint:forcetypeassert,errcheck // the report's own map, its types known
	}
	return plugin.JSON(200, out), nil
}

// setClientReport switches the report off (or on again) for one client:
// {hidden: true}.
func setClientReport(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	id, err := pathID(req)
	if err != nil {
		return plugin.Answer(err), nil
	}
	client, err := api.Record(ctx, fmt.Sprintf("Client:%d", id))
	if err != nil {
		if r := plugin.Answer(err); r != nil {
			return r, nil
		}
		return nil, err
	}
	var in struct {
		Hidden bool `json:"hidden"`
	}
	if r := decodeBody(req, "client_report", &in); r != nil {
		return r, nil
	}
	name, _ := client["name"].(string)
	if in.Hidden {
		who := ""
		if req.Actor != nil {
			who = req.Actor.Name
		}
		if _, err := api.Exec(ctx, `INSERT INTO reports_portal_hidden (client_id, hidden_by, updated_at) VALUES (?, ?, ?) ON CONFLICT (client_id) DO UPDATE SET hidden_by = excluded.hidden_by, updated_at = excluded.updated_at`, id, who, now()); err != nil {
			return nil, err
		}
		return plugin.JSON(200, map[string]any{"summary": name + " no longer sees their progress report.", "hidden": true}), nil
	}
	if _, err := api.Exec(ctx, `DELETE FROM reports_portal_hidden WHERE client_id = ?`, id); err != nil {
		return nil, err
	}
	return plugin.JSON(200, map[string]any{"summary": name + " sees their progress report in their portal again.", "hidden": false}), nil
}
