package core

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// The personal half of home, around the briefing (Briefing::Home): the day
// and a greeting, what's due today, what's coming up for this person, their
// engagements with how each is going, what the team is doing, and the
// latest notes. Each part is a few rows from one or two queries.

const (
	homeComingUpDays = 7
	homeShown        = 6
)

// homeStateSQL is an engagement's derived state, worked out by the
// database (Engagement::STATE_SQL), with the joins it needs.
const homeStateJoins = ` JOIN clients state_clients ON state_clients.id = e.client_id
	LEFT JOIN agreement_versions latest_versions ON latest_versions.engagement_id = e.id
		AND latest_versions.number = (SELECT MAX(av.number) FROM agreement_versions av WHERE av.engagement_id = e.id)
	LEFT JOIN approvals latest_approvals ON latest_approvals.agreement_version_id = latest_versions.id`

const homeStateSQL = `CASE
	WHEN e.closed_at IS NOT NULL THEN 'closed'
	WHEN state_clients.internal THEN 'internal'
	WHEN latest_versions.id IS NULL OR latest_versions.sent_at IS NULL THEN 'draft'
	WHEN latest_approvals.id IS NULL AND latest_versions.superseded_by_id IS NULL THEN 'sent'
	WHEN latest_approvals.decision = 'changes_requested' THEN 'changes_requested'
	ELSE 'approved' END`

// homeMyWork is "this person's open work": anything they're on.
const homeMyWork = `t.status != 'done' AND t.id IN (SELECT todo_id FROM assignments WHERE user_id = ?)`

func (a *App) briefingHome(ctx context.Context, actor *Actor, team bool) (Obj, error) {
	me := actor.User.Person()
	now := time.Now().In(a.zone(ctx))
	today := now.Format("2006-01-02")
	part := "evening"
	switch {
	case now.Hour() < 12:
		part = "morning"
	case now.Hour() < 18:
		part = "afternoon"
	}
	first := strings.Fields(me.DisplayName())
	name := me.DisplayName()
	if len(first) > 0 {
		name = first[0]
	}
	count := func(q string, args ...any) int {
		n, _ := store.Count(ctx, a.store, q, args...)
		return n
	}
	day := O(
		"work", count(`SELECT count(*) FROM todos t WHERE `+homeMyWork+` AND t.due_on = ?`, me.ID, today),
		"promises", count(`SELECT count(*) FROM commitments WHERE resolution IS NULL AND due_on = ?`, today),
		"late_work", count(`SELECT count(*) FROM todos t WHERE `+homeMyWork+` AND t.due_on < ?`, me.ID, today),
		"late_promises", count(`SELECT count(*) FROM commitments WHERE resolution IS NULL AND user_id = ? AND owner_kind = 'us' AND due_on < ?`, me.ID, today),
	)
	coming, err := a.homeComingUp(ctx, me.ID, today)
	if err != nil {
		return nil, err
	}
	engagements, err := a.homeEngagements(ctx, me.ID, today)
	if err != nil {
		return nil, err
	}
	var people any
	if team {
		if people, err = a.homeTeam(ctx); err != nil {
			return nil, err
		}
	}
	discussion, err := a.homeDiscussion(ctx, me.ID)
	if err != nil {
		return nil, err
	}
	return O("date", today, "time", now.Format(time.RFC3339), "greeting", fmt.Sprintf("Good %s, %s.", part, name), "today", day,
		"coming_up", coming, "engagements", engagements, "team", people, "discussion", discussion), nil
}

// homeAction is a promise read after "will": its first letter lowered
// unless it starts an acronym.
func homeAction(s string) string {
	r := []rune(s)
	if len(r) > 1 && unicode.IsUpper(r[0]) && unicode.IsLower(r[1]) {
		r[0] = unicode.ToLower(r[0])
	}
	return string(r)
}

