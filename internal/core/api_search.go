package core

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Search everything (SearchesController): the index's best 50 matches, each
// with where it opens in the app and what it is in this install's words.

func init() { routeSets = append(routeSets, (*App).searchRoutes) }

func (a *App) searchRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/search", a.showSearch, tool("search", "Search everything",
		note("Full-text search across clients, contacts, engagements, work, requests, commitments, notes and documents, ranked by relevance."),
		query("q", "string", "What to look for (required).")))
}

func (a *App) showSearch(c *Ctx) error {
	ctx := c.R.Context()
	q := strings.TrimSpace(c.Query("q"))
	hits, err := a.search(ctx, q)
	if err != nil {
		return err
	}
	refs := make([]searchRef, len(hits))
	for i, h := range hits {
		refs[i] = searchRef{h.Type, h.ID}
	}
	infos, err := a.recordInfos(ctx, refs)
	if err != nil {
		return err
	}
	clientIDs := []int64{}
	for _, h := range hits {
		if h.ClientID != 0 && h.Type != "Client" {
			clientIDs = append(clientIDs, h.ClientID)
		}
	}
	clientNames, err := namesByID(ctx, a.store, "clients", clientIDs)
	if err != nil {
		return err
	}
	results := make([]Obj, 0, len(hits))
	for i, h := range hits {
		info := infos[refs[i]]
		if info == nil {
			continue // gone since it was indexed
		}
		context := info.Label
		if name := clientNames[h.ClientID]; name != "" && h.Type != "Client" {
			context += " · " + name
		}
		var snippet any
		if h.Snippet != "" {
			snippet = h.Snippet
		}
		results = append(results, a.agentRef(h.Type, h.ID, info.Ref, info.Display, info.Path).
			Set("title", h.Title).Set("snippet", snippet).Set("label", info.Label).Set("context", context).Set("path", info.Path))
	}
	if q == "" {
		c.Summary = "Give a query."
	} else {
		c.Summary = fmt.Sprintf("%s for “%s”", pluralize(len(results), "result"), q)
	}
	return c.JSON(200, O("summary", c.Summary, "query", q, "results", results))
}

// namesByID are the names of rows in table, by id.
func namesByID(ctx context.Context, db store.Querier, table string, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id, name FROM `+table+` WHERE id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, err
		}
		out[id] = name
	}
	return out, rows.Err()
}

// recordInfo is how a record shows wherever another names it: in search,
// a notification, a note's or document's link back.
type recordInfo struct {
	Display  string // its agent_label
	Path     string // where it opens in the app
	Ref      string // an engagement's ref
	Label    string // what it is, in this install's words
	ClientID int64
	// What a notification's header says it's about: an id-like slot and a
	// name (the Rails notification_target_label).
	HeadRef, HeadName string
}

// recordInfos looks up how each record shows, a query per type; a record
// that's gone has none.
func (a *App) recordInfos(ctx context.Context, refs []searchRef) (map[searchRef]*recordInfo, error) {
	out := map[searchRef]*recordInfo{}
	byType := map[string][]int64{}
	for _, r := range refs {
		if _, seen := out[r]; seen || r.ID == 0 {
			continue
		}
		out[r] = nil
		byType[r.Type] = append(byType[r.Type], r.ID)
	}
	if len(byType) == 0 {
		return out, nil
	}
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	var heldOn []heldRecord
	for typ, ids := range byType {
		q := recordInfoQuery(typ, `(`+store.Placeholders(len(ids))+`)`)
		if q == "" {
			continue
		}
		held, err := a.scanRecordInfos(ctx, s, typ, q, idArgs(ids), out)
		if err != nil {
			return nil, err
		}
		heldOn = append(heldOn, held...)
	}
	if len(heldOn) > 0 {
		subjects := make([]searchRef, len(heldOn))
		for i, h := range heldOn {
			subjects[i] = h.subject
		}
		sub, err := a.recordInfos(ctx, subjects)
		if err != nil {
			return nil, err
		}
		for _, h := range heldOn {
			info, si := out[h.ref], sub[h.subject]
			if si == nil {
				continue
			}
			info.Path, info.ClientID = si.Path, si.ClientID
			if h.ref.Type == "Note" {
				info.HeadRef, info.HeadName = si.HeadRef, si.HeadName
			}
		}
	}
	for r, info := range out {
		if info == nil {
			delete(out, r)
		}
	}
	return out, nil
}

// heldRecord is a note or document, which shows as what it's kept on.
type heldRecord struct {
	ref, subject searchRef
	display      string
}

