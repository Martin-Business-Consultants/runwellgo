// Package websitechecklist is Websites, ported from the Rails plugin
// (github.com/Martin-Business-Consultants/plugin-website-checklist) onto
// the Go plugin API alone: every client's websites, each with a numbered
// checklist to take it live (access first, then forms and email, launch and
// after launch), each step recorded and confirmed by a person, with files,
// webhooks, work on the client's engagements and a history of every change.
// It keeps the Rails plugin's tables (website_checklist_*) and its files
// (WebsiteChecklist::Item's "files"), so a Rails install's data carries
// over, and draws its own screens (ui/src/plugins/website_checklist).
package websitechecklist

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Key is the plugin's key.
const Key = "website_checklist"

// manage is the permission to add, change and delete websites.
const manage = Key + ":manage_websites"

// The record types its files attach to, as the Rails models are named.
const (
	itemType = "WebsiteChecklist::Item"
	scanType = "WebsiteChecklist::Scan"
)

func q(name, typ, desc string, required bool) plugin.ToolParam {
	return plugin.ToolParam{Name: name, Type: typ, Description: desc, Required: required}
}

// Module is the plugin, for the core to register.
func Module() *plugin.Module {
	return &plugin.Module{
		Manifest: plugin.Manifest{
			Key:      Key,
			Name:     "Websites",
			Version:  "0.4.0",
			Author:   "Martin Business Consultants",
			Homepage: "https://github.com/Martin-Business-Consultants/plugin-website-checklist",
			Description: "Every client's websites, each with a step-by-step checklist to take it live: access to the " +
				"domain, DNS, Search Console and Analytics first, then forms and email, launch and after launch, " +
				"each recorded and confirmed by a person.",
			Permissions: plugin.Grants{RecordsRead: true, RecordsWrite: true, Tables: true, Files: true, PublicHTTP: true},
			Hooks:       []string{"client.deleted", "user.removed"},
			Schema:      schema,
			Capabilities: []plugin.Capability{
				{Key: manage, Label: "Add, change and delete websites", Roles: []string{"owner", "manager"}},
			},
			Routes: []plugin.Route{
				{Method: "GET", Path: "/websites", Export: "index", Tool: &plugin.Tool{
					Name: "list_websites", Title: "List websites",
					Description: "Every client's websites with how much of each checklist is confirmed. show: attention (the default: a step still to do) or all. client_id: one client's.",
					Query:       []plugin.ToolParam{q("show", "", "attention (the default) or all.", false), q("client_id", "integer", "One client's websites.", false)},
				}},
				{Method: "GET", Path: "/websites/{id}", Export: "show", Tool: &plugin.Tool{
					Name: "show_website", Title: "Show a website's checklist",
					Description: "A website and its go-live checklist, in order: each step's value, whether it's confirmed (and who confirmed it, when) or not applicable, and how to do it. next_step is the first one still to do. engagements are its client's open ones, for add_website_work.",
				}},
				{Method: "POST", Path: "/websites", Export: "create", Capability: manage, Tool: &plugin.Tool{
					Name: "create_website", Title: "Add a website",
					Description: "A client's website, with the standard go-live checklist: access (domain, DNS, Search Console, Analytics…), forms and email, launch, after launch.",
					Wrap:        "site", Body: `{client_id (required): integer, name (required), url}`,
				}},
				{Method: "PATCH", Path: "/websites/{id}", Export: "update", Capability: manage, Tool: &plugin.Tool{
					Name: "update_website", Title: "Change a website", Description: "Its name or address.",
					Wrap: "site", Body: `{name, url}`,
				}},
				{Method: "DELETE", Path: "/websites/{id}", Export: "destroy", Capability: manage, Tool: &plugin.Tool{
					Name: "delete_website", Title: "Delete a website", Description: "Forgets the website and its checklist.",
				}},
				{Method: "GET", Path: "/websites/clients", Export: "clients"},

				{Method: "POST", Path: "/websites/{site_id}/items", Export: "addItem", Tool: &plugin.Tool{
					Name: "add_website_item", Title: "Add an item to a website's checklist",
					Description: "An item of your own, beyond the standard checklist (it goes under Custom): what to check, and its value if known.",
					Wrap:        "item", Body: `{title (required), detail, value, note}`,
				}},
				{Method: "PATCH", Path: "/websites/{site_id}/items/{id}", Export: "updateItem", Tool: &plugin.Tool{
					Name: "update_website_item", Title: "Record a website checklist item's value",
					Description: "The item's value (what is true now) and a note. Saving a value confirms it as yours, with the time; an empty value is to confirm again. A custom item's title and detail can change too. The Webhooks item records webhooks_attributes instead of a value: each webhook's name, url and pairs (keys and values); pass id to change one, id with _destroy: true to remove it.",
					Wrap:        "item", Body: `{value, note, title, detail, webhooks_attributes: [{id: integer, name, url, pairs: [{key, value}], _destroy: boolean}]}`,
				}},
				{Method: "DELETE", Path: "/websites/{site_id}/items/{id}", Export: "deleteItem", Tool: &plugin.Tool{
					Name: "delete_website_item", Title: "Remove a custom item from a website's checklist",
					Description: "Only items added beyond the standard checklist can be removed; mark the others not applicable.",
				}},
				{Method: "POST", Path: "/websites/{site_id}/items/{id}/confirmation", Export: "confirm", Tool: &plugin.Tool{
					Name: "confirm_website_item", Title: "Confirm a website checklist item",
					Description: "Says the item's value is true, as you checked it: it's kept as what was confirmed, with your name and the time. value and note are recorded first when given.",
					Wrap:        "item", Body: `{value, note}`,
				}},
				{Method: "DELETE", Path: "/websites/{site_id}/items/{id}/confirmation", Export: "reopen", Tool: &plugin.Tool{
					Name: "reopen_website_item", Title: "Reopen a website checklist item",
					Description: "Undoes a confirmation or not applicable: the item is to confirm again.",
				}},
				{Method: "POST", Path: "/websites/{site_id}/items/{id}/not_applicable", Export: "notApplicable", Tool: &plugin.Tool{
					Name: "mark_website_item_not_applicable", Title: "Mark a website checklist item not applicable",
					Description: "This site doesn't need it (a site with no forms has no form emails). It stops counting toward the checklist. note: why.",
					Wrap:        "item", Body: `{note}`,
				}},
				{Method: "POST", Path: "/websites/{site_id}/items/{item_id}/files", Export: "addFiles", MaxBody: 10 << 20, Tool: &plugin.Tool{
					Name: "add_website_item_file", Title: "Add a file to a website checklist step",
					Description: "Keeps a file on the step (a sitemap, a DNS export, a screenshot), in its history too. Up to 10 MB.",
					Params:      []plugin.ToolParam{q("files", "file", "The file: {filename, content_base64, content_type}.", true)},
				}},
				{Method: "DELETE", Path: "/websites/{site_id}/items/{item_id}/files/{id}", Export: "removeFile", Tool: &plugin.Tool{
					Name: "remove_website_item_file", Title: "Remove a file from a website checklist step",
					Description: "The file's id is in show_website's items[].files.",
				}},
				{Method: "GET", Path: "/websites/{site_id}/items/{item_id}/files/{id}", Export: "download"},

				{Method: "POST", Path: "/websites/{site_id}/todos", Export: "addWork", Tool: &plugin.Tool{
					Name: "add_website_work", Title: "Add work from a website's checklist",
					Description: "A todo on one of the website's client's engagements (engagement_id: show_website's engagements), about one of its checklist steps (item_id, optional): it shows on that step, and its description links back to it. owner_id leads it; other_owner_ids are also on it.",
					Wrap:        "todo", Body: `{engagement_id (required): integer, item_id: integer, title (required), description, owner_id: integer, due_on: YYYY-MM-DD, other_owner_ids: [integer]}`,
				}},

				{Method: "GET", Path: "/websites/history", Export: "history", Tool: &plugin.Tool{
					Name: "website_history", Title: "Read the websites' history",
					Description: "Every change to websites, their checklist steps and webhooks, newest first: who made it (and whether through an agent), when, and each field's before and after. site_id: one website's; item_id (with site_id): one step's, with its webhooks; before: an audit id, for the page after.",
					Query: []plugin.ToolParam{q("site_id", "integer", "One website's.", false), q("item_id", "integer", "One step's (with site_id).", false),
						q("before", "integer", "An audit id: the changes before it.", false)},
				}},

				{Method: "GET", Path: "/websites/{site_id}/scans", Export: "scans", Tool: &plugin.Tool{
					Name: "show_website_ai_audit", Title: "Show a website's AI audit",
					Description: "The latest audit of a website: whether it's still running, what was found about the domain (DNS, registrar, pages, sitemap) and each recommendation for a checklist step (value, note, evidence, confidence) with whether it's pending, approved or rejected.",
				}},
				{Method: "POST", Path: "/websites/{site_id}/scans", Export: "scan", Capability: manage, Tool: &plugin.Tool{
					Name: "audit_website_with_ai", Title: "Run an AI audit of a website",
					Description: "Finds out what it can about the domain from outside (DNS, the registry, the pages, robots.txt, the sitemap), in the background (a minute or so); show_website_ai_audit has the result. Recommendations for the checklist's steps need the AI plugin, which this install doesn't have, so it gathers the facts and keeps the sitemap as a file; nothing is recorded on the checklist.",
					Wrap:        "scan", Body: `{domain (required)}`,
				}},
				{Method: "POST", Path: "/websites/{site_id}/recommendations/{recommendation_id}/approval", Export: "approve", Tool: &plugin.Tool{
					Name: "approve_website_recommendation", Title: "Approve an AI audit's recommendation",
					Description: "Records the recommendation on its checklist step as yours (confirmed, in the history), with value and note changed first when given, and attaches any file it brought (the sitemap).",
					Wrap:        "recommendation", Body: `{value, note}`,
				}},
				{Method: "POST", Path: "/websites/{site_id}/recommendations/{recommendation_id}/rejection", Export: "reject", Tool: &plugin.Tool{
					Name: "reject_website_recommendation", Title: "Reject an AI audit's recommendation",
					Description: "Leaves its checklist step as it is.",
				}},
			},
			Nav: &plugin.NavPage{Label: "Websites", Path: "/websites", Icon: "globe"},
		},
		Handlers: map[string]plugin.Handler{
			"index": index, "show": show, "create": create, "update": update, "destroy": destroy, "clients": clients,
			"addItem": addItem, "updateItem": updateItem, "deleteItem": deleteItem, "confirm": confirm, "reopen": reopen,
			"notApplicable": notApplicable, "addFiles": addFiles, "removeFile": removeFile, "download": download,
			"addWork": addWork, "history": history, "scans": scans, "scan": scan, "approve": approve, "reject": reject,
		},
		OnEvent: onEvent,
	}
}

