package reports

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// The report: every client with its place, health and money, and the
// snapshot's sections (money, delivery, reach), for the clients the filters
// choose. The page reads it whole (/overview); each agent tool answers its
// part.

// filter is which clients the report covers.
type filter struct {
	Status string `json:"status"` // active (default), dormant, former, all
	Health string `json:"health"` // all (default), on_track, watch, at_risk, inactive
	Type   string `json:"type"`   // "" or an engagement label: project, work_order, service
}

var (
	statuses = []string{"active", "dormant", "former", "all"}
	bands    = []string{"all", "on_track", "watch", "at_risk", "inactive"}
	labels   = []string{"project", "work_order", "service"}
)

func filterOf(q url.Values) filter {
	f := filter{Status: q.Get("status"), Health: q.Get("health"), Type: q.Get("type")}
	if !slices.Contains(statuses, f.Status) {
		f.Status = "active"
	}
	if !slices.Contains(bands, f.Health) {
		f.Health = "all"
	}
	if !slices.Contains(labels, f.Type) {
		f.Type = ""
	}
	return f
}

// clientRow is one client in the report.
type clientRow struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Status    string    `json:"status"`
	Internal  bool      `json:"internal"`
	URL       string    `json:"url"`
	CreatedAt time.Time `json:"created_at"`
	Place     Place     `json:"place"`
	// TimeZone is the client's own zone ("" when it shares ours); IANA is
	// the zone the app tells their time in.
	TimeZone string `json:"time_zone"`
	IANA     string `json:"iana"`
	// Address is their address as last looked up, and how that went (ok,
	// not_found, error); "" when they have none.
	Address       string `json:"address"`
	AddressStatus string `json:"address_status"`
	Health        Health `json:"health"`
	// Money: agreed on open engagements (a recurring one's amount per
	// period), approved this calendar year, waiting on them, in drafts.
	AgreedCents   int64    `json:"agreed_cents"`
	ThisYearCents int64    `json:"this_year_cents"`
	PendingCents  int64    `json:"pending_cents"`
	Labels        []string `json:"labels"`
	Engagements   int      `json:"engagements"` // open ones
	OpenWork      int      `json:"open_work"`
	LateWork      int      `json:"late_work"`
	DueSoonWork   int      `json:"due_soon_work"`
	OpenRequests  int      `json:"open_requests"`
	Commitments   int      `json:"open_commitments"`
	Overdue       int      `json:"overdue_commitments"`
	DueSoon       int      `json:"due_soon_commitments"`
	Waiting       int      `json:"waiting_approvals"`
	Contacts      int      `json:"contacts"`
	Portal        int      `json:"portal_contacts"`
	// LastTouchAt is the latest note, work done, decision or request;
	// Days since it (-1: never).
	LastTouchAt   *time.Time `json:"last_touch_at"`
	LastTouchKind string     `json:"last_touch_kind"`
	DaysSince     int        `json:"days_since_touch"`
	// Live: something happened in the last three days, or a request waits.
	Live bool `json:"live"`
	// Activity is the last 12 weeks, oldest first: work done, notes,
	// decisions and requests, a week at a time.
	Activity []int `json:"activity"`

	engagements []plugin.EngagementMetrics
}

type report struct {
	site   plugin.Site
	loc    *time.Location
	now    time.Time
	words  words
	m      *plugin.Metrics
	home   Home
	filter filter
	all    []*clientRow
	rows   []*clientRow // the ones the filter keeps
	byID   map[int64]*clientRow
}

const activityWeeks = 12

