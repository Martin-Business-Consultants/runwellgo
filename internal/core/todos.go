package core

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Work: a piece of work under an engagement, usually one per approved scope
// item. Small on purpose: who's on it (a lead and others), a status and a
// date. No time, no money. It lives at /work; the tables and tools stay
// todo_* and *_work, as the Rails core named them.

func init() {
	routeSets = append(routeSets, (*App).todoRoutes)
	searchable("Todo", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		var desc string
		err := db.QueryRowContext(ctx, `SELECT t.title, COALESCE(t.description, ''), e.client_id, t.created_at FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.id = ?`, id).
			Scan(&d.Title, &desc, &d.ClientID, &d.CreatedAt)
		if isNoRows(err) {
			return d, false, nil
		}
		d.Content = strings.TrimSpace(plainText(desc) + " " + customSearchText(ctx, db, "Todo", id))
		return d, err == nil, err
	})
	// An engagement's show answer has its work, by date then position.
	addSection("Engagement", "work", func(a *App, ctx context.Context, id int64) (any, error) {
		list, err := a.loadTodos(ctx, a.store, `t.engagement_id = ? ORDER BY t.due_on, t.position, t.id`, id)
		if err != nil {
			return nil, err
		}
		return a.todosJSON(ctx, a.store, list)
	})
	// Deleting an engagement takes its work with it; deleting a scope item
	// leaves its work as "other work".
	onDelete("Engagement", func(t *Tx, id int64) error {
		ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM todos WHERE engagement_id = ?`, id)
		if err != nil {
			return err
		}
		for _, tid := range ids {
			if err := t.deleteTodo(tid); err != nil {
				return err
			}
		}
		return nil
	})
	onDelete("ScopeItem", func(t *Tx, id int64) error {
		_, err := t.Exec(`UPDATE todos SET scope_item_id = NULL WHERE scope_item_id = ?`, id)
		return err
	})
}

// TodoStatuses are work's statuses. in_review: finished by whoever did it
// (often an agent), waiting for a person to check and mark it done.
var TodoStatuses = []string{"planned", "in_progress", "in_review", "blocked", "done"}

// TodoBoardColumns are the board's doing columns, between the planned
// stream and done (Fizzy's anatomy).
var TodoBoardColumns = []string{"in_progress", "in_review", "blocked"}

const todoFields = `{title (required), description: rich text, owner_id: who leads it (a person's id or name), other_owner_ids: [ids] everyone else on it, due_on: YYYY-MM-DD, scope_item_id, client_visible: boolean, custom_fields: {key: value}}`

func (a *App) todoRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/work", a.listTodos, tool("list_work", "List work (todos)",
		note("Todos across engagements. owner: me (the person you act for), none, any, or a person’s id or name (work they lead or are on); engagement: a ref or all."),
		query("status", "string", "open (the default) or all."),
		query("owner", "string", "me, none, any, or a person’s id or name."),
		query("engagement", "string", "An engagement’s ref, or all.")))
	a.handle(mux, "GET /api/work/{id}", a.showTodo, tool("show_work", "Show a todo"))
	a.handle(mux, "POST /api/engagements/{ref}/work", a.createTodo, tool("create_work", "Add work (a todo) to an engagement",
		note("owner_id: who leads it; other_owner_ids: anyone else on it."),
		body("todo", todoFields, true)))
	a.handle(mux, "PATCH /api/work/{id}", a.updateTodo, tool("update_work", "Change a todo",
		note("Change its status (planned, in_progress, in_review when finished and waiting for a person to check, blocked, done), owners, due date or details. owner_id is the lead (replacing the lead, keeping the others; blank hands the lead to whoever else is on it); other_owner_ids is everyone else on it (replaces them; [] for nobody else). Both blank: nobody. client_visible shows it in the client’s portal."),
		body("todo", `{title, description, status: planned|in_progress|in_review|blocked|done, owner_id, other_owner_ids: [ids], due_on: YYYY-MM-DD, scope_item_id, client_visible: boolean, custom_fields: {key: value}}`, true)))
	a.handle(mux, "DELETE /api/work/{id}", a.deleteTodoHandler, need("delete_records"), tool("delete_work", "Remove a todo"))
	a.handle(mux, "GET /api/work/{id}/brief", a.briefTodo, tool("brief_work", "Get a brief for doing a todo",
		note("Everything needed to do this todo and report back, as Markdown: the work, its engagement and agreed scope, recent notes, open commitments, repositories when the Code plugin is on, and how to update status and leave notes."),
		next("update_work", "add_note")))
}

// Todo is one piece of work.
type Todo struct {
	ID            int64
	EngagementID  int64
	ScopeItemID   int64
	Title         string
	Description   string
	Status        string
	OwnerID       int64
	DueOn         string
	Position      int64
	BoardPosition int64
	ClientVisible bool
	CompletedAt   store.Time
	CreatedByID   int64
	CreatedAt     store.Time
	UpdatedAt     store.Time
}

const todoColumns = `t.id, t.engagement_id, COALESCE(t.scope_item_id, 0), t.title, COALESCE(t.description, ''), t.status, COALESCE(t.owner_id, 0),
	COALESCE(t.due_on, ''), t.position, t.board_position, t.client_visible, t.completed_at, COALESCE(t.created_by_id, 0), t.created_at, t.updated_at`

func scanTodo(r interface{ Scan(...any) error }) (*Todo, error) {
	var td Todo
	err := r.Scan(&td.ID, &td.EngagementID, &td.ScopeItemID, &td.Title, &td.Description, &td.Status, &td.OwnerID,
		&td.DueOn, &td.Position, &td.BoardPosition, &td.ClientVisible, &td.CompletedAt, &td.CreatedByID, &td.CreatedAt, &td.UpdatedAt)
	td.DueOn = workDate(td.DueOn)
	return &td, err
}

// workDate is a date column as YYYY-MM-DD, whatever SQLite handed back.
func workDate(s string) string {
	if len(s) > 10 {
		return s[:10]
	}
	return s
}

// loadTodos are the todos matching where (which may end in ORDER BY).
func (a *App) loadTodos(ctx context.Context, db store.Querier, where string, args ...any) ([]*Todo, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+todoColumns+` FROM todos t WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (*Todo, error) { return scanTodo(r) })
}

// todo is the todo with id; notFound when there's none.
func (a *App) todo(ctx context.Context, db store.Querier, id int64) (*Todo, error) {
	td, err := scanTodo(db.QueryRowContext(ctx, `SELECT `+todoColumns+` FROM todos t WHERE t.id = ?`, id))
	if isNoRows(err) {
		return nil, notFound("todo")
	}
	return td, err
}

// workEngagement is what work shows of its engagement and client.
type workEngagement struct {
	ID         int64
	Ref        string
	Title      string
	Label      string
	ClientID   int64
	ClientName string
	Closed     bool
}

const workEngagementColumns = `e.id, e.ref, e.title, e.label, e.client_id, COALESCE(c.name, ''), e.closed_at IS NOT NULL`

func scanWorkEngagement(r interface{ Scan(...any) error }) (*workEngagement, error) {
	var e workEngagement
	err := r.Scan(&e.ID, &e.Ref, &e.Title, &e.Label, &e.ClientID, &e.ClientName, &e.Closed)
	return &e, err
}

// workEngagementByRef is the engagement a ref names (any case); notFound
// when there's none.
func (a *App) workEngagementByRef(ctx context.Context, db store.Querier, ref string) (*workEngagement, error) {
	e, err := scanWorkEngagement(db.QueryRowContext(ctx, `SELECT `+workEngagementColumns+` FROM engagements e LEFT JOIN clients c ON c.id = e.client_id WHERE e.ref = ?`, strings.ToUpper(strings.TrimSpace(ref))))
	if isNoRows(err) {
		return nil, notFound("engagement")
	}
	return e, err
}

// workEngagements are engagements by id, with their clients' names.
func (a *App) workEngagements(ctx context.Context, db store.Querier, ids []int64) (map[int64]*workEngagement, error) {
	out := map[int64]*workEngagement{}
	ids = workIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT `+workEngagementColumns+` FROM engagements e LEFT JOIN clients c ON c.id = e.client_id WHERE e.id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (*workEngagement, error) { return scanWorkEngagement(r) })
	for _, e := range list {
		out[e.ID] = e
	}
	return out, err
}

func (a *App) engagementRef(e *workEngagement) Obj {
	return a.agentRef("Engagement", e.ID, e.Ref, e.Ref+" "+e.Title, "/engagements/"+e.Ref)
}

// workPeople are people's display names by id (a name, else the address).
func workPeople(ctx context.Context, db store.Querier, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	ids = workIDs(ids)
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT id, CASE WHEN name = '' THEN email_address ELSE name END FROM users WHERE id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...)
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

// workIDs drops zeros and repeats, keeping the order.
func workIDs(ids []int64) []int64 {
	out := make([]int64, 0, len(ids))
	seen := map[int64]bool{}
	for _, id := range ids {
		if id != 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// todoAssignees are who's on each todo, in the order they came on.
func todoAssignees(ctx context.Context, db store.Querier, ids []int64) (map[int64][]int64, error) {
	out := map[int64][]int64{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := db.QueryContext(ctx, `SELECT todo_id, user_id FROM assignments WHERE todo_id IN (`+store.Placeholders(len(ids))+`) ORDER BY created_at, id`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var todo, user int64
		if err := rows.Scan(&todo, &user); err != nil {
			return nil, err
		}
		out[todo] = append(out[todo], user)
	}
	return out, rows.Err()
}

// todoOwners is everyone on a todo, the lead first (Todo#owners).
func todoOwners(td *Todo, assigned []int64) []int64 {
	people := slices.Clone(assigned)
	if td.OwnerID != 0 && !slices.Contains(people, td.OwnerID) {
		people = append([]int64{td.OwnerID}, people...)
	}
	slices.SortStableFunc(people, func(x, y int64) int {
		return workRank(x != td.OwnerID) - workRank(y != td.OwnerID)
	})
	return people
}

func workRank(b bool) int {
	if b {
		return 1
	}
	return 0
}

// todosJSON are todos as answers show them (todos/_todo.json), with what
// the lists show beside them (the client, when it was done). One query per
// association, never one per row.
func (a *App) todosJSON(ctx context.Context, db store.Querier, list []*Todo) ([]Obj, error) {
	out := make([]Obj, 0, len(list))
	if len(list) == 0 {
		return out, nil
	}
	ids, engIDs, itemIDs := make([]int64, len(list)), []int64{}, []int64{}
	for i, td := range list {
		ids[i] = td.ID
		engIDs = append(engIDs, td.EngagementID)
		itemIDs = append(itemIDs, td.ScopeItemID)
	}
	engagements, err := a.workEngagements(ctx, db, engIDs)
	if err != nil {
		return nil, err
	}
	assigned, err := todoAssignees(ctx, db, ids)
	if err != nil {
		return nil, err
	}
	peopleIDs := []int64{}
	for _, td := range list {
		peopleIDs = append(peopleIDs, td.OwnerID)
		peopleIDs = append(peopleIDs, assigned[td.ID]...)
	}
	people, err := workPeople(ctx, db, peopleIDs)
	if err != nil {
		return nil, err
	}
	items := map[int64]string{}
	if itemIDs = workIDs(itemIDs); len(itemIDs) > 0 {
		rows, err := db.QueryContext(ctx, `SELECT id, description FROM scope_items WHERE id IN (`+store.Placeholders(len(itemIDs))+`)`, idArgs(itemIDs)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var id int64
			var desc string
			if err := rows.Scan(&id, &desc); err != nil {
				rows.Close()
				return nil, err
			}
			items[id] = desc
		}
		rows.Close()
	}
	custom, err := a.customValues(ctx, db, "Todo", ids)
	if err != nil {
		return nil, err
	}
	today := a.today(ctx)
	for _, td := range list {
		var owner any
		others := []any{}
		for _, uid := range todoOwners(td, assigned[td.ID]) {
			if uid == td.OwnerID {
				owner = userRef(uid, people[uid])
			} else {
				others = append(others, userRef(uid, people[uid]))
			}
		}
		if td.OwnerID != 0 && owner == nil {
			owner = userRef(td.OwnerID, people[td.OwnerID])
		}
		var eng, client, item any
		if e := engagements[td.EngagementID]; e != nil {
			eng = a.engagementRef(e)
			client = a.agentRef("Client", e.ClientID, "", e.ClientName, fmt.Sprintf("/clients/%d", e.ClientID))
		}
		if desc, ok := items[td.ScopeItemID]; ok {
			item = a.agentRef("ScopeItem", td.ScopeItemID, "", desc, fmt.Sprintf("/scope_items/%d", td.ScopeItemID))
		}
		cf := custom[td.ID]
		if cf == nil {
			cf = Obj{}
		}
		out = append(out, a.agentRef("Todo", td.ID, "", td.Title, fmt.Sprintf("/work/%d", td.ID)).
			Set("title", td.Title).Set("status", td.Status).Set("due_on", nullable(td.DueOn)).Set("client_visible", td.ClientVisible).
			Set("overdue", td.overdue(today)).Set("owner", owner).Set("other_owners", others).
			Set("engagement", eng).Set("scope_item", item).Set("custom_fields", cf).
			Set("client", client).Set("completed_at", td.CompletedAt))
	}
	return out, nil
}

// overdue is whether it's late and not done.
func (td *Todo) overdue(today string) bool {
	return td.DueOn != "" && td.DueOn < today && td.Status != "done"
}

// ---- reading -----------------------------------------------------------------

// todoSorts are the columns a list sorts by (?sort=&direction=).
var todoSorts = map[string]string{
	"title":      "t.title",
	"engagement": "(SELECT ref FROM engagements WHERE id = t.engagement_id)",
	"client":     "(SELECT c.name FROM engagements e JOIN clients c ON c.id = e.client_id WHERE e.id = t.engagement_id)",
	"owner":      "(SELECT name FROM users WHERE id = t.owner_id)",
	"due":        "t.due_on",
	"status":     "CASE t.status WHEN 'planned' THEN 0 WHEN 'in_progress' THEN 1 WHEN 'in_review' THEN 2 WHEN 'blocked' THEN 3 WHEN 'done' THEN 4 END",
}

// sortClause is ORDER BY's terms for ?sort=&direction= (nulls last, then
// id), or fallback when the list doesn't sort by that.
func workSortClause(c *Ctx, sorts map[string]string, fallback string) string {
	expr, ok := sorts[c.Query("sort")]
	if !ok {
		return fallback
	}
	dir := "ASC"
	if strings.EqualFold(c.Query("direction"), "desc") {
		dir = "DESC"
	}
	return expr + " IS NULL, " + expr + " " + dir + ", id"
}

// workFilter is the Work page's owner and engagement filters as SQL (Todo.filtered).
func (a *App) workFilter(c *Ctx, owner, engagement string) (string, []any, error) {
	ctx := c.R.Context()
	where, args := []string{"1 = 1"}, []any{}
	switch owner {
	case "", "any":
	case "none":
		where = append(where, "t.owner_id IS NULL")
	default:
		var id int64
		if owner == "me" {
			id = c.Actor.User.Person().ID
		} else if n, err := strconv.ParseInt(owner, 10, 64); err == nil {
			id = n
		} else if found, err := a.workPeopleNamed(ctx, owner); err != nil {
			return "", nil, err
		} else if len(found) > 0 {
			id = found[0]
		}
		where = append(where, "t.id IN (SELECT todo_id FROM assignments WHERE user_id = ?)")
		args = append(args, id)
	}
	if engagement != "" && engagement != "all" {
		e, err := a.workEngagementByRef(ctx, a.store, engagement)
		if err != nil {
			return "", nil, err
		}
		where = append(where, "t.engagement_id = ?")
		args = append(args, e.ID)
	}
	return strings.Join(where, " AND "), args, nil
}

func (a *App) listTodos(c *Ctx) error {
	ctx := c.R.Context()
	status := c.Query("status")
	if status != "all" {
		status = "open"
	}
	owner := strOr(c.Query("owner"), "any")
	engagement := strOr(c.Query("engagement"), "all")
	where, args, err := a.workFilter(c, owner, engagement)
	if err != nil {
		return err
	}
	if status == "open" {
		where += " AND t.status != 'done'"
	}
	list, err := a.loadTodos(ctx, a.store, where+" ORDER BY "+workSortClause(c, todoSorts, "t.due_on, t.position, t.id"), args...)
	if err != nil {
		return err
	}
	work, err := a.todosJSON(ctx, a.store, list)
	if err != nil {
		return err
	}
	// The engagement filter's choices: engagements with open work, and the
	// one chosen, whatever it is.
	rows, err := a.store.QueryContext(ctx, `SELECT ref, title FROM engagements WHERE id IN (SELECT engagement_id FROM todos WHERE status != 'done') OR ref = ? ORDER BY ref`, strings.ToUpper(engagement))
	if err != nil {
		return err
	}
	choices, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var ref, title string
		err := r.Scan(&ref, &title)
		return O("ref", ref, "title", title), err
	})
	if err != nil {
		return err
	}
	if choices == nil {
		choices = []Obj{}
	}
	total, err := store.Count(ctx, a.store, `SELECT count(*) FROM todos`)
	if err != nil {
		return err
	}
	c.Summary = pluralize(len(list), "todo")
	out := O("summary", c.Summary, "status", status, "owner", owner, "engagement", engagement, "work", work,
		"engagements", choices, "any_work", total > 0)
	// One engagement's work (its Work tab) comes with its agreed scope items,
	// for grouping and the Add work form.
	if engagement != "all" {
		e, err := a.workEngagementByRef(ctx, a.store, engagement)
		if err != nil {
			return err
		}
		items, err := a.agreedItems(ctx, a.store, e.ID)
		if err != nil {
			return err
		}
		out = out.Set("scope_items", items).Set("engagement_closed", e.Closed)
	}
	return c.pagedAnswer(out, "work", work, todoSorts)
}

// agreedItems are the scope in force: every item of every approved
// version, in order, each with its work's delivery state and progress.
func (a *App) agreedItems(ctx context.Context, db store.Querier, engagementID int64) ([]Obj, error) {
	rows, err := db.QueryContext(ctx, `SELECT si.id, si.description,
			(SELECT count(*) FROM todos WHERE scope_item_id = si.id AND status = 'done'),
			(SELECT count(*) FROM todos WHERE scope_item_id = si.id),
			(SELECT count(*) FROM todos WHERE scope_item_id = si.id AND status = 'blocked')
		FROM scope_items si JOIN agreement_versions av ON av.id = si.agreement_version_id
		WHERE av.engagement_id = ? AND EXISTS (SELECT 1 FROM approvals ap WHERE ap.agreement_version_id = av.id AND ap.decision = 'approved')
		ORDER BY av.number, si.position, si.id`, engagementID)
	if err != nil {
		return nil, err
	}
	out, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var desc string
		var done, total, blocked int
		if err := r.Scan(&id, &desc, &done, &total, &blocked); err != nil {
			return nil, err
		}
		state := "in_progress"
		switch {
		case total == 0:
			state = "not_started"
		case done == total:
			state = "done"
		case blocked > 0:
			state = "blocked"
		}
		return a.agentRef("ScopeItem", id, "", desc, fmt.Sprintf("/scope_items/%d", id)).
			Set("description", desc).Set("delivery_state", state).Set("done", done).Set("total", total), nil
	})
	if out == nil {
		out = []Obj{}
	}
	return out, err
}

func (a *App) showTodo(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	o, td, err := a.todoShowJSON(ctx, id)
	if err != nil {
		return err
	}
	c.Summary = td.Title + " (" + strings.ToLower(humanize(td.Status)) + ")"
	c.Crumbs = []Crumb{
		step("Change it", "update_work", "id", id),
		step("Get a brief for doing it", "brief_work", "id", id),
		step("Leave a note", "add_note", "record", fmt.Sprintf("Todo:%d", id)),
	}
	return c.JSON(200, O("summary", c.Summary, "todo", o))
}

// todoShowJSON is a todo as show answers it (todos/show.json): the todo,
// its description as plain text, then what areas keep on it (notes,
// documents) and its history. The page's own extras ride along: the stored
// description to edit, the engagement's agreed items to pick from.
func (a *App) todoShowJSON(ctx context.Context, id int64) (Obj, *Todo, error) {
	td, err := a.todo(ctx, a.store, id)
	if err != nil {
		return nil, nil, err
	}
	list, err := a.todosJSON(ctx, a.store, []*Todo{td})
	if err != nil {
		return nil, nil, err
	}
	o := list[0].Set("description", nullable(workPlain(td.Description)))
	o, err = a.withSections(ctx, "Todo", id, o)
	if err != nil {
		return nil, nil, err
	}
	history, err := a.eventsJSON(ctx, `e.subject_type = 'Todo' AND e.subject_id = ?`, 20, id)
	if err != nil {
		return nil, nil, err
	}
	items, err := a.agreedItems(ctx, a.store, td.EngagementID)
	if err != nil {
		return nil, nil, err
	}
	people, err := workPeople(ctx, a.store, []int64{td.CreatedByID})
	if err != nil {
		return nil, nil, err
	}
	var closed bool
	_ = a.store.QueryRowContext(ctx, `SELECT closed_at IS NOT NULL FROM engagements WHERE id = ?`, td.EngagementID).Scan(&closed)
	o = o.Set("history", history).Set("description_html", td.Description).Set("agreed_items", items).
		Set("created_at", td.CreatedAt).Set("created_by", userRef(td.CreatedByID, people[td.CreatedByID])).Set("engagement_closed", closed)
	return o, td, nil
}

var workBreaks = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>|</h[1-6]>|</div>`)
var workNewlines = regexp.MustCompile(`\n+`)

// workPlain is rich text as plain text for a model to read (agent_text):
// tags gone, a line per paragraph or item.
func workPlain(s string) string {
	if strings.TrimSpace(s) == "" {
		return ""
	}
	s = workBreaks.ReplaceAllString(s, "$0\n")
	s = tags.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	return strings.TrimSpace(workNewlines.ReplaceAllString(s, "\n"))
}

// ---- writing -----------------------------------------------------------------

// todoChange is what a form, an agent or a bulk change asks of a todo.
type todoChange struct {
	fields    map[string]any // plain columns: title, description, due_on, client_visible, scope_item_id, position
	status    string         // "" leaves it
	ownerSet  bool
	owner     int64 // 0: nobody
	othersSet bool
	others    []int64
	custom    map[string]any
}

// todoParams reads and checks a todo's fields; partial for an update.
func (a *App) todoParams(c *Ctx, body map[string]any, partial bool) (*todoChange, error) {
	inv := apierr.Invalid{}
	ch := &todoChange{fields: map[string]any{}}
	if has(body, "title") || !partial {
		title := strings.TrimSpace(str(body["title"]))
		if title == "" {
			inv.Add("title", "can't be blank")
		}
		ch.fields["title"] = title
	}
	if has(body, "description") {
		ch.fields["description"] = nullable(sanitizeRichText(str(body["description"])))
	}
	if has(body, "status") {
		ch.status = str(body["status"])
		if !slices.Contains(TodoStatuses, ch.status) {
			inv.Add("status", "is not included in the list")
		}
	} else if !partial {
		ch.status = "planned"
	}
	if has(body, "owner_id") {
		id, err := a.workOwnerID(c, body["owner_id"])
		if err != nil {
			return nil, err
		}
		ch.ownerSet, ch.owner = true, id
	}
	if has(body, "other_owner_ids") {
		ch.othersSet = true
		for _, v := range asList(body["other_owner_ids"]) {
			if id, ok := intOf(v); ok && id > 0 {
				ch.others = append(ch.others, id)
			}
		}
	}
	if has(body, "due_on") {
		ch.fields["due_on"] = workDay(body["due_on"])
	}
	if has(body, "client_visible") {
		ch.fields["client_visible"] = boolOf(body["client_visible"])
	}
	if has(body, "scope_item_id") {
		if id, ok := intOf(body["scope_item_id"]); ok && id > 0 {
			ch.fields["scope_item_id"] = id
		} else {
			ch.fields["scope_item_id"] = nil
		}
	}
	if has(body, "position") {
		if n, ok := intOf(body["position"]); ok && n > 0 {
			ch.fields["position"] = n
		}
	}
	if custom, ok := body["custom_fields"].(map[string]any); ok {
		ch.custom = custom
	}
	return ch, inv.Err()
}

// workDay is a date input as YYYY-MM-DD, nil when blank or not a date
// (Rails casts those to nil).
func workDay(v any) any {
	s := strings.TrimSpace(str(v))
	if len(s) > 10 {
		s = s[:10]
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return nil
	}
	return s
}

// workOwnerID is an owner as given: blank or none for nobody, me for the
// person acting (an agent's person), an id, or a person's name.
func (a *App) workOwnerID(c *Ctx, v any) (int64, error) {
	s := strings.TrimSpace(str(v))
	switch s {
	case "", "none":
		return 0, nil
	case "me":
		return c.Actor.User.Person().ID, nil
	}
	if id, ok := intOf(v); ok {
		return id, nil
	}
	found, err := a.workPeopleNamed(c.R.Context(), s)
	switch {
	case err != nil:
		return 0, err
	case len(found) == 0:
		return 0, &apierr.Error{Status: 422, Code: "not_found", Message: "No user matches “" + s + "”."}
	case len(found) > 1:
		return 0, &apierr.Error{Status: 422, Code: "ambiguous", Message: "More than one user matches “" + s + "”."}
	}
	return found[0], nil
}

// workPeopleNamed are the active people a name stands for, best first:
// the name or address exactly (any case), then starting with it, then
// containing it (Agent::Resolver).
func (a *App) workPeopleNamed(ctx context.Context, text string) ([]int64, error) {
	lowered := strings.ToLower(strings.TrimSpace(text))
	like := strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(lowered)
	for _, try := range []struct{ op, value string }{{"= ?", lowered}, {`LIKE ? ESCAPE '\'`, like + "%"}, {`LIKE ? ESCAPE '\'`, "%" + like + "%"}} {
		ids, err := store.Int64s(ctx, a.store, `SELECT id FROM users WHERE deactivated_at IS NULL AND agent_owner_id IS NULL
			AND (lower(name) `+try.op+` OR lower(email_address) `+try.op+`) ORDER BY name LIMIT 8`, try.value, try.value)
		if err != nil || len(ids) > 0 {
			return ids, err
		}
	}
	return nil, nil
}

// nextTodoPosition is the end of a list: one past the highest col where.
func (t *Tx) nextTodoPosition(col, where string, args ...any) (int64, error) {
	var n int64
	err := t.QueryRow(`SELECT COALESCE(MAX(`+col+`), 0) + 1 FROM todos WHERE `+where, args...).Scan(&n)
	return n, err
}

// actorPersonID is who assigns: the person acting (an agent's person), or
// nobody for the system.
func (t *Tx) actorPersonID() any {
	if t.Actor == nil || t.Actor.User == nil {
		return nil
	}
	return t.Actor.User.Person().ID
}

// createTodo adds work to an engagement: at the end of its lists, its
// lead and anyone else on it assigned.
func (t *Tx) createTodo(engagementID int64, ch *todoChange) (int64, error) {
	status := ch.status
	if status == "" {
		status = "planned"
	}
	pos, err := t.nextTodoPosition("position", "engagement_id = ? AND status = ?", engagementID, status)
	if err != nil {
		return 0, err
	}
	if p, ok := ch.fields["position"].(int64); ok {
		pos = p
	}
	board, err := t.nextTodoPosition("board_position", "status = ?", status)
	if err != nil {
		return 0, err
	}
	var completed, owner any
	now := store.Now().DB()
	if status == "done" {
		completed = now
	}
	if ch.owner != 0 {
		owner = ch.owner
	}
	visible := true
	if v, ok := ch.fields["client_visible"].(bool); ok {
		visible = v
	}
	id, err := t.Insert(`INSERT INTO todos (engagement_id, scope_item_id, title, description, status, owner_id, due_on, position, board_position, client_visible, completed_at, created_by_id, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		engagementID, ch.fields["scope_item_id"], ch.fields["title"], ch.fields["description"], status, owner, ch.fields["due_on"],
		pos, board, visible, completed, t.actorPersonOrUser(), now, now)
	if err != nil {
		return 0, err
	}
	if err := t.setCustomFields("Todo", id, ch.custom); err != nil {
		return 0, err
	}
	if ch.owner != 0 {
		if err := t.followLead(id, 0, ch.owner, ch.othersSet); err != nil {
			return 0, err
		}
	}
	if ch.othersSet {
		if err := t.applyOtherOwners(id, ch.others); err != nil {
			return 0, err
		}
	}
	if err := t.recordTodoMentions(id, str(ch.fields["description"])); err != nil {
		return 0, err
	}
	t.Reindex("Todo", id)
	return id, t.Event("Todo", id, "todo.created", nil)
}

// actorPersonOrUser is who made something: the user acting, as Rails'
// created_by: current_user (an agent is itself).
func (t *Tx) actorPersonOrUser() any {
	if id := t.Actor.UserID(); id != 0 {
		return id
	}
	return nil
}

// changeTodo applies a change to a todo as Todo#update does: a new status
// goes to the end of its lists and stamps or clears completion; a new lead
// keeps everyone else; other_owner_ids replaces everyone else.
func (t *Tx) changeTodo(td *Todo, ch *todoChange) error {
	fields := map[string]any{}
	for k, v := range ch.fields {
		fields[k] = v
	}
	status := td.Status
	if ch.status != "" {
		status = ch.status
	}
	if status != td.Status {
		fields["status"] = status
		pos, err := t.nextTodoPosition("position", "engagement_id = ? AND status = ? AND id != ?", td.EngagementID, status, td.ID)
		if err != nil {
			return err
		}
		if _, given := ch.fields["position"]; !given {
			fields["position"] = pos
		}
		if _, given := ch.fields["board_position"]; !given {
			board, err := t.nextTodoPosition("board_position", "status = ? AND id != ?", status, td.ID)
			if err != nil {
				return err
			}
			fields["board_position"] = board
		}
	}
	switch {
	case status == "done" && !td.CompletedAt.Valid:
		fields["completed_at"] = store.Now().DB()
	case status != "done" && td.CompletedAt.Valid:
		fields["completed_at"] = nil
	}
	if ch.ownerSet {
		if ch.owner == 0 {
			fields["owner_id"] = nil
		} else {
			fields["owner_id"] = ch.owner
		}
	}
	if err := t.update("todos", td.ID, fields); err != nil {
		return err
	}
	if err := t.setCustomFields("Todo", td.ID, ch.custom); err != nil {
		return err
	}
	if ch.ownerSet && ch.owner != td.OwnerID {
		if err := t.followLead(td.ID, td.OwnerID, ch.owner, ch.othersSet); err != nil {
			return err
		}
	}
	if ch.othersSet {
		if err := t.applyOtherOwners(td.ID, ch.others); err != nil {
			return err
		}
	}
	if d, ok := ch.fields["description"]; ok && str(d) != td.Description {
		if err := t.recordTodoMentions(td.ID, str(d)); err != nil {
			return err
		}
	}
	t.Reindex("Todo", td.ID)
	if status != td.Status {
		return t.Event("Todo", td.ID, "todo.status", map[string]any{"from": td.Status, "to": status})
	}
	return nil
}

// recordTodoMentions notes the people a description mentions (active
// people, each once), as Rails' Mentions did after a save.
func (t *Tx) recordTodoMentions(id int64, description string) error {
	return t.recordMentions("Todo", id, description)
}

// deleteTodo removes a todo with what hangs off it: its assignments,
// mentions, history, custom values, and whatever areas keep on it.
func (t *Tx) deleteTodo(id int64) error {
	if err := t.cascade("Todo", id); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM assignments WHERE todo_id = ?`,
		`DELETE FROM mentions WHERE source_type = 'Todo' AND source_id = ?`,
		`DELETE FROM todos WHERE id = ?`,
	} {
		if _, err := t.Exec(q, id); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) createTodo(c *Ctx) error {
	ctx := c.R.Context()
	e, err := a.workEngagementByRef(ctx, a.store, c.Param("ref"))
	if err != nil {
		return err
	}
	body, err := c.Wrapped("todo")
	if err != nil {
		return err
	}
	ch, err := a.todoParams(c, body, false)
	if err != nil {
		return err
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		id, err = t.createTodo(e.ID, ch)
		return err
	})
	if err != nil {
		return err
	}
	o, td, err := a.todoShowJSON(ctx, id)
	if err != nil {
		return err
	}
	c.Summary = fmt.Sprintf("Added “%s” to %s %s.", td.Title, e.Ref, e.Title)
	c.Crumbs = []Crumb{step("Change it", "update_work", "id", id), step("Get a brief for doing it", "brief_work", "id", id)}
	return c.JSON(201, O("summary", c.Summary, "todo", o))
}

func (a *App) updateTodo(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("todo")
	if err != nil {
		return err
	}
	td, err := a.todo(ctx, a.store, id)
	if err != nil {
		return err
	}
	ch, err := a.todoParams(c, body, true)
	if err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.changeTodo(td, ch) }); err != nil {
		return err
	}
	o, td, err := a.todoShowJSON(ctx, id)
	if err != nil {
		return err
	}
	c.Summary = "Saved: " + td.Title + " (" + strings.ToLower(humanize(td.Status)) + ")."
	return c.JSON(200, O("summary", c.Summary, "todo", o))
}

func (a *App) deleteTodoHandler(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	td, err := a.todo(ctx, a.store, id)
	if err != nil {
		return err
	}
	var ref string
	_ = a.store.QueryRowContext(ctx, `SELECT ref FROM engagements WHERE id = ?`, td.EngagementID).Scan(&ref)
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteTodo(id) }); err != nil {
		return err
	}
	c.Summary = "Removed."
	return c.JSON(200, O("summary", c.Summary, "deleted", true, "engagement", O("ref", ref)))
}