// recordInfoQuery is the query for how records of typ show (id, display,
// and three columns by type), for the ids in in; "" for a type that has
// none.
func recordInfoQuery(typ, in string) string {
	switch typ {
	case "Client":
		return `SELECT id, name, '', '', id, '' FROM clients WHERE id IN ` + in
	case "Contact":
		return `SELECT id, name, '', '', client_id, '' FROM contacts WHERE id IN ` + in
	case "Engagement":
		return `SELECT e.id, e.ref || ' ' || e.title, e.ref, e.label, e.client_id, c.name FROM engagements e JOIN clients c ON c.id = e.client_id WHERE e.id IN ` + in
	case "Todo":
		return `SELECT t.id, t.title, e.ref, e.title, e.client_id, '' FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.id IN ` + in
	case "Request":
		return `SELECT id, subject, '', '', COALESCE(client_id, 0), '' FROM requests WHERE id IN ` + in
	case "Commitment":
		return `SELECT m.id, m.description, COALESCE(e.ref, ''), '', m.client_id, '' FROM commitments m LEFT JOIN engagements e ON e.id = m.engagement_id WHERE m.id IN ` + in
	case "ScopeItem":
		return `SELECT s.id, s.description, e.ref, e.title, e.client_id, '' FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id JOIN engagements e ON e.id = v.engagement_id WHERE s.id IN ` + in
	case "AgreementVersion":
		return `SELECT v.id, CASE v.kind WHEN 'initial' THEN 'Agreement' ELSE upper(substr(replace(v.kind, '_', ' '), 1, 1)) || substr(replace(v.kind, '_', ' '), 2) || ' #' || v.number END,
			e.ref, e.title, e.client_id, '' FROM agreement_versions v JOIN engagements e ON e.id = v.engagement_id WHERE v.id IN ` + in
	case "Note":
		return `SELECT id, kind, subject_type, '', subject_id, '' FROM notes WHERE id IN ` + in
	case "Document":
		return `SELECT d.id, COALESCE((SELECT b.filename FROM active_storage_attachments x JOIN active_storage_blobs b ON b.id = x.blob_id
				WHERE x.record_type = 'Document' AND x.record_id = d.id AND x.name = 'file' ORDER BY x.id DESC LIMIT 1), ''),
			d.documentable_type, '', d.documentable_id, '' FROM documents d WHERE d.id IN ` + in
	default:
		return ""
	}
}

// scanRecordInfos adds what q finds for records of typ to out: the notes
// and documents among them still to be named after what they're on.
func (a *App) scanRecordInfos(ctx context.Context, s *Settings, typ, q string, args []any, out map[searchRef]*recordInfo) ([]heldRecord, error) {
	rows, err := a.store.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var heldOn []heldRecord
	for rows.Next() {
		var id, num int64
		var display, x, y, z string
		if err := rows.Scan(&id, &display, &x, &y, &num, &z); err != nil {
			return nil, err
		}
		ref := searchRef{typ, id}
		info := &recordInfo{Display: display, ClientID: num}
		switch typ {
		case "Client":
			info.Path, info.Label = fmt.Sprintf("/clients/%d", id), s.Term("client", 1)
			info.HeadRef, info.HeadName = s.Term("client", 1), display
		case "Contact":
			info.Path, info.Label = fmt.Sprintf("/clients/%d", num), "Contact"
			info.HeadRef, info.HeadName = "Contact", display
		case "Engagement":
			info.Ref, info.Path, info.Label = x, "/engagements/"+url.PathEscape(x), s.LabelName(y, 1)
			if info.Label == "" {
				info.Label = s.Term("engagement", 1)
			}
			info.HeadRef, info.HeadName = x, z
		case "Todo":
			info.Path, info.Label = fmt.Sprintf("/work/%d", id), s.Term("work", 1)
			info.HeadRef, info.HeadName = x, y
		case "Request":
			info.Path, info.Label = fmt.Sprintf("/requests/%d", id), "Request"
			info.HeadRef, info.HeadName = "Request", display
		case "Commitment":
			// On its engagement's page, else its client's.
			if x != "" {
				info.Path = "/engagements/" + url.PathEscape(x) + "?tab=commitments"
			} else {
				info.Path = fmt.Sprintf("/clients/%d?tab=commitments", num)
			}
			info.Label = s.Term("commitment", 1)
			info.HeadRef, info.HeadName = s.Term("commitment", 1), display
		case "ScopeItem":
			info.Path, info.Label = fmt.Sprintf("/scope_items/%d", id), s.Term("scope_item", 1)
			info.HeadRef, info.HeadName = x, y
		case "AgreementVersion":
			info.Display = x + " " + display
			info.Path, info.Label = "/engagements/"+url.PathEscape(x), "Agreement version"
			info.HeadRef, info.HeadName = x, y
		case "Note":
			info.Display, info.Label, info.ClientID = humanize(display)+" note", "Note", 0
			heldOn = append(heldOn, heldRecord{ref, searchRef{x, num}, info.Display})
		case "Document":
			info.Label, info.ClientID = "Document", 0
			info.HeadRef, info.HeadName = "Document", display
			heldOn = append(heldOn, heldRecord{ref, searchRef{x, num}, display})
		}
		out[ref] = info
	}
	return heldOn, rows.Err()
}
