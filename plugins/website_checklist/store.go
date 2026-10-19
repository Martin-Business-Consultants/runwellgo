package websitechecklist

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"sort"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// A client's websites (Site), each step to take one live (Item), the
// webhooks on its Webhooks step (Webhook), and the history of every change
// to them (Audit), in the plugin's own tables, as the Rails models kept
// them.

// Site is a client's website.
type Site struct {
	ID        int64
	ClientID  int64
	Name      string
	URL       string // "" for none
	CreatedAt store.Time
	UpdatedAt store.Time
}

// Item is one step to take a website live.
type Item struct {
	ID              int64
	SiteID          int64
	Key             string
	Group           string
	Title           string
	Detail          string
	Value           string
	ConfirmedValue  string
	State           string // unconfirmed, confirmed or not_applicable
	Note            string
	ConfirmedByName string
	ConfirmedAt     store.Time
	Position        int
	Custom          bool
	UpdatedAt       store.Time
}

// stateLabels are the states as people read them (Item::STATE_LABELS).
var stateLabels = map[string]string{"unconfirmed": "To do", "confirmed": "Confirmed", "not_applicable": "Not applicable"}

func (i *Item) confirmed() bool     { return i.State == "confirmed" }
func (i *Item) notApplicable() bool { return i.State == "not_applicable" }
func (i *Item) settled() bool       { return i.confirmed() || i.notApplicable() }
func (i *Item) webhooks() bool      { return i.Key == webhooksKey }
func (i *Item) how() string         { return byKey[i.Key].How }
func (i *Item) example() string     { return byKey[i.Key].Example }

// Webhook is one recorded on a site's Webhooks step.
type Webhook struct {
	ID       int64
	ItemID   int64
	Name     string
	URL      string
	Pairs    []Pair
	Position int

	changed bool // while saving: its fields moved
	diff    diff
}

// Pair is one of a webhook's keys and values.
type Pair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// String is a webhook written out: "Zapier → https://hooks.zapier.com/… (list: 12, source: website)".
func (w *Webhook) String() string {
	line := strings.Join(nonBlank(w.Name, w.URL), " → ")
	if len(w.Pairs) == 0 {
		return line
	}
	return line + " (" + pairsText(w.Pairs) + ")"
}

func pairsText(pairs []Pair) string {
	parts := make([]string, len(pairs))
	for i, p := range pairs {
		parts[i] = p.Key + ": " + p.Value
	}
	return strings.Join(parts, ", ")
}

func (w *Webhook) label() string {
	if s := nonBlank(w.Name, w.URL); len(s) > 0 {
		return s[0]
	}
	return "Webhook"
}

