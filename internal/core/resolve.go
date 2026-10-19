package core

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// What a person calls a record, turned into the record: "Bloom", "priya",
// "the holiday hours job", "WO-12" (Rails' Agent::Resolver). A tool's ids
// (path ids, client_id, owner_id… and "Type:name" records) may be names;
// an MCP call resolves them before it runs. One match is used; several
// answer "ambiguous" with the candidates to choose from; none answers
// "not_found". The resolve tool offers the same lookup directly.

func init() { routeSets = append(routeSets, (*App).resolveRoutes) }

func (a *App) resolveRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/resolve", a.showResolution, tool("resolve", "Find the record a name refers to",
		note(`Give what the person called it (a client's name, a person, an engagement's ref or title, a todo's title…) and optionally its type. Answers the matching records, best first, with the id and record ("Type:id") other tools take. Most tools also accept the name in place of an id.`),
		query("q", "string", "What the person called it."),
		query("type", "string", "One of "+strings.Join(resolverTypes, ", ")+" (optional).")))
}

// resolverFields are the name columns each kind of record is called by.
var resolverFields = map[string][]string{
	"Client": {"name"}, "Contact": {"name", "email"}, "User": {"name", "email_address"},
	"Engagement": {"title"}, "Todo": {"title"}, "Request": {"subject"},
	"Commitment": {"description"}, "ScopeItem": {"description"},
}

// resolverTypes are the kinds of record a name resolves to, in order.
var resolverTypes = []string{"Client", "Contact", "User", "Engagement", "Todo", "Request", "Commitment", "ScopeItem"}

var engagementRef = regexp.MustCompile(`^[A-Za-z]+-\d+$`)

// isID is whether v is an id already (a number, or digits).
func isID(v any) bool {
	switch x := v.(type) {
	case float64, int, int64:
		return true
	case string:
		_, err := strconv.ParseInt(x, 10, 64)
		return err == nil
	}
	return false
}

// resolved is one record a name matched.
type resolved struct {
	Type     string
	ID       int64
	Ref      string
	Name     string
	ClientID int64
}

// ambiguousError is several records matching a name.
type ambiguousError struct {
	typ, text  string
	candidates []resolved
}

func (e *ambiguousError) Error() string {
	return "More than one " + strings.ToLower(humanize(snake(e.typ))) + " matches “" + e.text + "”."
}

// missingError is no record matching a name.
type missingError struct{ typ, text string }

func (e *missingError) Error() string {
	return "No " + strings.ToLower(humanize(snake(e.typ))) + " matches “" + e.text + "”."
}

// resolverScope is where a type's records are looked for: every one on
// staff, or for a client's agent only their own client's (engagements,
// shared work, requests; nothing else).
func resolverScope(typ string, clientID int64) (from, where string, args []any, ok bool) {
	if clientID != 0 {
		switch typ {
		case "Engagement":
			return "engagements", "client_id = ?", []any{clientID}, true
		case "Todo":
			return "todos", "client_visible = 1 AND engagement_id IN (SELECT id FROM engagements WHERE client_id = ?)", []any{clientID}, true
		case "Request":
			return "requests", "client_id = ?", []any{clientID}, true
		}
		return "", "", nil, false
	}
	switch typ {
	case "User":
		return "users", "deactivated_at IS NULL AND agent_owner_id IS NULL", nil, true
	case "Contact":
		return "contacts", "archived_at IS NULL", nil, true
	}
	if _, known := resolverFields[typ]; !known {
		return "", "", nil, false
	}
	return searchTable(typ), "1 = 1", nil, true
}

// candidates are the records of typ that text names, best first: an
// engagement's ref, then names equal to it (any case), then starting with
// it, then containing it, then full-text search.
func (a *App) candidates(ctx context.Context, typ, text string, limit int, clientID int64) ([]resolved, error) {
	text = strings.TrimSpace(text)
	from, where, args, ok := resolverScope(typ, clientID)
	if text == "" || !ok {
		return nil, nil
	}
	var ids []int64
	var err error
	if typ == "Engagement" && engagementRef.MatchString(text) {
		ids, err = store.Int64s(ctx, a.store, `SELECT id FROM engagements WHERE `+where+` AND ref = ?`, append(slices.Clone(args), strings.ToUpper(text))...)
		if err != nil {
			return nil, err
		}
		return a.describeAll(ctx, typ, ids)
	}
	lowered := strings.ToLower(text)
	like := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(lowered)
	for _, try := range []struct{ op, value string }{{"= ?", lowered}, {`LIKE ? ESCAPE '\'`, like + "%"}, {`LIKE ? ESCAPE '\'`, "%" + like + "%"}} {
		var conds []string
		var vals []any
		for _, f := range resolverFields[typ] {
			conds = append(conds, "lower("+f+") "+try.op)
			vals = append(vals, try.value)
		}
		ids, err = store.Int64s(ctx, a.store, `SELECT id FROM `+from+` WHERE `+where+` AND (`+strings.Join(conds, " OR ")+`) ORDER BY id LIMIT ?`,
			append(append(slices.Clone(args), vals...), limit)...)
		if err != nil {
			return nil, err
		}
		if len(ids) > 0 {
			return a.describeAll(ctx, typ, ids)
		}
	}
	hits, err := a.search(ctx, text)
	if err != nil {
		return nil, err
	}
	for _, h := range hits {
		if h.Type != typ || len(ids) >= limit {
			continue
		}
		n, err := store.Count(ctx, a.store, `SELECT count(*) FROM `+from+` WHERE `+where+` AND id = ?`, append(slices.Clone(args), h.ID)...)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			ids = append(ids, h.ID)
		}
	}
	return a.describeAll(ctx, typ, ids)
}