// homeComingUp is this person's open work and our promises they hold, due
// within the week (late ones too), soonest first.
func (a *App) homeComingUp(ctx context.Context, me int64, today string) ([]Obj, error) {
	horizon := briefDateAdd(today, homeComingUpDays)
	type item struct {
		due string
		o   Obj
	}
	var items []item
	rows, err := a.store.QueryContext(ctx, `SELECT t.id, t.title, t.due_on, e.ref FROM todos t JOIN engagements e ON e.id = t.engagement_id
		WHERE `+homeMyWork+` AND t.due_on <= ? ORDER BY t.due_on LIMIT ?`, me, horizon, homeShown)
	if err != nil {
		return nil, err
	}
	work, err := store.ScanAll(rows, func(r *sqlRows) (item, error) {
		var id int64
		var title, due, ref string
		err := r.Scan(&id, &title, &due, &ref)
		due = due[:min(10, len(due))]
		return item{due, O("type", "Todo", "id", id, "title", title, "path", fmt.Sprintf("/work/%d", id), "meta", ref, "due_on", due, "late", due < today)}, err
	})
	if err != nil {
		return nil, err
	}
	items = append(items, work...)
	rows, err = a.store.QueryContext(ctx, `SELECT c.id, c.description, c.due_on, c.client_id, COALESCE(e.ref, ''), COALESCE(NULLIF(u.name, ''), u.email_address, '')
		FROM commitments c LEFT JOIN engagements e ON e.id = c.engagement_id LEFT JOIN users u ON u.id = c.user_id
		WHERE c.resolution IS NULL AND c.user_id = ? AND c.owner_kind = 'us' AND c.due_on <= ? ORDER BY c.due_on LIMIT ?`, me, horizon, homeShown)
	if err != nil {
		return nil, err
	}
	promises, err := store.ScanAll(rows, func(r *sqlRows) (item, error) {
		var id, client int64
		var desc, due, ref, who string
		err := r.Scan(&id, &desc, &due, &client, &ref, &who)
		due = due[:min(10, len(due))]
		path := fmt.Sprintf("/clients/%d", client)
		if ref != "" {
			path = "/engagements/" + ref
		}
		if who == "" {
			who = "We"
		}
		return item{due, O("type", "Commitment", "id", id, "title", who+" will "+homeAction(desc), "promiser", who, "action", homeAction(desc),
			"path", path, "meta", "commitment", "due_on", due, "late", due < today)}, err
	})
	if err != nil {
		return nil, err
	}
	items = append(items, promises...)
	sort.SliceStable(items, func(i, j int) bool { return items[i].due < items[j].due })
	out := []Obj{}
	for i := 0; i < len(items) && i < homeShown; i++ {
		out = append(out, items[i].o)
	}
	return out, nil
}

// homeEngagements are the open engagements this person has work on (or,
// with none, the latest open ones), with their work done of total and how
// each is going: behind, waiting on the client, a draft, or on track.
func (a *App) homeEngagements(ctx context.Context, me int64, today string) ([]Obj, error) {
	where := `e.closed_at IS NULL AND e.id IN (SELECT t.engagement_id FROM todos t WHERE t.id IN (SELECT todo_id FROM assignments WHERE user_id = ?))`
	args := []any{me}
	if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM engagements e WHERE `+where, me); err != nil {
		return nil, err
	} else if n == 0 {
		where, args = `e.closed_at IS NULL`, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT e.id, e.ref, e.title, e.client_id, state_clients.name, `+homeStateSQL+`
		FROM engagements e`+homeStateJoins+` WHERE `+where+` ORDER BY e.updated_at DESC LIMIT ?`, append(args, homeShown)...)
	if err != nil {
		return nil, err
	}
	type eng struct {
		id, client              int64
		ref, title, name, state string
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (eng, error) {
		var e eng
		return e, r.Scan(&e.id, &e.ref, &e.title, &e.client, &e.name, &e.state)
	})
	if err != nil || len(list) == 0 {
		return []Obj{}, err
	}
	ids := make([]int64, len(list))
	for i, e := range list {
		ids[i] = e.id
	}
	in := store.Placeholders(len(ids))
	done, err := countBy(ctx, a.store, `SELECT engagement_id, count(*) FROM todos WHERE status = 'done' AND engagement_id IN (`+in+`) GROUP BY engagement_id`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	open, err := countBy(ctx, a.store, `SELECT engagement_id, count(*) FROM todos WHERE status != 'done' AND engagement_id IN (`+in+`) GROUP BY engagement_id`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	late, err := countBy(ctx, a.store, `SELECT engagement_id, count(*) FROM (
			SELECT engagement_id FROM todos WHERE status != 'done' AND due_on < ? AND engagement_id IN (`+in+`)
			UNION ALL SELECT engagement_id FROM commitments WHERE resolution IS NULL AND owner_kind = 'us' AND due_on < ? AND engagement_id IN (`+in+`)
		) GROUP BY engagement_id`, append(append(append([]any{today}, idArgs(ids)...), today), idArgs(ids)...)...)
	if err != nil {
		return nil, err
	}
	out := make([]Obj, len(list))
	for i, e := range list {
		health := "On track"
		switch {
		case late[e.id] > 0:
			health = "Behind"
		case e.state == "sent" || e.state == "changes_requested":
			health = "Waiting"
		case e.state == "draft":
			health = "Draft"
		}
		out[i] = O("id", e.id, "ref", e.ref, "title", e.title, "path", "/engagements/"+e.ref,
			"client", O("id", e.client, "name", e.name), "state", e.state,
			"open", open[e.id], "done", done[e.id], "total", open[e.id]+done[e.id], "health", health)
	}
	return out, nil
}

// homeTeam is who has work in progress, and the first two things each is
// doing, by name.
func (a *App) homeTeam(ctx context.Context) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT u.id, COALESCE(NULLIF(u.name, ''), u.email_address), t.id, t.title, e.ref
		FROM todos t JOIN users u ON u.id = t.owner_id JOIN engagements e ON e.id = t.engagement_id
		WHERE t.status = 'in_progress' AND u.deactivated_at IS NULL ORDER BY t.updated_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byUser := map[int64]Obj{}
	var order []int64
	for rows.Next() {
		var uid, tid int64
		var name, title, ref string
		if err := rows.Scan(&uid, &name, &tid, &title, &ref); err != nil {
			return nil, err
		}
		p, ok := byUser[uid]
		if !ok {
			p = O("user", O("id", uid, "name", name), "doing", []Obj{})
			order = append(order, uid)
		}
		if doing := p.Get("doing").([]Obj); len(doing) < 2 {
			p = p.Set("doing", append(doing, O("id", tid, "title", title, "ref", ref, "path", fmt.Sprintf("/work/%d", tid))))
		}
		byUser[uid] = p
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make([]Obj, 0, len(order))
	for _, id := range order {
		out = append(out, byUser[id])
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Get("user").(Obj).Get("name").(string) < out[j].Get("user").(Obj).Get("name").(string)
	})
	return out, nil
}