// build reads the numbers and works the report out.
func build(ctx context.Context, api *plugin.API, f filter, clientID int64) (*report, error) {
	site, err := api.Site(ctx)
	if err != nil {
		return nil, err
	}
	loc := site.Location
	if loc == nil {
		loc = time.UTC
	}
	now := time.Now().In(loc)
	m, err := api.Metrics(ctx, plugin.MetricsQuery{Since: now.AddDate(0, -13, 0), ClientID: clientID})
	if err != nil {
		return nil, err
	}
	set, err := places(ctx, api)
	if err != nil {
		return nil, err
	}
	addrs, err := addressPlaces(ctx, api)
	if err != nil {
		return nil, err
	}
	all := m
	if clientID != 0 {
		// Home is the internal client, which one client's numbers leave out.
		if all, err = api.Metrics(ctx, plugin.MetricsQuery{Since: now.AddDate(0, 0, -1)}); err != nil {
			return nil, err
		}
	}
	r := &report{site: site, loc: loc, now: now, words: wordsOf(site), m: m, home: homeOf(ctx, api, site, all.Clients, set, addrs), filter: f, byID: map[int64]*clientRow{}}

	engs := map[int64][]plugin.EngagementMetrics{}
	for _, e := range m.Engagements {
		engs[e.ClientID] = append(engs[e.ClientID], e)
	}
	mo, activity, thisYear, lively := clientFacts(m.Facts, now, loc)

	// Where each is: set by hand, else its address, else its zone's city
	// (ours when it has none).
	approx := map[[2]float64]int{}
	for _, c := range m.Clients {
		r.all = append(r.all, &clientRow{ID: c.ID})
		if p, ok := placeFor(c.ID, zoneOr(c.TimeZone, site.TimeZone), set, addrs); ok && p.Approximate {
			approx[[2]float64{p.Lat, p.Lng}]++
		}
	}
	for i, c := range m.Clients {
		row := r.all[i]
		*row = clientRow{ID: c.ID, Name: c.Name, Status: c.Status, Internal: c.Internal, URL: fmt.Sprintf("/clients/%d", c.ID), CreatedAt: c.CreatedAt,
			TimeZone: c.TimeZone, IANA: ianaOf(zoneOr(c.TimeZone, site.TimeZone)), Labels: []string{},
			OpenWork: c.OpenWork, LateWork: c.LateWork, DueSoonWork: c.DueSoonWork, OpenRequests: c.OpenRequests,
			Commitments: c.OpenCommitments, Overdue: c.OverdueOurs + c.OverdueTheirs, DueSoon: c.DueSoonCommitments, Waiting: c.WaitingApprovals,
			Contacts: c.Contacts, Portal: c.PortalContacts, LastTouchAt: c.LastTouchAt, LastTouchKind: c.LastTouchKind, DaysSince: daysSince(c.LastTouchAt, now),
			Live: lively[c.ID] || c.OpenRequests > 0, Activity: activity[c.ID], ThisYearCents: thisYear[c.ID], engagements: engs[c.ID]}
		if row.Activity == nil {
			row.Activity = make([]int, activityWeeks)
		}
		if p, ok := placeFor(c.ID, zoneOr(c.TimeZone, site.TimeZone), set, addrs); ok {
			if p.Approximate {
				p = spread(p, c.ID, approx[[2]float64{p.Lat, p.Lng}])
			}
			row.Place = p
		}
		if a, ok := addrs[c.ID]; ok {
			row.Address, row.AddressStatus = a.Address, a.Status
		}
		var m momentum
		if mm := mo[c.ID]; mm != nil {
			m = *mm
		}
		row.Health = healthOf(c, m, now, r.words)
		for _, e := range engs[c.ID] {
			if e.ClosedAt != nil {
				continue
			}
			row.Engagements++
			row.AgreedCents += e.AgreedCents
			row.PendingCents += e.PendingCents
			if !slices.Contains(row.Labels, e.Label) {
				row.Labels = append(row.Labels, e.Label)
			}
		}
		r.byID[c.ID] = row
	}
	for _, row := range r.all {
		if r.keeps(row) {
			r.rows = append(r.rows, row)
		}
	}
	// At risk first, then by score, then by name.
	sort.SliceStable(r.rows, func(i, j int) bool {
		a, b := r.rows[i], r.rows[j]
		if (a.Health.Band == "inactive") != (b.Health.Band == "inactive") {
			return b.Health.Band == "inactive"
		}
		if a.Health.Score != b.Health.Score {
			return a.Health.Score < b.Health.Score
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	if r.rows == nil {
		r.rows = []*clientRow{}
	}
	return r, nil
}

// clientFacts is the facts by client: work done in the last 4 weeks and
// the 4 before, activity by the week, this year's approvals, and whether
// there's been any in the last three days.
func clientFacts(facts []plugin.Fact, now time.Time, loc *time.Location) (mo map[int64]*momentum, activity map[int64][]int, thisYear map[int64]int64, lively map[int64]bool) {
	year := time.Date(now.Year(), 1, 1, 0, 0, 0, 0, loc)
	monday := startOfWeek(now)
	first := monday.AddDate(0, 0, -7*(activityWeeks-1))
	recent, before := now.AddDate(0, 0, -28), now.AddDate(0, 0, -56)
	live := now.Add(-72 * time.Hour)
	mo, activity, thisYear, lively = map[int64]*momentum{}, map[int64][]int{}, map[int64]int64{}, map[int64]bool{}
	for _, fc := range facts {
		if fc.ClientID == 0 {
			continue
		}
		at := fc.At.In(loc)
		switch fc.Kind {
		case "todo.done":
			if mo[fc.ClientID] == nil {
				mo[fc.ClientID] = &momentum{}
			}
			if at.After(recent) {
				mo[fc.ClientID].recent++
			} else if at.After(before) {
				mo[fc.ClientID].before++
			}
		case "agreement.approved":
			if !at.Before(year) {
				thisYear[fc.ClientID] += fc.Cents
			}
		}
		switch fc.Kind {
		case "todo.done", "note.added", "agreement.approved", "agreement.changes_requested", "request.received", "commitment.done":
			if i := int(at.Sub(first).Hours() / (24 * 7)); i >= 0 && i < activityWeeks && !at.Before(first) {
				if activity[fc.ClientID] == nil {
					activity[fc.ClientID] = make([]int, activityWeeks)
				}
				activity[fc.ClientID][i]++
			}
			if at.After(live) {
				lively[fc.ClientID] = true
			}
		}
	}
	return mo, activity, thisYear, lively
}

func zoneOr(zone, fallback string) string {
	if zone != "" {
		return zone
	}
	return fallback
}

func startOfWeek(t time.Time) time.Time {
	d := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location())
	return d.AddDate(0, 0, -((int(d.Weekday()) + 6) % 7))
}

func (r *report) keeps(c *clientRow) bool {
	f := r.filter
	// The business itself is home, not one of the clients it serves.
	if c.Internal {
		return false
	}
	if f.Status != "all" && c.Status != f.Status {
		return false
	}
	if f.Health != "all" && c.Health.Band != f.Health {
		return false
	}
	if f.Type != "" && !slices.ContainsFunc(c.engagements, func(e plugin.EngagementMetrics) bool { return e.Label == f.Type }) {
		return false
	}
	return true
}

// kept is whether a client is in the report.
func (r *report) kept(id int64) bool {
	c := r.byID[id]
	return c != nil && slices.Contains(r.rows, c)
}

// ---- the headline and totals ------------------------------------------

func (r *report) totals() map[string]any {
	countries, zones := map[string]bool{}, map[int]bool{}
	var agreed, year, monthly int64
	var open, late, requests, waiting, approx int
	bandsN := map[string]int{}
	for _, c := range r.rows {
		if c.Place.CountryCode != "" {
			countries[c.Place.CountryCode] = true
		}
		if loc, err := time.LoadLocation(c.IANA); err == nil {
			_, off := r.now.In(loc).Zone()
			zones[off] = true
		}
		agreed += c.AgreedCents
		year += c.ThisYearCents
		open += c.OpenWork
		late += c.LateWork
		requests += c.OpenRequests
		waiting += c.Waiting
		bandsN[c.Health.Band]++
		if c.Place.Approximate {
			approx++
		}
		for _, e := range c.engagements {
			if e.ClosedAt == nil && e.Shape == "recurring" {
				monthly += perMonth(e.AgreedCents, e.Cadence)
			}
		}
	}
	return map[string]any{
		"clients": len(r.rows), "countries": len(countries), "time_zones": len(zones),
		"agreed_cents": agreed, "this_year_cents": year, "monthly_recurring_cents": monthly,
		"open_work": open, "late_work": late, "open_requests": requests, "waiting_approvals": waiting,
		"on_track": bandsN["on_track"], "watch": bandsN["watch"], "at_risk": bandsN["at_risk"], "inactive": bandsN["inactive"],
		"approximate": approx, "placed": len(r.rows) - approx,
	}
}

// perMonth is a recurring amount a month.
func perMonth(cents int64, cadence string) int64 {
	switch cadence {
	case "weekly":
		return int64(math.Round(float64(cents) * 52 / 12))
	case "quarterly":
		return int64(math.Round(float64(cents) / 3))
	case "yearly", "annual", "annually":
		return int64(math.Round(float64(cents) / 12))
	}
	return cents
}

// headline is the report in a sentence: "You serve 12 clients in 5
// countries across 4 time zones."
func (r *report) headline(t map[string]any) string {
	n := t["clients"].(int) //nolint:forcetypeassert,errcheck // the report's own map, its types known
	if n == 0 {
		return "No " + r.words.clientN + " match these filters."
	}
	s := "You serve " + plural(n, r.words.client1, r.words.clientN)
	if c := t["countries"].(int); c > 0 { //nolint:forcetypeassert,errcheck // the report's own map, its types known
		s += " in " + plural(c, "country", "countries")
	}
	if z := t["time_zones"].(int); z > 1 { //nolint:forcetypeassert,errcheck // the report's own map, its types known
		s += fmt.Sprintf(" across %d time zones", z)
	}
	return s
}

func (r *report) filterOptions() map[string]any {
	count := func(keep func(c *clientRow) bool) int {
		n := 0
		for _, c := range r.all {
			if !c.Internal && keep(c) {
				n++
			}
		}
		return n
	}
	status := []map[string]any{}
	for _, s := range statuses {
		label := map[string]string{"active": "Active", "dormant": "Dormant", "former": "Former", "all": "All statuses"}[s]
		status = append(status, map[string]any{"value": s, "label": label, "count": count(func(c *clientRow) bool { return s == "all" || c.Status == s })})
	}
	health := []map[string]any{}
	for _, b := range bands {
		label := "Any health"
		if b != "all" {
			label = bandLabels[b]
		}
		health = append(health, map[string]any{"value": b, "label": label, "count": count(func(c *clientRow) bool { return b == "all" || c.Health.Band == b })})
	}
	types := []map[string]any{}
	for _, l := range labels {
		n := count(func(c *clientRow) bool {
			return slices.ContainsFunc(c.engagements, func(e plugin.EngagementMetrics) bool { return e.Label == l })
		})
		if n > 0 {
			types = append(types, map[string]any{"value": l, "count": n})
		}
	}
	return map[string]any{"status": status, "health": health, "type": types}
}

// ---- money ------------------------------------------------------------

func (r *report) money() map[string]any {
	if !r.m.Prices {
		return map[string]any{"hidden": true, "summary": "Prices are hidden on this install, so there's no money to show."}
	}
	months := r.months(12)
	byMonth := make([]map[string]any, len(months))
	idx := map[string]int{}
	for i, mth := range months {
		idx[mth] = i
		byMonth[i] = map[string]any{"month": mth, "approved_cents": int64(0), "sent_cents": int64(0), "approvals": 0}
	}
	for _, f := range r.m.Facts {
		if !r.kept(f.ClientID) {
			continue
		}
		i, ok := idx[f.At.In(r.loc).Format("2006-01")]
		if !ok {
			continue
		}
		switch f.Kind {
		case "agreement.approved":
			byMonth[i]["approved_cents"] = byMonth[i]["approved_cents"].(int64) + f.Cents //nolint:forcetypeassert,errcheck // the report's own map, its types known
			byMonth[i]["approvals"] = byMonth[i]["approvals"].(int) + 1                   //nolint:forcetypeassert,errcheck // the report's own map, its types known
		case "agreement.sent":
			byMonth[i]["sent_cents"] = byMonth[i]["sent_cents"].(int64) + f.Cents //nolint:forcetypeassert,errcheck // the report's own map, its types known
		}
	}

	type amount struct {
		Cents int64 `json:"cents"`
		Count int   `json:"count"`
	}
	byLabel := map[string]*amount{}
	pipeline := map[string]*amount{"draft": {}, "sent": {}, "changes_requested": {}}
	var recurring, fixed amount
	var monthly int64
	cadences := map[string]*amount{}
	clients := []map[string]any{}
	var agreed, year int64
	for _, c := range r.rows {
		agreed += c.AgreedCents
		year += c.ThisYearCents
		if c.AgreedCents > 0 || c.ThisYearCents > 0 || c.PendingCents > 0 {
			clients = append(clients, map[string]any{"id": c.ID, "name": c.Name, "url": c.URL, "agreed_cents": c.AgreedCents, "this_year_cents": c.ThisYearCents, "pending_cents": c.PendingCents})
		}
		for _, e := range c.engagements {
			if e.ClosedAt != nil {
				continue
			}
			if p := pipeline[e.State]; p != nil {
				p.Count++
				p.Cents += e.DraftCents + e.PendingCents
			}
			if e.AgreedCents == 0 && e.State != "approved" {
				continue
			}
			if byLabel[e.Label] == nil {
				byLabel[e.Label] = &amount{}
			}
			byLabel[e.Label].Cents += e.AgreedCents
			byLabel[e.Label].Count++
			if e.Shape == "recurring" {
				recurring.Cents += e.AgreedCents
				recurring.Count++
				monthly += perMonth(e.AgreedCents, e.Cadence)
				cad := e.Cadence
				if cad == "" {
					cad = "monthly"
				}
				if cadences[cad] == nil {
					cadences[cad] = &amount{}
				}
				cadences[cad].Cents += e.AgreedCents
				cadences[cad].Count++
			} else {
				fixed.Cents += e.AgreedCents
				fixed.Count++
			}
		}
	}
	sort.SliceStable(clients, func(i, j int) bool {
		a, b := clients[i]["agreed_cents"].(int64), clients[j]["agreed_cents"].(int64) //nolint:forcetypeassert,errcheck // the report's own map, its types known
		if a != b {
			return a > b
		}
		return clients[i]["this_year_cents"].(int64) > clients[j]["this_year_cents"].(int64) //nolint:forcetypeassert,errcheck // the report's own map, its types known
	})
	if len(clients) > 10 {
		clients = clients[:10]
	}
	labelList := []map[string]any{}
	for _, l := range labels {
		if a := byLabel[l]; a != nil {
			labelList = append(labelList, map[string]any{"label": l, "cents": a.Cents, "count": a.Count})
		}
	}
	return map[string]any{
		"agreed_cents": agreed, "this_year_cents": year, "monthly_recurring_cents": monthly,
		"by_month": byMonth, "by_client": clients, "by_label": labelList,
		"pipeline": pipeline, "recurring": recurring, "fixed": fixed, "cadences": cadences,
	}
}

// months are the last n months, oldest first, "2026-10".
func (r *report) months(n int) []string {
	first := time.Date(r.now.Year(), r.now.Month(), 1, 0, 0, 0, 0, r.loc).AddDate(0, -(n - 1), 0)
	out := make([]string, n)
	for i := range out {
		out[i] = first.AddDate(0, i, 0).Format("2006-01")
	}
	return out
}

// ---- delivery -----------------------------------------------------------

const deliveryWeeks = 26

func (r *report) delivery() map[string]any {
	first := startOfWeek(r.now).AddDate(0, 0, -7*(deliveryWeeks-1))
	type week struct {
		Week     string `json:"week"`
		Added    int    `json:"added"`
		Done     int    `json:"done"`
		Requests int    `json:"requests"`
		Triaged  int    `json:"triaged"`
	}
	weeks := make([]week, deliveryWeeks)
	for i := range weeks {
		weeks[i].Week = first.AddDate(0, 0, 7*i).Format(time.DateOnly)
	}
	var withDue, onTime, approvals, triaged int
	var approvalHours, triageHours float64
	for _, f := range r.m.Facts {
		// Requests from no client yet count only when nothing narrows the
		// report: they're everyone's.
		if f.ClientID == 0 {
			if !strings.HasPrefix(f.Kind, "request.") || r.filter != (filter{Status: "active", Health: "all"}) {
				continue
			}
		} else if !r.kept(f.ClientID) {
			continue
		}
		at := f.At.In(r.loc)
		i := int(at.Sub(first).Hours() / (24 * 7))
		inWindow := !at.Before(first) && i >= 0 && i < deliveryWeeks
		switch f.Kind {
		case "todo.created":
			if inWindow {
				weeks[i].Added++
			}
		case "todo.done":
			if inWindow {
				weeks[i].Done++
				if f.Due != "" {
					withDue++
					if at.Format(time.DateOnly) <= f.Due {
						onTime++
					}
				}
			}
		case "request.received":
			if inWindow {
				weeks[i].Requests++
			}
		case "request.triaged":
			if inWindow {
				weeks[i].Triaged++
				if f.Start != nil {
					triaged++
					triageHours += f.At.Sub(*f.Start).Hours()
				}
			}
		case "agreement.approved":
			if f.Start != nil && at.After(r.now.AddDate(-1, 0, 0)) {
				approvals++
				approvalHours += f.At.Sub(*f.Start).Hours()
			}
		}
	}
	var added, done, requests, tri int
	for _, w := range weeks {
		added, done, requests, tri = added+w.Added, done+w.Done, requests+w.Requests, tri+w.Triaged
	}
	out := map[string]any{"weeks": weeks, "added": added, "done": done, "requests": requests, "triaged": tri,
		"with_due": withDue, "on_time": onTime, "on_time_rate": nil, "approvals": approvals, "approval_days": nil,
		"triage_count": triaged, "triage_hours": nil}
	if withDue > 0 {
		out["on_time_rate"] = math.Round(float64(onTime) / float64(withDue) * 100)
	}
	if approvals > 0 {
		out["approval_days"] = math.Round(approvalHours/24/float64(approvals)*10) / 10
	}
	if triaged > 0 {
		out["triage_hours"] = math.Round(triageHours/float64(triaged)*10) / 10
	}
	return out
}

// ---- reach ------------------------------------------------------------

func (r *report) reach() map[string]any {
	type country struct {
		Code        string  `json:"code"`
		Name        string  `json:"name"`
		Clients     []int64 `json:"clients"`
		Approximate int     `json:"approximate"`
	}
	type zone struct {
		Zone    string  `json:"zone"` // as stored; "" is ours
		IANA    string  `json:"iana"`
		City    string  `json:"city"`
		Offset  int     `json:"offset"` // seconds east of UTC, now
		Clients []int64 `json:"clients"`
	}
	countriesBy, zonesBy := map[string]*country{}, map[string]*zone{}
	for _, c := range r.rows {
		code := c.Place.CountryCode
		if code == "" {
			code = "??"
		}
		if countriesBy[code] == nil {
			name := c.Place.Country
			if name == "" {
				name = "Somewhere"
			}
			countriesBy[code] = &country{Code: code, Name: name}
		}
		countriesBy[code].Clients = append(countriesBy[code].Clients, c.ID)
		if c.Place.Approximate {
			countriesBy[code].Approximate++
		}
		if zonesBy[c.IANA] == nil {
			z := &zone{Zone: c.TimeZone, IANA: c.IANA}
			if p, ok := placeOfZone(zoneOr(c.TimeZone, r.site.TimeZone)); ok {
				z.City = p.City
			}
			if loc, err := time.LoadLocation(c.IANA); err == nil {
				_, z.Offset = r.now.In(loc).Zone()
			}
			zonesBy[c.IANA] = z
		}
		zonesBy[c.IANA].Clients = append(zonesBy[c.IANA].Clients, c.ID)
	}
	countryList, zoneList := []*country{}, []*zone{}
	for _, c := range countriesBy {
		countryList = append(countryList, c)
	}
	sort.Slice(countryList, func(i, j int) bool {
		if len(countryList[i].Clients) != len(countryList[j].Clients) {
			return len(countryList[i].Clients) > len(countryList[j].Clients)
		}
		return countryList[i].Name < countryList[j].Name
	})
	for _, z := range zonesBy {
		zoneList = append(zoneList, z)
	}
	sort.Slice(zoneList, func(i, j int) bool { return zoneList[i].Offset < zoneList[j].Offset })

	months := r.months(12)
	idx := map[string]int{}
	newBy := make([]map[string]any, len(months))
	for i, mth := range months {
		idx[mth] = i
		newBy[i] = map[string]any{"month": mth, "count": 0}
	}
	for _, c := range r.rows {
		if i, ok := idx[c.CreatedAt.In(r.loc).Format("2006-01")]; ok {
			newBy[i]["count"] = newBy[i]["count"].(int) + 1 //nolint:forcetypeassert,errcheck // the report's own map, its types known
		}
	}
	var far float64
	var farthest *clientRow
	for _, c := range r.rows {
		if d := distanceKm(r.home.Lat, r.home.Lng, c.Place.Lat, c.Place.Lng); d > far {
			far, farthest = d, c
		}
	}
	out := map[string]any{"countries": countryList, "zones": zoneList, "new_by_month": newBy, "home": r.home}
	if farthest != nil {
		out["farthest"] = map[string]any{"id": farthest.ID, "name": farthest.Name, "km": math.Round(far)}
	}
	return out
}

// distanceKm is the great-circle distance between two points.
func distanceKm(lat1, lng1, lat2, lng2 float64) float64 {
	rad := math.Pi / 180
	dLat, dLng := (lat2-lat1)*rad, (lng2-lng1)*rad
	a := math.Sin(dLat/2)*math.Sin(dLat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dLng/2)*math.Sin(dLng/2)
	return 6371 * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

func lower(s string) string { return strings.ToLower(s) }
