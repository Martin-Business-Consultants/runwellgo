package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Runs: an agent at work on one mention (ai_runs). A mention of @handle in
// a note or in chat (mention.created, heard with the person who wrote it
// as the caller) makes a run: queued, then working, waiting while a change
// it proposed waits on that person, then done or failed (or cancelled).
// It says "On it…" where it was mentioned at once and replaces that with
// its answer, or with why it couldn't finish, in plain words. Its
// transcript is an ai_chats row of its own (purpose "agent"), so its
// messages, tool calls and cost are kept as Ask's are and count toward the
// month's budget. Runs execute in the background, two at a time (the rest
// wait their turn); a change it proposes runs only when the person who
// mentioned it approves it, in their own request (the "Approve" on the
// run's page, which their notification opens). A run Runwell restarted
// under is failed, and says so, by the sweep a minute later.

const (
	runQueued    = "queued"
	runWorking   = "working"
	runWaiting   = "waiting"
	runDone      = "done"
	runFailed    = "failed"
	runCancelled = "cancelled"

	maxRunning = 2
)

var statusLabels = map[string]string{runQueued: "Queued", runWorking: "Working", runWaiting: "Waiting for approval", runDone: "Done", runFailed: "Failed", runCancelled: "Cancelled"}

type run struct {
	ID         int64
	AgentID    int64
	UserID     int64
	ChatID     int64
	MentionID  int64
	Status     string
	Source     string // note, chat (a plugin's key) or tool
	Record     string
	Path       string
	Subject    string
	Prompt     string
	Ack        string // the "On it…" reply's id, replaced by the answer
	Error      string
	CreatedAt  store.Time
	StartedAt  store.Time
	FinishedAt store.Time
}

const runColumns = `id, agent_id, user_id, ai_chat_id, COALESCE(mention_id, 0), status, source, COALESCE(record, ''), COALESCE(path, ''), COALESCE(subject, ''),
	COALESCE(prompt, ''), COALESCE(ack_reply_id, ''), COALESCE(error, ''), created_at, started_at, finished_at`

func scanRun(row interface{ Scan(...any) error }) (*run, error) {
	r := &run{}
	err := row.Scan(&r.ID, &r.AgentID, &r.UserID, &r.ChatID, &r.MentionID, &r.Status, &r.Source, &r.Record, &r.Path, &r.Subject, &r.Prompt, &r.Ack, &r.Error, &r.CreatedAt, &r.StartedAt, &r.FinishedAt)
	return r, err
}

