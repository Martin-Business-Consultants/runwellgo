package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Approvals: one inbox (ai_approvals) where everything an AI or a machine
// made waits for a person before it counts. Items come from:
//
//   - the changes Ask and the agents propose (source "ask" or "agent"):
//     approving one runs it through decide, as Ask's own Approve does;
//   - any plugin, through the event contract in docs/plugins.md ("Asking a
//     person to approve"): <key>.approval.requested adds (or refreshes) an
//     item, <key>.approval.withdrawn takes it away. Approving runs the
//     item's approve tool with its args through the tools grant, as the
//     person deciding, inside their own request; declining runs its
//     decline tool, when it names one.
//
// Each item says who may decide it (people by id, a role and those above
// it, or a permission); they're notified and their open tabs told, and
// the outcome (done, or failed and why) stays on the item.

type approval struct {
	ID          int64
	Source      string // ask, agent, or a plugin's key
	SourceID    string // the source's own id
	Kind        string
	Title       string
	Summary     string
	Record      string
	Link        string
	Details     string // markdown
	RequestedBy int64
	Approvers   approvers
	Approve     *toolCallSpec
	Decline     *toolCallSpec
	Status      string // waiting, approved, declined, withdrawn
	Outcome     string // done or failed, once decided
	OutcomeText string
	Note        string
	DecidedBy   int64
	DecidedAt   store.Time
	ChatID      int64
	ToolCallID  string
	RunID       int64
	CreatedAt   store.Time
}

type approvers struct {
	UserIDs    []int64 `json:"user_ids,omitempty"`
	Role       string  `json:"role,omitempty"`
	Capability string  `json:"capability,omitempty"`
}

type toolCallSpec struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

const approvalColumns = `id, source, source_id, COALESCE(kind, ''), title, COALESCE(summary, ''), COALESCE(record, ''), COALESCE(link, ''), COALESCE(details, ''),
	COALESCE(requested_by, 0), COALESCE(approvers, '{}'), COALESCE(approve, ''), COALESCE(decline, ''), status, COALESCE(outcome, ''), COALESCE(outcome_text, ''),
	COALESCE(note, ''), COALESCE(decided_by, 0), decided_at, COALESCE(ai_chat_id, 0), COALESCE(tool_call_id, ''), COALESCE(run_id, 0), created_at`

func scanApproval(row interface{ Scan(...any) error }) (*approval, error) {
	a := &approval{}
	var ap, approve, decline string
	err := row.Scan(&a.ID, &a.Source, &a.SourceID, &a.Kind, &a.Title, &a.Summary, &a.Record, &a.Link, &a.Details, &a.RequestedBy, &ap, &approve, &decline,
		&a.Status, &a.Outcome, &a.OutcomeText, &a.Note, &a.DecidedBy, &a.DecidedAt, &a.ChatID, &a.ToolCallID, &a.RunID, &a.CreatedAt)
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(ap), &a.Approvers)
	if approve != "" {
		a.Approve = &toolCallSpec{}
		_ = json.Unmarshal([]byte(approve), a.Approve)
	}
	if decline != "" {
		a.Decline = &toolCallSpec{}
		_ = json.Unmarshal([]byte(decline), a.Decline)
	}
	return a, nil
}

