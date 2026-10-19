package websitechecklist

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// The audit of a site's domain (the Rails ScansController and
// Recommendations::*). In Rails, the install's AI (the AI plugin) turned
// what the probe found into a recommendation per step, which a person
// approved, changed or rejected. Runwell Go has no AI plugin, so an audit
// here gathers the facts only; recommendations an imported Rails install
// made can still be approved or rejected.

// stuckAfter is when an audit that never finished counts as failed.
const stuckAfter = 10 * time.Minute

type scanRow struct {
	ID         int64
	SiteID     int64
	UserID     int64
	Domain     string
	State      string
	Facts      string
	Error      string
	StartedAt  store.Time
	FinishedAt store.Time
	UpdatedAt  store.Time
}

func (s *scanRow) working() bool {
	return (s.State == "queued" || s.State == "running") && time.Since(s.UpdatedAt.Time) < stuckAfter
}

func (s *scanRow) failed() bool {
	return s.State == "failed" || ((s.State == "queued" || s.State == "running") && !s.working())
}

type recommendation struct {
	ID            int64
	ScanID        int64
	ItemID        int64
	Value         string
	Note          string
	Evidence      string
	Confidence    string
	AttachFile    string
	State         string
	DecidedByName string
	DecidedAt     store.Time
}

func latestScan(ctx context.Context, q plugin.Querier, siteID int64) (*scanRow, error) {
	s := &scanRow{}
	err := q.QueryRow(ctx, `SELECT id, site_id, user_id, domain, state, COALESCE(facts, ''), COALESCE(error, ''), started_at, finished_at, updated_at
		FROM website_checklist_scans WHERE site_id = ? ORDER BY id DESC LIMIT 1`, siteID).
		Scan(&s.ID, &s.SiteID, &s.UserID, &s.Domain, &s.State, &s.Facts, &s.Error, &s.StartedAt, &s.FinishedAt, &s.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

func recommendationsOf(ctx context.Context, q plugin.Querier, scanID int64) ([]*recommendation, error) {
	rs, err := q.Query(ctx, `SELECT id, scan_id, item_id, COALESCE(value, ''), COALESCE(note, ''), COALESCE(evidence, ''), COALESCE(confidence, ''), COALESCE(attach_file, ''),
		state, COALESCE(decided_by_name, ''), decided_at FROM website_checklist_recommendations WHERE scan_id = ? ORDER BY position, id`, scanID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []*recommendation
	for rs.Next() {
		r := &recommendation{}
		if err := rs.Scan(&r.ID, &r.ScanID, &r.ItemID, &r.Value, &r.Note, &r.Evidence, &r.Confidence, &r.AttachFile, &r.State, &r.DecidedByName, &r.DecidedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rs.Err()
}

// scans is a site's latest audit (show_website_ai_audit).
func scans(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "site_id"))
	if err != nil {
		return failedSite(err)
	}
	body, err := scanJSON(ctx, api, req, s)
	if err != nil {
		return nil, err
	}
	return plugin.JSON(http.StatusOK, body), nil
}

func scanJSON(ctx context.Context, api *plugin.API, req *plugin.Request, s *Site) (obj, error) {
	sc, err := latestScan(ctx, api, s.ID)
	if err != nil {
		return nil, err
	}
	out := o("summary", s.Name+" hasn't been audited yet", "ai_available", false, "can_manage", can(req), "host", nilIfBlank(s.host()))
	if sc == nil {
		return out.set("scan", nil), nil
	}
	recs, err := recommendationsOf(ctx, api, sc.ID)
	if err != nil {
		return nil, err
	}
	items, err := itemsOf(ctx, api, s.ID)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*Item{}
	for _, i := range items[s.ID] {
		byID[i.ID] = i
	}
	pending := 0
	rl := []obj{}
	for _, r := range recs {
		if r.State == "pending" {
			pending++
		}
		var item any
		if i := byID[r.ItemID]; i != nil {
			item = o("id", i.ID, "key", i.Key, "title", i.Title, "group", i.Group, "value", nilIfBlank(i.Value), "state", i.State, "state_label", stateLabels[i.State])
		}
		var decided any
		if r.DecidedAt.Valid {
			decided = r.DecidedAt
		}
		rl = append(rl, o("id", r.ID, "state", r.State, "value", nilIfBlank(r.Value), "note", nilIfBlank(r.Note), "evidence", nilIfBlank(r.Evidence),
			"confidence", nilIfBlank(r.Confidence), "attach_file", nilIfBlank(r.AttachFile), "decided_by_name", nilIfBlank(r.DecidedByName), "decided_at", decided, "item", item))
	}
	files, err := api.Attachments(ctx, scanType, sc.ID, "files")
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, f := range files {
		names = append(names, f.Filename)
	}
	var facts any
	if sc.Facts != "" {
		facts = json.RawMessage(sc.Facts)
	}
	state := sc.State
	switch {
	case sc.working():
		out = out.set("summary", fmt.Sprintf("%s: audit of %s running", s.Name, sc.Domain))
	case sc.failed():
		state = "failed"
		out = out.set("summary", fmt.Sprintf("%s: audit of %s failed", s.Name, sc.Domain))
	default:
		out = out.set("summary", fmt.Sprintf("%s: audit of %s ready, %d of %d recommendations to review", s.Name, sc.Domain, pending, len(recs)))
	}
	at := func(t store.Time) any {
		if t.Valid {
			return t
		}
		return nil
	}
	return out.set("scan", o("id", sc.ID, "domain", sc.Domain, "state", state, "error", nilIfBlank(sc.Error), "started_at", at(sc.StartedAt),
		"finished_at", at(sc.FinishedAt), "working", sc.working(), "user_id", sc.UserID, "facts", facts, "files", names, "recommendations", rl)), nil
}

// scan audits a site's domain (audit_website_with_ai): the facts, found
// while the caller waits (a few seconds), kept as the site's latest audit.
func scan(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "site_id"))
	if err != nil {
		return failedSite(err)
	}
	domain := normalizeDomain(textOf(wrapped(params(req), "scan"), "domain"))
	if domain == "" {
		return refuse(http.StatusUnprocessableEntity, "Domain can't be blank"), nil
	}
	if !domainFormat.MatchString(domain) {
		return refuse(http.StatusUnprocessableEntity, "Domain isn't a domain like example.com"), nil
	}
	now := store.Now().DB()
	var userID int64
	if req.Actor != nil {
		userID = req.Actor.ID
	}
	res, err := api.Exec(ctx, `INSERT INTO website_checklist_scans (site_id, user_id, domain, state, created_at, updated_at) VALUES (?, ?, ?, 'queued', ?, ?)`,
		s.ID, userID, domain, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := api.Background("audit", func(ctx context.Context, api *plugin.API) { runScan(ctx, api, id, domain) }); err != nil {
		_, _ = api.Exec(ctx, `UPDATE website_checklist_scans SET state = 'failed', error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, err.Error(), now, now, id)
		return refuse(http.StatusUnprocessableEntity, err.Error()), nil
	}
	body, err := scanJSON(ctx, api, req, s)
	if err != nil {
		return nil, err
	}
	msg := fmt.Sprintf("Auditing %s: it takes a minute or so.", domain)
	return plugin.JSON(http.StatusCreated, body.set("summary", msg).set("message", msg)), nil
}

// runScan is the audit itself, off the request (the Rails ScanJob): the
// probe's facts and files kept on the scan. Never fails loudly: what went
// wrong is kept in error. Recommendations need the AI plugin, which Runwell
// Go doesn't have, so it stops at the facts.
func runScan(ctx context.Context, api *plugin.API, id int64, domain string) {
	fail := func(err error) {
		now := store.Now().DB()
		_, _ = api.Exec(context.Background(), `UPDATE website_checklist_scans SET state = 'failed', error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, truncate(err.Error(), 500), now, now, id)
		api.Log(fmt.Sprintf("audit %d: %v", id, err))
	}
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `UPDATE website_checklist_scans SET state = 'running', started_at = ?, updated_at = ? WHERE id = ?`, now, now, id); err != nil {
		fail(err)
		return
	}
	facts, files := probe(ctx, api, domain)
	raw, _ := json.Marshal(facts)
	if _, err := api.Exec(ctx, `UPDATE website_checklist_scans SET facts = ?, updated_at = ? WHERE id = ?`, string(raw), store.Now().DB(), id); err != nil {
		fail(err)
		return
	}
	for _, f := range files {
		if _, err := api.Attach(ctx, scanType, id, "files", f.name, f.contentType, bytes.NewReader(f.body)); err != nil {
			fail(err)
			return
		}
	}
	done := store.Now().DB()
	if _, err := api.Exec(ctx, `UPDATE website_checklist_scans SET state = 'ready', finished_at = ?, updated_at = ? WHERE id = ?`, done, done, id); err != nil {
		fail(err)
	}
}

// theRecommendation is the recommendation a call is about, on the site's
// audits.
func theRecommendation(ctx context.Context, api *plugin.API, req *plugin.Request) (*Site, *recommendation, *plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "site_id"))
	if errors.Is(err, errNotFound) {
		return nil, nil, missing("website"), nil
	} else if err != nil {
		return nil, nil, nil, err
	}
	r := &recommendation{}
	err = api.QueryRow(ctx, `SELECT r.id, r.scan_id, r.item_id, COALESCE(r.value, ''), COALESCE(r.note, ''), COALESCE(r.attach_file, ''), r.state
		FROM website_checklist_recommendations r JOIN website_checklist_scans s ON s.id = r.scan_id WHERE s.site_id = ? AND r.id = ?`, s.ID, queryID(req, "recommendation_id")).
		Scan(&r.ID, &r.ScanID, &r.ItemID, &r.Value, &r.Note, &r.AttachFile, &r.State)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil, missing("recommendation"), nil
	}
	return s, r, nil, err
}