func loadRun(ctx context.Context, q plugin.Querier, id int64) (*run, error) {
	r, err := scanRun(q.QueryRow(ctx, `SELECT `+runColumns+` FROM ai_runs WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return r, err
}

func (r *run) finished() bool {
	return r.Status == runDone || r.Status == runFailed || r.Status == runCancelled
}

// ---- the queue -----------------------------------------------------------------------------

// queue is the runs this process holds: running (at most two) and waiting
// their turn, each with the API it was started with (its caller).
var queue = struct {
	sync.Mutex
	active  int
	waiting []queued
	mine    map[int64]bool
	cancels map[int64]context.CancelFunc
}{mine: map[int64]bool{}, cancels: map[int64]context.CancelFunc{}}

type queued struct {
	id  int64
	api *plugin.API
}

// enqueue starts a run now, or when one of the two running finishes.
func enqueue(ctx context.Context, api *plugin.API, id int64) {
	queue.Lock()
	queue.mine[id] = true
	if queue.active >= maxRunning {
		queue.waiting = append(queue.waiting, queued{id, api})
		queue.Unlock()
		return
	}
	queue.active++
	queue.Unlock()
	start(ctx, api, id)
}

func start(ctx context.Context, api *plugin.API, id int64) {
	err := api.Background("agent run", func(ctx context.Context, api *plugin.API) {
		defer next(ctx)
		execute(ctx, api, id)
	})
	if err != nil {
		queue.Lock()
		queue.active--
		delete(queue.mine, id)
		queue.Unlock()
		if r, _ := loadRun(ctx, api, id); r != nil {
			finish(ctx, api, r, runFailed, "too much is running right now. Mention me again in a minute.", "")
		}
	}
}

// next starts the run waiting longest, once one finishes.
func next(ctx context.Context) {
	queue.Lock()
	queue.active--
	if len(queue.waiting) == 0 {
		queue.Unlock()
		return
	}
	q := queue.waiting[0]
	queue.waiting = queue.waiting[1:]
	queue.active++
	queue.Unlock()
	start(context.WithoutCancel(ctx), q.api, q.id)
}

func forget(id int64) {
	queue.Lock()
	delete(queue.mine, id)
	delete(queue.cancels, id)
	queue.Unlock()
}

// ---- starting one ----------------------------------------------------------------------

// onEvent hears mentions of agents, other plugins asking a person to
// approve something (approvals.go), and someone's removal (removed.go).
func onEvent(ctx context.Context, api *plugin.API, event string, data json.RawMessage) error {
	if strings.HasSuffix(event, ".approval.requested") || strings.HasSuffix(event, ".approval.withdrawn") {
		return externalApproval(ctx, api, event, data)
	}
	if event == "user.removed" {
		return removedPerson(ctx, api, data)
	}
	if event != "mention.created" {
		return nil
	}
	var ev plugin.MentionEvent
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	return mentioned(ctx, api, ev)
}

// mentioned starts a run for a mention, or answers why it can't.
func mentioned(ctx context.Context, api *plugin.API, ev plugin.MentionEvent) error {
	caller := api.Caller()
	if caller == nil {
		return errors.New("a mention without the person who made it")
	}
	g, err := loadAgent(ctx, api, intOf(ev.Mentionable))
	if err != nil || g == nil {
		return err
	}
	source := ev.Source
	r, err := createRun(ctx, api, g, caller, source, ev.ID, ev.Record, ev.Path, ev.Subject, ev.Text, mentionContext(ctx, api, g, caller, ev))
	if err != nil {
		return err
	}
	if g.Paused {
		finish(ctx, api, r, runFailed, "", fmt.Sprintf("I’m paused at the moment, so I can’t help with this. An owner can switch me back on in Settings › AI › Agents."))
		return nil
	}
	if msg := cantStart(ctx, api, g, caller, ev.Record); msg != "" {
		finish(ctx, api, r, runFailed, msg, "")
		return nil
	}
	if ack, err := api.Reply(ctx, plugin.Reply{Mention: ev.ID, Body: "On it…"}); err == nil {
		r.Ack = ack
		_, _ = api.Exec(ctx, `UPDATE ai_runs SET ack_reply_id = ? WHERE id = ?`, ack, r.ID)
	} else {
		api.Log("agent ack: " + err.Error())
	}
	enqueue(ctx, api, r.ID)
	return nil
}

// cantStart is why a run can't start now, in plain words ("" when it can).
func cantStart(ctx context.Context, api *plugin.API, g *agent, caller *plugin.Caller, record string) string {
	if !g.mayMention(caller.Role) {
		return "only some roles may ask me, and yours isn’t one of them."
	}
	s, err := loadSetting(ctx, api)
	if err != nil {
		return "I couldn’t read the AI settings."
	}
	reason, err := available(ctx, api, s)
	if err != nil {
		return "I couldn’t check this month’s budget."
	}
	if reason != "" {
		return plainReason(reason)
	}
	if typ, id, ok := strings.Cut(record, ":"); ok {
		n, _ := strconv.ParseInt(id, 10, 64)
		if ex, err := excludedRecord(ctx, api, typ, n); err == nil && ex {
			return "AI is kept off this " + clientWord(ctx, api, false) + " by the owner."
		}
		if len(g.ClientIDs) > 0 {
			if recs, err := api.Records(ctx, plugin.Query{Type: typ, ID: n}); err == nil && len(recs) > 0 && !containsID(g.ClientIDs, clientOf(ctx, api, recs[0])) {
				return "I may only read some " + clientWord(ctx, api, true) + "’ records, and this isn’t one of them."
			}
		}
	}
	return ""
}

func containsID(list []int64, id int64) bool {
	for _, x := range list {
		if x == id {
			return true
		}
	}
	return false
}

// plainReason is why AI can't be used, as an agent says it.
func plainReason(reason string) string {
	switch {
	case strings.Contains(reason, "budget"):
		return "the budget for this month is used up."
	case strings.Contains(reason, "switched on"), strings.Contains(reason, "set up"):
		return "AI isn’t switched on here."
	}
	return strings.TrimSuffix(reason, ".") + "."
}

// mentionContext is what the run starts from, before what was asked:
// today, who asked and where, and the conversation so far.
func mentionContext(ctx context.Context, api *plugin.API, g *agent, caller *plugin.Caller, ev plugin.MentionEvent) string {
	site, _ := api.Site(ctx)
	loc := time.UTC
	if site.Location != nil {
		loc = site.Location
	}
	zone := site.TimeZone
	if zone == "" {
		zone = "UTC"
	}
	where := "in " + ev.Subject
	if ev.Source == "note" {
		where = fmt.Sprintf("in a note on %s (%s): read it with read_record before you answer", ev.Record, truncate(ev.Subject, 80))
	} else if ev.Source != "" && ev.Source != "tool" {
		where = fmt.Sprintf("in %s in %s; your answer goes in that thread", humanize(ev.Source), ev.Subject)
	}
	out := fmt.Sprintf("[Today is %s (%s). %s (%s) mentioned you, @%s, %s.]", time.Now().In(loc).Format("Monday, January 2, 2006, 3:04 PM"), zone, caller.Name, caller.Role, g.Handle, where)
	if strings.TrimSpace(ev.Context) != "" {
		out += "\n[The conversation so far, oldest first:\n" + ev.Context + "]"
	}
	return out
}

// createRun keeps a run and its transcript's first message.
func createRun(ctx context.Context, api *plugin.API, g *agent, caller *plugin.Caller, source string, mentionID int64, record, path, subject, prompt, preface string) (*run, error) {
	typ, rid := "", int64(0)
	if t, id, ok := strings.Cut(record, ":"); ok && contains(chatSubjects, t) {
		typ, rid = t, intOf(id)
	}
	chatID, err := insertChat(ctx, api, caller.ID, 0, typ, rid, "agent", truncate("@"+g.Handle+": "+strings.Join(strings.Fields(prompt), " "), 80))
	if err != nil {
		return nil, err
	}
	prompt = truncate(strings.TrimSpace(prompt), 8000)
	raw, _ := json.Marshal(map[string]any{"text": preface + "\n\n" + prompt})
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `INSERT INTO ai_messages (ai_chat_id, role, content, raw_content, created_at, updated_at) VALUES (?, 'user', ?, ?, ?, ?)`, chatID, prompt, string(raw), now, now); err != nil {
		return nil, err
	}
	res, err := api.Exec(ctx, `INSERT INTO ai_runs (agent_id, user_id, ai_chat_id, mention_id, status, source, record, path, subject, prompt, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'queued', ?, ?, ?, ?, ?, ?, ?)`, g.ID, caller.ID, chatID, nullID(mentionID), source, nullStr(record), nullStr(path), nullStr(subject), prompt, now, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	r, err := loadRun(ctx, api, id)
	if err == nil {
		publishRun(ctx, api, r)
	}
	return r, err
}

// ---- running -------------------------------------------------------------------------

// execute works the run on: from its start, or past a decision, until the
// model's done, a change waits on the person, or a limit stops it.
func execute(ctx context.Context, api *plugin.API, id int64) {
	r, err := loadRun(ctx, api, id)
	if err != nil || r == nil || r.finished() {
		forget(id)
		return
	}
	ctx, cancel := context.WithCancel(ctx)
	queue.Lock()
	queue.mine[id] = true
	queue.cancels[id] = cancel
	queue.Unlock()
	defer func() {
		cancel()
		queue.Lock()
		delete(queue.cancels, id)
		queue.Unlock()
		now := store.Now().DB()
		_, _ = api.Exec(context.WithoutCancel(ctx), `UPDATE ai_chats SET replying_since = NULL, updated_at = ? WHERE id = ?`, now, r.ChatID)
	}()
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `UPDATE ai_runs SET status = 'working', started_at = COALESCE(started_at, ?), updated_at = ? WHERE id = ?`, now, now, id); err != nil {
		api.Log("agent run: " + err.Error())
		return
	}
	_, _ = api.Exec(ctx, `UPDATE ai_chats SET replying_since = ? WHERE id = ?`, now, r.ChatID)
	r.Status = runWorking
	publishRun(ctx, api, r)
	g, err := loadAgent(ctx, api, r.AgentID)
	if err != nil || g == nil {
		finish(ctx, api, r, runFailed, "I was deleted before I could finish.", "")
		return
	}
	s, err := loadSetting(ctx, api)
	if err != nil {
		finish(ctx, api, r, runFailed, "I couldn’t read the AI settings.", "")
		return
	}
	caller := api.Caller()
	if caller == nil || caller.ID != r.UserID {
		finish(ctx, api, r, runFailed, "I lost track of who asked me. Mention me again.", "")
		return
	}
	ar, err := newAgentRunner(ctx, api, g, caller)
	if err != nil {
		finish(ctx, api, r, runFailed, "I couldn’t see which tools I may use: "+err.Error(), "")
		return
	}
	model := g.modelName(s)
	for {
		if cur, _ := loadRun(ctx, api, id); cur == nil || cur.Status == runCancelled {
			forget(id)
			return
		}
		var steps int
		_ = api.QueryRow(ctx, `SELECT count(*) FROM ai_messages WHERE ai_chat_id = ? AND role = 'assistant' AND COALESCE(finish_reason, '') <> 'error'`, r.ChatID).Scan(&steps)
		if steps >= g.MaxSteps {
			finish(ctx, api, r, runFailed, fmt.Sprintf("I stopped after %d steps without finishing. Mention me again with a narrower ask.", g.MaxSteps), "")
			return
		}
		if reason, err := available(ctx, api, s); err != nil || reason != "" {
			finish(ctx, api, r, runFailed, plainReason(reason), "")
			return
		}
		if g.RunBudgetCents != nil {
			if spentOn(ctx, api, r.ChatID)*100 >= float64(*g.RunBudgetCents) {
				finish(ctx, api, r, runFailed, fmt.Sprintf("one run of mine may spend %s, and this one has.", budgetText(g.RunBudgetCents)), "")
				return
			}
		}
		turns, pending, err := history(ctx, api, r.ChatID)
		if err != nil {
			finish(ctx, api, r, runFailed, "I couldn’t read what I’d done so far.", "")
			return
		}
		if pending > 0 {
			waitOnPerson(ctx, api, r, g, ar)
			return
		}
		l, err := s.llmFor(api, model)
		if err != nil {
			finish(ctx, api, r, runFailed, plainError(ctx, err), "")
			return
		}
		ans, err := l.turn(ctx, ask{System: ar.system(ctx), Tools: ar.tools(), Turns: turns})
		if err != nil {
			if cur, _ := loadRun(context.WithoutCancel(ctx), api, id); cur != nil && cur.Status == runCancelled {
				forget(id)
				return
			}
			finish(ctx, api, r, runFailed, plainError(ctx, err), "")
			return
		}
		text := ans.Text
		switch ans.Stop {
		case "refusal":
			if text == "" {
				text = "I can’t help with that one."
			}
		case "max_tokens":
			text = strings.TrimSpace(text + "\n\n(That answer was cut short.)")
		}
		msgID, err := saveAssistant(ctx, api, r.ChatID, text, ans)
		if err != nil {
			finish(ctx, api, r, runFailed, "I couldn’t keep my answer.", "")
			return
		}
		recordUsage(ctx, api, r.ChatID, msgID, l, ans.Usage)
		if len(ans.Calls) == 0 {
			finish(ctx, api, r, runDone, "", text)
			return
		}
		runCalls := ans.Stop == "tool_use"
		waiting := false
		for i, call := range ans.Calls {
			args, _ := json.Marshal(inputOf(call.Input))
			now := store.Now().DB()
			res, err := api.Exec(ctx, `INSERT INTO ai_tool_calls (ai_message_id, tool_call_id, name, arguments, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				msgID, call.ID, call.Name, string(args), i, now, now)
			if err != nil {
				finish(ctx, api, r, runFailed, "I couldn’t keep what I was doing.", "")
				return
			}
			rowID, _ := res.LastInsertId()
			result, isError, pending := "This wasn't run.", true, false
			if runCalls {
				result, isError, pending = ar.call(ctx, call.Name, call.Input)
			}
			if pending {
				waiting = true
				continue
			}
			_, _ = api.Exec(ctx, `UPDATE ai_tool_calls SET result = ?, is_error = ?, updated_at = ? WHERE id = ?`, result, isError, store.Now().DB(), rowID)
		}
		if waiting {
			waitOnPerson(ctx, api, r, g, ar)
			return
		}
		if !runCalls {
			finish(ctx, api, r, runDone, "", text)
			return
		}
	}
}

