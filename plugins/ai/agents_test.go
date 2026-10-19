package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// The test env as the core's mentions: replies kept by id, a Replace
// changing one in place.
var replies = struct {
	byID  map[string]string
	order []string
}{byID: map[string]string{}}

func (e *env) RecordMention(context.Context, string, string, plugin.Actor, plugin.Mention) (plugin.MentionEvent, *plugin.Caller, error) {
	return plugin.MentionEvent{}, nil, nil
}

func (e *env) ReplyToMention(_ context.Context, _ string, r plugin.Reply) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if r.Replace != "" {
		if _, ok := replies.byID[r.Replace]; ok {
			replies.byID[r.Replace] = r.Body
			return r.Replace, nil
		}
	}
	id := fmt.Sprintf("r%d", len(replies.order)+1)
	replies.byID[id] = r.Body
	replies.order = append(replies.order, id)
	return id, nil
}

func newAgent(t *testing.T, h *plugin.Host, fields map[string]any) int64 {
	t.Helper()
	body, _ := json.Marshal(map[string]any{"agent": fields})
	var out struct {
		Agent struct{ ID int64 }
	}
	raw := serve(t, h, "POST", "createAgent", nil, string(body), nil)
	if err := json.Unmarshal([]byte(raw), &out); err != nil || out.Agent.ID == 0 {
		t.Fatalf("agent: %s", raw)
	}
	return out.Agent.ID
}

func runStatus(t *testing.T, h *plugin.Host, id int64) map[string]any {
	t.Helper()
	var out map[string]any
	raw := serve(t, h, "GET", "showRun", map[string]string{"id": fmt.Sprint(id)}, "", nil)
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	return out
}

// A mention starts a run that says "On it…", reads as the person who
// asked, proposes a change that waits in their Approvals, runs it once they
// approve, and replaces "On it…" with its answer.
func TestAgentRunMentionApproveReply(t *testing.T) {
	p := &provider{answers: []string{
		anthropicAnswer("tool_use", `{"type":"tool_use","id":"tu_1","name":"read_record","input":{"record":"Client:7"}}`),
		anthropicAnswer("tool_use", `{"type":"text","text":"Adding a note with what I found."}`, `{"type":"tool_use","id":"tu_2","name":"run_tool","input":{"name":"add_note","arguments":{"record":"Client:7","note":{"body":"Checked"}}}}`),
		anthropicAnswer("end_turn", `{"type":"text","text":"Done: **Client:7** is in good shape."}`),
	}}
	h, e := setup(t, "anthropic", p, nil)
	replies.byID, replies.order = map[string]string{}, nil
	id := newAgent(t, h, map[string]any{"name": "Researcher", "handle": "researcher", "tools": "propose", "instructions": "Be brief."})
	// It's offered to @mention.
	if got := h.Mentionables(t.Context()); len(got) != 1 || got[0].ID != fmt.Sprintf("ai:%d", id) || got[0].Handle != "researcher" {
		t.Fatalf("mentionables %+v", got)
	}
	// A person's name can't be a handle.
	if raw := serve(t, h, "POST", "createAgent", nil, `{"agent":{"name":"Ted bot","handle":"ted","tools":"read"}}`, nil); !strings.Contains(raw, "too like Ted") {
		t.Fatalf("handle clash: %s", raw)
	}
	h.DeliverMention(t.Context(), Key, plugin.MentionEvent{ID: 41, Mentionable: fmt.Sprint(id), Source: "note", Record: "Client:7", Subject: "Client 7",
		Author: plugin.Person{ID: 1, Name: "Ted"}, Text: "@Researcher check this client", Path: "/clients/7"}, ted, "ted")
	var runID int64
	raw := serve(t, h, "GET", "listRuns", nil, "", nil)
	var list struct{ Runs []struct{ ID int64 } }
	_ = json.Unmarshal([]byte(raw), &list)
	if len(list.Runs) != 1 {
		t.Fatalf("runs: %s", raw)
	}
	runID = list.Runs[0].ID
	if s := runStatus(t, h, runID); s["run"].(map[string]any)["status"] != "waiting" {
		t.Fatalf("not waiting: %v", s)
	}
	if strings.Join(e.ran, ",") != "show_client via AI" {
		t.Fatalf("ran before approval: %v", e.ran)
	}
	if len(replies.order) != 1 || !strings.Contains(replies.byID[replies.order[0]], "review the change") {
		t.Fatalf("replies: %v", replies.byID)
	}
	// The change is in Approvals, for Ted.
	var inbox struct {
		Approvals []struct {
			ID        int64
			Title     string
			Source    string
			CanDecide bool `json:"can_decide"`
		}
	}
	raw = serve(t, h, "GET", "listApprovals", nil, "", nil)
	if err := json.Unmarshal([]byte(raw), &inbox); err != nil || len(inbox.Approvals) != 1 || inbox.Approvals[0].Source != "agent" || !inbox.Approvals[0].CanDecide ||
		!strings.Contains(inbox.Approvals[0].Title, "@researcher wants to add a note") {
		t.Fatalf("approvals: %s", raw)
	}
	if raw := serve(t, h, "GET", "approvalsBadge", nil, "", nil); !strings.Contains(raw, `"count":1`) {
		t.Fatalf("badge: %s", raw)
	}
	raw = serve(t, h, "POST", "decideApproval", map[string]string{"id": fmt.Sprint(inbox.Approvals[0].ID)}, `{"decision":"approve"}`, nil)
	if !strings.Contains(raw, `"status":"approved"`) || !strings.Contains(raw, `"outcome":"done"`) {
		t.Fatalf("decision: %s", raw)
	}
	if strings.Join(e.ran, ",") != "show_client via AI,add_note via AI" {
		t.Fatalf("ran %v", e.ran)
	}
	s := runStatus(t, h, runID)
	if s["run"].(map[string]any)["status"] != "done" {
		t.Fatalf("not done: %v", s)
	}
	final := replies.byID[replies.order[0]]
	if len(replies.order) != 1 || !strings.Contains(final, "<strong>") || !strings.Contains(final, `href="/clients/7"`) {
		t.Fatalf("the answer replaced On it…: %v", replies.byID)
	}
	// What it was told: who asked and where, its instructions.
	sys := p.bodies[0]["system"].([]any)[0].(map[string]any)["text"].(string)
	first := fmt.Sprint(p.bodies[0]["messages"])
	if !strings.Contains(sys, "@researcher") || !strings.Contains(sys, "Be brief.") || !strings.Contains(first, "Ted (owner) mentioned you") {
		t.Fatalf("system %q / first %s", sys, first)
	}
}

