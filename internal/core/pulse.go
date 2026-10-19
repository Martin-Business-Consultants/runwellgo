package core

import (
	"net/http"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// The pulse: Home's picture of the install, by Runwell's three things.
// Who we work for (active clients, requests waiting), what we agreed
// (engagements running, waiting on a client, in draft, what's agreed) and
// what's happening (work open, due this week, late, in review, blocked),
// with twelve weeks of work added and done and requests in, by the week
// (Mondays, in the install's zone).

const pulseWeeks = 12

func init() {
	routeSets = append(routeSets, func(a *App, mux *http.ServeMux) {
		a.handle(mux, "GET /api/home/pulse", a.showPulse, tool("show_pulse", "Show the install's pulse",
			note("Counts by Runwell's three things (clients and requests; engagements and what's agreed; work open, due, late, in review and blocked) and twelve weeks of work added and done and requests in, by the week.")))
	})
}

func (a *App) showPulse(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	loc := a.zone(ctx)
	now := time.Now().In(loc)
	today := now.Format(time.DateOnly)
	week := now.AddDate(0, 0, 7).Format(time.DateOnly)

	count := func(q string, args ...any) int {
		n, _ := store.Count(ctx, a.store, q, args...)
		return n
	}
	clients := O("active", count(`SELECT count(*) FROM clients WHERE status = 'active'`),
		"requests_open", count(`SELECT count(*) FROM requests WHERE status = 'open'`))

	engs, err := a.loadEngagements(ctx, a.store, `e.closed_at IS NULL`)
	if err != nil {
		return err
	}
	var active, waiting, drafts int
	var agreed int64
	for _, e := range engs {
		switch e.State() {
		case "sent", "changes_requested":
			waiting++
		case "draft":
			drafts++
		}
		if e.Active() {
			active++
			agreed += e.AgreedAmountCents()
		}
	}
	engagements := O("active", active, "waiting", waiting, "drafts", drafts)
	if s.Prices {
		engagements = engagements.Set("agreed_cents", agreed)
	}

	work := O("open", count(`SELECT count(*) FROM todos WHERE status <> 'done'`),
		"due_week", count(`SELECT count(*) FROM todos WHERE status <> 'done' AND due_on >= ? AND due_on <= ?`, today, week),
		"late", count(`SELECT count(*) FROM todos WHERE status <> 'done' AND due_on < ?`, today),
		"in_review", count(`SELECT count(*) FROM todos WHERE status = 'in_review'`),
		"blocked", count(`SELECT count(*) FROM todos WHERE status = 'blocked'`))

	// The weeks, oldest first, each starting on a Monday.
	monday := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	monday = monday.AddDate(0, 0, -((int(monday.Weekday()) + 6) % 7))
	first := monday.AddDate(0, 0, -7*(pulseWeeks-1))
	type bucket struct{ added, done, requests int }
	buckets := make([]bucket, pulseWeeks)
	tally := func(q string, add func(*bucket)) error {
		rows, err := a.store.QueryContext(ctx, q, first.UTC().Format("2006-01-02 15:04:05"))
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var t store.Time
			if err := rows.Scan(&t); err != nil {
				return err
			}
			if !t.Valid {
				continue
			}
			i := int(t.Time.In(loc).Sub(first).Hours() / (24 * 7))
			if i >= 0 && i < pulseWeeks {
				add(&buckets[i])
			}
		}
		return rows.Err()
	}
	if err := tally(`SELECT created_at FROM todos WHERE created_at >= ?`, func(b *bucket) { b.added++ }); err != nil {
		return err
	}
	if err := tally(`SELECT completed_at FROM todos WHERE completed_at >= ?`, func(b *bucket) { b.done++ }); err != nil {
		return err
	}
	if err := tally(`SELECT COALESCE(received_at, created_at) FROM requests WHERE COALESCE(received_at, created_at) >= ?`, func(b *bucket) { b.requests++ }); err != nil {
		return err
	}
	weeks := make([]Obj, pulseWeeks)
	for i, b := range buckets {
		weeks[i] = O("week", first.AddDate(0, 0, 7*i).Format(time.DateOnly), "added", b.added, "done", b.done, "requests", b.requests)
	}
	c.Summary = "The pulse, by clients, engagements and work."
	return c.JSON(200, O("summary", c.Summary, "clients", clients, "engagements", engagements, "work", work, "weeks", weeks))
}
