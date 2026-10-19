package ai

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// env is Runwell as the plugin sees it: two clients (8 kept out of AI in
// some tests), one person, a search that finds both, and a note to add.
type env struct {
	mu  sync.Mutex
	ran []string
}

func (e *env) Setting(context.Context, string) (map[string]any, error)      { return map[string]any{}, nil }
func (e *env) ReplaceSetting(context.Context, string, map[string]any) error { return nil }
func (e *env) Site(context.Context) (plugin.Site, error) {
	return plugin.Site{Name: "Runwell", Terms: map[string]map[string]string{"client": {"one": "Client", "other": "Clients"}}}, nil
}
func (e *env) SendMail(context.Context, mail.Message) error { return nil }
func (e *env) Records(_ context.Context, q plugin.Query) ([]map[string]any, error) {
	client := func(id int64) map[string]any {
		return map[string]any{"type": "Client", "id": id, "record": fmt.Sprintf("Client:%d", id), "display_name": fmt.Sprintf("Client %d", id)}
	}
	switch q.Type {
	case "Client":
		if q.ID == 7 || q.ID == 8 {
			return []map[string]any{client(q.ID)}, nil
		}
		var out []map[string]any
		for _, id := range q.IDs {
			out = append(out, client(id))
		}
		return out, nil
	case "User":
		return []map[string]any{{"type": "User", "id": int64(1), "name": "Ted", "role": "owner"}}, nil
	}
	return nil, nil
}
func (e *env) Write(_ context.Context, _ plugin.Actor, _ string, w any) (map[string]any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.ran = append(e.ran, fmt.Sprintf("write %T", w))
	return map[string]any{}, nil
}
func (e *env) Audit(context.Context, plugin.Actor, string, int64, string, map[string]any) {}
func (e *env) AttachFile(context.Context, string, int64, string, string, string, io.Reader) (*plugin.File, error) {
	return nil, errors.New("no")
}
func (e *env) Attachments(context.Context, string, int64, string) ([]plugin.File, error) {
	return nil, nil
}
func (e *env) OpenFile(context.Context, string, int64, int64) (io.ReadCloser, *plugin.File, error) {
	return nil, nil, errors.New("no")
}
func (e *env) DetachFile(context.Context, string, int64, int64) error { return nil }
func (e *env) Tools(context.Context, plugin.Actor) ([]plugin.ToolInfo, error) {
	return []plugin.ToolInfo{
		{Name: "search", Title: "Search everything", ReadOnly: true, InputSchema: map[string]any{"type": "object"}},
		{Name: "show_client", Title: "Show a client", ReadOnly: true},
		{Name: "add_note", Title: "Add a note to a record", InputSchema: map[string]any{"type": "object"}},
	}, nil
}
func (e *env) RunTool(_ context.Context, _ plugin.Actor, via, name string, args map[string]any) (map[string]any, error) {
	e.mu.Lock()
	e.ran = append(e.ran, name+" via "+via)
	e.mu.Unlock()
	switch name {
	case "search":
		return map[string]any{"status": "ok", "summary": "2 results", "data": map[string]any{"results": []any{
			map[string]any{"type": "Client", "id": float64(7), "record": "Client:7", "display_name": "Harbor"},
			map[string]any{"type": "Client", "id": float64(8), "record": "Client:8", "display_name": "Secret Co"},
		}}}, nil
	case "show_client":
		id := intOf(args["id"])
		return map[string]any{"status": "ok", "data": map[string]any{"client": map[string]any{"type": "Client", "id": float64(id), "record": fmt.Sprintf("Client:%d", id)}}}, nil
	}
	return map[string]any{"status": "ok", "summary": "Note added."}, nil
}
func (e *env) Seal(plain string) string { return "sealed:" + plain }
func (e *env) Unseal(sealed string) (string, bool) {
	return strings.TrimPrefix(sealed, "sealed:"), strings.HasPrefix(sealed, "sealed:")
}

// provider is a fake model API: each call gets the next scripted answer,
// and every request body is kept.
type provider struct {
	mu      sync.Mutex
	bodies  []map[string]any
	headers []http.Header
	answers []string
	status  []int
}

