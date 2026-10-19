package websitechecklist

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// The history (the Rails AuditsController): every change to websites, their
// steps and webhooks, newest first, for one site, one step and its
// webhooks, or all; downloadable as CSV.

const (
	perPage  = 100
	csvLimit = 50000
)

type auditRow struct {
	ID          int64
	SiteID      int64
	SiteName    string
	ItemID      int64
	RecordType  string
	RecordID    int64
	RecordLabel string
	Action      string
	Diff        diff
	ActorID     int64
	ActorName   string
	Source      string
	Via         string
	CreatedAt   store.Time
}

// who is "Sarah", or "Ted's agent via Claude" (Audit#actor_label).
func (a *auditRow) who() string {
	name := a.ActorName
	if name == "" {
		name = "Runwell"
	}
	if a.Source == "agent" {
		via := a.Via
		if via == "" {
			via = "an agent"
		}
		name += " via " + via
	}
	return name
}

// subject is the step's title, or "Webhook · Zapier" (Audit#subject_label).
func (a *auditRow) subject() string {
	if a.RecordType == "Item" {
		return a.RecordLabel
	}
	return a.RecordType + " · " + a.RecordLabel
}

func history(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	var site *Site
	var item *Item
	if id := queryID(req, "site_id"); id != 0 {
		s, err := findSite(ctx, api, id)
		if err != nil {
			return failedSite(err)
		}
		site = s
		if iid := queryID(req, "item_id"); iid != 0 {
			i, err := findItem(ctx, api, s.ID, iid)
			if errors.Is(err, errNotFound) {
				return missing("checklist item"), nil
			} else if err != nil {
				return nil, err
			}
			item = i
		}
	}
	where, args := []string{"1 = 1"}, []any{}
	if site != nil {
		where, args = append(where, "site_id = ?"), append(args, site.ID)
	}
	if item != nil {
		where, args = append(where, "item_id = ?"), append(args, item.ID)
	}
	if req.Query.Get("format") == "csv" {
		list, err := audits(ctx, api, where, args, csvLimit)
		if err != nil {
			return nil, err
		}
		name := "websites"
		if site != nil {
			name = parameterize(site.Name)
		}
		return &plugin.Response{Status: http.StatusOK, Body: historyCSV(list), Headers: map[string]string{
			"Content-Type":        "text/csv; charset=utf-8",
			"Content-Disposition": fmt.Sprintf(`attachment; filename="%s-history-%s.csv"`, name, time.Now().Format(time.DateOnly)),
		}}, nil
	}
	if before := queryID(req, "before"); before != 0 {
		where, args = append(where, "id < ?"), append(args, before)
	}
	list, err := audits(ctx, api, where, args, perPage+1)
	if err != nil {
		return nil, err
	}
	var older any
	if len(list) > perPage {
		older = list[perPage-1].ID
		list = list[:perPage]
	}
	out := []obj{}
	for _, a := range list {
		changes := []obj{}
		for _, c := range a.Diff {
			changes = append(changes, o("field", fieldLabel(c.Field), "before", c.Before, "after", c.After))
		}
		var itemID, actor any
		if a.ItemID != 0 {
			itemID = a.ItemID
		}
		switch {
		case a.ActorID != 0:
			actor = o("id", a.ActorID, "name", a.ActorName)
		case a.ActorName != "":
			// Someone since removed: their name, no person to link.
			actor = o("id", nil, "name", a.ActorName)
		}
		out = append(out, o("id", a.ID, "created_at", a.CreatedAt, "action", a.Action, "source", a.Source, "site_id", a.SiteID, "site_name", a.SiteName,
			"item_id", itemID, "record_type", a.RecordType, "record_id", a.RecordID, "who", a.who(), "subject", a.subject(), "changes", changes, "actor", actor))
	}
	word := "changes"
	if len(out) == 1 {
		word = "change"
	}
	summary := fmt.Sprintf("%d %s", len(out), word)
	var siteObj, itemObj any
	if site != nil {
		to := site.Name
		siteObj = o("id", site.ID, "name", site.Name)
		if item != nil {
			to = item.Title
			itemObj = o("id", item.ID, "title", item.Title)
		}
		summary += " to " + to
	}
	summary += ", newest first"
	return plugin.JSON(http.StatusOK, o("summary", summary, "site", siteObj, "item", itemObj, "audits", out, "older_before", older)), nil
}

func audits(ctx context.Context, api *plugin.API, where []string, args []any, limit int) ([]*auditRow, error) {
	rs, err := api.Query(ctx, `SELECT id, COALESCE(site_id, 0), COALESCE(site_name, ''), COALESCE(item_id, 0), record_type, COALESCE(record_id, 0), COALESCE(record_label, ''),
		action, COALESCE(diff, ''), COALESCE(actor_id, 0), COALESCE(actor_name, ''), COALESCE(source, ''), COALESCE(via, ''), created_at
		FROM website_checklist_audits WHERE `+strings.Join(where, " AND ")+fmt.Sprintf(` ORDER BY id DESC LIMIT %d`, limit), args...)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	var out []*auditRow
	for rs.Next() {
		a := &auditRow{}
		var d string
		if err := rs.Scan(&a.ID, &a.SiteID, &a.SiteName, &a.ItemID, &a.RecordType, &a.RecordID, &a.RecordLabel, &a.Action, &d,
			&a.ActorID, &a.ActorName, &a.Source, &a.Via, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Diff = readDiff(d)
		out = append(out, a)
	}
	return out, rs.Err()
}

// historyCSV is a row per field changed (one for a change with none).
func historyCSV(list []*auditRow) []byte {
	var b bytes.Buffer
	w := csv.NewWriter(&b)
	_ = w.Write([]string{"When", "Who", "Source", "Website", "Subject", "Action", "Field", "Before", "After"})
	cell := func(v any) string {
		if v == nil {
			return ""
		}
		return fmt.Sprint(v)
	}
	for _, a := range list {
		when := ""
		if a.CreatedAt.Valid {
			when = a.CreatedAt.Time.UTC().Format(time.RFC3339)
		}
		changes := a.Diff
		if len(changes) == 0 {
			changes = diff{{}}
		}
		for _, c := range changes {
			field := ""
			if c.Field != "" {
				field = fieldLabel(c.Field)
			}
			_ = w.Write([]string{when, a.who(), a.Source, a.SiteName, a.subject(), a.Action, field, cell(c.Before), cell(c.After)})
		}
	}
	w.Flush()
	return b.Bytes()
}

var notWord = regexp.MustCompile(`[^a-z0-9]+`)

// parameterize is a name as a file name's part: "Acme Main site" → "acme-main-site".
func parameterize(s string) string {
	return strings.Trim(notWord.ReplaceAllString(strings.ToLower(s), "-"), "-")
}
