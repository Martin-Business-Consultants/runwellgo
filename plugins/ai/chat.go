package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// The Ask panel (AiChat): a person's chat about the record on screen, or
// nothing in particular. A question is saved at once and answered in the
// background (replying_since set while it runs; the panel polls). The model
// works through four tools (search, read_record, find_tools, run_tool) that
// reach Runwell through the agent tools as the person; reads run at once,
// and a change waits on the chat until the person approves or declines it
// in the panel, which runs it as them in their own request and carries on.

// chatSubjects are what a chat can be about.
var chatSubjects = []string{"Client", "Engagement", "Todo", "Request", "ScopeItem"}

const (
	stuckAfter  = 5 * time.Minute
	resultLimit = 12000 // characters of a tool's answer the model sees
	maxSteps    = 12
)

type chat struct {
	ID          int64
	UserID      int64
	ContactID   int64
	SubjectType string
	SubjectID   int64
	Purpose     string
	Title       string
	Replying    *time.Time
	UpdatedAt   time.Time
	// Model is the person's pick for this chat: main, fast or one of the
	// provider's ("" is main).
	Model string
}

func (c *chat) record() string {
	if c.SubjectType == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d", c.SubjectType, c.SubjectID)
}

func (c *chat) replying() bool {
	return c.Replying != nil && time.Since(*c.Replying) < stuckAfter
}

const chatColumns = `id, COALESCE(user_id, 0), COALESCE(contact_id, 0), COALESCE(subject_type, ''), COALESCE(subject_id, 0), purpose, COALESCE(title, ''), replying_since, updated_at, COALESCE(model, '')`

func scanChat(row interface{ Scan(...any) error }) (*chat, error) {
	c := &chat{}
	var replying, updated store.Time
	if err := row.Scan(&c.ID, &c.UserID, &c.ContactID, &c.SubjectType, &c.SubjectID, &c.Purpose, &c.Title, &replying, &updated, &c.Model); err != nil {
		return nil, err
	}
	if replying.Valid {
		t := replying.Time
		c.Replying = &t
	}
	c.UpdatedAt = updated.Time
	return c, nil
}

func loadChat(ctx context.Context, api *plugin.API, id int64) (*chat, error) {
	return scanChat(api.QueryRow(ctx, `SELECT `+chatColumns+` FROM ai_chats WHERE id = ?`, id))
}