func nonBlank(ss ...string) []string {
	out := []string{}
	for _, s := range ss {
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// nullable is "" as NULL, as the Rails models normalize blank text to nil.
func nullable(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// ---- reading -------------------------------------------------------------------------------

const siteColumns = `id, client_id, name, COALESCE(url, ''), created_at, updated_at`

func scanSite(r interface{ Scan(...any) error }) (*Site, error) {
	s := &Site{}
	return s, r.Scan(&s.ID, &s.ClientID, &s.Name, &s.URL, &s.CreatedAt, &s.UpdatedAt)
}

// errNotFound is a site, step, file or recommendation that isn't there.
var errNotFound = errors.New("not found")

func findSite(ctx context.Context, q plugin.Querier, id int64) (*Site, error) {
	s, err := scanSite(q.QueryRow(ctx, `SELECT `+siteColumns+` FROM website_checklist_sites WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return s, err
}

// allSites are every site, or one client's (clientID), in the order the
// list shows them (Site.ordered): by address, A to Z, without its scheme or
// a leading "www."; those without one last, by name.
func allSites(ctx context.Context, api *plugin.API, clientID int64) ([]*Site, error) {
	query := `SELECT ` + siteColumns + ` FROM website_checklist_sites`
	args := []any{}
	if clientID != 0 {
		query += ` WHERE client_id = ?`
		args = append(args, clientID)
	}
	rows, err := api.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Site
	for rows.Next() {
		s, err := scanSite(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sort.SliceStable(out, func(i, j int) bool {
		a, b := out[i], out[j]
		if (a.URL == "") != (b.URL == "") {
			return a.URL != ""
		}
		if ka, kb := addressKey(a.URL), addressKey(b.URL); ka != kb {
			return ka < kb
		}
		return a.Name < b.Name
	})
	return out, nil
}

func addressKey(u string) string {
	s := strings.ToLower(strings.NewReplacer("https://", "", "http://", "").Replace(u))
	return strings.TrimPrefix(s, "www.")
}

// host is the address's host, without a leading "www." (Site#host).
func (s *Site) host() string {
	if s.URL == "" {
		return ""
	}
	u, err := url.Parse(s.URL)
	if err != nil || u.Host == "" {
		return s.URL
	}
	return strings.TrimPrefix(u.Hostname(), "www.")
}

const itemColumns = `id, site_id, "key", "group", title, COALESCE(detail, ''), COALESCE(value, ''), COALESCE(confirmed_value, ''), state, COALESCE(note, ''), COALESCE(confirmed_by_name, ''), confirmed_at, position, custom, updated_at`

func scanItem(r interface{ Scan(...any) error }) (*Item, error) {
	i := &Item{}
	return i, r.Scan(&i.ID, &i.SiteID, &i.Key, &i.Group, &i.Title, &i.Detail, &i.Value, &i.ConfirmedValue, &i.State, &i.Note,
		&i.ConfirmedByName, &i.ConfirmedAt, &i.Position, &i.Custom, &i.UpdatedAt)
}

// itemsOf are the sites' steps, by site, each site's in order (position, id).
func itemsOf(ctx context.Context, q plugin.Querier, siteIDs ...int64) (map[int64][]*Item, error) {
	out := map[int64][]*Item{}
	if len(siteIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT `+itemColumns+` FROM website_checklist_items WHERE site_id IN (`+placeholders(len(siteIDs))+`) ORDER BY position, id`, ids(siteIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		i, err := scanItem(rows)
		if err != nil {
			return nil, err
		}
		out[i.SiteID] = append(out[i.SiteID], i)
	}
	return out, rows.Err()
}

func findItem(ctx context.Context, q plugin.Querier, siteID, id int64) (*Item, error) {
	i, err := scanItem(q.QueryRow(ctx, `SELECT `+itemColumns+` FROM website_checklist_items WHERE site_id = ? AND id = ?`, siteID, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errNotFound
	}
	return i, err
}

// webhooksOf are the steps' webhooks, by step, in order.
func webhooksOf(ctx context.Context, q plugin.Querier, itemIDs ...int64) (map[int64][]*Webhook, error) {
	out := map[int64][]*Webhook{}
	if len(itemIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT id, item_id, COALESCE(name, ''), COALESCE(url, ''), COALESCE(pairs, ''), position FROM website_checklist_webhooks
		WHERE item_id IN (`+placeholders(len(itemIDs))+`) ORDER BY position, id`, ids(itemIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		w := &Webhook{}
		var pairs string
		if err := rows.Scan(&w.ID, &w.ItemID, &w.Name, &w.URL, &pairs, &w.Position); err != nil {
			return nil, err
		}
		w.Pairs = readPairs(pairs)
		out[w.ItemID] = append(out[w.ItemID], w)
	}
	return out, rows.Err()
}

// readPairs reads the pairs column (JSON, as the Rails serializer wrote it).
func readPairs(s string) []Pair {
	var raw []map[string]any
	_ = json.Unmarshal([]byte(s), &raw)
	out := []Pair{}
	for _, r := range raw {
		out = append(out, Pair{Key: fmt.Sprint(orBlank(r["key"])), Value: fmt.Sprint(orBlank(r["value"]))})
	}
	return out
}

func orBlank(v any) any {
	if v == nil {
		return ""
	}
	return v
}

func placeholders(n int) string { return strings.TrimSuffix(strings.Repeat("?,", n), ",") }

func ids(list []int64) []any {
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out
}

// ---- the checklist -------------------------------------------------------------------------

// Summary is a checklist's counts over the steps that apply.
type Summary struct {
	Total         int  `json:"total"`
	Confirmed     int  `json:"confirmed"`
	Unconfirmed   int  `json:"unconfirmed"`
	NotApplicable int  `json:"not_applicable"`
	Ready         bool `json:"ready"`
}

func summarize(items []*Item) Summary {
	var s Summary
	for _, i := range items {
		switch {
		case i.notApplicable():
			s.NotApplicable++
		case i.confirmed():
			s.Total++
			s.Confirmed++
		default:
			s.Total++
			s.Unconfirmed++
		}
	}
	s.Ready = s.Total > 0 && s.Confirmed == s.Total
	return s
}

// String is "All 26 confirmed" or "4 of 26 confirmed".
func (s Summary) String() string {
	if s.Ready {
		return fmt.Sprintf("All %d confirmed", s.Total)
	}
	return fmt.Sprintf("%d of %d confirmed", s.Confirmed, s.Total)
}

// attention is "4 to do", or "" when every step is done.
func (s Summary) attention() string {
	if s.Unconfirmed > 0 {
		return fmt.Sprintf("%d to do", s.Unconfirmed)
	}
	return ""
}

// ordered are the steps as the page lists them: by group (the checklist's,
// then any other), each group's in order (website_item_groups).
func ordered(items []*Item) []*Item {
	out := make([]*Item, 0, len(items))
	seen := map[string]bool{}
	for _, g := range allGroups {
		seen[g] = true
		for _, i := range items {
			if i.Group == g {
				out = append(out, i)
			}
		}
	}
	var others []string
	for _, i := range items {
		if !seen[i.Group] {
			seen[i.Group] = true
			others = append(others, i.Group)
		}
	}
	for _, g := range others {
		for _, i := range items {
			if i.Group == g {
				out = append(out, i)
			}
		}
	}
	return out
}

// nextItem is the first step still to do, in the page's order: where the
// checklist picks up.
func nextItem(items []*Item) *Item {
	for _, i := range ordered(items) {
		if !i.settled() {
			return i
		}
	}
	return nil
}

// applyStandard adds the standard steps a site is missing and brings the
// rest to their order and wording (Site#apply_standard_checklist). System
// work, so not in the history, as the Rails bulk writes weren't.
func applyStandard(ctx context.Context, q plugin.Querier, siteID int64, existing []*Item) error {
	have := map[string]*Item{}
	for _, i := range existing {
		have[i.Key] = i
	}
	now := store.Now().DB()
	for n, d := range defaults {
		pos := n * 10
		if i := have[d.Key]; i != nil {
			if i.Group != d.Group || i.Title != d.Title || i.Detail != d.Detail || i.Position != pos {
				if _, err := q.Exec(ctx, `UPDATE website_checklist_items SET "group" = ?, title = ?, detail = ?, position = ?, updated_at = ? WHERE id = ?`,
					d.Group, d.Title, nullable(d.Detail), pos, now, i.ID); err != nil {
					return err
				}
			}
			continue
		}
		if _, err := q.Exec(ctx, `INSERT INTO website_checklist_items (site_id, "key", "group", title, detail, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			siteID, d.Key, d.Group, d.Title, nullable(d.Detail), pos, now, now); err != nil {
			return err
		}
	}
	return nil
}

// outOfDate says whether a site's steps miss a standard one, or have one
// out of its order or wording.
func outOfDate(items []*Item) bool {
	have := map[string]*Item{}
	for _, i := range items {
		have[i.Key] = i
	}
	for n, d := range defaults {
		i := have[d.Key]
		if i == nil || i.Group != d.Group || i.Title != d.Title || i.Detail != d.Detail || i.Position != n*10 {
			return true
		}
	}
	return false
}

// touchSite marks a site changed when one of its steps is (belongs_to touch).
func touchSite(ctx context.Context, q plugin.Querier, siteID int64) error {
	_, err := q.Exec(ctx, `UPDATE website_checklist_sites SET updated_at = ? WHERE id = ?`, store.Now().DB(), siteID)
	return err
}

// ---- history -------------------------------------------------------------------------------

// change is one field's before and after, as people read them (nil for blank).
type change struct {
	Field  string
	Before any
	After  any
}

// diff is a change's fields in order, kept as a JSON object in that order.
type diff []change

// add keeps a field whose value moved; blank is nil.
func (d *diff) add(field, before, after string) {
	if before == after {
		return
	}
	*d = append(*d, change{field, nullable(before), nullable(after)})
}

func (d diff) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, c := range d {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(c.Field)
		v, _ := json.Marshal([]any{c.Before, c.After})
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// readDiff reads a diff column, keeping its fields' order.
func readDiff(s string) diff {
	dec := json.NewDecoder(strings.NewReader(s))
	if t, err := dec.Token(); err != nil || t != json.Delim('{') {
		return nil
	}
	var out diff
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return out
		}
		field, _ := t.(string)
		var pair []any
		if err := dec.Decode(&pair); err != nil {
			return out
		}
		c := change{Field: field}
		if len(pair) > 0 {
			c.Before = pair[0]
		}
		if len(pair) > 1 {
			c.After = pair[1]
		}
		out = append(out, c)
	}
	return out
}

// auditEntry is one change to a website, a step or a webhook.
type auditEntry struct {
	SiteID      int64
	SiteName    string
	ItemID      int64 // 0 for a site's own
	RecordType  string
	RecordID    int64
	RecordLabel string
	Action      string
	Diff        diff
}

// audit records a change with who made it (Audited#audit): a person, or
// their agent. An update that changed nothing people read isn't one.
func audit(ctx context.Context, q plugin.Querier, actor *plugin.Caller, e auditEntry) error {
	if e.Action == "updated" && len(e.Diff) == 0 {
		return nil
	}
	if e.Diff == nil {
		e.Diff = diff{}
	}
	d, _ := e.Diff.MarshalJSON()
	var actorID, actorName, via any
	source := "app"
	if actor != nil {
		actorID, actorName = actor.ID, nilIfBlank(actor.Name)
		if actor.Agent {
			// The person's agent, as the core names it ("Ted Martin's agent"),
			// and the app it came through.
			actorName, source, via = actor.Name+"'s agent", "agent", nilIfBlank(actor.Via)
			if actor.AgentID != 0 {
				actorID = actor.AgentID
			}
			if actor.Name == "" {
				actorName = nil
			}
		}
	}
	var itemID any
	if e.ItemID != 0 {
		itemID = e.ItemID
	}
	_, err := q.Exec(ctx, `INSERT INTO website_checklist_audits (site_id, site_name, item_id, record_type, record_id, record_label, action, diff, actor_id, actor_name, source, via, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.SiteID, e.SiteName, itemID, e.RecordType, e.RecordID, nullable(e.RecordLabel), e.Action, string(d), actorID, actorName, source, via, store.Now().DB())
	return err
}

// itemAction says what a step's change was (Item#audit_action): confirmed,
// marked not applicable or reopened; a value saved and confirmed at once
// was recorded.
func itemAction(d diff) string {
	var state *change
	value := false
	for i := range d {
		switch d[i].Field {
		case "state":
			state = &d[i]
		case "value":
			value = true
		}
	}
	if state == nil {
		return "updated"
	}
	if value {
		if state.After == "Confirmed" {
			return "recorded"
		}
		return "updated"
	}
	switch state.After {
	case "Confirmed":
		return "confirmed"
	case "Not applicable":
		return "marked not applicable"
	case "To do":
		return "reopened"
	}
	return "updated"
}

// fieldLabels are the fields as the history shows them (Audit::FIELD_LABELS).
var fieldLabels = map[string]string{"name": "Name", "url": "Address", "client_id": "Client", "title": "Title", "detail": "Detail",
	"group": "Group", "value": "Value", "confirmed_value": "Confirmed as", "state": "State", "note": "Note",
	"custom": "Custom", "pairs": "Keys and values", "files": "Files"}

func fieldLabel(f string) string {
	if l, ok := fieldLabels[f]; ok {
		return l
	}
	s := strings.ReplaceAll(strings.TrimSuffix(f, "_id"), "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// ---- a step's changes ------------------------------------------------------------------------

// itemChange is what a person sends for a step: its value and note, a
// custom step's title and detail, and the Webhooks step's webhooks. Nil is
// left as it is.
type itemChange struct {
	Value  *string
	Note   *string
	Title  *string
	Detail *string
}

// webhookChange is one row of the Webhooks step's form: id for one that's
// saved, Destroy to remove it.
type webhookChange struct {
	ID      int64
	Name    string
	URL     string
	Pairs   []Pair
	Destroy bool
}

// saveItem writes a step's new state and records it in the history as one
// change; before is the step as it was.
func saveItem(ctx context.Context, q plugin.Querier, actor *plugin.Caller, site *Site, before, after *Item) error {
	now := store.Now()
	var confirmedAt any
	if after.ConfirmedAt.Valid {
		confirmedAt = after.ConfirmedAt.DB()
	}
	if _, err := q.Exec(ctx, `UPDATE website_checklist_items SET title = ?, detail = ?, value = ?, confirmed_value = ?, state = ?, note = ?, confirmed_by_name = ?, confirmed_at = ?, updated_at = ? WHERE id = ?`,
		after.Title, nullable(after.Detail), nullable(after.Value), nullable(after.ConfirmedValue), after.State, nullable(after.Note),
		nullable(after.ConfirmedByName), confirmedAt, now.DB(), after.ID); err != nil {
		return err
	}
	if err := touchSite(ctx, q, site.ID); err != nil {
		return err
	}
	var d diff
	d.add("title", before.Title, after.Title)
	d.add("detail", before.Detail, after.Detail)
	d.add("value", before.Value, after.Value)
	d.add("confirmed_value", before.ConfirmedValue, after.ConfirmedValue)
	d.add("state", stateLabels[before.State], stateLabels[after.State])
	d.add("note", before.Note, after.Note)
	return audit(ctx, q, actor, auditEntry{SiteID: site.ID, SiteName: site.Name, ItemID: after.ID, RecordType: "Item", RecordID: after.ID,
		RecordLabel: after.Title, Action: itemAction(d), Diff: d})
}

// saveWebhooks applies the Webhooks step's rows (nested attributes: a new
// row with no name or address is skipped, _destroy removes one) and sets
// the step's value to them, written out, each change in the history.
func saveWebhooks(ctx context.Context, q plugin.Querier, actor *plugin.Caller, site *Site, item *Item, rows []webhookChange) (string, error) {
	current, err := webhooksOf(ctx, q, item.ID)
	if err != nil {
		return "", err
	}
	byID := map[int64]*Webhook{}
	for _, w := range current[item.ID] {
		byID[w.ID] = w
	}
	now := store.Now().DB()
	entry := func(w *Webhook, action string, d diff) auditEntry {
		return auditEntry{SiteID: site.ID, SiteName: site.Name, ItemID: item.ID, RecordType: "Webhook", RecordID: w.ID, RecordLabel: w.label(), Action: action, Diff: d}
	}
	touched := map[int64]bool{}
	var kept []*Webhook
	for _, r := range rows {
		r.Name, r.URL = strings.TrimSpace(r.Name), strings.TrimSpace(r.URL)
		pairs := []Pair{}
		for _, p := range r.Pairs {
			if k := strings.TrimSpace(p.Key); k != "" {
				pairs = append(pairs, Pair{Key: k, Value: strings.TrimSpace(p.Value)})
			}
		}
		if r.ID == 0 {
			if r.Destroy || (r.Name == "" && r.URL == "") {
				continue
			}
			kept = append(kept, &Webhook{Name: r.Name, URL: r.URL, Pairs: pairs})
			continue
		}
		w := byID[r.ID]
		if w == nil || touched[w.ID] {
			continue
		}
		touched[w.ID] = true
		if r.Destroy {
			if _, err := q.Exec(ctx, `DELETE FROM website_checklist_webhooks WHERE id = ?`, w.ID); err != nil {
				return "", err
			}
			var d diff
			d.add("name", w.Name, "")
			d.add("url", w.URL, "")
			d.add("pairs", pairsText(w.Pairs), "")
			if err := audit(ctx, q, actor, entry(w, "deleted", d)); err != nil {
				return "", err
			}
			continue
		}
		var d diff
		d.add("name", w.Name, r.Name)
		d.add("url", w.URL, r.URL)
		d.add("pairs", pairsText(w.Pairs), pairsText(pairs))
		w.Name, w.URL, w.Pairs = r.Name, r.URL, pairs
		w.changed = len(d) > 0
		w.diff = d
		kept = append(kept, w)
	}
	// Saved ones not in the form stay, after the ones that are.
	for _, w := range current[item.ID] {
		if !touched[w.ID] {
			kept = append(kept, w)
		}
	}
	lines := []string{}
	for pos, w := range kept {
		pairs, _ := json.Marshal(w.Pairs)
		if w.ID == 0 {
			res, err := q.Exec(ctx, `INSERT INTO website_checklist_webhooks (item_id, name, url, pairs, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				item.ID, nullable(w.Name), nullable(w.URL), string(pairs), pos, now, now)
			if err != nil {
				return "", err
			}
			w.ID, _ = res.LastInsertId()
			var d diff
			d.add("name", "", w.Name)
			d.add("url", "", w.URL)
			d.add("pairs", "", pairsText(w.Pairs))
			if err := audit(ctx, q, actor, entry(w, "created", d)); err != nil {
				return "", err
			}
		} else if w.changed || w.Position != pos {
			if _, err := q.Exec(ctx, `UPDATE website_checklist_webhooks SET name = ?, url = ?, pairs = ?, position = ?, updated_at = ? WHERE id = ?`,
				nullable(w.Name), nullable(w.URL), string(pairs), pos, now, w.ID); err != nil {
				return "", err
			}
			if err := audit(ctx, q, actor, entry(w, "updated", w.diff)); err != nil {
				return "", err
			}
		}
		lines = append(lines, w.String())
	}
	item.Value = strings.Join(lines, "\n")
	return item.Value, nil
}

// ---- a step's verbs (Item) ---------------------------------------------------------------------

// assignRevision applies a person's change to a step (Item#revise): its
// value and note, a custom step's title and detail; a value that moved from
// previous (what it was before the save, webhooks included) is to confirm
// again.
func assignRevision(i *Item, ch itemChange, previous string) {
	if ch.Value != nil {
		i.Value = strings.TrimSpace(*ch.Value)
	}
	if ch.Note != nil {
		i.Note = strings.TrimSpace(*ch.Note)
	}
	if i.Custom {
		if ch.Title != nil {
			i.Title = strings.TrimSpace(*ch.Title)
		}
		if ch.Detail != nil {
			i.Detail = *ch.Detail
		}
	}
	if i.Value != previous && !i.notApplicable() {
		i.State = "unconfirmed"
	}
}

// record is a person saying what's true (Item#record): the value saved and
// confirmed as theirs, or an empty value back to confirm.
func record(i *Item, by *plugin.Caller) {
	if i.Value != "" {
		i.ConfirmedValue, i.State, i.ConfirmedByName, i.ConfirmedAt = i.Value, "confirmed", displayName(by), store.Now()
	} else {
		i.State, i.ConfirmedValue, i.ConfirmedByName, i.ConfirmedAt = "unconfirmed", "", "", store.Time{}
	}
}

// displayName is who did it, as a step keeps it.
func displayName(c *plugin.Caller) string {
	if c == nil {
		return "Runwell"
	}
	if c.Agent {
		return c.Name + "'s agent"
	}
	return c.Name
}

// ---- deleting ------------------------------------------------------------------------------

// deleteSite forgets a site, its steps, their webhooks, files, work links
// and audits' recommendations; its history stays, with the deletion in it.
func deleteSite(ctx context.Context, api *plugin.API, actor *plugin.Caller, s *Site, clientName string) error {
	items, err := itemsOf(ctx, api, s.ID)
	if err != nil {
		return err
	}
	scanIDs, err := int64s(ctx, api, `SELECT id FROM website_checklist_scans WHERE site_id = ?`, s.ID)
	if err != nil {
		return err
	}
	err = api.Tx(ctx, func(tx plugin.Querier) error {
		for _, q := range []string{
			`DELETE FROM website_checklist_webhooks WHERE item_id IN (SELECT id FROM website_checklist_items WHERE site_id = ?)`,
			`DELETE FROM website_checklist_recommendations WHERE item_id IN (SELECT id FROM website_checklist_items WHERE site_id = ?)`,
			`DELETE FROM website_checklist_recommendations WHERE scan_id IN (SELECT id FROM website_checklist_scans WHERE site_id = ?)`,
			`DELETE FROM website_checklist_todo_links WHERE site_id = ?`,
			`DELETE FROM website_checklist_items WHERE site_id = ?`,
			`DELETE FROM website_checklist_scans WHERE site_id = ?`,
			`DELETE FROM website_checklist_sites WHERE id = ?`,
		} {
			if _, err := tx.Exec(ctx, q, s.ID); err != nil {
				return err
			}
		}
		var d diff
		d.add("client_id", clientName, "")
		d.add("name", s.Name, "")
		d.add("url", s.URL, "")
		return audit(ctx, tx, actor, auditEntry{SiteID: s.ID, SiteName: s.Name, RecordType: "Site", RecordID: s.ID, RecordLabel: s.Name, Action: "deleted", Diff: d})
	})
	if err != nil {
		return err
	}
	// Their files after: the store isn't the tables' transaction.
	for _, i := range items[s.ID] {
		files, _ := api.Attachments(ctx, itemType, i.ID, "files")
		for _, f := range files {
			_ = api.Detach(ctx, itemType, i.ID, f.ID)
		}
	}
	for _, id := range scanIDs {
		files, _ := api.Attachments(ctx, scanType, id, "files")
		for _, f := range files {
			_ = api.Detach(ctx, scanType, id, f.ID)
		}
	}
	return nil
}

func int64s(ctx context.Context, q plugin.Querier, query string, args ...any) ([]int64, error) {
	rows, err := q.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// uniq is the ids once each, in order.
func uniq(list []int64) []int64 {
	out := []int64{}
	for _, v := range list {
		if v != 0 && !slices.Contains(out, v) {
			out = append(out, v)
		}
	}
	return out
}