// plainError is a failure in the words an agent replies with.
func plainError(ctx context.Context, err error) string {
	err = ctxErr(ctx, err)
	var u *unavailable
	if errors.As(err, &u) {
		return plainReason(u.msg)
	}
	return truncate(err.Error(), 300)
}

// spentOn is what a run's calls to the provider cost, in dollars.
func spentOn(ctx context.Context, api *plugin.API, chatID int64) float64 {
	var total sql.NullFloat64
	_ = api.QueryRow(ctx, `SELECT SUM(total_cost) FROM ai_usages WHERE ai_chat_id = ?`, chatID).Scan(&total)
	return total.Float64
}

// waitOnPerson says where it was mentioned that a change waits on the
// person who asked, and puts it in their bell with the run's page to
// approve or decline it on.
func waitOnPerson(ctx context.Context, api *plugin.API, r *run, g *agent, ar *agentRunner) {
	now := store.Now().DB()
	_, _ = api.Exec(ctx, `UPDATE ai_runs SET status = 'waiting', updated_at = ? WHERE id = ?`, now, r.ID)
	r.Status = runWaiting
	publishRun(ctx, api, r)
	what := "make a change"
	var detailLines []string
	var name string
	var args sql.NullString
	if err := api.QueryRow(ctx, `SELECT t.arguments FROM ai_tool_calls t JOIN ai_messages m ON m.id = t.ai_message_id
		WHERE m.ai_chat_id = ? AND t.approval IS NULL AND t.result IS NULL AND t.name = 'run_tool' ORDER BY t.id LIMIT 1`, r.ChatID).Scan(&args); err == nil {
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal([]byte(args.String), &call)
		name = call.Name
		if t := ar.find(ctx, call.Name); t != nil && t.Title != "" {
			what = strings.ToLower(t.Title[:1]) + t.Title[1:]
		} else if name != "" {
			what = strings.ToLower(humanize(name))
		}
		for _, d := range details(call.Arguments, nil) {
			detailLines = append(detailLines, d[0]+": "+d[1])
		}
	}
	_ = detailLines
	ids := proposed(ctx, api, r.ChatID, r.UserID, "agent", r.ID, func(name string) string {
		if t := ar.find(ctx, name); t != nil && t.Title != "" {
			return fmt.Sprintf("@%s wants to %s", g.Handle, strings.ToLower(t.Title[:1])+t.Title[1:])
		}
		return fmt.Sprintf("@%s wants to %s", g.Handle, strings.ToLower(humanize(name)))
	}, true)
	link := fmt.Sprintf("/ai/runs/%d", r.ID)
	if len(ids) > 0 {
		link = fmt.Sprintf("/ai/approvals?item=%d", ids[0])
	}
	if r.MentionID != 0 {
		body := fmt.Sprintf(`<p>I’d like to %s. It waits for %s to approve or decline it: <a href="%s">review the change</a>.</p>`, html.EscapeString(what), html.EscapeString(firstName(ar.caller.Name)), link)
		replyTo(ctx, api, r, body)
	}
}