// insertChat starts a chat. A Rails database's chats each name a RubyLLM
// model row; a new one names the latest chat's, here none.
func insertChat(ctx context.Context, q plugin.Querier, userID, contactID int64, subjectType string, subjectID int64, purpose, title string) (int64, error) {
	now := store.Now().DB()
	res, err := q.Exec(ctx, `INSERT INTO ai_chats (ruby_llm_model_id, cancelled, user_id, contact_id, subject_type, subject_id, purpose, title, created_at, updated_at)
		VALUES ((SELECT ruby_llm_model_id FROM ai_chats ORDER BY id DESC LIMIT 1), 0, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nullID(userID), nullID(contactID), nullStr(subjectType), nullID(subjectID), purpose, nullStr(title), now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func nullID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// subjectOf reads "Type:id" (an engagement by ref too) into a record the
// person can see; ok false for none.
func subjectOf(ctx context.Context, api *plugin.API, record string, types []string) (string, int64, map[string]any, bool) {
	typ, id, found := strings.Cut(strings.TrimSpace(record), ":")
	if !found || !contains(types, typ) {
		return "", 0, nil, false
	}
	q := plugin.Query{Type: typ}
	if n, err := strconv.ParseInt(id, 10, 64); err == nil {
		q.ID = n
	} else if typ == "Engagement" {
		q.Ref = strings.ToUpper(id)
	} else {
		return "", 0, nil, false
	}
	recs, err := api.Records(ctx, q)
	if err != nil || len(recs) == 0 {
		return "", 0, nil, false
	}
	return typ, intOf(recs[0]["id"]), recs[0], true
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// clientOf is the client a record belongs to (0 for none): itself, its
// client, or its engagement's.
func clientOf(ctx context.Context, api *plugin.API, rec map[string]any) int64 {
	if rec == nil {
		return 0
	}
	if rec["type"] == "Client" {
		return intOf(rec["id"])
	}
	if c, ok := rec["client"].(map[string]any); ok {
		return intOf(c["id"])
	}
	if id := intOf(rec["client_id"]); id != 0 {
		return id
	}
	if e, ok := rec["engagement"].(map[string]any); ok && intOf(e["id"]) != 0 {
		if recs, err := api.Records(ctx, plugin.Query{Type: "Engagement", ID: intOf(e["id"])}); err == nil && len(recs) > 0 {
			return clientOf(ctx, api, recs[0])
		}
	}
	return 0
}

// excludedRecord is whether record's client is kept out of AI.
func excludedRecord(ctx context.Context, api *plugin.API, typ string, id int64) (bool, error) {
	if typ == "" || typ == "User" {
		return false, nil
	}
	ex, err := excludedClients(ctx, api)
	if err != nil || len(ex) == 0 {
		return false, err
	}
	recs, err := api.Records(ctx, plugin.Query{Type: typ, ID: id})
	if err != nil || len(recs) == 0 {
		return false, err
	}
	return ex[clientOf(ctx, api, recs[0])], nil
}

// clientWord is the install's word for a client, in lowercase.
func clientWord(ctx context.Context, api *plugin.API, plural bool) string {
	w := "client"
	if site, err := api.Site(ctx); err == nil {
		if t := site.Terms["client"]; t != nil {
			w = t["one"]
			if plural {
				w = t["other"]
			}
		}
	}
	return strings.ToLower(w)
}

// ---- routes --------------------------------------------------------------------------------

// status is whether the Ask button shows, for the header: switched on and
// set up (over budget, it still opens, to say so).
func status(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	reason, err := available(ctx, api, s)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"ready": s.ready(), "available": reason == "", "reason": nullStr(reason)}
	if record := req.Query.Get("record"); record != "" {
		if typ, id, _, ok := subjectOf(ctx, api, record, []string{"Client", "Engagement", "Todo", "Request", "ScopeItem", "Commitment"}); ok {
			ex, err := excludedRecord(ctx, api, typ, id)
			if err != nil {
				return nil, err
			}
			out["excluded"] = ex
		}
	}
	return plugin.JSON(200, out), nil
}

func listChats(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return res, nil
	}
	rows, err := api.Query(ctx, `SELECT `+chatColumns+` FROM ai_chats WHERE user_id = ? AND purpose = 'ask' AND title IS NOT NULL ORDER BY updated_at DESC LIMIT 30`, caller.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		c, err := scanChat(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, map[string]any{"id": c.ID, "title": c.Title, "record": nullStr(c.record()), "updated_at": c.UpdatedAt.UTC().Format(time.RFC3339)})
	}
	return plugin.JSON(200, map[string]any{"chats": out}), rows.Err()
}

// currentChat is the person's ongoing chat about the record (from the last
// half day), or a new one.
func currentChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return res, nil
	}
	typ, id, _, _ := subjectOf(ctx, api, req.Query.Get("record"), chatSubjects)
	since := store.TimeOf(time.Now().Add(-12 * time.Hour)).DB()
	c, err := scanChat(api.QueryRow(ctx, `SELECT `+chatColumns+` FROM ai_chats WHERE user_id = ? AND purpose = 'ask' AND COALESCE(subject_type, '') = ? AND COALESCE(subject_id, 0) = ? AND updated_at >= ?
		ORDER BY updated_at DESC LIMIT 1`, caller.ID, typ, id, since))
	if errors.Is(err, sql.ErrNoRows) {
		chatID, err := insertChat(ctx, api, caller.ID, 0, typ, id, "ask", "")
		if err != nil {
			return nil, err
		}
		c, err = loadChat(ctx, api, chatID)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, req.Query.Get("page"), req.Actor)
}

func newChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return res, nil
	}
	p := params(req)
	typ, id, _, _ := subjectOf(ctx, api, str(p["record"]), chatSubjects)
	chatID, err := insertChat(ctx, api, caller.ID, 0, typ, id, "ask", "")
	if err != nil {
		return nil, err
	}
	c, err := loadChat(ctx, api, chatID)
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, str(p["page"]), req.Actor)
}

// ownChat is the caller's chat by the route's id.
func ownChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*chat, *plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return nil, res, nil
	}
	c, err := loadChat(ctx, api, pathID(req, "id"))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (c.UserID != caller.ID || c.Purpose != "ask")) {
		return nil, missing("chat"), nil
	}
	return c, nil, err
}

func showChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	return chatAnswer(ctx, api, c, req.Query.Get("page"), req.Actor)
}

func askChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	text := strings.TrimSpace(str(wrapped(params(req), "message")["content"]))
	if err := askSoon(ctx, api, c, text, func(ctx context.Context, api *plugin.API) { reply(ctx, api, c.ID, req.Actor, nil) }); err != nil {
		return failed(err)
	}
	c, err = loadChat(ctx, api, c.ID)
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, "", req.Actor)
}

// askSoon saves the question and starts the reply (AiChat#ask_soon!).
func askSoon(ctx context.Context, api *plugin.API, c *chat, text string, run func(context.Context, *plugin.API)) error {
	return askSoonWith(ctx, api, c, text, nil, run)
}

// askSoonWith is askSoon with pictures sent beside the question.
func askSoonWith(ctx context.Context, api *plugin.API, c *chat, text string, images []image, run func(context.Context, *plugin.API)) error {
	if text == "" && len(images) > 0 {
		text = "What do you make of this?"
	}
	if text == "" {
		return notNow("Write a question first.")
	}
	s, err := loadSetting(ctx, api)
	if err != nil {
		return err
	}
	if !s.ready() {
		return notNow("AI isn't set up: Settings > Plugins > AI.")
	}
	ex, err := chatExcluded(ctx, api, c)
	if err != nil {
		return err
	}
	if ex {
		return notNow("AI is off for this %s.", clientWord(ctx, api, false))
	}
	if over, err := s.overBudget(ctx, api); err != nil {
		return err
	} else if over {
		return notNow("This month's AI budget is spent: Settings > Plugins > AI.")
	}
	if c.replying() {
		return notNow("Wait for the answer above first.")
	}
	if n, err := pendingCalls(ctx, api, c.ID); err != nil {
		return err
	} else if n > 0 {
		return notNow("Approve or decline the change above first.")
	}
	text = truncate(text, 8000)
	sent := contextLine(ctx, api, c) + "\n\n" + text
	rawMap := map[string]any{"text": sent}
	if len(images) > 0 {
		rawMap["images"] = images
	}
	raw, _ := json.Marshal(rawMap)
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `INSERT INTO ai_messages (ai_chat_id, role, content, raw_content, created_at, updated_at) VALUES (?, 'user', ?, ?, ?, ?)`, c.ID, text, string(raw), now, now); err != nil {
		return err
	}
	title := c.Title
	if title == "" {
		title = truncate(strings.Join(strings.Fields(text), " "), 80)
	}
	if _, err := api.Exec(ctx, `UPDATE ai_chats SET title = ?, replying_since = ?, updated_at = ? WHERE id = ?`, title, now, now, c.ID); err != nil {
		return err
	}
	if err := api.Background("reply", run); err != nil {
		_, _ = api.Exec(ctx, `UPDATE ai_chats SET replying_since = NULL WHERE id = ?`, c.ID)
		return notNow("%s", err.Error())
	}
	return nil
}

// chatExcluded is whether the chat's record (or the contact's client) is
// kept out of AI.
func chatExcluded(ctx context.Context, api *plugin.API, c *chat) (bool, error) {
	if c.ContactID != 0 {
		ex, err := excludedClients(ctx, api)
		if err != nil {
			return false, err
		}
		var clientID int64
		if recs, err := api.Records(ctx, plugin.Query{Type: "Contact", ID: c.ContactID}); err == nil && len(recs) > 0 {
			clientID = clientOf(ctx, api, recs[0])
		}
		return ex[clientID], nil
	}
	return excludedRecord(ctx, api, c.SubjectType, c.SubjectID)
}

func decideChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	run := func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
		return api.RunTool(ctx, req.Actor, name, args)
	}
	again := func(ctx context.Context, api *plugin.API) { reply(ctx, api, c.ID, req.Actor, nil) }
	if res, err := decide(ctx, api, c, params(req), run, again); res != nil || err != nil {
		return res, err
	}
	c, err = loadChat(ctx, api, c.ID)
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, "", req.Actor)
}

// decide records the person's decision on a proposed change: approved, it
// runs now, as them; declined, the model is told. Once every change in
// that reply is decided, the reply carries on in the background.
func decide(ctx context.Context, api *plugin.API, c *chat, p map[string]any, run func(context.Context, string, map[string]any) (map[string]any, error), again func(context.Context, *plugin.API)) (*plugin.Response, error) {
	if c.replying() {
		return refuse(http.StatusConflict, "Wait for the answer above first."), nil
	}
	var id, msgID int64
	var name string
	var args sql.NullString
	err := api.QueryRow(ctx, `SELECT t.id, t.ai_message_id, t.name, t.arguments FROM ai_tool_calls t JOIN ai_messages m ON m.id = t.ai_message_id
		WHERE m.ai_chat_id = ? AND t.tool_call_id = ? AND t.approval IS NULL AND t.result IS NULL`, c.ID, str(p["tool_call_id"])).Scan(&id, &msgID, &name, &args)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse(http.StatusNotFound, "That change was already decided, or isn't waiting any more."), nil
	} else if err != nil {
		return nil, err
	}
	approve := str(p["decision"]) == "approve"
	approval, result, isError := "denied", "The person declined this change. Don't make it; ask what they'd like instead, if anything.", false
	if approve {
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal([]byte(args.String), &call)
		approval = "approved"
		out, err := run(ctx, call.Name, call.Arguments)
		if err != nil {
			result, isError = err.Error(), true
		} else {
			result, isError = trim(out), out["status"] == "error"
		}
	}
	if _, err := api.Exec(ctx, `UPDATE ai_tool_calls SET approval = ?, result = ?, is_error = ?, updated_at = ? WHERE id = ?`, approval, result, isError, store.Now().DB(), id); err != nil {
		return nil, err
	}
	outcome, outcomeText := "done", ""
	if approve {
		outcomeText = resultSummary(result)
		if isError {
			outcome = "failed"
		}
	}
	settleApproval(ctx, api, c.ID, str(p["tool_call_id"]), map[bool]string{true: "approved", false: "declined"}[approve], 0, outcome, outcomeText)
	var left int
	if err := api.QueryRow(ctx, `SELECT COUNT(*) FROM ai_tool_calls WHERE ai_message_id = ? AND result IS NULL AND approval IS NULL`, msgID).Scan(&left); err != nil {
		return nil, err
	}
	if left == 0 {
		now := store.Now().DB()
		if _, err := api.Exec(ctx, `UPDATE ai_chats SET replying_since = ?, updated_at = ? WHERE id = ?`, now, now, c.ID); err != nil {
			return nil, err
		}
		if err := api.Background("reply", again); err != nil {
			_, _ = api.Exec(ctx, `UPDATE ai_chats SET replying_since = NULL WHERE id = ?`, c.ID)
			return refuse(http.StatusConflict, err.Error()), nil
		}
	}
	return nil, nil
}

// pendingCalls is how many proposed changes wait on the person.
func pendingCalls(ctx context.Context, api *plugin.API, chatID int64) (int, error) {
	var n int
	err := api.QueryRow(ctx, `SELECT COUNT(*) FROM ai_tool_calls t JOIN ai_messages m ON m.id = t.ai_message_id WHERE m.ai_chat_id = ? AND t.approval IS NULL AND t.result IS NULL AND t.name IN ('run_tool', 'portal_action')`, chatID).Scan(&n)
	return n, err
}

// ---- the reply ---------------------------------------------------------------------------

// runner is the chat's tools, for staff or a portal contact: the tool
// definitions, and running one (pending when it's a change).
type runner interface {
	system(ctx context.Context) string
	tools() []toolDef
	call(ctx context.Context, name string, input json.RawMessage) (result string, isError, pending bool)
}

// reply runs the conversation on: answering the question, or carrying on
// past a decision, until the model's done, a change waits on the person,
// or it has taken enough steps (AiChat#reply!).
func reply(ctx context.Context, api *plugin.API, chatID int64, caller *plugin.Caller, contact *plugin.Contact) {
	ctx, stop := stoppable(ctx, chatID)
	var st *streamer
	if contact == nil && caller != nil {
		st = &streamer{api: api, chatID: chatID, to: caller.ID}
	}
	defer func() {
		stop()
		now := store.Now().DB()
		if _, err := api.Exec(context.WithoutCancel(ctx), `UPDATE ai_chats SET replying_since = NULL, updated_at = ? WHERE id = ?`, now, chatID); err != nil {
			api.Log("reply: " + err.Error())
		}
		st.done(context.WithoutCancel(ctx))
	}()
	var r runner
	if contact != nil {
		r = &portalRunner{api: api, contact: contact}
	} else {
		r = &staffRunner{api: api, caller: caller}
	}
	s, err := loadSetting(ctx, api)
	if err != nil {
		api.Log("reply: " + err.Error())
		return
	}
	model := s.modelFor(false)
	if c, err := loadChat(ctx, api, chatID); err == nil {
		model = pickedModel(s, c.Model)
	}
	fail := func(err error) {
		saveFailure(ctx, api, chatID, fmt.Sprintf("I couldn’t answer (%s): %s", routeName(s.Provider, model), truncate(ctxErr(ctx, err).Error(), 300)))
	}
	for step := 0; ; step++ {
		if step == maxSteps {
			saveFailure(ctx, api, chatID, "I stopped there, after many steps. Ask me to carry on if you'd like more.")
			return
		}
		if reason, err := available(ctx, api, s); err != nil || reason != "" {
			if err == nil {
				err = errors.New(reason)
			}
			fail(err)
			return
		}
		turns, pending, err := history(ctx, api, chatID)
		if err != nil {
			fail(err)
			return
		}
		if pending > 0 {
			return
		}
		l, err := s.llmFor(api, model)
		if err != nil {
			fail(err)
			return
		}
		var ans *answer
		if st != nil {
			st.begin()
			ans, err = l.streamTurn(ctx, ask{System: r.system(ctx), Tools: r.tools(), Turns: turns}, st.add)
		} else {
			ans, err = l.turn(ctx, ask{System: r.system(ctx), Tools: r.tools(), Turns: turns})
		}
		if err != nil {
			if stopped(ctx) {
				saveStopped(context.WithoutCancel(ctx), api, chatID, st)
				return
			}
			fail(err)
			return
		}
		text := ans.Text
		switch ans.Stop {
		case "refusal":
			if text == "" {
				text = "I can’t help with that one. Try asking another way."
			}
		case "max_tokens":
			text = strings.TrimSpace(text + "\n\n(That answer was cut short: ask for less, or one thing at a time.)")
		}
		msgID, err := saveAssistant(ctx, api, chatID, text, ans)
		if err != nil {
			api.Log("reply: " + err.Error())
			return
		}
		recordUsage(ctx, api, chatID, msgID, l, ans.Usage)
		st.saved(ctx)
		if len(ans.Calls) == 0 {
			return
		}
		// Tools run only when the model stopped to call them: a refusal or a
		// cut-short answer leaves its calls unrun, and says so.
		run := ans.Stop == "tool_use"
		waiting := false
		for i, call := range ans.Calls {
			args, _ := json.Marshal(inputOf(call.Input))
			now := store.Now().DB()
			res, err := api.Exec(ctx, `INSERT INTO ai_tool_calls (ai_message_id, tool_call_id, name, arguments, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
				msgID, call.ID, call.Name, string(args), i, now, now)
			if err != nil {
				api.Log("reply: " + err.Error())
				return
			}
			rowID, _ := res.LastInsertId()
			result, isError, pending := "This wasn't run.", true, false
			if run {
				result, isError, pending = r.call(ctx, call.Name, call.Input)
			}
			if pending {
				waiting = true
				continue
			}
			if _, err := api.Exec(ctx, `UPDATE ai_tool_calls SET result = ?, is_error = ?, updated_at = ? WHERE id = ?`, result, isError, store.Now().DB(), rowID); err != nil {
				api.Log("reply: " + err.Error())
				return
			}
		}
		if waiting && contact == nil {
			// Into the person's Approvals too; they're looking at the panel,
			// so their bell isn't rung.
			sr := r.(*staffRunner)
			proposed(ctx, api, chatID, caller.ID, "ask", 0, func(name string) string {
				if t := sr.find(ctx, name); t != nil && t.Title != "" {
					return t.Title
				}
				return humanize(name)
			}, false)
		}
		if waiting || !run {
			return
		}
	}
}