// A paused agent answers a mention politely and does nothing; a read-only
// one can't find a change to propose.
func TestAgentPausedAndReadOnly(t *testing.T) {
	p := &provider{answers: []string{
		anthropicAnswer("tool_use", `{"type":"tool_use","id":"tu_1","name":"run_tool","input":{"name":"add_note","arguments":{}}}`),
		anthropicAnswer("end_turn", `{"type":"text","text":"I can only read, so I can't add that note."}`),
	}}
	h, e := setup(t, "anthropic", p, nil)
	paused := newAgent(t, h, map[string]any{"name": "Sleepy", "handle": "sleepy", "tools": "read", "paused": true})
	before := len(replies.order)
	h.DeliverMention(t.Context(), Key, plugin.MentionEvent{ID: 50, Mentionable: fmt.Sprint(paused), Source: "note", Record: "Client:7", Author: plugin.Person{ID: 1, Name: "Ted"}, Text: "hi"}, ted, "ted")
	if len(replies.order) != before+1 || !strings.Contains(replies.byID[replies.order[before]], "paused") || len(p.bodies) != 0 {
		t.Fatalf("paused: %v, %d calls", replies.byID, len(p.bodies))
	}
	reader := newAgent(t, h, map[string]any{"name": "Reader", "handle": "reader", "tools": "read", "max_steps": 4})
	h.DeliverMention(t.Context(), Key, plugin.MentionEvent{ID: 51, Mentionable: fmt.Sprint(reader), Source: "note", Record: "Client:7", Author: plugin.Person{ID: 1, Name: "Ted"}, Text: "add a note"}, ted, "ted")
	for _, r := range e.ran {
		if strings.HasPrefix(r, "add_note") {
			t.Fatalf("a read-only agent changed something: %v", e.ran)
		}
	}
	last := replies.byID[replies.order[len(replies.order)-1]]
	if !strings.Contains(last, "only read") {
		t.Fatalf("reply: %q", last)
	}
	// Its catalogue had no change in it.
	if strings.Contains(fmt.Sprint(p.bodies[1]["messages"]), `"changes_something":true`) {
		t.Fatal("a change was offered")
	}
}

// Another plugin asks a person to approve through the event contract;
// approving runs its tool as the person deciding, and withdrawing takes the
// item away.
func TestExternalApprovals(t *testing.T) {
	h, e := setup(t, "anthropic", &provider{}, nil)
	ev := `{"id":"pr-9","kind":"pull_request","title":"Merge the fix for Todo:18","summary":"A runner finished.","record":"Todo:18","link":"https://example.com/pr/9",
		"details_markdown":"` + "```diff\\n-a\\n+b\\n```" + `","requested_by":1,"approvers":[1],"approve":{"tool":"add_note","args":{"record":"Todo:18"}},"decline":{"tool":"search","args":{"q":"x"}}}`
	h.Emit(t.Context(), "runners.approval.requested", json.RawMessage(ev))
	var inbox struct {
		Approvals []struct {
			ID     int64
			Source string
			Link   string
		}
	}
	raw := serve(t, h, "GET", "listApprovals", nil, "", nil)
	if err := json.Unmarshal([]byte(raw), &inbox); err != nil || len(inbox.Approvals) != 1 || inbox.Approvals[0].Source != "runners" || inbox.Approvals[0].Link != "https://example.com/pr/9" {
		t.Fatalf("inbox: %s", raw)
	}
	raw = serve(t, h, "POST", "decideApproval", map[string]string{"id": fmt.Sprint(inbox.Approvals[0].ID)}, `{"decision":"approve","note":"ship it"}`, nil)
	if !strings.Contains(raw, `"outcome":"done"`) || !strings.Contains(strings.Join(e.ran, ","), "add_note via AI") {
		t.Fatalf("approve: %s %v", raw, e.ran)
	}
	h.Emit(t.Context(), "runners.approval.requested", json.RawMessage(`{"id":"pr-10","title":"Another","approvers":"owner"}`))
	h.Emit(t.Context(), "runners.approval.withdrawn", json.RawMessage(`{"id":"pr-10"}`))
	raw = serve(t, h, "GET", "listApprovals", nil, "", map[string][]string{"status": {"withdrawn"}})
	if !strings.Contains(raw, `"source_id":"pr-10"`) {
		t.Fatalf("withdrawn: %s", raw)
	}
}