func (p *provider) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if r.Method == http.MethodGet { // the models list, fetched on saving settings
		io.WriteString(w, `{"data":[{"id":"claude-opus-5-5"}]}`)
		return
	}
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	p.bodies = append(p.bodies, body)
	p.headers = append(p.headers, r.Header.Clone())
	status := 200
	if len(p.status) > 0 {
		status, p.status = p.status[0], p.status[1:]
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if status != 200 {
		io.WriteString(w, `{"type":"error","error":{"type":"x","message":"nope"}}`)
		return
	}
	if len(p.answers) == 0 {
		io.WriteString(w, `{"error":"no more answers"}`)
		return
	}
	a := p.answers[0]
	p.answers = p.answers[1:]
	io.WriteString(w, a)
}

const usageJSON = `"usage":{"input_tokens":1000000,"output_tokens":0,"cache_read_input_tokens":0,"cache_creation_input_tokens":0}`

func anthropicAnswer(stop string, content ...string) string {
	return `{"type":"message","role":"assistant","content":[` + strings.Join(content, ",") + `],"stop_reason":"` + stop + `",` + usageJSON + `}`
}

// setup is a host with the AI plugin on (its grants, this server's network
// included), set up to reach the fake provider.
func setup(t *testing.T, providerKey string, p *provider, more map[string]any) (*plugin.Host, *env) {
	t.Helper()
	srv := httptest.NewServer(p)
	t.Cleanup(srv.Close)
	db, err := store.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{}
	spawn := func(ctx context.Context, _ string, fn func(context.Context)) bool { fn(ctx); return true }
	h := plugin.New(e, db, slog.New(slog.NewTextHandler(io.Discard, nil)), spawn, Module())
	if err := h.LoadAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Install(t.Context(), nil, Key); err != nil {
		t.Fatal(err)
	}
	grants := Module().Manifest.Permissions
	on := true
	if _, err := h.Configure(t.Context(), nil, Key, &grants, &on); err != nil {
		t.Fatal(err)
	}
	s := map[string]any{"enabled": true, "provider": providerKey, "api_key": "sk-test", "api_base": srv.URL + "/v1"}
	maps.Copy(s, more)
	body, _ := json.Marshal(map[string]any{"ai_setting": s})
	if res := serve(t, h, "PATCH", "updateSettings", nil, string(body), nil); !strings.Contains(res, "AI is on") {
		t.Fatalf("settings: %s", res)
	}
	return h, e
}

var ted = &plugin.Caller{ID: 1, Name: "Ted", Role: "owner", Capabilities: []string{"manage_settings"}}

func serve(t *testing.T, h *plugin.Host, method, export string, params map[string]string, body string, query map[string][]string) string {
	t.Helper()
	req := &plugin.Request{Method: method, Params: params, Query: query, Body: []byte(body), Actor: ted}
	if req.Query == nil {
		req.Query = map[string][]string{}
	}
	res, err := h.Serve(t.Context(), Key, export, req, "ted")
	if err != nil {
		t.Fatal(err)
	}
	return string(res.Body)
}

func chatID(t *testing.T, h *plugin.Host, record string) string {
	t.Helper()
	var c struct {
		Chat struct{ ID int64 } `json:"chat"`
	}
	if err := json.Unmarshal([]byte(serve(t, h, "GET", "currentChat", nil, "", map[string][]string{"record": {record}})), &c); err != nil {
		t.Fatal(err)
	}
	return strconv.FormatInt(c.Chat.ID, 10)
}

type panel struct {
	Messages []struct {
		Role      string
		Content   string
		ToolCalls []struct {
			ToolCallID string `json:"tool_call_id"`
			State      string
			Proposal   bool
		} `json:"tool_calls"`
	}
	Waiting bool
}

func readPanel(t *testing.T, raw string) panel {
	t.Helper()
	var p panel
	if err := json.Unmarshal([]byte(raw), &p); err != nil {
		t.Fatalf("%v: %s", err, raw)
	}
	return p
}

