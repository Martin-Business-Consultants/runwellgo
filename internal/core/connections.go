package core

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
)

// Connections: everything one client's records tie together, for the
// overview the app draws (Svelte Flow) from a client, an engagement or a
// piece of work. Runwell is three things and their parts: who we work for
// (the client, and the requests that came from them), what we agreed (its
// engagements, their agreed scope items and commitments) and what's
// happening (the work). Asked from any of them, the answer is the client's
// whole set, with the one asked from as its focus.

func init() { routeSets = append(routeSets, (*App).connectionRoutes) }

func (a *App) connectionRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/connections", a.showConnections, tool("show_connections", "Show how a client's records connect",
		note("Everything a client's records tie together: the client, its requests, its engagements with their agreed scope items and open commitments, and their work, as nodes and the links between them (a request to what it became, work to its scope item). record: Client:<id>, Engagement:<ref or id> or Todo:<id>; the answer's focus is that one."),
		query("record", "string", "Client:<id>, Engagement:<ref or id> or Todo:<id> (required).")))
}

// connNode is one record in the overview.
type connNode struct {
	ID     string `json:"id"` // "Engagement:12"
	Type   string `json:"type"`
	Label  string `json:"label"`
	Detail string `json:"detail"` // its state, in words
	State  string `json:"state"`
	Path   string `json:"path"`
	Due    string `json:"due,omitempty"`    // a commitment's, YYYY-MM-DD
	Parent string `json:"parent,omitempty"` // the node it hangs from
}

type connEdge struct {
	Source string `json:"source"`
	Target string `json:"target"`
	Kind   string `json:"kind"` // has, became
}

func (a *App) showConnections(c *Ctx) error {
	ctx := c.R.Context()
	typ, key, _ := strings.Cut(c.Query("record"), ":")
	clientID, focus, err := a.connectionsFocus(ctx, typ, key)
	if err != nil {
		return err
	}
	nodes, edges, err := a.connections(ctx, clientID)
	if err != nil {
		return err
	}
	c.Summary = fmt.Sprintf("%d records and %d links", len(nodes), len(edges))
	return c.JSON(200, O("summary", c.Summary, "focus", focus, "nodes", nodes, "edges", edges))
}

// connectionsFocus finds the client a record belongs to, and the record's
// node id.
func (a *App) connectionsFocus(ctx context.Context, typ, key string) (int64, string, error) {
	notFound := apierr.NotFound("Couldn't find that record.")
	id, _ := strconv.ParseInt(key, 10, 64)
	switch typ {
	case "Client":
		var cid int64
		if err := a.store.QueryRowContext(ctx, `SELECT id FROM clients WHERE id = ?`, id).Scan(&cid); err != nil {
			return 0, "", notFound
		}
		return cid, "Client:" + key, nil
	case "Engagement":
		var eid, cid int64
		if err := a.store.QueryRowContext(ctx, `SELECT id, client_id FROM engagements WHERE id = ? OR ref = ?`, id, key).Scan(&eid, &cid); err != nil {
			return 0, "", notFound
		}
		return cid, fmt.Sprintf("Engagement:%d", eid), nil
	case "Todo":
		var cid int64
		if err := a.store.QueryRowContext(ctx, `SELECT e.client_id FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.id = ?`, id).Scan(&cid); err != nil {
			return 0, "", notFound
		}
		return cid, "Todo:" + key, nil
	}
	return 0, "", apierr.BadRequest("record is Client:<id>, Engagement:<ref or id> or Todo:<id>.")
}

