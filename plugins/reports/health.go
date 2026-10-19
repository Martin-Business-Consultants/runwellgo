package reports

import (
	"fmt"
	"sort"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// The health score: each client out of 100, every point taken off (or
// added) with a reason anyone can read, so the number is never a mystery.
//
//	late work               −10 each, at most −30
//	our overdue commitments −12 each, at most −30
//	their overdue ones      −4 each, at most −12
//	agreements waiting      −8 each once a week old (else −3), at most −16
//	requests to triage      −5 each, at most −15; −5 more once one is 3 days old
//	blocked work            −4 each, at most −12
//	last touch              −8 after 14 days, −15 after 30, −25 after 60; −10 if never
//	momentum                −10 when open work saw nothing done in 4 weeks;
//	                        +5 when more was done than in the 4 weeks before
//
// 80 and up is on track, 55 to 79 needs watching, under 55 is at risk. A
// client that isn't active is scored but shown as inactive.

// Health is a client's score, its band and the reasons.
type Health struct {
	Score   int      `json:"score"`
	Band    string   `json:"band"` // on_track, watch, at_risk, inactive
	Label   string   `json:"label"`
	Reasons []Reason `json:"reasons"`
}

// Reason is one thing that moved the score: Points (negative takes off),
// in words, with where to see it.
type Reason struct {
	Text   string `json:"text"`
	Points int    `json:"points"`
	Href   string `json:"href,omitempty"`
}

// momentum is work done in the last four weeks and the four before.
type momentum struct{ recent, before int }

// Bands and their words.
var bandLabels = map[string]string{"on_track": "On track", "watch": "Needs attention", "at_risk": "At risk", "inactive": "Inactive"}

func bandOf(score int) string {
	switch {
	case score >= 80:
		return "on_track"
	case score >= 55:
		return "watch"
	}
	return "at_risk"
}

func plural(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %s", n, many)
}

func daysSince(t *time.Time, now time.Time) int {
	if t == nil {
		return -1
	}
	return int(now.Sub(*t).Hours() / 24)
}

// healthOf scores c as of now. terms are the install's words (work,
// commitment, engagement), lowercase, one and many.
func healthOf(c plugin.ClientMetrics, mo momentum, now time.Time, t words) Health {
	h := Health{Score: 100, Reasons: []Reason{}}
	client := fmt.Sprintf("/clients/%d", c.ID)
	take := func(points int, text, href string) {
		if points == 0 {
			return
		}
		h.Score += points
		h.Reasons = append(h.Reasons, Reason{Text: text, Points: points, Href: href})
	}
	capped := func(n, each, most int) int { return -min(n*each, most) }

	if c.LateWork > 0 {
		take(capped(c.LateWork, 10, 30), plural(c.LateWork, t.work1+" late", t.workN+" late"), client+"?tab=engagements")
	}
	if c.OverdueOurs > 0 {
		take(capped(c.OverdueOurs, 12, 30), plural(c.OverdueOurs, t.commitment1+" we owe is overdue", t.commitmentN+" we owe are overdue"), client+"?tab=commitments")
	}
	if c.OverdueTheirs > 0 {
		take(capped(c.OverdueTheirs, 4, 12), plural(c.OverdueTheirs, t.commitment1+" of theirs is overdue", t.commitmentN+" of theirs are overdue"), client+"?tab=commitments")
	}
	if c.WaitingApprovals > 0 {
		days := daysSince(c.WaitingSince, now)
		each := 3
		text := plural(c.WaitingApprovals, "agreement waiting on them", "agreements waiting on them")
		if days >= 7 {
			each = 8
			text += fmt.Sprintf(", the oldest %d days", days)
		}
		take(capped(c.WaitingApprovals, each, 16), text, client+"?tab=engagements")
	}
	if c.OpenRequests > 0 {
		pts := capped(c.OpenRequests, 5, 15)
		text := plural(c.OpenRequests, "request to triage", "requests to triage")
		if d := daysSince(c.OldestRequestAt, now); d >= 3 {
			pts -= 5
			text += fmt.Sprintf(", one %d days old", d)
		}
		take(pts, text, "/requests")
	}
	if c.BlockedWork > 0 {
		take(capped(c.BlockedWork, 4, 12), plural(c.BlockedWork, t.work1+" blocked", t.workN+" blocked"), "/work?view=board")
	}
	if c.Status == "active" {
		switch d := daysSince(c.LastTouchAt, now); {
		case d < 0:
			take(-10, "Nothing logged with them yet", client+"?tab=notes")
		case d >= 60:
			take(-25, fmt.Sprintf("No touch in %d days", d), client+"?tab=notes")
		case d >= 30:
			take(-15, fmt.Sprintf("No touch in %d days", d), client+"?tab=notes")
		case d >= 14:
			take(-8, fmt.Sprintf("No touch in %d days", d), client+"?tab=notes")
		}
	}
	switch {
	case c.OpenWork > 0 && mo.recent == 0:
		take(-10, "Nothing done in 4 weeks", client+"?tab=engagements")
	case mo.recent > mo.before && mo.recent >= 2:
		take(5, fmt.Sprintf("Momentum: %d done in 4 weeks, up from %d", mo.recent, mo.before), "")
	}
	h.Score = max(0, min(100, h.Score))
	h.Band = bandOf(h.Score)
	if c.Status != "active" {
		h.Band = "inactive"
	}
	h.Label = bandLabels[h.Band]
	// The biggest first; good news last.
	sort.SliceStable(h.Reasons, func(i, j int) bool { return h.Reasons[i].Points < h.Reasons[j].Points })
	return h
}

// words are the install's words for the things the reasons name.
type words struct {
	work1, workN, commitment1, commitmentN, client1, clientN, engagement1, engagementN string
}

func wordsOf(site plugin.Site) words {
	term := func(k, n, fallback string) string {
		if t, ok := site.Terms[k][n]; ok && t != "" {
			return lower(t)
		}
		return fallback
	}
	w := words{
		work1: term("work", "one", "work"), workN: term("work", "other", "work"),
		commitment1: term("commitment", "one", "commitment"), commitmentN: term("commitment", "other", "commitments"),
		client1: term("client", "one", "client"), clientN: term("client", "other", "clients"),
		engagement1: term("engagement", "one", "engagement"), engagementN: term("engagement", "other", "engagements"),
	}
	// Work is uncountable: counted, it's pieces of it.
	if w.work1 == w.workN {
		w.work1, w.workN = "piece of "+w.work1, "pieces of "+w.workN
	}
	return w
}