func firstName(name string) string {
	f, _, _ := strings.Cut(strings.TrimSpace(name), " ")
	return f
}

// replyTo puts body where the run was mentioned, in place of its last
// reply ("On it…").
func replyTo(ctx context.Context, api *plugin.API, r *run, body string) {
	if r.MentionID == 0 {
		return
	}
	id, err := api.Reply(context.WithoutCancel(ctx), plugin.Reply{Mention: r.MentionID, Body: body, Replace: r.Ack})
	if err != nil {
		api.Log("agent reply: " + err.Error())
		return
	}
	if id != r.Ack {
		r.Ack = id
		_, _ = api.Exec(context.WithoutCancel(ctx), `UPDATE ai_runs SET ack_reply_id = ? WHERE id = ?`, id, r.ID)
	}
}

// finish ends a run: done with its answer, or failed with why, said where
// it was mentioned and in the person's bell.
func finish(ctx context.Context, api *plugin.API, r *run, status, why, answer string) {
	ctx = context.WithoutCancel(ctx)
	defer forget(r.ID)
	now := store.Now().DB()
	errText := why
	if status == runFailed && why == "" {
		errText = answer
	}
	cost := spentOn(ctx, api, r.ChatID)
	if _, err := api.Exec(ctx, `UPDATE ai_runs SET status = ?, error = ?, cost = ?, finished_at = ?, updated_at = ? WHERE id = ? AND status <> 'cancelled'`,
		status, nullStr(errText), cost, now, now, r.ID); err != nil {
		api.Log("agent run: " + err.Error())
	}
	r.Status, r.Error = status, errText
	if status == runFailed && answer == "" && why != "" {
		saveFailure(ctx, api, r.ChatID, "I couldn’t finish: "+why)
	}
	publishRun(ctx, api, r)
	body := answer
	if body == "" && why != "" {
		body = "I couldn’t finish: " + why
	}
	if body != "" {
		replyTo(ctx, api, r, markdownHTML(body))
	}
	g, _ := loadAgent(ctx, api, r.AgentID)
	if g == nil || status == runCancelled {
		return
	}
	title := fmt.Sprintf("@%s replied", g.Handle)
	if status == runFailed {
		title = fmt.Sprintf("@%s couldn’t finish", g.Handle)
	}
	path := r.Path
	if path == "" || !strings.HasPrefix(path, "/") {
		path = fmt.Sprintf("/ai/runs/%d", r.ID)
	}
	if err := api.Notify(ctx, plugin.Notification{UserID: r.UserID, Title: title, Body: truncate(plainOf(body), 300), Path: path,
		Source: "AI", Subject: g.Name, Group: fmt.Sprintf("ai-run:%d", r.ID)}); err != nil {
		api.Log("agent notify: " + err.Error())
	}
}