// ---- answers -------------------------------------------------------------------------------

// obj is a JSON object that keeps its keys in order, as the jbuilder views
// wrote them.
type obj []kv

type kv struct {
	k string
	v any
}

func o(pairs ...any) obj {
	out := make(obj, 0, len(pairs)/2)
	for i := 0; i+1 < len(pairs); i += 2 {
		out = append(out, kv{pairs[i].(string), pairs[i+1]})
	}
	return out
}

func (m obj) set(k string, v any) obj {
	for i := range m {
		if m[i].k == k {
			m[i].v = v
			return m
		}
	}
	return append(m, kv{k, v})
}

func (m obj) MarshalJSON() ([]byte, error) {
	var b bytes.Buffer
	b.WriteByte('{')
	for i, p := range m {
		if i > 0 {
			b.WriteByte(',')
		}
		k, _ := json.Marshal(p.k)
		v, err := json.Marshal(p.v)
		if err != nil {
			return nil, err
		}
		b.Write(k)
		b.WriteByte(':')
		b.Write(v)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// refuse is an answer the caller should read: what the Rails page said in
// its alert.
func refuse(status int, message string) *plugin.Response {
	code := map[int]string{http.StatusNotFound: "not_found", http.StatusForbidden: "forbidden"}[status]
	if code == "" {
		code = "invalid"
	}
	return plugin.JSON(status, map[string]any{"error": code, "message": message})
}

func missing(what string) *plugin.Response {
	return refuse(http.StatusNotFound, "Couldn't find that "+what+".")
}

// failed answers a write's error: one the caller should see, or the plugin
// failing.
func failed(err error) (*plugin.Response, error) {
	if errors.Is(err, errNotFound) {
		return missing("record"), nil
	}
	if res := plugin.Answer(err); res != nil {
		return res, nil
	}
	return nil, err
}

// ---- requests ------------------------------------------------------------------------------

// queryID is an id from the route's path ({site_id}) or its query, 0 when
// it's missing or not one.
func queryID(req *plugin.Request, name string) int64 {
	v, ok := req.Params[name]
	if !ok {
		v = req.Query.Get(name)
	}
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}

// params is the request's JSON body.
func params(req *plugin.Request) map[string]any {
	m := map[string]any{}
	if len(bytes.TrimSpace(req.Body)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(req.Body))
		dec.UseNumber()
		_ = dec.Decode(&m)
	}
	return m
}

// wrapped is the object under key ({"site": {…}}), empty when there's none.
func wrapped(m map[string]any, key string) map[string]any {
	if w, ok := m[key].(map[string]any); ok {
		return w
	}
	return map[string]any{}
}

// text is a field as text, and whether it was sent.
func text(m map[string]any, key string) (*string, bool) {
	v, ok := m[key]
	if !ok {
		return nil, false
	}
	var s string
	if v != nil {
		s = fmt.Sprint(v)
	}
	return &s, true
}

func textOf(m map[string]any, key string) string {
	if s, ok := text(m, key); ok {
		return *s
	}
	return ""
}

func intOf(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, _ := n.Int64()
		return i
	case float64:
		return int64(n)
	case int64:
		return n
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	}
	return 0
}

func truthy(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "1" || b == "true"
	case json.Number:
		return b.String() == "1"
	}
	return false
}

// rows reads a list sent as an array or, as a form sends it, an object of
// rows by index.
func rows(v any) []map[string]any {
	var out []map[string]any
	switch l := v.(type) {
	case []any:
		for _, r := range l {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(l))
		for k := range l {
			keys = append(keys, k)
		}
		sortNumeric(keys)
		for _, k := range keys {
			if m, ok := l[k].(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

func sortNumeric(keys []string) {
	less := func(a, b string) bool {
		x, e1 := strconv.ParseInt(a, 10, 64)
		y, e2 := strconv.ParseInt(b, 10, 64)
		if e1 == nil && e2 == nil {
			return x < y
		}
		return a < b
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && less(keys[j], keys[j-1]); j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
}

// can is whether the caller may add, change and delete websites.
func can(req *plugin.Request) bool { return req.Actor.Can(manage) }

// baseURL is the install's address, for links to the app.
func baseURL(ctx context.Context, api *plugin.API) string {
	s, err := api.Site(ctx)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(s.URL, "/")
}
