package core

import (
	"context"
	"database/sql"
	"sort"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// A plugin's metrics (the metrics grant, plugin.API.Metrics): the numbers a
// report needs, read with a few grouped queries rather than record by
// record. Read-only; money is left out (zero) when the install hides prices.

const maxMetricFacts = 50000

// noteClient is each note with the client and engagement it's under,
// whatever it's on (a client, an engagement, a scope item, work or a
// request).
const noteClient = `SELECT n.id AS id, n.occurred_at AS at,
	CASE n.subject_type
		WHEN 'Client' THEN n.subject_id
		WHEN 'Engagement' THEN (SELECT client_id FROM engagements WHERE id = n.subject_id)
		WHEN 'Todo' THEN (SELECT e.client_id FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.id = n.subject_id)
		WHEN 'ScopeItem' THEN (SELECT e.client_id FROM scope_items si JOIN agreement_versions av ON av.id = si.agreement_version_id JOIN engagements e ON e.id = av.engagement_id WHERE si.id = n.subject_id)
		WHEN 'Request' THEN (SELECT client_id FROM requests WHERE id = n.subject_id)
	END AS client_id,
	CASE n.subject_type
		WHEN 'Engagement' THEN n.subject_id
		WHEN 'Todo' THEN (SELECT engagement_id FROM todos WHERE id = n.subject_id)
		WHEN 'ScopeItem' THEN (SELECT av.engagement_id FROM scope_items si JOIN agreement_versions av ON av.id = si.agreement_version_id WHERE si.id = n.subject_id)
	END AS engagement_id
	FROM notes n`

// Metrics answers the install's numbers (plugin.MetricsEnv).
func (e pluginEnv) Metrics(ctx context.Context, q plugin.MetricsQuery) (*plugin.Metrics, error) { //nolint:gocyclo // one query after another, each simple
	a := e.a
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	loc := a.zone(ctx)
	now := time.Now().In(loc)
	today := now.Format(time.DateOnly)
	week := now.AddDate(0, 0, 7).Format(time.DateOnly)
	since := q.Since.UTC().Format("2006-01-02 15:04:05")
	m := &plugin.Metrics{Today: today, Prices: s.Prices, Clients: []plugin.ClientMetrics{}, Engagements: []plugin.EngagementMetrics{}, Facts: []plugin.Fact{}}
	cents := func(c int64) int64 {
		if !s.Prices {
			return 0
		}
		return c
	}
	ptr := func(t store.Time) *time.Time {
		if !t.Valid {
			return nil
		}
		v := t.UTC()
		return &v
	}
	want := func(clientID int64) bool { return q.ClientID == 0 || clientID == q.ClientID }

	// The clients.
	rows, err := a.store.QueryContext(ctx, `SELECT id, name, status, COALESCE(time_zone, ''), internal, created_at FROM clients ORDER BY name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	clients, err := store.ScanAll(rows, func(r *sqlRows) (plugin.ClientMetrics, error) {
		var c plugin.ClientMetrics
		var created store.Time
		err := r.Scan(&c.ID, &c.Name, &c.Status, &c.TimeZone, &c.Internal, &created)
		c.CreatedAt = created.UTC()
		return c, err
	})
	if err != nil {
		return nil, err
	}
	byID := map[int64]*plugin.ClientMetrics{}
	for i := range clients {
		byID[clients[i].ID] = &clients[i]
	}
	// each runs a grouped query, one row per client: (client id, …).
	each := func(query string, args []any, fn func(c *plugin.ClientMetrics, r *sqlRows) error, dest ...any) error {
		rows, err := a.store.QueryContext(ctx, query, args...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id sql.NullInt64
			if err := rows.Scan(append([]any{&id}, dest...)...); err != nil {
				return err
			}
			if c := byID[id.Int64]; c != nil && id.Valid {
				if err := fn(c, rows); err != nil {
					return err
				}
			}
		}
		return rows.Err()
	}
	touch := func(c *plugin.ClientMetrics, t store.Time, kind string) {
		if t.Valid && (c.LastTouchAt == nil || t.After(*c.LastTouchAt)) {
			c.LastTouchAt, c.LastTouchKind = ptr(t), kind
		}
	}

	var n1, n2, n3, n4, n5 int
	var t1 store.Time
	if err := each(`SELECT e.client_id, count(CASE WHEN t.status <> 'done' THEN 1 END), count(CASE WHEN t.status <> 'done' AND t.due_on < ? THEN 1 END), count(CASE WHEN t.status <> 'done' AND t.due_on >= ? AND t.due_on <= ? THEN 1 END),
			count(CASE WHEN t.status = 'blocked' THEN 1 END), count(CASE WHEN t.status = 'in_review' THEN 1 END), max(t.completed_at)
			FROM todos t JOIN engagements e ON e.id = t.engagement_id GROUP BY e.client_id`, []any{today, today, week},
		func(c *plugin.ClientMetrics, _ *sqlRows) error {
			c.OpenWork, c.LateWork, c.DueSoonWork, c.BlockedWork, c.InReviewWork = n1, n2, n3, n4, n5
			touch(c, t1, "work")
			return nil
		}, &n1, &n2, &n3, &n4, &n5, &t1); err != nil {
		return nil, err
	}
	var t2 store.Time
	if err := each(`SELECT client_id, count(CASE WHEN status = 'open' THEN 1 END), min(CASE WHEN status = 'open' THEN received_at END), max(received_at) FROM requests WHERE client_id IS NOT NULL GROUP BY client_id`, nil,
		func(c *plugin.ClientMetrics, _ *sqlRows) error {
			c.OpenRequests, c.OldestRequestAt = n1, ptr(t1)
			touch(c, t2, "request")
			return nil
		}, &n1, &t1, &t2); err != nil {
		return nil, err
	}
	if err := each(`SELECT client_id, count(*), count(CASE WHEN due_on < ? AND owner_kind = 'us' THEN 1 END), count(CASE WHEN due_on < ? AND owner_kind <> 'us' THEN 1 END), count(CASE WHEN due_on >= ? AND due_on <= ? THEN 1 END)
			FROM commitments WHERE resolution IS NULL GROUP BY client_id`, []any{today, today, today, week},
		func(c *plugin.ClientMetrics, _ *sqlRows) error {
			c.OpenCommitments, c.OverdueOurs, c.OverdueTheirs, c.DueSoonCommitments = n1, n2, n3, n4
			return nil
		}, &n1, &n2, &n3, &n4); err != nil {
		return nil, err
	}
	if err := each(`SELECT client_id, count(*), count(CASE WHEN portal_access THEN 1 END) FROM contacts WHERE archived_at IS NULL GROUP BY client_id`, nil,
		func(c *plugin.ClientMetrics, _ *sqlRows) error {
			c.Contacts, c.PortalContacts = n1, n2
			return nil
		}, &n1, &n2); err != nil {
		return nil, err
	}
	if err := each(`SELECT client_id, max(at) FROM (`+noteClient+`) GROUP BY client_id`, nil,
		func(c *plugin.ClientMetrics, _ *sqlRows) error {
			c.LastNoteAt = ptr(t1)
			touch(c, t1, "note")
			return nil
		}, &t1); err != nil {
		return nil, err
	}

	// The engagements, their states and money, and their work.
	engs, err := a.loadEngagements(ctx, a.store, `1 = 1`)
	if err != nil {
		return nil, err
	}
	drafts := map[int64]int64{}
	if err := func() error {
		rows, err := a.store.QueryContext(ctx, `SELECT si.agreement_version_id, sum(si.price_cents) FROM scope_items si JOIN agreement_versions av ON av.id = si.agreement_version_id WHERE av.sent_at IS NULL GROUP BY 1`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id, sum int64
			if err := rows.Scan(&id, &sum); err != nil {
				return err
			}
			drafts[id] = sum
		}
		return rows.Err()
	}(); err != nil {
		return nil, err
	}
	type workCount struct{ open, done, late int }
	work := map[int64]workCount{}
	if err := func() error {
		rows, err := a.store.QueryContext(ctx, `SELECT engagement_id, count(CASE WHEN status <> 'done' THEN 1 END), count(CASE WHEN status = 'done' THEN 1 END), count(CASE WHEN status <> 'done' AND due_on < ? THEN 1 END) FROM todos GROUP BY engagement_id`, today)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			var w workCount
			if err := rows.Scan(&id, &w.open, &w.done, &w.late); err != nil {
				return err
			}
			work[id] = w
		}
		return rows.Err()
	}(); err != nil {
		return nil, err
	}
	// versionCents is what a version is worth as a fact: the whole amount
	// for an initial version, a revision or a recurring service, the change
	// for a change order or add-on.
	versionCents := func(eng *Engagement, v *AgreementVersion) int64 {
		if eng.Recurring() {
			return v.AmountCents
		}
		return v.TotalCents()
	}
	for _, eng := range engs {
		if !want(eng.ClientID) {
			continue
		}
		w := work[eng.ID]
		em := plugin.EngagementMetrics{ID: eng.ID, ClientID: eng.ClientID, Ref: eng.Ref, Title: eng.Title, Label: eng.Label, Shape: eng.Shape,
			State: eng.State(), CloseOutcome: eng.CloseOutcome, AgreedCents: cents(eng.AgreedAmountCents()),
			WorkOpen: w.open, WorkDone: w.done, WorkLate: w.late, CreatedAt: eng.CreatedAt.UTC(), ClosedAt: ptr(eng.ClosedAt)}
		if cur := eng.CurrentVersion(); cur != nil {
			em.Cadence = cur.Cadence
		}
		for _, v := range eng.Versions {
			if v.Approved() && em.ApprovedAt == nil {
				em.ApprovedAt = ptr(v.Approval.DecidedAt)
			}
			if v.AwaitingDecision() {
				em.PendingCents += cents(versionCents(eng, v))
			}
			if v.Draft() {
				em.DraftCents += cents(drafts[v.ID])
			}
			if em.Cadence == "" && v.Cadence != "" {
				em.Cadence = v.Cadence
			}
		}
		if c := byID[eng.ClientID]; c != nil && !eng.Closed() {
			if st := em.State; st == "sent" || st == "changes_requested" {
				c.WaitingApprovals++
				if latest := eng.Versions[len(eng.Versions)-1]; latest.SentAt.Valid && (c.WaitingSince == nil || latest.SentAt.Before(*c.WaitingSince)) {
					c.WaitingSince = ptr(latest.SentAt)
				}
			}
		}
		m.Engagements = append(m.Engagements, em)

		// Its agreements' facts, and the client's latest decision.
		if !eng.CreatedAt.Before(q.Since) {
			m.Facts = append(m.Facts, plugin.Fact{Kind: "engagement.created", At: eng.CreatedAt.UTC(), ClientID: eng.ClientID, EngagementID: eng.ID, ID: eng.ID})
		}
		for _, v := range eng.Versions {
			if v.SentAt.Valid && !v.SentAt.Before(q.Since) {
				m.Facts = append(m.Facts, plugin.Fact{Kind: "agreement.sent", At: v.SentAt.UTC(), ClientID: eng.ClientID, EngagementID: eng.ID, ID: v.ID, Cents: cents(versionCents(eng, v))})
			}
			if ap := v.Approval; ap != nil && ap.DecidedAt.Valid {
				if c := byID[eng.ClientID]; c != nil {
					touch(c, ap.DecidedAt, "approval")
				}
				if !ap.DecidedAt.Before(q.Since) {
					m.Facts = append(m.Facts, plugin.Fact{Kind: "agreement." + ap.Decision, At: ap.DecidedAt.UTC(), ClientID: eng.ClientID, EngagementID: eng.ID, ID: v.ID,
						Cents: cents(versionCents(eng, v)), Start: ptr(v.SentAt)})
				}
			}
		}
	}

	// The dated facts.
	fact := func(kind, query string, scan func(r *sqlRows) (plugin.Fact, error)) error {
		rows, err := a.store.QueryContext(ctx, query, since)
		if err != nil {
			return err
		}
		list, err := store.ScanAll(rows, scan)
		if err != nil {
			return err
		}
		for _, f := range list {
			if want(f.ClientID) && (f.ClientID != 0 || q.ClientID == 0) {
				if f.Kind == "" {
					f.Kind = kind
				}
				m.Facts = append(m.Facts, f)
			}
		}
		return nil
	}
	plain := func(r *sqlRows) (plugin.Fact, error) {
		var f plugin.Fact
		var at store.Time
		var client, eng sql.NullInt64
		err := r.Scan(&f.ID, &at, &client, &eng)
		f.At, f.ClientID, f.EngagementID = at.UTC(), client.Int64, eng.Int64
		return f, err
	}
	if err := fact("client.created", `SELECT id, created_at, id, NULL FROM clients WHERE created_at >= ?`, plain); err != nil {
		return nil, err
	}
	if err := fact("todo.created", `SELECT t.id, t.created_at, e.client_id, e.id FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.created_at >= ?`, plain); err != nil {
		return nil, err
	}
	if err := fact("todo.done", `SELECT t.id, t.completed_at, e.client_id, e.id, COALESCE(t.due_on, '') FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.completed_at >= ?`, func(r *sqlRows) (plugin.Fact, error) {
		var f plugin.Fact
		var at store.Time
		err := r.Scan(&f.ID, &at, &f.ClientID, &f.EngagementID, &f.Due)
		f.At = at.UTC()
		return f, err
	}); err != nil {
		return nil, err
	}
	if err := fact("request.received", `SELECT id, received_at, client_id, NULL FROM requests WHERE received_at >= ?`, plain); err != nil {
		return nil, err
	}
	if err := fact("request.triaged", `SELECT id, triaged_at, client_id, received_at FROM requests WHERE triaged_at >= ?`, func(r *sqlRows) (plugin.Fact, error) {
		var f plugin.Fact
		var at, received store.Time
		var client sql.NullInt64
		err := r.Scan(&f.ID, &at, &client, &received)
		f.At, f.ClientID, f.Start = at.UTC(), client.Int64, ptr(received)
		return f, err
	}); err != nil {
		return nil, err
	}
	if err := fact("note.added", `SELECT id, at, client_id, engagement_id FROM (`+noteClient+`) WHERE at >= ?`, plain); err != nil {
		return nil, err
	}
	if err := fact("", `SELECT id, resolved_at, client_id, engagement_id, resolution, due_on FROM commitments WHERE resolved_at >= ? AND resolution IS NOT NULL`, func(r *sqlRows) (plugin.Fact, error) {
		var f plugin.Fact
		var at store.Time
		var eng sql.NullInt64
		var resolution string
		err := r.Scan(&f.ID, &at, &f.ClientID, &eng, &resolution, &f.Due)
		f.At, f.EngagementID, f.Kind = at.UTC(), eng.Int64, "commitment."+resolution
		return f, err
	}); err != nil {
		return nil, err
	}
	sort.SliceStable(m.Facts, func(i, j int) bool { return m.Facts[i].At.Before(m.Facts[j].At) })
	if len(m.Facts) > maxMetricFacts {
		m.Facts, m.Truncated = m.Facts[len(m.Facts)-maxMetricFacts:], true
	}
	for _, c := range clients {
		if want(c.ID) {
			m.Clients = append(m.Clients, c)
		}
	}
	return m, nil
}