func saveAssistant(ctx context.Context, api *plugin.API, chatID int64, text string, ans *answer) (int64, error) {
	now := store.Now().DB()
	res, err := api.Exec(ctx, `INSERT INTO ai_messages (ai_chat_id, role, content, raw_content, thinking_text, finish_reason, created_at, updated_at) VALUES (?, 'assistant', ?, ?, ?, ?, ?, ?)`,
		chatID, text, string(ans.Raw), nullStr(ans.Thinking), ans.Stop, now, now)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

// saveFailure says in the chat why there's no answer; it never goes back
// to the model.
func saveFailure(ctx context.Context, api *plugin.API, chatID int64, text string) {
	now := store.Now().DB()
	if _, err := api.Exec(context.WithoutCancel(ctx), `INSERT INTO ai_messages (ai_chat_id, role, content, raw_content, finish_reason, created_at, updated_at) VALUES (?, 'assistant', ?, '{"error":true}', 'error', ?, ?)`,
		chatID, text, now, now); err != nil {
		api.Log("reply: " + err.Error())
	}
}

// recordUsage keeps what a call cost, for the budget.
func recordUsage(ctx context.Context, api *plugin.API, chatID, msgID int64, l *llm, u usage) {
	if _, err := api.Exec(context.WithoutCancel(ctx), `INSERT INTO ai_usages (ai_chat_id, ai_message_id, provider, model, input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, total_cost, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, chatID, nullID(msgID), l.provider, l.model, u.Input, u.Output, u.CacheRead, u.CacheWrite, cost(l.provider, l.model, u), store.Now().DB()); err != nil {
		api.Log("usage: " + err.Error())
	}
}

type storedMessage struct {
	ID      int64
	Role    string
	Content string
	Raw     string
	Finish  string
	At      time.Time
	Calls   []storedCall
	// Thinking is what the model said it thought, where it says.
	Thinking string
}

type storedCall struct {
	ID       int64
	CallID   string
	Name     string
	Args     string
	Approval string
	Result   *string
	IsError  bool
}

func (m *storedMessage) failed() bool { return m.Finish == "error" }

func loadMessages(ctx context.Context, api *plugin.API, chatID int64) ([]*storedMessage, error) {
	rows, err := api.Query(ctx, `SELECT id, role, COALESCE(content, ''), COALESCE(raw_content, ''), COALESCE(finish_reason, ''), created_at, COALESCE(thinking_text, '') FROM ai_messages
		WHERE ai_chat_id = ? AND role IN ('user', 'assistant') ORDER BY id`, chatID)
	if err != nil {
		return nil, err
	}
	var out []*storedMessage
	byID := map[int64]*storedMessage{}
	for rows.Next() {
		m := &storedMessage{}
		var at store.Time
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &m.Raw, &m.Finish, &at, &m.Thinking); err != nil {
			rows.Close()
			return nil, err
		}
		m.At = at.Time
		out = append(out, m)
		byID[m.ID] = m
	}
	rows.Close()
	if len(out) == 0 {
		return out, rows.Err()
	}
	rows, err = api.Query(ctx, `SELECT t.id, t.ai_message_id, t.tool_call_id, t.name, COALESCE(t.arguments, '{}'), COALESCE(t.approval, ''), t.result, t.is_error
		FROM ai_tool_calls t JOIN ai_messages m ON m.id = t.ai_message_id WHERE m.ai_chat_id = ? ORDER BY t.ai_message_id, t.position, t.id`, chatID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var c storedCall
		var msgID int64
		var result sql.NullString
		if err := rows.Scan(&c.ID, &msgID, &c.CallID, &c.Name, &c.Args, &c.Approval, &result, &c.IsError); err != nil {
			return nil, err
		}
		if result.Valid {
			r := result.String
			c.Result = &r
		}
		if m := byID[msgID]; m != nil {
			m.Calls = append(m.Calls, c)
		}
	}
	return out, rows.Err()
}

// history is the conversation as the model gets it, and how many changes
// still wait on the person. Every call of an assistant turn is answered in
// the next user turn (one not run says so), so the conversation always
// holds together; failures said in the chat aren't part of it.
func history(ctx context.Context, api *plugin.API, chatID int64) ([]turn, int, error) {
	msgs, err := loadMessages(ctx, api, chatID)
	if err != nil {
		return nil, 0, err
	}
	var turns []turn
	pending := 0
	for _, m := range msgs {
		if m.failed() || m.Finish == "agent" {
			continue
		}
		if m.Role == "user" {
			var raw struct {
				Text   string  `json:"text"`
				Images []image `json:"images"`
			}
			text := m.Content
			if json.Unmarshal([]byte(m.Raw), &raw) == nil && raw.Text != "" {
				text = raw.Text
			}
			turns = append(turns, turn{Role: "user", Text: text, Images: raw.Images})
			continue
		}
		t := turn{Role: "assistant", Text: m.Content, Raw: json.RawMessage(m.Raw)}
		if !json.Valid(t.Raw) {
			t.Raw = nil
		}
		var results []toolResult
		for _, c := range m.Calls {
			input := json.RawMessage(c.Args)
			if !json.Valid(input) {
				input = json.RawMessage("{}")
			}
			t.Calls = append(t.Calls, toolCall{ID: c.CallID, Name: c.Name, Input: input})
			switch {
			case c.Result != nil:
				results = append(results, toolResult{ID: c.CallID, Name: c.Name, Content: *c.Result, IsError: c.IsError})
			case c.Approval == "" && m.Finish == "tool_use" && (c.Name == "run_tool" || c.Name == "portal_action"):
				pending++
			default:
				results = append(results, toolResult{ID: c.CallID, Name: c.Name, Content: "This wasn't run.", IsError: true})
			}
		}
		turns = append(turns, t)
		if len(results) > 0 {
			turns = append(turns, turn{Role: "user", Results: results})
		}
	}
	return turns, pending, nil
}

// trim is a tool's answer as the model sees it: at most 12,000 characters.
func trim(v any) string {
	text, ok := v.(string)
	if !ok {
		raw, _ := json.Marshal(v)
		text = string(raw)
	}
	if len(text) > resultLimit {
		return text[:resultLimit] + "… (cut short: ask for less, or one record at a time)"
	}
	return text
}

// ---- what the model is told --------------------------------------------------------------

// words are the install's own words for its things.
func words(site plugin.Site) string {
	w := func(k string, plural bool) string {
		t := site.Terms[k]
		if t == nil {
			return k
		}
		if plural {
			return strings.ToLower(t["other"])
		}
		return strings.ToLower(t["one"])
	}
	return fmt.Sprintf("%s, %s, %s, %s and %s", w("client", true), w("engagement", true), w("scope_item", true), w("work", true), w("commitment", true))
}

func term(site plugin.Site, k string, plural bool) string {
	t := site.Terms[k]
	if t == nil {
		return k
	}
	if plural {
		return t["other"]
	}
	return t["one"]
}

// staffSystem is the instructions (Ai::Instructions.for), the same for
// every turn of a person's chats so the provider can cache them: what
// changes (today, the record on screen) goes with each question instead.
func staffSystem(site plugin.Site, caller *plugin.Caller) string {
	runsOn := "this install runs on"
	if site.Name != "" && site.Name != "Runwell" {
		runsOn = site.Name + " runs on"
	}
	client := strings.ToLower(term(site, "client", false))
	clients := strings.ToLower(term(site, "client", true))
	return fmt.Sprintf(`You are the assistant inside Runwell, the project management %s. You help %s (%s), working on their behalf with exactly their permissions.

This install calls its %s. Always use those words, never others, and never words from one industry: it may be a business, a team or a household.

Each question starts with today's date and the record they're looking at, in brackets.

How to work:
- Get facts from the tools (search, read_record, find_tools then run_tool). Never guess or invent a record, a date, a name or a number. If you can't find it, say so.
- Mention records by their ref exactly as "Type:id" (Client:4, Todo:12, Engagement:P-3); Runwell turns those into links.
- To change anything, find the action with find_tools and call run_tool. The person sees each change and approves or declines it, so propose one clear change at a time and say why in a sentence. Never claim something is done until its result says so.
- Anything that reaches a %s (sending an agreement, an email, making something visible to them) needs extra care: say exactly what they'll receive.
- Internal estimates and estimate notes never go into anything written for a %s.
- Some %s are kept out of AI by the owner: if a tool refuses one, say so and stop.
- Be brief and plain: short paragraphs or a few bullets, no preamble, no headings for short answers. Dates as people say them ("Friday, October 9").`,
		runsOn, caller.Name, caller.Role, words(site), client, client, clients)
}

// contextLine is what goes before a question: today, and the record on
// screen (Ai::Instructions' subject line).
func contextLine(ctx context.Context, api *plugin.API, c *chat) string {
	site, _ := api.Site(ctx)
	loc := time.UTC
	if site.Location != nil {
		loc = site.Location
	}
	today := time.Now().In(loc).Format("Monday, January 2, 2006")
	zone := site.TimeZone
	if zone == "" {
		zone = "UTC"
	}
	about := "They're not on any one record; they may ask about anything."
	if c.ContactID != 0 {
		about = "They're in their portal."
	} else if c.SubjectType != "" {
		ref, name := c.record(), ""
		if recs, err := api.Records(ctx, plugin.Query{Type: c.SubjectType, ID: c.SubjectID}); err == nil && len(recs) > 0 {
			name = str(recs[0]["display_name"])
			if c.SubjectType == "Engagement" && str(recs[0]["ref"]) != "" {
				ref = "Engagement:" + str(recs[0]["ref"])
			}
		}
		about = fmt.Sprintf(`They're looking at %s (%s). Questions about "this" mean that record: read it with read_record first.`, ref, truncate(name, 80))
	}
	return fmt.Sprintf("[Today is %s (%s). %s]", today, zone, about)
}

// ---- the staff tools (Ai::Tools) ------------------------------------------------------

type staffRunner struct {
	api    *plugin.API
	caller *plugin.Caller
	list   []plugin.ToolInfo
	// excluded are the clients whose records the model mustn't see: those
	// kept out of AI, unless it says more (an agent's narrower reach).
	excluded func(context.Context) (map[int64]bool, error)
}

func (r *staffRunner) system(ctx context.Context) string {
	site, _ := r.api.Site(ctx)
	return staffSystem(site, r.caller)
}

// readTypes are what read_record reads, by the show tool that does.
var readTypes = []struct{ typ, tool, key string }{
	{"Client", "show_client", "id"}, {"Engagement", "show_engagement", "ref"}, {"Todo", "show_work", "id"},
	{"Request", "show_request", "id"}, {"ScopeItem", "show_scope_item", "id"},
}

func object(props map[string]any, required ...string) map[string]any {
	if required == nil {
		required = []string{}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func (r *staffRunner) tools() []toolDef {
	return []toolDef{
		{Name: "search", Description: `Search Runwell (clients, contacts, engagements, work, requests, notes, commitments) by words. Answers matching records, each with its record ref ("Type:id") to read with read_record.`,
			Schema: object(map[string]any{"query": map[string]any{"type": "string", "description": "The words to look for"}}, "query")},
		{Name: "read_record", Description: `Read one record in full: "Type:id" (Client:4, Todo:12), or an engagement by ref (Engagement:P-3). Types: Client, Engagement, Todo, Request, ScopeItem.`,
			Schema: object(map[string]any{"record": map[string]any{"type": "string", "description": `"Type:id", as search and other answers give it`}}, "record")},
		{Name: "find_tools", Description: `Find the Runwell actions for a job ("create work", "resolve commitment", "briefing", "send agreement"). Answers each action's name, what it does, whether it changes anything, and its input schema, to call with run_tool.`,
			Schema: object(map[string]any{"query": map[string]any{"type": "string", "description": "What you want to do, in a few words"}}, "query")},
		{Name: "run_tool", Description: "Run a Runwell action by name (from find_tools) with its arguments. Reading actions answer at once. Actions that change something wait for the person to approve them: propose one change per call, and say briefly what it does and why.",
			Schema: object(map[string]any{
				"name":      map[string]any{"type": "string", "description": "The action's name, from find_tools"},
				"arguments": map[string]any{"type": "object", "description": "Its input, as its schema says", "additionalProperties": true},
			}, "name")},
	}
}

func (r *staffRunner) catalogue(ctx context.Context) ([]plugin.ToolInfo, error) {
	if r.list == nil {
		list, err := r.api.Tools(ctx, r.caller)
		if err != nil {
			return nil, err
		}
		r.list = list
	}
	return r.list, nil
}

func (r *staffRunner) find(ctx context.Context, name string) *plugin.ToolInfo {
	list, err := r.catalogue(ctx)
	if err != nil {
		return nil
	}
	for i := range list {
		if list[i].Name == name {
			return &list[i]
		}
	}
	return nil
}

func (r *staffRunner) call(ctx context.Context, name string, input json.RawMessage) (string, bool, bool) {
	var in struct {
		Query     string         `json:"query"`
		Record    string         `json:"record"`
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(input, &in); err != nil {
		return "The input wasn't JSON: " + err.Error(), true, false
	}
	errorOut := func(msg string) (string, bool, bool) { return trim(map[string]any{"error": msg}), true, false }
	switch name {
	case "search":
		return r.read(ctx, "search", map[string]any{"q": in.Query}, false)
	case "read_record":
		typ, id, _ := strings.Cut(in.Record, ":")
		for _, t := range readTypes {
			if t.typ == typ && id != "" {
				return r.read(ctx, t.tool, map[string]any{t.key: id}, true)
			}
		}
		return errorOut(fmt.Sprintf("Can't read %s: use one of Client, Engagement, Todo, Request, ScopeItem with an id.", in.Record))
	case "find_tools":
		list, err := r.catalogue(ctx)
		if err != nil {
			return errorOut(err.Error())
		}
		return trim(findTools(list, in.Query)), false, false
	case "run_tool":
		t := r.find(ctx, in.Name)
		if t == nil {
			return errorOut("No action called " + in.Name + " for this person. Use find_tools.")
		}
		if !t.ReadOnly {
			return "", false, true // waits on the person
		}
		args := in.Arguments
		if args == nil {
			args = map[string]any{}
		}
		return r.read(ctx, t.Name, args, false)
	}
	return errorOut("There's no tool called " + name + ".")
}

// read runs a tool that only looks, keeping kept-out clients' records from
// the model: a record of theirs is refused, and lists lose theirs.
func (r *staffRunner) read(ctx context.Context, tool string, args map[string]any, whole bool) (string, bool, bool) {
	out, err := r.api.RunTool(ctx, r.caller, tool, args)
	if err != nil {
		return trim(map[string]any{"error": err.Error()}), true, false
	}
	exclude := r.excluded
	if exclude == nil {
		exclude = func(ctx context.Context) (map[int64]bool, error) { return excludedClients(ctx, r.api) }
	}
	ex, err := exclude(ctx)
	if err != nil {
		return trim(map[string]any{"error": err.Error()}), true, false
	}
	if len(ex) > 0 {
		if whole && mentions(out, ex) {
			return trim(map[string]any{"error": "AI is off for this " + clientWord(ctx, r.api, false) + ": the owner keeps it out of AI."}), true, false
		}
		if mentions(out, ex) {
			scrub(out, ex)
			out["summary"] = "Some records were left out: their " + clientWord(ctx, r.api, false) + " is kept out of AI."
		}
		if mentions(out["data"], ex) {
			return trim(map[string]any{"error": "AI is off for this " + clientWord(ctx, r.api, false) + ": the owner keeps it out of AI."}), true, false
		}
	}
	return trim(out), out["status"] == "error", false
}

var clientRef = regexp.MustCompile(`^Client:(\d+)$`)

// mentions is whether v names a kept-out client: as a record of its own,
// its ref, or a client_id.
func mentions(v any, ex map[int64]bool) bool {
	switch x := v.(type) {
	case map[string]any:
		if x["type"] == "Client" && ex[intOf(x["id"])] {
			return true
		}
		if m := clientRef.FindStringSubmatch(str(x["record"])); m != nil {
			if id, _ := strconv.ParseInt(m[1], 10, 64); ex[id] {
				return true
			}
		}
		if id := intOf(x["client_id"]); id != 0 && ex[id] {
			return true
		}
		for _, inner := range x {
			if mentions(inner, ex) {
				return true
			}
		}
	case []any:
		for _, inner := range x {
			if mentions(inner, ex) {
				return true
			}
		}
	}
	return false
}

// scrub drops the items of every list in v that name a kept-out client.
func scrub(v any, ex map[int64]bool) {
	switch x := v.(type) {
	case map[string]any:
		for k, inner := range x {
			if list, ok := inner.([]any); ok {
				kept := []any{}
				for _, item := range list {
					if !mentions(item, ex) {
						kept = append(kept, item)
					}
				}
				x[k] = kept
				for _, item := range kept {
					scrub(item, ex)
				}
				continue
			}
			scrub(inner, ex)
		}
	case []any:
		for _, inner := range x {
			scrub(inner, ex)
		}
	}
}

// findTools ranks the person's actions by the words asked for.
func findTools(list []plugin.ToolInfo, query string) map[string]any {
	words := regexp.MustCompile(`\w+`).FindAllString(strings.ToLower(query), -1)
	type scored struct {
		t plugin.ToolInfo
		s int
	}
	var ranked []scored
	for _, t := range list {
		hay := strings.ToLower(strings.ReplaceAll(t.Name, "_", " ") + " " + t.Title + " " + t.Description)
		s := 0
		for _, w := range words {
			if strings.Contains(t.Name, w) {
				s += 3
			}
			if strings.Contains(hay, w) {
				s++
			}
		}
		if s > 0 {
			ranked = append(ranked, scored{t, s})
		}
	}
	for i := 1; i < len(ranked); i++ {
		for j := i; j > 0 && ranked[j].s > ranked[j-1].s; j-- {
			ranked[j], ranked[j-1] = ranked[j-1], ranked[j]
		}
	}
	if len(ranked) == 0 {
		return map[string]any{"found": []any{}, "hint": "Nothing matched; try other words, or search for records instead."}
	}
	found := []any{}
	for i, x := range ranked {
		if i == 8 {
			break
		}
		input := map[string]any{}
		for k, v := range x.t.InputSchema {
			if k != "type" {
				input[k] = v
			}
		}
		found = append(found, map[string]any{"name": x.t.Name, "does": truncate(x.t.Description, 400), "changes_something": !x.t.ReadOnly, "input": input})
	}
	return map[string]any{"found": found}
}