// homeDiscussion is the latest four notes by others, on anything: what was
// said and where.
func (a *App) homeDiscussion(ctx context.Context, me int64) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT n.id, n.subject_type, n.subject_id, n.body, n.occurred_at, COALESCE(n.author_id, 0), COALESCE(NULLIF(u.name, ''), u.email_address, '')
		FROM notes n LEFT JOIN users u ON u.id = n.author_id WHERE n.author_id IS NULL OR n.author_id != ? ORDER BY n.occurred_at DESC LIMIT 4`, me)
	if err != nil {
		return nil, err
	}
	type note struct {
		id, subject, author int64
		typ, body, name     string
		at                  store.Time
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (note, error) {
		var n note
		return n, r.Scan(&n.id, &n.typ, &n.subject, &n.body, &n.at, &n.author, &n.name)
	})
	if err != nil {
		return nil, err
	}
	out := make([]Obj, len(list))
	for i, n := range list {
		label, path := a.homeSubject(ctx, n.typ, n.subject)
		text := plainText(n.body)
		if r := []rune(text); len(r) > 140 {
			text = string(r[:139]) + "…"
		}
		out[i] = O("id", n.id, "author", userRef(n.author, n.name), "text", text, "subject_label", label, "path", nullable(path), "occurred_at", n.at)
	}
	return out, nil
}

// homeSubject is a note's subject in a few words, and its page.
func (a *App) homeSubject(ctx context.Context, typ string, id int64) (label, path string) {
	q := map[string]string{
		"Engagement": `SELECT ref, '/engagements/' || ref FROM engagements WHERE id = ?`,
		"Client":     `SELECT name, '/clients/' || id FROM clients WHERE id = ?`,
		"Todo":       `SELECT title, '/work/' || id FROM todos WHERE id = ?`,
		"Request":    `SELECT subject, '/requests/' || id FROM requests WHERE id = ?`,
		"ScopeItem":  `SELECT description, '/scope_items/' || id FROM scope_items WHERE id = ?`,
	}[typ]
	if q == "" || a.store.QueryRowContext(ctx, q, id).Scan(&label, &path) != nil {
		return humanize(searchTable(typ)[:len(searchTable(typ))-1]), ""
	}
	return label, path
}