// resolveName is the one record text names, or missingError / ambiguousError.
func (a *App) resolveName(ctx context.Context, typ, text string, clientID int64) (resolved, error) {
	found, err := a.candidates(ctx, typ, text, 8, clientID)
	if err != nil {
		return resolved{}, err
	}
	switch len(found) {
	case 0:
		return resolved{}, &missingError{typ, text}
	case 1:
		return found[0], nil
	}
	return resolved{}, &ambiguousError{typ, text, found}
}

// idFor is an id as given, or the id of the record a name stands for.
func (a *App) idFor(ctx context.Context, typ string, v any, clientID int64) (any, error) {
	if isID(v) {
		return v, nil
	}
	r, err := a.resolveName(ctx, typ, fmt.Sprint(v), clientID)
	if err != nil {
		return nil, err
	}
	return r.ID, nil
}

// describeAll reads what's said of each record: its name, ref and client.
func (a *App) describeAll(ctx context.Context, typ string, ids []int64) ([]resolved, error) {
	var out []resolved
	for _, id := range ids {
		info, ok, err := a.subjectInfo(ctx, typ, id)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, resolved{Type: typ, ID: id, Ref: info.Ref, Name: info.Name, ClientID: info.ClientID})
		}
	}
	return out, nil
}

// describe is a record as the resolver answers it.
func (a *App) describeResolved(ctx context.Context, r resolved) Obj {
	o := O("type", r.Type, "id", r.ID, "record", fmt.Sprintf("%s:%d", r.Type, r.ID))
	if r.Ref != "" {
		o = o.Set("ref", r.Ref)
	}
	o = o.Set("name", r.Name)
	if r.ClientID != 0 && r.Type != "Client" {
		var name string
		if a.store.QueryRowContext(ctx, `SELECT name FROM clients WHERE id = ?`, r.ClientID).Scan(&name) == nil {
			o = o.Set("client", name)
		}
	}
	return o
}

// subject is what's known of a record an event or a name points at.
type subject struct {
	Name         string
	Ref          string
	Path         string
	ClientID     int64
	EngagementID int64
}

