package websitechecklist

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Websites (the Rails SitesController): the list, a site's page with its
// checklist, and adding, changing and deleting one (for those who may).

// siteRef is a website as agents name it (agent_ref).
func siteRef(base string, s *Site) obj {
	return o("type", "WebsiteChecklist::Site", "id", s.ID, "record", fmt.Sprintf("WebsiteChecklist::Site:%d", s.ID),
		"display_name", s.Name, "url", fmt.Sprintf("%s/websites/%d", base, s.ID))
}

// clientsByID are the clients named, as agent refs, by id: a client
// that's gone isn't there.
func clientsByID(ctx context.Context, api *plugin.API, list []int64) (map[int64]obj, error) {
	out := map[int64]obj{}
	list = uniq(list)
	if len(list) == 0 {
		return out, nil
	}
	recs, err := api.Records(ctx, plugin.Query{Type: "Client", IDs: list, Limit: 500})
	if err != nil {
		return nil, err
	}
	for _, r := range recs {
		out[intOf(r["id"])] = o("type", r["type"], "id", r["id"], "record", r["record"], "display_name", r["display_name"], "url", r["url"])
	}
	return out, nil
}

func clientName(c obj) string {
	for _, p := range c {
		if p.k == "display_name" {
			return fmt.Sprint(p.v)
		}
	}
	return ""
}

func nilIfBlank(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// index is every client's websites (list_websites), those with steps still
// to do first unless show is all.
func index(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	show := req.Query.Get("show")
	if show != "all" {
		show = "attention"
	}
	clientID := queryID(req, "client_id")
	var client any
	if clientID != 0 {
		c, err := clientsByID(ctx, api, []int64{clientID})
		if err != nil {
			return nil, err
		}
		if c[clientID] == nil {
			return missing("client"), nil
		}
		client = c[clientID]
	}
	sites, err := allSites(ctx, api, clientID)
	if err != nil {
		return nil, err
	}
	siteIDs, clientIDs := []int64{}, []int64{}
	for _, s := range sites {
		siteIDs = append(siteIDs, s.ID)
		clientIDs = append(clientIDs, s.ClientID)
	}
	items, err := itemsOf(ctx, api, siteIDs...)
	if err != nil {
		return nil, err
	}
	clients, err := clientsByID(ctx, api, clientIDs)
	if err != nil {
		return nil, err
	}
	base := baseURL(ctx, api)
	list := []obj{}
	counts := map[string]int{"attention": 0, "all": 0}
	for _, s := range sites {
		c := clients[s.ClientID]
		if c == nil {
			continue // its client was deleted
		}
		sum := summarize(items[s.ID])
		counts["all"]++
		if !sum.Ready {
			counts["attention"]++
		} else if show == "attention" {
			continue
		}
		var next any
		if n := nextItem(items[s.ID]); n != nil {
			next = n.Title
		}
		list = append(list, siteRef(base, s).set("name", s.Name).set("address", nilIfBlank(s.URL)).set("host", nilIfBlank(s.host())).
			set("client", c).set("checklist", sum).set("checklist_text", sum.String()).set("needs_attention", nilIfBlank(sum.attention())).set("next_step", next))
	}
	word := "websites"
	if len(list) == 1 {
		word = "website"
	}
	summary := fmt.Sprintf("%d %s", len(list), word)
	if show == "attention" {
		summary += " needing attention"
	}
	return plugin.JSON(http.StatusOK, o("summary", summary, "show", show, "counts", counts, "client", client, "can_manage", can(req), "websites", list)), nil
}

// show is a website and its checklist (show_website).
func show(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "id"))
	if err != nil {
		return failedSite(err)
	}
	body, err := siteJSON(ctx, api, req, s)
	if err != nil {
		return failedSite(err)
	}
	return plugin.JSON(http.StatusOK, body), nil
}

func failedSite(err error) (*plugin.Response, error) {
	if errors.Is(err, errNotFound) {
		return missing("website"), nil
	}
	return nil, err
}

