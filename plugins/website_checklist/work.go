package websitechecklist

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Work from a website's page (the Rails TodosController): a piece of the
// core's work on one of the site's client's open engagements, made through
// the core (todo.created, as whoever asked), linked to the site and the
// step it's about, whose description links back to that step.

func addWork(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "site_id"))
	if err != nil {
		return failedSite(err)
	}
	p := wrapped(params(req), "todo")
	var item *Item
	if id := intOf(p["item_id"]); id != 0 {
		if item, err = findItem(ctx, api, s.ID, id); errors.Is(err, errNotFound) {
			item = nil
		} else if err != nil {
			return nil, err
		}
	}
	engagements, err := api.Records(ctx, plugin.Query{Type: "Engagement", ClientID: s.ClientID, Open: true, Limit: 500})
	if err != nil {
		return nil, err
	}
	var engagement map[string]any
	for _, e := range engagements {
		if intOf(e["id"]) == intOf(p["engagement_id"]) {
			engagement = e
		}
	}
	if engagement == nil {
		clients, err := clientsByID(ctx, api, []int64{s.ClientID})
		if err != nil {
			return nil, err
		}
		return refuse(http.StatusUnprocessableEntity, fmt.Sprintf("Choose one of %s's open %s.", clientName(clients[s.ClientID]), strings.ToLower(term(ctx, api, "engagement", "Engagements")))), nil
	}

	// The person's details, then where it came from: the step on the
	// website's page.
	link := fmt.Sprintf("%s/websites/%d", baseURL(ctx, api), s.ID)
	label := s.Name
	if item != nil {
		link += fmt.Sprintf("?item=%d", item.ID)
		label += " · " + item.Title
	}
	description := strings.TrimSpace(textOf(p, "description")) +
		fmt.Sprintf(`<p>From the website checklist: <a href="%s">%s</a></p>`, html.EscapeString(link), html.EscapeString(label))

	work, err := api.AddWork(ctx, plugin.Work{Ref: fmt.Sprint(engagement["ref"]), Title: textOf(p, "title"), Description: description,
		OwnerID: intOf(p["owner_id"]), DueOn: strings.TrimSpace(textOf(p, "due_on"))})
	if err != nil {
		return failed(err)
	}
	todoID := intOf(work["id"])
	if others := otherOwners(p["other_owner_ids"], intOf(p["owner_id"])); len(others) > 0 {
		if _, err := api.ChangeWork(ctx, plugin.WorkChange{ID: todoID, OtherOwnerIDs: &others}); err != nil {
			if res := plugin.Answer(err); res == nil {
				return nil, err
			}
		}
	}
	var itemID any
	if item != nil {
		itemID = item.ID
	}
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `INSERT INTO website_checklist_todo_links (site_id, item_id, todo_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
		s.ID, itemID, todoID, now, now); err != nil {
		return nil, err
	}
	res, err := answerSite(ctx, api, req, s.ID, http.StatusCreated,
		fmt.Sprintf("Added “%v” to %v %v.", work["title"], engagement["ref"], engagement["title"]))
	if item != nil {
		res = withItem(res, item.ID)
	}
	return res, err
}

// otherOwners are the others on a piece of work, without its lead.
func otherOwners(v any, lead int64) []int64 {
	out := []int64{}
	list, _ := v.([]any)
	for _, x := range list {
		if id := intOf(x); id != 0 && id != lead {
			out = append(out, id)
		}
	}
	return uniq(out)
}

// term is the install's word for one of its things, many of them
// (Settings › Names), or fallback.
func term(ctx context.Context, api *plugin.API, key, fallback string) string {
	if site, err := api.Site(ctx); err == nil {
		if t := site.Terms[key]["other"]; t != "" {
			return t
		}
	}
	return fallback
}