var tagRE = regexp.MustCompile(`<[^>]+>`)

func plainOf(s string) string {
	return strings.TrimSpace(html.UnescapeString(tagRE.ReplaceAllString(s, "")))
}

// publishRun tells the person's open tabs, and the owners', that a run
// moved on.
func publishRun(ctx context.Context, api *plugin.API, r *run) {
	if r == nil {
		return
	}
	to := []int64{r.UserID}
	if users, err := api.Records(ctx, plugin.Query{Type: "User", Limit: 500}); err == nil {
		for _, u := range users {
			if str(u["role"]) == "owner" && intOf(u["id"]) != r.UserID {
				to = append(to, intOf(u["id"]))
			}
		}
	}
	_ = api.Publish(ctx, "run", to, map[string]any{"id": r.ID, "status": r.Status, "agent_id": r.AgentID})
}

// ---- what the model is told, and may use ------------------------------------------------

type agentRunner struct {
	*staffRunner
	g *agent
}

// newAgentRunner is the agent's tools: the person's, only those it may
// use (only reading ones unless it may propose changes), never another
// agent's run; and only the records of the clients it may read.
func newAgentRunner(ctx context.Context, api *plugin.API, g *agent, caller *plugin.Caller) (*agentRunner, error) {
	all, err := api.Tools(ctx, caller)
	if err != nil {
		return nil, err
	}
	list := []plugin.ToolInfo{}
	for _, t := range all {
		if t.Name == "run_ai_agent" || (g.Tools != "propose" && !t.ReadOnly) || (len(g.Allowed) > 0 && !contains(g.Allowed, t.Name)) {
			continue
		}
		list = append(list, t)
	}
	sr := &staffRunner{api: api, caller: caller, list: list}
	sr.excluded = func(ctx context.Context) (map[int64]bool, error) {
		ex, err := excludedClients(ctx, api)
		if err != nil || len(g.ClientIDs) == 0 {
			return ex, err
		}
		clients, err := api.Records(ctx, plugin.Query{Type: "Client", Limit: 500})
		if err != nil {
			return nil, err
		}
		for _, c := range clients {
			if id := intOf(c["id"]); !containsID(g.ClientIDs, id) {
				ex[id] = true
			}
		}
		return ex, nil
	}
	return &agentRunner{staffRunner: sr, g: g}, nil
}