// siteJSON is a site's page as the app and agents read it (sites/show):
// its checklist in order, each step with its guidance, files, webhooks and
// work, and its client's open engagements to put work on.
func siteJSON(ctx context.Context, api *plugin.API, req *plugin.Request, s *Site) (obj, error) {
	all, err := itemsOf(ctx, api, s.ID)
	if err != nil {
		return nil, err
	}
	// The standard checklist, brought up to date (as the Rails migration did
	// whenever the list changed).
	if outOfDate(all[s.ID]) {
		if err := api.Tx(ctx, func(tx plugin.Querier) error { return applyStandard(ctx, tx, s.ID, all[s.ID]) }); err != nil {
			return nil, err
		}
		if all, err = itemsOf(ctx, api, s.ID); err != nil {
			return nil, err
		}
	}
	items := ordered(all[s.ID])
	clients, err := clientsByID(ctx, api, []int64{s.ClientID})
	if err != nil {
		return nil, err
	}
	client := clients[s.ClientID]
	if client == nil {
		return nil, errNotFound
	}
	base := baseURL(ctx, api)
	sum := summarize(items)
	next := nextItem(items)

	var hookItemIDs []int64
	for _, i := range items {
		if i.webhooks() {
			hookItemIDs = append(hookItemIDs, i.ID)
		}
	}
	hooks, err := webhooksOf(ctx, api, hookItemIDs...)
	if err != nil {
		return nil, err
	}
	work, err := workOf(ctx, api, s.ID)
	if err != nil {
		return nil, err
	}

	list := []obj{}
	for n, i := range items {
		files, err := api.Attachments(ctx, itemType, i.ID, "files")
		if err != nil {
			return nil, err
		}
		fl := []obj{}
		for _, f := range files {
			path := fmt.Sprintf("/api/x/%s/websites/%d/items/%d/files/%d", Key, s.ID, i.ID, f.ID)
			fl = append(fl, o("id", f.ID, "filename", f.Filename, "byte_size", f.ByteSize, "content_type", f.ContentType, "url", base+path, "path", path))
		}
		var confirmedAt any
		if i.ConfirmedAt.Valid {
			confirmedAt = i.ConfirmedAt
		}
		row := o("step", n+1, "id", i.ID, "key", i.Key, "group", i.Group, "title", i.Title, "detail", nilIfBlank(i.Detail),
			"how", nilIfBlank(i.how()), "example", nilIfBlank(i.example()), "value", nilIfBlank(i.Value), "confirmed_value", nilIfBlank(i.ConfirmedValue),
			"state", i.State, "state_label", stateLabels[i.State], "note", nilIfBlank(i.Note), "confirmed_by_name", nilIfBlank(i.ConfirmedByName),
			"confirmed_at", confirmedAt, "custom", i.Custom, "files", fl)
		if i.webhooks() {
			wl := []obj{}
			for _, w := range hooks[i.ID] {
				wl = append(wl, o("id", w.ID, "name", nilIfBlank(w.Name), "url", nilIfBlank(w.URL), "pairs", w.Pairs))
			}
			row = row.set("webhooks", wl)
		}
		wk := work[i.ID]
		if wk == nil {
			wk = []obj{}
		}
		list = append(list, row.set("work", wk))
	}
	summary := fmt.Sprintf("%s: %s", s.Name, sum)
	var nextStep any
	if next != nil {
		summary += "; next: " + next.Title
		nextStep = o("id", next.ID, "title", next.Title)
	}
	engagements, err := api.Records(ctx, plugin.Query{Type: "Engagement", ClientID: s.ClientID, Open: true, Limit: 100})
	if err != nil {
		return nil, err
	}
	el := []obj{}
	for _, e := range engagements {
		el = append(el, o("id", e["id"], "ref", e["ref"], "title", e["title"]))
	}
	users, err := api.Records(ctx, plugin.Query{Type: "User", Open: true, Limit: 500})
	if err != nil {
		return nil, err
	}
	people := []obj{}
	for _, u := range users {
		people = append(people, o("id", u["id"], "name", u["name"]))
	}
	siteWork := work[0]
	if siteWork == nil {
		siteWork = []obj{}
	}
	return siteRef(base, s).set("summary", summary).set("name", s.Name).set("address", nilIfBlank(s.URL)).set("host", nilIfBlank(s.host())).
		set("client", client).set("checklist", sum).set("checklist_text", sum.String()).set("needs_attention", nilIfBlank(sum.attention())).
		set("next_step", nextStep).set("items", list).set("work", siteWork).set("engagements", el).set("people", people).set("can_manage", can(req)), nil
}