// connections are a client's records and their links.
func (a *App) connections(ctx context.Context, clientID int64) ([]connNode, []connEdge, error) {
	nodes, edges := []connNode{}, []connEdge{}
	clientNode := fmt.Sprintf("Client:%d", clientID)
	var name, status string
	if err := a.store.QueryRowContext(ctx, `SELECT name, COALESCE(status, '') FROM clients WHERE id = ?`, clientID).Scan(&name, &status); err != nil {
		return nil, nil, err
	}
	nodes = append(nodes, connNode{ID: clientNode, Type: "client", Label: name, Detail: humanize(status), State: status, Path: fmt.Sprintf("/clients/%d", clientID)})
	link := func(from, to, kind string) { edges = append(edges, connEdge{Source: from, Target: to, Kind: kind}) }

	engs, err := a.loadEngagements(ctx, a.store, `e.client_id = ?`, clientID)
	if err != nil {
		return nil, nil, err
	}
	have := map[string]bool{clientNode: true}
	for _, e := range engs {
		id := fmt.Sprintf("Engagement:%d", e.ID)
		state := e.State()
		nodes = append(nodes, connNode{ID: id, Type: "engagement", Label: e.Ref + " " + e.Title, Detail: e.StateLabel(), State: state, Path: "/engagements/" + e.Ref, Parent: clientNode})
		have[id] = true
		link(clientNode, id, "has")
		items, err := a.agreedItems(ctx, a.store, e.ID)
		if err != nil {
			return nil, nil, err
		}
		for _, it := range items {
			sid := fmt.Sprintf("ScopeItem:%v", it.Get("id"))
			nodes = append(nodes, connNode{ID: sid, Type: "scope_item", Label: fmt.Sprint(it.Get("description")),
				Detail: fmt.Sprintf("%v of %v done", it.Get("done"), it.Get("total")), State: fmt.Sprint(it.Get("delivery_state")), Path: fmt.Sprintf("/scope_items/%v", it.Get("id")), Parent: id})
			have[sid] = true
			link(id, sid, "has")
		}
	}

	err = eachRow(ctx, a.store, `SELECT t.id, t.title, t.status, t.engagement_id, COALESCE(t.scope_item_id, 0) FROM todos t
		JOIN engagements e ON e.id = t.engagement_id WHERE e.client_id = ? ORDER BY t.engagement_id, t.due_on IS NULL, t.due_on, t.id`, []any{clientID}, func(rows *sql.Rows) error {
		var id, eid, sid int64
		var title, st string
		if err := rows.Scan(&id, &title, &st, &eid, &sid); err != nil {
			return err
		}
		parent := fmt.Sprintf("ScopeItem:%d", sid)
		if sid == 0 || !have[parent] {
			parent = fmt.Sprintf("Engagement:%d", eid)
		}
		tid := fmt.Sprintf("Todo:%d", id)
		nodes = append(nodes, connNode{ID: tid, Type: "work", Label: title, Detail: humanize(st), State: st, Path: fmt.Sprintf("/work/%d", id), Parent: parent})
		have[tid] = true
		link(parent, tid, "has")
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	err = eachRow(ctx, a.store, `SELECT id, description, due_on, COALESCE(engagement_id, 0) FROM commitments
		WHERE client_id = ? AND resolution IS NULL ORDER BY due_on, id`, []any{clientID}, func(rows *sql.Rows) error {
		var id, eid int64
		var desc string
		var due any
		if err := rows.Scan(&id, &desc, &due, &eid); err != nil {
			return err
		}
		parent := fmt.Sprintf("Engagement:%d", eid)
		if !have[parent] {
			parent = clientNode
		}
		cid := fmt.Sprintf("Commitment:%d", id)
		nodes = append(nodes, connNode{ID: cid, Type: "commitment", Label: desc, Detail: "Open", Due: dateOnly(due), State: "open", Path: fmt.Sprintf("/commitments/%d/edit", id), Parent: parent})
		link(parent, cid, "has")
		return nil
	})
	if err != nil {
		return nil, nil, err
	}

	err = eachRow(ctx, a.store, `SELECT id, subject, status, COALESCE(promoted_type, ''), COALESCE(promoted_id, 0) FROM requests
		WHERE client_id = ? ORDER BY created_at DESC, id DESC`, []any{clientID}, func(rows *sql.Rows) error {
		var id, pid int64
		var subject, st, ptype string
		if err := rows.Scan(&id, &subject, &st, &ptype, &pid); err != nil {
			return err
		}
		rid := fmt.Sprintf("Request:%d", id)
		nodes = append(nodes, connNode{ID: rid, Type: "request", Label: subject, Detail: humanize(st), State: st, Path: fmt.Sprintf("/requests/%d", id), Parent: clientNode})
		link(clientNode, rid, "has")
		if became := fmt.Sprintf("%s:%d", ptype, pid); ptype != "" && have[became] {
			link(rid, became, "became")
		}
		return nil
	})
	if err != nil {
		return nil, nil, err
	}
	return nodes, edges, nil
}

// dateOnly is a date column as YYYY-MM-DD, however SQLite gave it.
func dateOnly(v any) string {
	s := ""
	switch x := v.(type) {
	case string:
		s = x
	case []byte:
		s = string(x)
	case interface{ Format(string) string }:
		s = x.Format("2006-01-02")
	}
	if len(s) >= 10 {
		return s[:10]
	}
	return s
}