// Ask, answered with Anthropic's tool_use loop: a read runs at once, a
// change waits for the person's approval and runs as them, and every turn
// goes back exactly as it came (thinking blocks too), the system prompt
// and last tool marked for caching, and what it cost counted.
func TestAnthropicLoopWithApproval(t *testing.T) {
	t.Parallel()
	p := &provider{answers: []string{
		anthropicAnswer("tool_use", `{"type":"thinking","thinking":"","signature":"sig-1"}`, `{"type":"tool_use","id":"tu_1","name":"search","input":{"query":"Harbor"}}`),
		anthropicAnswer("tool_use", `{"type":"thinking","thinking":"","signature":"sig-2"}`, `{"type":"text","text":"I'll add a note."}`, `{"type":"tool_use","id":"tu_2","name":"run_tool","input":{"name":"add_note","arguments":{"record":"Client:7","note":{"body":"Hi"}}}}`),
		anthropicAnswer("end_turn", `{"type":"text","text":"Done: the note is on Client:7."}`),
	}}
	h, e := setup(t, "anthropic", p, nil)
	id := chatID(t, h, "Client:7")
	got := readPanel(t, serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"Add a note"}}`, nil))
	if !got.Waiting || len(got.Messages) != 3 || got.Messages[2].ToolCalls[0].State != "pending" || !got.Messages[2].ToolCalls[0].Proposal {
		t.Fatalf("not waiting on the change: %+v", got)
	}
	if strings.Join(e.ran, ",") != "search via AI" {
		t.Fatalf("ran before approval: %v", e.ran)
	}
	// Asking again waits for the decision.
	if res := serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"more"}}`, nil); !strings.Contains(res, "Approve or decline the change above first.") {
		t.Fatalf("asked past a pending change: %s", res)
	}
	got = readPanel(t, serve(t, h, "POST", "decideChat", map[string]string{"id": id}, `{"tool_call_id":"tu_2","decision":"approve"}`, nil))
	if strings.Join(e.ran, ",") != "search via AI,add_note via AI" {
		t.Fatalf("ran %v", e.ran)
	}
	last := got.Messages[len(got.Messages)-1]
	if last.Content != "Done: the note is on Client:7." || got.Waiting {
		t.Fatalf("after approval: %+v", got)
	}
	if len(p.bodies) != 3 {
		t.Fatalf("%d calls", len(p.bodies))
	}
	b := p.bodies[2]
	if b["model"] != "claude-opus-5-5" || b["max_tokens"] != float64(16000) || b["thinking"].(map[string]any)["type"] != "adaptive" || b["tool_choice"].(map[string]any)["type"] != "auto" {
		t.Fatalf("request: %v", b)
	}
	if _, ok := b["fallbacks"]; ok || p.headers[2].Get("anthropic-beta") != "" {
		t.Fatal("fallbacks sent to an address that isn't Anthropic's")
	}
	if p.headers[2].Get("x-api-key") != "sk-test" || p.headers[2].Get("anthropic-version") != "2023-06-01" {
		t.Fatalf("headers %v", p.headers[2])
	}
	system := b["system"].([]any)[0].(map[string]any)
	tools := b["tools"].([]any)
	if system["cache_control"] == nil || tools[len(tools)-1].(map[string]any)["cache_control"] == nil {
		t.Fatal("no cache_control on the system prompt and last tool")
	}
	if strings.Contains(system["text"].(string), "Today is") {
		t.Fatal("the date is in the system prompt")
	}
	msgs := b["messages"].([]any)
	if len(msgs) != 5 {
		t.Fatalf("%d messages: %v", len(msgs), msgs)
	}
	first := msgs[1].(map[string]any)["content"].([]any)[0].(map[string]any)
	if first["type"] != "thinking" || first["signature"] != "sig-1" {
		t.Fatalf("the thinking block didn't go back: %v", first)
	}
	results := msgs[4].(map[string]any)["content"].([]any)
	if len(results) != 1 || results[0].(map[string]any)["tool_use_id"] != "tu_2" || !strings.Contains(results[0].(map[string]any)["content"].(string), "Note added") {
		t.Fatalf("tool results: %v", results)
	}
	// A million input tokens on Opus 5.5: $4, three times.
	if res := serve(t, h, "GET", "showSettings", nil, "", nil); !strings.Contains(res, `"spent":"$12.00"`) || !strings.Contains(res, `"spent_cents":1200`) {
		t.Fatalf("spend: %s", res)
	}
}