// approve records a recommendation on its step as the caller's
// (approve_website_recommendation, Recommendation#approve!), changed first
// when they say so, with any file it brought.
func approve(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, r, res, err := theRecommendation(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	p := wrapped(params(req), "recommendation")
	value, note := r.Value, r.Note
	if v, ok := text(p, "value"); ok {
		value = *v
	}
	if v, ok := text(p, "note"); ok {
		note = *v
	}
	value, note = strings.TrimSpace(value), strings.TrimSpace(note)
	if value == "" {
		return refuse(http.StatusUnprocessableEntity, "Value can't be blank"), nil
	}
	before, err := findItem(ctx, api, s.ID, r.ItemID)
	if err != nil {
		return failed(err)
	}
	after := *before
	notes := nonBlank(before.Note)
	if note != "" && note != before.Note {
		notes = append(notes, note)
	}
	joined := strings.Join(notes, "\n")
	assignRevision(&after, itemChange{Value: &value, Note: &joined}, before.Value)
	record(&after, req.Actor)
	err = api.Tx(ctx, func(tx plugin.Querier) error {
		if err := saveItem(ctx, tx, req.Actor, s, before, &after); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `UPDATE website_checklist_recommendations SET state = 'approved', value = ?, note = ?, decided_by_name = ?, decided_at = ?, updated_at = ? WHERE id = ?`,
			value, nullable(note), displayName(req.Actor), store.Now().DB(), store.Now().DB(), r.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if r.AttachFile != "" {
		files, _ := api.Attachments(ctx, scanType, r.ScanID, "files")
		for _, f := range files {
			if f.Filename != r.AttachFile {
				continue
			}
			rd, _, err := api.OpenFile(ctx, scanType, r.ScanID, f.ID)
			if err != nil {
				break
			}
			_, err = api.Attach(ctx, itemType, after.ID, "files", f.Filename, f.ContentType, rd)
			rd.Close()
			if err == nil {
				_ = fileAudit(ctx, api, req.Actor, s, &after, "file added", "", f.Filename)
			}
			break
		}
	}
	return answerScan(ctx, api, req, s, fmt.Sprintf("Recorded on “%s”.", after.Title))
}

// reject leaves a recommendation's step as it is
// (reject_website_recommendation).
func reject(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, r, res, err := theRecommendation(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `UPDATE website_checklist_recommendations SET state = 'rejected', decided_by_name = ?, decided_at = ?, updated_at = ? WHERE id = ?`,
		displayName(req.Actor), now, now, r.ID); err != nil {
		return nil, err
	}
	title := ""
	if i, err := findItem(ctx, api, s.ID, r.ItemID); err == nil {
		title = i.Title
	}
	return answerScan(ctx, api, req, s, fmt.Sprintf("Rejected the recommendation for “%s”.", title))
}

func answerScan(ctx context.Context, api *plugin.API, req *plugin.Request, s *Site, msg string) (*plugin.Response, error) {
	body, err := scanJSON(ctx, api, req, s)
	if err != nil {
		return nil, err
	}
	return plugin.JSON(http.StatusOK, body.set("message", msg)), nil
}