func (r *agentRunner) system(ctx context.Context) string {
	site, _ := r.api.Site(ctx)
	out := staffSystem(site, r.caller)
	out += fmt.Sprintf("\n\nYou are %s (@%s), an agent the owner set up.", r.g.Name, r.g.Handle)
	if r.g.Description != "" {
		out += " " + r.g.Description
	}
	if r.g.Tools != "propose" {
		out += "\nYou only read: you can't change anything. If a change is needed, say what it is and who should make it."
	}
	out += "\nYou were mentioned in a note or a chat: your last message is posted as your reply there, for the people in it to read. Make it the answer itself: short, plain, no preamble, no sign-off."
	if r.g.Instructions != "" {
		out += "\n\nThe owner's instructions for you:\n" + r.g.Instructions
	}
	return out
}

// ---- markdown, as a reply's HTML -------------------------------------------------------

var (
	boldRE = regexp.MustCompile(`\*\*(.+?)\*\*`)
	codeRE = regexp.MustCompile("`([^`]+)`")
	refRE  = regexp.MustCompile(`\b(Client|Engagement|Todo|Request|ScopeItem):([A-Za-z]{1,4}-\d+|\d+)\b`)
	listRE = regexp.MustCompile(`^\s*(?:[-*•]|\d+\.)\s+`)
)

var refPaths = map[string]string{"Client": "/clients/", "Engagement": "/engagements/", "Todo": "/work/", "Request": "/requests/", "ScopeItem": "/scope_items/"}

// markdownHTML is a reply as rich text: paragraphs, bullet lists, bold,
// code, and record refs as links (what rich text keeps).
func markdownHTML(text string) string {
	if strings.Contains(text, "<p>") || strings.Contains(text, "<a ") {
		return text
	}
	inline := func(s string) string {
		s = html.EscapeString(s)
		s = boldRE.ReplaceAllString(s, "<strong>$1</strong>")
		s = codeRE.ReplaceAllString(s, "<code>$1</code>")
		return refRE.ReplaceAllStringFunc(s, func(m string) string {
			p := refRE.FindStringSubmatch(m)
			return `<a href="` + refPaths[p[1]] + p[2] + `">` + m + `</a>`
		})
	}
	var b strings.Builder
	for _, block := range regexp.MustCompile(`\n\s*\n`).Split(strings.TrimSpace(text), -1) {
		lines := strings.Split(block, "\n")
		list := true
		for _, l := range lines {
			if !listRE.MatchString(l) {
				list = false
			}
		}
		if list {
			b.WriteString("<ul>")
			for _, l := range lines {
				b.WriteString("<li>" + inline(listRE.ReplaceAllString(l, "")) + "</li>")
			}
			b.WriteString("</ul>")
			continue
		}
		parts := make([]string, len(lines))
		for i, l := range lines {
			parts[i] = inline(strings.TrimSpace(strings.TrimLeft(l, "# ")))
		}
		b.WriteString("<p>" + strings.Join(parts, "<br>") + "</p>")
	}
	return b.String()
}

// ---- routes ----------------------------------------------------------------------------

// runJSON is a run as the runs page and agents see it.
func runJSON(r *run, g *agent, people map[int64]string, cost float64) map[string]any {
	var ag any
	if g != nil {
		ag = map[string]any{"id": g.ID, "name": g.Name, "handle": g.Handle, "avatar_url": nullStr(g.avatarPath())}
	} else {
		ag = map[string]any{"id": r.AgentID, "name": "Deleted agent", "handle": "", "avatar_url": nil}
	}
	at := func(t store.Time) any {
		if !t.Valid {
			return nil
		}
		return stamp(t)
	}
	return map[string]any{"id": r.ID, "agent": ag, "user": map[string]any{"id": r.UserID, "name": people[r.UserID]},
		"status": r.Status, "status_label": statusLabels[r.Status], "source": r.Source, "record": nullStr(r.Record), "path": nullStr(r.Path),
		"subject": nullStr(r.Subject), "prompt": r.Prompt, "error": nullStr(r.Error), "cost": dollarsText(cost, 3), "cost_cents": math.Round(cost*10000) / 100,
		"created_at": at(r.CreatedAt), "started_at": at(r.StartedAt), "finished_at": at(r.FinishedAt), "url": fmt.Sprintf("/ai/runs/%d", r.ID)}
}

func peopleNames(ctx context.Context, api *plugin.API) map[int64]string {
	out := map[int64]string{}
	if users, err := api.Records(ctx, plugin.Query{Type: "User", Limit: 500}); err == nil {
		for _, u := range users {
			out[intOf(u["id"])] = str(u["name"])
		}
	}
	return withRemoved(ctx, api, out)
}