// workOf is the work made from a site's page, by the step it's about (0
// for the site as a whole); work deleted in the core drops out.
func workOf(ctx context.Context, api *plugin.API, siteID int64) (map[int64][]obj, error) {
	rs, err := api.Query(ctx, `SELECT COALESCE(item_id, 0), todo_id FROM website_checklist_todo_links WHERE site_id = ? ORDER BY id`, siteID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	type link struct{ item, todo int64 }
	var links []link
	for rs.Next() {
		var l link
		if err := rs.Scan(&l.item, &l.todo); err != nil {
			return nil, err
		}
		links = append(links, l)
	}
	if err := rs.Err(); err != nil {
		return nil, err
	}
	out := map[int64][]obj{}
	if len(links) == 0 {
		return out, nil
	}
	todoIDs := []int64{}
	for _, l := range links {
		todoIDs = append(todoIDs, l.todo)
	}
	todos, err := api.Records(ctx, plugin.Query{Type: "Todo", IDs: uniq(todoIDs), Limit: 500})
	if err != nil {
		return nil, err
	}
	byID := map[int64]map[string]any{}
	for _, t := range todos {
		byID[intOf(t["id"])] = t
	}
	for _, l := range links {
		t := byID[l.todo]
		if t == nil {
			continue
		}
		var ref any
		if e, ok := t["engagement"].(map[string]any); ok {
			ref = e["ref"]
		}
		out[l.item] = append(out[l.item], o("id", t["id"], "record", t["record"], "title", t["title"], "status", t["status"],
			"due_on", t["due_on"], "engagement_ref", ref, "path", fmt.Sprintf("/work/%v", t["id"])))
	}
	return out, nil
}

// clients are the clients a website can be for (the form's choices).
func clients(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	recs, err := api.Records(ctx, plugin.Query{Type: "Client", Limit: 500})
	if err != nil {
		return nil, err
	}
	list := []obj{}
	for _, r := range recs {
		list = append(list, o("id", r["id"], "name", r["name"]))
	}
	return plugin.JSON(http.StatusOK, o("clients", list)), nil
}

var urlFormat = regexp.MustCompile(`^https?://\S+$`)

// siteFields reads and checks a site's name and address (Site's
// normalizations and validations); errors by field.
func siteFields(w map[string]any, s *Site) map[string][]string {
	errs := map[string][]string{}
	if v, ok := text(w, "name"); ok {
		s.Name = strings.TrimSpace(*v)
	}
	if v, ok := text(w, "url"); ok {
		s.URL = strings.TrimSuffix(strings.TrimSpace(*v), "/")
	}
	if s.Name == "" {
		errs["name"] = []string{"can't be blank"}
	}
	if s.URL != "" && !urlFormat.MatchString(s.URL) {
		errs["url"] = []string{"must start with http:// or https://"}
	}
	return errs
}

// create adds a website with the standard checklist (create_website).
func create(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	w := wrapped(params(req), "site")
	s := &Site{ClientID: intOf(w["client_id"])}
	errs := siteFields(w, s)
	var client obj
	if s.ClientID != 0 {
		c, err := clientsByID(ctx, api, []int64{s.ClientID})
		if err != nil {
			return nil, err
		}
		client = c[s.ClientID]
	}
	if client == nil {
		errs["client"] = []string{"must exist"}
	}
	if len(errs) > 0 {
		return plugin.Invalid(errs), nil
	}
	now := store.Now().DB()
	err := api.Tx(ctx, func(tx plugin.Querier) error {
		res, err := tx.Exec(ctx, `INSERT INTO website_checklist_sites (client_id, name, url, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			s.ClientID, s.Name, nullable(s.URL), now, now)
		if err != nil {
			return err
		}
		s.ID, _ = res.LastInsertId()
		var d diff
		d.add("client_id", "", clientName(client))
		d.add("name", "", s.Name)
		d.add("url", "", s.URL)
		if err := audit(ctx, tx, req.Actor, auditEntry{SiteID: s.ID, SiteName: s.Name, RecordType: "Site", RecordID: s.ID, RecordLabel: s.Name, Action: "created", Diff: d}); err != nil {
			return err
		}
		return applyStandard(ctx, tx, s.ID, nil)
	})
	if err != nil {
		return nil, err
	}
	return answerSite(ctx, api, req, s.ID, http.StatusCreated, s.Name+" added, with the go-live checklist.")
}

// answerSite is a write's answer: the site's page as it is now, with what
// happened.
func answerSite(ctx context.Context, api *plugin.API, req *plugin.Request, id int64, status int, message string) (*plugin.Response, error) {
	s, err := findSite(ctx, api, id)
	if err != nil {
		return failedSite(err)
	}
	body, err := siteJSON(ctx, api, req, s)
	if err != nil {
		return failedSite(err)
	}
	return plugin.JSON(status, body.set("summary", message).set("message", message)), nil
}

// update changes a website's name or address (update_website).
func update(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "id"))
	if err != nil {
		return failedSite(err)
	}
	before := *s
	if errs := siteFields(wrapped(params(req), "site"), s); len(errs) > 0 {
		return plugin.Invalid(errs), nil
	}
	err = api.Tx(ctx, func(tx plugin.Querier) error {
		if _, err := tx.Exec(ctx, `UPDATE website_checklist_sites SET name = ?, url = ?, updated_at = ? WHERE id = ?`, s.Name, nullable(s.URL), store.Now().DB(), s.ID); err != nil {
			return err
		}
		var d diff
		d.add("name", before.Name, s.Name)
		d.add("url", before.URL, s.URL)
		return audit(ctx, tx, req.Actor, auditEntry{SiteID: s.ID, SiteName: s.Name, RecordType: "Site", RecordID: s.ID, RecordLabel: s.Name, Action: "updated", Diff: d})
	})
	if err != nil {
		return nil, err
	}
	return answerSite(ctx, api, req, s.ID, http.StatusOK, s.Name+" saved.")
}

// destroy forgets a website and its checklist (delete_website); its
// history stays.
func destroy(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "id"))
	if err != nil {
		return failedSite(err)
	}
	clients, err := clientsByID(ctx, api, []int64{s.ClientID})
	if err != nil {
		return nil, err
	}
	if err := deleteSite(ctx, api, req.Actor, s, clientName(clients[s.ClientID])); err != nil {
		return nil, err
	}
	msg := s.Name + " deleted, with its checklist."
	return plugin.JSON(http.StatusOK, o("summary", msg, "message", msg)), nil
}