func loadApproval(ctx context.Context, q plugin.Querier, id int64) (*approval, error) {
	a, err := scanApproval(q.QueryRow(ctx, `SELECT `+approvalColumns+` FROM ai_approvals WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return a, err
}

var roleRank = map[string]int{"member": 1, "manager": 2, "owner": 3}

// mayDecide is whether the caller may approve or decline it: named, of the
// role named or above, or holding the permission named.
func (a *approval) mayDecide(c *plugin.Caller) bool {
	if c == nil || c.Agent {
		return false
	}
	ap := a.Approvers
	switch {
	case slices.Contains(ap.UserIDs, c.ID):
		return true
	case ap.Role != "" && roleRank[c.Role] >= roleRank[ap.Role] && roleRank[ap.Role] > 0:
		return true
	case ap.Capability != "" && c.Can(ap.Capability):
		return true
	}
	return false
}

// approverIDs are the people to tell: those named, those of the role (or
// above), and for a permission the owners (who hold every one).
func (a *approval) approverIDs(ctx context.Context, api *plugin.API) []int64 {
	out := slices.Clone(a.Approvers.UserIDs)
	if a.Approvers.Role != "" || a.Approvers.Capability != "" {
		if users, err := api.Records(ctx, plugin.Query{Type: "User", Limit: 500}); err == nil {
			for _, u := range users {
				role, id := str(u["role"]), intOf(u["id"])
				if active, ok := u["active"].(bool); ok && !active {
					continue
				}
				if (a.Approvers.Role != "" && roleRank[role] >= roleRank[a.Approvers.Role] && roleRank[a.Approvers.Role] > 0) ||
					(a.Approvers.Capability != "" && role == "owner") {
					if !slices.Contains(out, id) {
						out = append(out, id)
					}
				}
			}
		}
	}
	return out
}

// readApprovers reads the contract's approvers: user ids, a role, a
// permission, or an object of them; the requester when none.
func readApprovers(v any, requestedBy int64) approvers {
	var ap approvers
	switch x := v.(type) {
	case []any:
		for _, id := range x {
			if n := intOf(id); n > 0 {
				ap.UserIDs = append(ap.UserIDs, n)
			}
		}
	case string:
		if roleRank[x] > 0 {
			ap.Role = x
		} else if x != "" {
			ap.Capability = x
		}
	case map[string]any:
		for _, id := range anyList(x["user_ids"]) {
			if n := intOf(id); n > 0 {
				ap.UserIDs = append(ap.UserIDs, n)
			}
		}
		if r := str(x["role"]); roleRank[r] > 0 {
			ap.Role = r
		}
		ap.Capability = str(x["capability"])
	default:
		if n := intOf(v); n > 0 {
			ap.UserIDs = []int64{n}
		}
	}
	if len(ap.UserIDs) == 0 && ap.Role == "" && ap.Capability == "" && requestedBy > 0 {
		ap.UserIDs = []int64{requestedBy}
	}
	return ap
}

func anyList(v any) []any {
	l, _ := v.([]any)
	return l
}

func readToolCall(v any) *toolCallSpec {
	m, ok := v.(map[string]any)
	if !ok || strings.TrimSpace(str(m["tool"])) == "" {
		return nil
	}
	args, _ := m["args"].(map[string]any)
	if args == nil {
		args = map[string]any{}
	}
	return &toolCallSpec{Tool: strings.TrimSpace(str(m["tool"])), Args: args}
}

// ---- adding and settling --------------------------------------------------------------

// putApproval adds an item, or brings a waiting one from the same source
// up to date; it tells the people who may decide it when it's new.
func putApproval(ctx context.Context, api *plugin.API, a *approval, notify bool) (int64, error) {
	ap, _ := json.Marshal(a.Approvers)
	var approve, decline any
	if a.Approve != nil {
		b, _ := json.Marshal(a.Approve)
		approve = string(b)
	}
	if a.Decline != nil {
		b, _ := json.Marshal(a.Decline)
		decline = string(b)
	}
	now := store.Now().DB()
	var id int64
	err := api.QueryRow(ctx, `SELECT id FROM ai_approvals WHERE source = ? AND source_id = ?`, a.Source, a.SourceID).Scan(&id)
	fresh := errors.Is(err, sql.ErrNoRows)
	if err != nil && !fresh {
		return 0, err
	}
	if fresh {
		res, err := api.Exec(ctx, `INSERT INTO ai_approvals (source, source_id, kind, title, summary, record, link, details, requested_by, approvers, approve, decline, status, ai_chat_id, tool_call_id, run_id, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'waiting', ?, ?, ?, ?, ?)`, a.Source, a.SourceID, nullStr(a.Kind), a.Title, nullStr(a.Summary), nullStr(a.Record), nullStr(a.Link),
			nullStr(a.Details), nullID(a.RequestedBy), string(ap), approve, decline, nullID(a.ChatID), nullStr(a.ToolCallID), nullID(a.RunID), now, now)
		if err != nil {
			return 0, err
		}
		id, _ = res.LastInsertId()
	} else if _, err := api.Exec(ctx, `UPDATE ai_approvals SET kind = ?, title = ?, summary = ?, record = ?, link = ?, details = ?, approvers = ?, approve = ?, decline = ?, updated_at = ?
		WHERE id = ? AND status = 'waiting'`, nullStr(a.Kind), a.Title, nullStr(a.Summary), nullStr(a.Record), nullStr(a.Link), nullStr(a.Details), string(ap), approve, decline, now, id); err != nil {
		return 0, err
	}
	a.ID = id
	to := a.approverIDs(ctx, api)
	_ = api.Publish(ctx, "approval", to, map[string]any{"id": id, "status": "waiting"})
	if fresh && notify {
		who := "Someone"
		if a.RequestedBy > 0 {
			who = peopleNames(ctx, api)[a.RequestedBy]
		}
		for _, uid := range to {
			body := a.Summary
			if body == "" {
				body = who + " asks for your approval."
			}
			if err := api.Notify(ctx, plugin.Notification{UserID: uid, FromID: a.RequestedBy, Title: a.Title, Body: body,
				Path: fmt.Sprintf("/ai/approvals?item=%d", id), Source: "Approvals", Subject: sourceName(a.Source), Group: fmt.Sprintf("approval:%d", id)}); err != nil {
				api.Log("approval notify: " + err.Error())
			}
		}
	}
	return id, nil
}

func sourceName(s string) string {
	switch s {
	case "ask":
		return "Ask"
	case "agent":
		return "Agents"
	}
	return humanize(s)
}

// settleApproval records the decision on Ask's or an agent's proposed
// change made outside the inbox (the Ask panel's own buttons).
func settleApproval(ctx context.Context, api *plugin.API, chatID int64, toolCallID, status string, decidedBy int64, outcome, outcomeText string) {
	now := store.Now().DB()
	var id int64
	if err := api.QueryRow(ctx, `SELECT id FROM ai_approvals WHERE ai_chat_id = ? AND tool_call_id = ? AND status = 'waiting'`, chatID, toolCallID).Scan(&id); err != nil {
		return
	}
	if _, err := api.Exec(ctx, `UPDATE ai_approvals SET status = ?, outcome = ?, outcome_text = ?, decided_by = ?, decided_at = ?, updated_at = ? WHERE id = ?`,
		status, nullStr(outcome), nullStr(outcomeText), nullID(decidedBy), now, now, id); err != nil {
		api.Log("approval: " + err.Error())
	}
	if a, _ := loadApproval(ctx, api, id); a != nil {
		_ = api.Publish(ctx, "approval", a.approverIDs(ctx, api), map[string]any{"id": id, "status": status})
	}
}

// proposed puts the changes a chat's last reply proposes in the inbox:
// Ask's (source ask, for the person in the panel) or an agent run's.
func proposed(ctx context.Context, api *plugin.API, chatID int64, userID int64, source string, runID int64, title func(name string) string, notify bool) []int64 {
	rows, err := api.Query(ctx, `SELECT t.tool_call_id, COALESCE(t.arguments, '{}'), COALESCE(m.content, ''), COALESCE(c.subject_type, ''), COALESCE(c.subject_id, 0)
		FROM ai_tool_calls t JOIN ai_messages m ON m.id = t.ai_message_id JOIN ai_chats c ON c.id = m.ai_chat_id
		WHERE m.ai_chat_id = ? AND t.approval IS NULL AND t.result IS NULL AND t.name = 'run_tool' ORDER BY t.id`, chatID)
	if err != nil {
		return nil
	}
	type pend struct {
		callID, args, why, typ string
		sid                    int64
	}
	var list []pend
	for rows.Next() {
		var p pend
		if rows.Scan(&p.callID, &p.args, &p.why, &p.typ, &p.sid) == nil {
			list = append(list, p)
		}
	}
	rows.Close()
	people := peopleNames(ctx, api)
	var ids []int64
	for _, p := range list {
		var call struct {
			Name      string         `json:"name"`
			Arguments map[string]any `json:"arguments"`
		}
		_ = json.Unmarshal([]byte(p.args), &call)
		lines := []string{}
		for _, d := range details(call.Arguments, people) {
			lines = append(lines, "- **"+d[0]+":** "+d[1])
		}
		record := ""
		if p.typ != "" {
			record = fmt.Sprintf("%s:%d", p.typ, p.sid)
		}
		link := ""
		if runID != 0 {
			link = fmt.Sprintf("/ai/runs/%d", runID)
		}
		id, err := putApproval(ctx, api, &approval{Source: source, SourceID: fmt.Sprintf("chat:%d:%s", chatID, p.callID), Kind: call.Name, Title: title(call.Name),
			Summary: truncate(strings.TrimSpace(p.why), 400), Record: record, Link: link, Details: strings.Join(lines, "\n"), RequestedBy: userID,
			Approvers: approvers{UserIDs: []int64{userID}}, ChatID: chatID, ToolCallID: p.callID, RunID: runID}, notify)
		if err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// externalApproval is the contract's events: <key>.approval.requested and
// <key>.approval.withdrawn.
func externalApproval(ctx context.Context, api *plugin.API, event string, data json.RawMessage) error {
	source, rest, _ := strings.Cut(event, ".")
	if source == Key {
		return nil
	}
	var ev map[string]any
	dec := json.NewDecoder(strings.NewReader(string(data)))
	dec.UseNumber()
	if err := dec.Decode(&ev); err != nil {
		return err
	}
	p := ev
	// RecordEvent's events carry it under payload.
	if inner, ok := ev["payload"].(map[string]any); ok && ev["kind"] != nil {
		p = inner
	}
	sourceID := strings.TrimSpace(str(p["id"]))
	if sourceID == "" {
		return errors.New(event + " without an id")
	}
	if rest == "approval.withdrawn" {
		now := store.Now().DB()
		_, err := api.Exec(ctx, `UPDATE ai_approvals SET status = 'withdrawn', updated_at = ? WHERE source = ? AND source_id = ? AND status = 'waiting'`, now, source, sourceID)
		if a, _ := scanApproval(api.QueryRow(ctx, `SELECT `+approvalColumns+` FROM ai_approvals WHERE source = ? AND source_id = ?`, source, sourceID)); a != nil {
			_ = api.Publish(ctx, "approval", a.approverIDs(ctx, api), map[string]any{"id": a.ID, "status": "withdrawn"})
		}
		return err
	}
	title := strings.TrimSpace(str(p["title"]))
	if title == "" {
		title = "Approve this?"
	}
	by := intOf(p["requested_by"])
	_, err := putApproval(ctx, api, &approval{Source: source, SourceID: sourceID, Kind: str(p["kind"]), Title: truncate(title, 200), Summary: truncate(str(p["summary"]), 1000),
		Record: str(p["record"]), Link: safeLink(str(p["link"])), Details: truncate(str(p["details_markdown"]), 60000), RequestedBy: by,
		Approvers: readApprovers(p["approvers"], by), Approve: readToolCall(p["approve"]), Decline: readToolCall(p["decline"])}, true)
	return err
}

// safeLink keeps a link that's a web address or a path in the app.
func safeLink(s string) string {
	s = strings.TrimSpace(s)
	if strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "http://") || (strings.HasPrefix(s, "/") && !strings.HasPrefix(s, "//")) {
		return s
	}
	return ""
}

// ---- routes ----------------------------------------------------------------------------

var approvalStatuses = []string{"waiting", "approved", "declined", "withdrawn"}

func approvalJSON(a *approval, me *plugin.Caller, people map[int64]string) map[string]any {
	at := func(t store.Time) any {
		if !t.Valid {
			return nil
		}
		return stamp(t)
	}
	var by any
	if a.RequestedBy > 0 {
		by = map[string]any{"id": a.RequestedBy, "name": people[a.RequestedBy]}
	}
	var decider any
	if a.DecidedBy > 0 {
		decider = map[string]any{"id": a.DecidedBy, "name": people[a.DecidedBy]}
	}
	recordPath := ""
	if t, id, ok := strings.Cut(a.Record, ":"); ok && refPaths[t] != "" {
		recordPath = refPaths[t] + id
	}
	return map[string]any{"id": a.ID, "source": a.Source, "source_name": sourceName(a.Source), "source_id": a.SourceID, "kind": nullStr(a.Kind),
		"title": a.Title, "summary": nullStr(a.Summary), "record": nullStr(a.Record), "record_path": nullStr(recordPath), "link": nullStr(a.Link),
		"details_markdown": nullStr(a.Details), "requested_by": by, "status": a.Status, "outcome": nullStr(a.Outcome), "outcome_text": nullStr(a.OutcomeText),
		"note": nullStr(a.Note), "decided_by": decider, "decided_at": at(a.DecidedAt), "created_at": at(a.CreatedAt), "run_id": nullID(a.RunID),
		"can_decide": a.Status == "waiting" && a.mayDecide(me)}
}

// visibleTo is whether the caller sees the item: they may decide it, they
// asked for it, or they manage settings.
func (a *approval) visibleTo(c *plugin.Caller) bool {
	return a.mayDecide(c) || a.RequestedBy == c.ID || c.Can("manage_settings")
}

func listApprovals(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if req.Actor == nil {
		return refuse(http.StatusUnauthorized, "Sign in first."), nil
	}
	status := req.Query.Get("status")
	if status == "" {
		status = "waiting"
	}
	where, args := []string{"1 = 1"}, []any{}
	if status != "all" && slices.Contains(approvalStatuses, status) {
		where, args = append(where, "status = ?"), append(args, status)
	}
	if s := req.Query.Get("source"); s != "" {
		where, args = append(where, "source = ?"), append(args, s)
	}
	if p := intOf(req.Query.Get("person")); p > 0 {
		where, args = append(where, "requested_by = ?"), append(args, p)
	}
	rows, err := api.Query(ctx, `SELECT `+approvalColumns+` FROM ai_approvals WHERE `+strings.Join(where, " AND ")+` ORDER BY id DESC LIMIT 300`, args...)
	if err != nil {
		return nil, err
	}
	var list []*approval
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if a.visibleTo(req.Actor) {
			list = append(list, a)
		}
	}
	rows.Close()
	people := peopleNames(ctx, api)
	out := []map[string]any{}
	for _, a := range list {
		out = append(out, approvalJSON(a, req.Actor, people))
	}
	sources := []map[string]any{}
	if rows, err := api.Query(ctx, `SELECT DISTINCT source FROM ai_approvals ORDER BY source`); err == nil {
		for rows.Next() {
			var s string
			if rows.Scan(&s) == nil {
				sources = append(sources, map[string]any{"value": s, "label": sourceName(s)})
			}
		}
		rows.Close()
	}
	peopleOpts := []map[string]any{}
	if rows, err := api.Query(ctx, `SELECT DISTINCT requested_by FROM ai_approvals WHERE requested_by IS NOT NULL`); err == nil {
		for rows.Next() {
			var id int64
			if rows.Scan(&id) == nil && people[id] != "" {
				peopleOpts = append(peopleOpts, map[string]any{"value": fmt.Sprint(id), "label": people[id]})
			}
		}
		rows.Close()
	}
	statusOpts := []map[string]any{{"value": "waiting", "label": "Waiting"}, {"value": "approved", "label": "Approved"}, {"value": "declined", "label": "Declined"},
		{"value": "withdrawn", "label": "Withdrawn"}, {"value": "all", "label": "All"}}
	return plugin.JSON(200, map[string]any{"summary": fmt.Sprintf("%d %s.", len(out), map[bool]string{true: "item", false: "items"}[len(out) == 1]), "approvals": out,
		"filters": map[string]any{"statuses": statusOpts, "sources": sources, "people": peopleOpts}}), nil
}

// approvalsBadge is how many wait on the caller, beside Approvals in the
// sidebar.
func approvalsBadge(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if req.Actor == nil {
		return plugin.JSON(200, map[string]any{"count": 0}), nil
	}
	rows, err := api.Query(ctx, `SELECT `+approvalColumns+` FROM ai_approvals WHERE status = 'waiting' ORDER BY id DESC LIMIT 500`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		a, err := scanApproval(rows)
		if err != nil {
			return nil, err
		}
		if a.mayDecide(req.Actor) {
			n++
		}
	}
	return plugin.JSON(200, map[string]any{"count": n}), rows.Err()
}

func decideApproval(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if req.Actor == nil {
		return refuse(http.StatusUnauthorized, "Sign in first."), nil
	}
	a, err := loadApproval(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if a == nil || !a.visibleTo(req.Actor) {
		return missing("item"), nil
	}
	if a.Status != "waiting" {
		return refuse(http.StatusConflict, "That was already decided, or withdrawn."), nil
	}
	if !a.mayDecide(req.Actor) {
		return refuse(http.StatusForbidden, "It waits on someone else to decide."), nil
	}
	p := params(req)
	approve := str(p["decision"]) == "approve"
	if !approve && str(p["decision"]) != "decline" {
		return plugin.Invalid(map[string][]string{"decision": {"is approve or decline"}}), nil
	}
	note := truncate(strings.TrimSpace(str(p["note"])), 2000)
	status := map[bool]string{true: "approved", false: "declined"}[approve]
	outcome, outcomeText := "done", ""
	switch {
	case a.ChatID != 0:
		// Ask's or an agent's: decided as the panel does, and carried on.
		c, err := loadChat(ctx, api, a.ChatID)
		if err != nil {
			return nil, err
		}
		run := func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
			return api.RunTool(ctx, req.Actor, name, args)
		}
		again := func(ctx context.Context, api *plugin.API) { reply(ctx, api, c.ID, req.Actor, nil) }
		if a.RunID != 0 {
			id := a.RunID
			again = func(ctx context.Context, api *plugin.API) { execute(ctx, api, id) }
		}
		decision := map[string]any{"tool_call_id": a.ToolCallID, "decision": map[bool]string{true: "approve", false: "decline"}[approve]}
		if res, err := decide(ctx, api, c, decision, run, again); res != nil || err != nil {
			return res, err
		}
		var result sql.NullString
		var isError bool
		_ = api.QueryRow(ctx, `SELECT t.result, t.is_error FROM ai_tool_calls t JOIN ai_messages m ON m.id = t.ai_message_id WHERE m.ai_chat_id = ? AND t.tool_call_id = ?`, a.ChatID, a.ToolCallID).Scan(&result, &isError)
		if approve && isError {
			outcome, outcomeText = "failed", resultSummary(result.String)
		} else if approve {
			outcomeText = resultSummary(result.String)
		}
	default:
		spec := a.Decline
		if approve {
			spec = a.Approve
		}
		if spec != nil {
			out, err := api.RunTool(ctx, req.Actor, spec.Tool, spec.Args)
			switch {
			case err != nil:
				outcome, outcomeText = "failed", err.Error()
			case str(out["status"]) == "error":
				outcome, outcomeText = "failed", str(out["summary"])
				if outcomeText == "" {
					outcomeText = resultSummary(trim(out))
				}
			default:
				outcomeText = str(out["summary"])
			}
		}
	}
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `UPDATE ai_approvals SET status = ?, outcome = ?, outcome_text = ?, note = ?, decided_by = ?, decided_at = ?, updated_at = ? WHERE id = ?`,
		status, outcome, nullStr(truncate(outcomeText, 1000)), nullStr(note), req.Actor.ID, now, now, a.ID); err != nil {
		return nil, err
	}
	_ = api.Publish(ctx, "approval", a.approverIDs(ctx, api), map[string]any{"id": a.ID, "status": status})
	a, err = loadApproval(ctx, api, a.ID)
	if err != nil {
		return nil, err
	}
	msg := map[bool]string{true: "Approved.", false: "Declined."}[approve]
	if outcome == "failed" {
		msg = "Approved, but it failed: " + outcomeText
		if !approve {
			msg = "Declined, but telling its source failed: " + outcomeText
		}
	}
	return plugin.JSON(200, map[string]any{"message": msg, "summary": msg, "approval": approvalJSON(a, req.Actor, peopleNames(ctx, api))}), nil
}