func listRuns(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if req.Actor == nil {
		return refuse(http.StatusUnauthorized, "Sign in first."), nil
	}
	everyone := req.Actor.Can("manage_settings")
	where, args := []string{"1 = 1"}, []any{}
	if !everyone {
		where, args = append(where, "r.user_id = ?"), append(args, req.Actor.ID)
	} else if u := intOf(req.Query.Get("user")); u > 0 {
		where, args = append(where, "r.user_id = ?"), append(args, u)
	}
	if a := intOf(req.Query.Get("agent")); a > 0 {
		where, args = append(where, "r.agent_id = ?"), append(args, a)
	}
	if s := req.Query.Get("status"); statusLabels[s] != "" {
		where, args = append(where, "r.status = ?"), append(args, s)
	}
	rows, err := api.Query(ctx, `SELECT r.id, r.agent_id, r.user_id, r.ai_chat_id, COALESCE(r.mention_id, 0), r.status, r.source, COALESCE(r.record, ''), COALESCE(r.path, ''), COALESCE(r.subject, ''),
		COALESCE(r.prompt, ''), COALESCE(r.ack_reply_id, ''), COALESCE(r.error, ''), r.created_at, r.started_at, r.finished_at,
		(SELECT COALESCE(SUM(total_cost), 0) FROM ai_usages u WHERE u.ai_chat_id = r.ai_chat_id)
		FROM ai_runs r WHERE `+strings.Join(where, " AND ")+` ORDER BY r.id DESC LIMIT 200`, args...)
	if err != nil {
		return nil, err
	}
	type row struct {
		r    *run
		cost float64
	}
	var list []row
	for rows.Next() {
		r := &run{}
		var cost float64
		if err := rows.Scan(&r.ID, &r.AgentID, &r.UserID, &r.ChatID, &r.MentionID, &r.Status, &r.Source, &r.Record, &r.Path, &r.Subject, &r.Prompt, &r.Ack, &r.Error, &r.CreatedAt, &r.StartedAt, &r.FinishedAt, &cost); err != nil {
			rows.Close()
			return nil, err
		}
		list = append(list, row{r, cost})
	}
	rows.Close()
	agents, err := loadAgents(ctx, api)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*agent{}
	agentOpts := []map[string]any{}
	for _, g := range agents {
		byID[g.ID] = g
		agentOpts = append(agentOpts, map[string]any{"value": fmt.Sprint(g.ID), "label": "@" + g.Handle})
	}
	people := peopleNames(ctx, api)
	out := []map[string]any{}
	for _, x := range list {
		out = append(out, runJSON(x.r, byID[x.r.AgentID], people, x.cost))
	}
	statusOpts := []map[string]any{}
	for _, s := range []string{runQueued, runWorking, runWaiting, runDone, runFailed, runCancelled} {
		statusOpts = append(statusOpts, map[string]any{"value": s, "label": statusLabels[s]})
	}
	peopleOpts := []map[string]any{}
	if everyone {
		for id, name := range people {
			peopleOpts = append(peopleOpts, map[string]any{"value": fmt.Sprint(id), "label": name})
		}
	}
	return plugin.JSON(200, map[string]any{"summary": fmt.Sprintf("%d %s.", len(out), map[bool]string{true: "run", false: "runs"}[len(out) == 1]), "runs": out,
		"filters": map[string]any{"agents": agentOpts, "statuses": statusOpts, "people": peopleOpts}, "everyone": everyone}), nil
}

// visibleRun is the run by the route's id, when the caller may see it:
// theirs, or any for those who manage settings.
func visibleRun(ctx context.Context, api *plugin.API, req *plugin.Request) (*run, *plugin.Response, error) {
	if req.Actor == nil {
		return nil, refuse(http.StatusUnauthorized, "Sign in first."), nil
	}
	r, err := loadRun(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, nil, err
	}
	if r == nil || (r.UserID != req.Actor.ID && !req.Actor.Can("manage_settings")) {
		return nil, missing("run"), nil
	}
	return r, nil, nil
}