// subjectInfo reads a record's name, ref, page and client: what agentRef
// and the resolver say of it, for every kind of record the core keeps. ok
// false when it's gone.
func (a *App) subjectInfo(ctx context.Context, typ string, id int64) (subject, bool, error) {
	var s subject
	var err error
	q := func(query string, dest ...any) { err = a.store.QueryRowContext(ctx, query, id).Scan(dest...) }
	switch typ {
	case "Client":
		q(`SELECT name FROM clients WHERE id = ?`, &s.Name)
		s.ClientID, s.Path = id, fmt.Sprintf("/clients/%d", id)
	case "Contact":
		q(`SELECT name, client_id FROM contacts WHERE id = ?`, &s.Name, &s.ClientID)
		s.Path = fmt.Sprintf("/clients/%d", s.ClientID)
	case "User":
		var email string
		q(`SELECT name, email_address FROM users WHERE id = ?`, &s.Name, &email)
		s.Name = strOr(s.Name, email)
	case "Engagement":
		var title string
		q(`SELECT ref, title, client_id FROM engagements WHERE id = ?`, &s.Ref, &title, &s.ClientID)
		s.Name, s.EngagementID, s.Path = s.Ref+" "+title, id, "/engagements/"+s.Ref
	case "Todo":
		q(`SELECT t.title, t.engagement_id, e.client_id FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.id = ?`, &s.Name, &s.EngagementID, &s.ClientID)
		s.Path = fmt.Sprintf("/work/%d", id)
	case "Request":
		var client store.NullInt
		q(`SELECT subject, client_id FROM requests WHERE id = ?`, &s.Name, &client)
		s.ClientID, s.Path = client.Int64, fmt.Sprintf("/requests/%d", id)
	case "Commitment":
		var eng store.NullInt
		q(`SELECT description, client_id, engagement_id FROM commitments WHERE id = ?`, &s.Name, &s.ClientID, &eng)
		s.EngagementID, s.Path = eng.Int64, fmt.Sprintf("/commitments/%d", id)
	case "ScopeItem":
		var ref string
		q(`SELECT s.description, v.engagement_id, e.client_id, e.ref FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id JOIN engagements e ON e.id = v.engagement_id WHERE s.id = ?`,
			&s.Name, &s.EngagementID, &s.ClientID, &ref)
		s.Path = "/engagements/" + ref
	case "AgreementVersion":
		var kind, ref string
		var number int
		q(`SELECT v.kind, v.number, v.engagement_id, e.client_id, e.ref FROM agreement_versions v JOIN engagements e ON e.id = v.engagement_id WHERE v.id = ?`,
			&kind, &number, &s.EngagementID, &s.ClientID, &ref)
		label := "Agreement"
		if kind != "initial" {
			label = fmt.Sprintf("%s #%d", humanize(kind), number)
		}
		s.Name, s.Path = ref+" "+label, "/engagements/"+ref
	case "Note":
		var kind, onType string
		var onID int64
		q(`SELECT kind, subject_type, subject_id FROM notes WHERE id = ?`, &kind, &onType, &onID)
		if err == nil {
			s.Name = humanize(kind) + " note"
			if on, ok, e := a.subjectInfo(ctx, onType, onID); e == nil && ok {
				s.ClientID, s.EngagementID, s.Path = on.ClientID, on.EngagementID, on.Path
			}
		}
	case "Document":
		var onType string
		var onID int64
		q(`SELECT documentable_type, documentable_id FROM documents WHERE id = ?`, &onType, &onID)
		if err == nil {
			if b, e := a.attachedBlob(ctx, a.store, "Document", id, "file"); e == nil && b != nil {
				s.Name = b.Filename
			}
			if on, ok, e := a.subjectInfo(ctx, onType, onID); e == nil && ok {
				s.ClientID, s.EngagementID, s.Path = on.ClientID, on.EngagementID, on.Path
			}
		}
	case "Question":
		q(`SELECT text FROM questions WHERE id = ?`, &s.Name)
		s.Path = "/"
	case "Setting":
		s.Name = "Settings"
	default:
		return subject{Name: fmt.Sprintf("%s %d", typ, id)}, true, nil
	}
	if isNoRows(err) {
		return s, false, nil
	}
	if err != nil {
		return s, false, err
	}
	if s.Name == "" {
		s.Name = fmt.Sprintf("%s %d", typ, id)
	}
	return s, true, nil
}

// subjectRef is a record as answers point at it (agent_ref), nil when it's
// gone.
func (a *App) recordRef(ctx context.Context, typ string, id int64) (any, error) {
	s, ok, err := a.subjectInfo(ctx, typ, id)
	if err != nil || !ok {
		return nil, err
	}
	return a.agentRef(typ, id, s.Ref, s.Name, s.Path), nil
}

func (a *App) showResolution(c *Ctx) error {
	ctx := c.R.Context()
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return apierr.BadRequest("param is missing or the value is empty: q")
	}
	types := resolverTypes
	if t := c.Query("type"); slices.Contains(resolverTypes, t) {
		types = []string{t}
	}
	var found []resolved
	for _, t := range types {
		list, err := a.candidates(ctx, t, q, 5, 0)
		if err != nil {
			return err
		}
		for _, r := range list {
			if !slices.ContainsFunc(found, func(o resolved) bool { return o.Type == r.Type && o.ID == r.ID }) {
				found = append(found, r)
			}
		}
	}
	found = found[:min(len(found), 15)]
	list := []Obj{}
	for _, r := range found {
		ref, err := a.recordRef(ctx, r.Type, r.ID)
		if err != nil {
			return err
		}
		o, _ := ref.(Obj)
		d := a.describeResolved(ctx, r)
		o = o.Set("name", d.Get("name"))
		if cl := d.Get("client"); cl != nil {
			o = o.Set("client", cl)
		}
		list = append(list, o)
	}
	switch len(found) {
	case 0:
		c.Summary = "Nothing matches “" + q + "”. Try search for a wider look."
	case 1:
		c.Summary = "“" + q + "” is " + found[0].Name + "."
	default:
		c.Summary = fmt.Sprintf("%d records match “%s”: ask the person which they mean if it isn't clear.", len(found), q)
	}
	return c.JSON(200, O("summary", c.Summary, "candidates", list))
}