// OpenAI-compatible providers get Chat Completions with function calling:
// the call's answer goes back as a tool message.
func TestOpenAIFunctionCalling(t *testing.T) {
	t.Parallel()
	p := &provider{answers: []string{
		`{"choices":[{"message":{"role":"assistant","content":null,"tool_calls":[{"id":"call_1","type":"function","function":{"name":"search","arguments":"{\"query\":\"Harbor\"}"}}]},"finish_reason":"tool_calls"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
		`{"choices":[{"message":{"role":"assistant","content":"Harbor is Client:7."},"finish_reason":"stop"}],"usage":{"prompt_tokens":10,"completion_tokens":5}}`,
	}}
	h, _ := setup(t, "openai", p, nil)
	id := chatID(t, h, "")
	got := readPanel(t, serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"Who is Harbor?"}}`, nil))
	if last := got.Messages[len(got.Messages)-1]; last.Content != "Harbor is Client:7." {
		t.Fatalf("answer: %+v", got)
	}
	b := p.bodies[1]
	if b["model"] != "gpt-5.6" || b["tool_choice"] != "auto" {
		t.Fatalf("request %v", b)
	}
	tool := b["tools"].([]any)[0].(map[string]any)
	if tool["type"] != "function" || tool["function"].(map[string]any)["name"] != "search" {
		t.Fatalf("tools %v", tool)
	}
	msgs := b["messages"].([]any)
	if msgs[0].(map[string]any)["role"] != "system" || msgs[2].(map[string]any)["tool_calls"] == nil {
		t.Fatalf("messages %v", msgs)
	}
	toolMsg := msgs[3].(map[string]any)
	if toolMsg["role"] != "tool" || toolMsg["tool_call_id"] != "call_1" {
		t.Fatalf("tool message %v", toolMsg)
	}
	if p.headers[0].Get("Authorization") != "Bearer sk-test" {
		t.Fatal("no bearer key")
	}
}

// Over the month's budget, nothing more is asked.
func TestBudgetRefusal(t *testing.T) {
	t.Parallel()
	p := &provider{}
	h, _ := setup(t, "anthropic", p, map[string]any{"monthly_budget_cents": "0"})
	id := chatID(t, h, "")
	if res := serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"hi"}}`, nil); !strings.Contains(res, "budget is spent") {
		t.Fatalf("over budget: %s", res)
	}
	if res := serve(t, h, "POST", "newSuggestion", nil, `{"kind":"client_week","record":"Client:7"}`, nil); !strings.Contains(res, "budget is spent") {
		t.Fatalf("suggestion over budget: %s", res)
	}
	if len(p.bodies) != 0 {
		t.Fatal("the provider was called")
	}
}

// A client kept out of AI: no chat about it, its records refused, and
// left out of what a search shows the model.
func TestExclusions(t *testing.T) {
	t.Parallel()
	p := &provider{answers: []string{
		anthropicAnswer("tool_use", `{"type":"tool_use","id":"tu_1","name":"search","input":{"query":"co"}}`, `{"type":"tool_use","id":"tu_2","name":"read_record","input":{"record":"Client:8"}}`),
		anthropicAnswer("end_turn", `{"type":"text","text":"ok"}`),
	}}
	h, _ := setup(t, "anthropic", p, nil)
	if res := serve(t, h, "POST", "exclude", map[string]string{"client_id": "8"}, "", nil); !strings.Contains(res, "kept out of AI") {
		t.Fatalf("exclude: %s", res)
	}
	id := chatID(t, h, "Client:8")
	if res := serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"hi"}}`, nil); !strings.Contains(res, "AI is off for this client") {
		t.Fatalf("asked about a kept-out client: %s", res)
	}
	id = chatID(t, h, "")
	serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"find co"}}`, nil)
	results := p.bodies[1]["messages"].([]any)[2].(map[string]any)["content"].([]any)
	if len(results) != 2 {
		t.Fatalf("results %v", results)
	}
	search := results[0].(map[string]any)["content"].(string)
	read := results[1].(map[string]any)
	if strings.Contains(search, "Secret Co") || !strings.Contains(search, "Harbor") {
		t.Fatalf("search showed a kept-out client: %s", search)
	}
	if read["is_error"] != true || !strings.Contains(read["content"].(string), "keeps it out of AI") {
		t.Fatalf("read a kept-out client: %v", read)
	}
}

// Errors say what went wrong and what to do; a rate limit is tried once
// more.
func TestProviderErrors(t *testing.T) {
	t.Parallel()
	p := &provider{status: []int{401}}
	h, _ := setup(t, "anthropic", p, nil)
	id := chatID(t, h, "")
	got := readPanel(t, serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"hi"}}`, nil))
	if last := got.Messages[len(got.Messages)-1]; !strings.Contains(last.Content, "refused the API key (401)") || !strings.Contains(last.Content, "Settings › AI") {
		t.Fatalf("401: %+v", got)
	}
	p.status = []int{429}
	p.answers = []string{anthropicAnswer("end_turn", `{"type":"text","text":"after a retry"}`)}
	got = readPanel(t, serve(t, h, "POST", "askChat", map[string]string{"id": id}, `{"message":{"content":"again"}}`, nil))
	if last := got.Messages[len(got.Messages)-1]; last.Content != "after a retry" {
		t.Fatalf("429 retry: %+v", got)
	}
}