func showRun(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, res, err := visibleRun(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	g, err := loadAgent(ctx, api, r.AgentID)
	if err != nil {
		return nil, err
	}
	msgs, err := loadMessages(ctx, api, r.ChatID)
	if err != nil {
		return nil, err
	}
	var tools []plugin.ToolInfo
	if req.Actor.ID == r.UserID {
		tools, _ = api.Tools(ctx, req.Actor)
	}
	c, err := loadChat(ctx, api, r.ChatID)
	if err != nil {
		return nil, err
	}
	out, waiting := transcript(ctx, api, msgs, tools, c.replying())
	j := runJSON(r, g, peopleNames(ctx, api), spentOn(ctx, api, r.ChatID))
	mine := req.Actor.ID == r.UserID && !req.Actor.Agent
	summary := fmt.Sprintf("%s's run: %s.", map[bool]string{true: "@" + str(j["agent"].(map[string]any)["handle"]), false: "An agent"}[g != nil], strings.ToLower(statusLabels[r.Status]))
	return plugin.JSON(200, map[string]any{"summary": summary, "run": j, "messages": out, "waiting": waiting && r.Status == runWaiting,
		"can_decide": mine && r.Status == runWaiting, "can_cancel": !r.finished() && (mine || req.Actor.Can("manage_settings"))}), nil
}

func cancelRun(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, res, err := visibleRun(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	if r.finished() {
		return refuse(http.StatusConflict, "That run has already finished."), nil
	}
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `UPDATE ai_runs SET status = 'cancelled', error = ?, finished_at = ?, updated_at = ? WHERE id = ?`, "Cancelled by "+req.Actor.Name+".", now, now, r.ID); err != nil {
		return nil, err
	}
	queue.Lock()
	if cancel := queue.cancels[r.ID]; cancel != nil {
		cancel()
	}
	kept := queue.waiting[:0]
	for _, q := range queue.waiting {
		if q.id != r.ID {
			kept = append(kept, q)
		}
	}
	queue.waiting = kept
	queue.Unlock()
	r.Status = runCancelled
	_, _ = api.Exec(ctx, `UPDATE ai_approvals SET status = 'withdrawn', updated_at = ? WHERE run_id = ? AND status = 'waiting'`, now, r.ID)
	replyTo(ctx, api, r, "<p>"+html.EscapeString(firstName(req.Actor.Name))+" cancelled this before I finished.</p>")
	publishRun(ctx, api, r)
	return showRun(ctx, api, req)
}

func decideRun(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	r, res, err := visibleRun(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	if r.UserID != req.Actor.ID || req.Actor.Agent {
		return refuse(http.StatusForbidden, "Only the person who asked can approve or decline its changes."), nil
	}
	if r.Status != runWaiting {
		return refuse(http.StatusConflict, "Nothing here waits on you any more."), nil
	}
	c, err := loadChat(ctx, api, r.ChatID)
	if err != nil {
		return nil, err
	}
	run := func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
		return api.RunTool(ctx, req.Actor, name, args)
	}
	id := r.ID
	again := func(ctx context.Context, api *plugin.API) { execute(ctx, api, id) }
	if res, err := decide(ctx, api, c, params(req), run, again); res != nil || err != nil {
		return res, err
	}
	return showRun(ctx, api, req)
}

// startRun is an agent at work on a record, asked directly (run_ai_agent)
// rather than mentioned: its answer is the run's, read with show_ai_run.
func startRun(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if req.Actor == nil {
		return refuse(http.StatusUnauthorized, "Sign in first."), nil
	}
	g, err := loadAgent(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if g == nil || !g.mayMention(req.Actor.Role) {
		return missing("agent"), nil
	}
	p := params(req)
	prompt := strings.TrimSpace(str(p["prompt"]))
	if prompt == "" {
		return plugin.Invalid(map[string][]string{"prompt": {"can't be blank"}}), nil
	}
	record := strings.TrimSpace(str(p["record"]))
	subject := ""
	if record != "" {
		typ, id, rec, ok := subjectOf(ctx, api, record, chatSubjects)
		if !ok {
			return plugin.Invalid(map[string][]string{"record": {`must be "Type:id" for a Client, Engagement, Todo, Request or ScopeItem you can see`}}), nil
		}
		record, subject = fmt.Sprintf("%s:%d", typ, id), str(rec["display_name"])
	}
	if g.Paused {
		return refuse(http.StatusConflict, g.Name+" is paused: an owner can switch it back on in Settings › AI › Agents."), nil
	}
	if msg := cantStart(ctx, api, g, req.Actor, record); msg != "" {
		return refuse(http.StatusUnprocessableEntity, "It can't start: "+msg), nil
	}
	ev := plugin.MentionEvent{Source: "tool", Record: record, Subject: subject}
	preface := mentionContext(ctx, api, g, req.Actor, ev)
	if record != "" {
		preface = strings.Replace(preface, "mentioned you, @"+g.Handle+", in "+subject+".", fmt.Sprintf("asked you, @%s, about %s (%s): read it with read_record first.", g.Handle, record, truncate(subject, 80)), 1)
	}
	r, err := createRun(ctx, api, g, req.Actor, "tool", 0, record, "", subject, prompt, preface)
	if err != nil {
		return nil, err
	}
	enqueue(ctx, api, r.ID)
	r, _ = loadRun(ctx, api, r.ID)
	return plugin.JSON(http.StatusAccepted, map[string]any{"summary": fmt.Sprintf("@%s is on it: read its answer with show_ai_run (run %d).", g.Handle, r.ID),
		"run": runJSON(r, g, peopleNames(ctx, api), 0)}), nil
}

// sweep fails the runs this process isn't running though they say they
// are (Runwell restarted under them), and says so where they were asked.
func sweep(ctx context.Context, api *plugin.API) error {
	rows, err := api.Query(ctx, `SELECT `+runColumns+` FROM ai_runs WHERE status IN ('queued', 'working') ORDER BY id`)
	if err != nil {
		return err
	}
	var stale []*run
	for rows.Next() {
		r, err := scanRun(rows)
		if err != nil {
			rows.Close()
			return err
		}
		queue.Lock()
		mine := queue.mine[r.ID]
		queue.Unlock()
		if !mine && time.Since(r.CreatedAt.Time) > time.Minute {
			stale = append(stale, r)
		}
	}
	rows.Close()
	for _, r := range stale {
		finish(ctx, api, r, runFailed, "Runwell restarted before I finished. Mention me again.", "")
	}
	return nil
}
