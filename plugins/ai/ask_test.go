package ai

import (
	"encoding/json"
	"strings"
	"testing"
)

const anthropicSSE = `event: message_start
data: {"type":"message_start","message":{"usage":{"input_tokens":10,"cache_read_input_tokens":2}}}

event: content_block_start
data: {"type":"content_block_start","index":0,"content_block":{"type":"thinking","thinking":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"thinking_delta","thinking":"Look it up."}}

event: content_block_delta
data: {"type":"content_block_delta","index":0,"delta":{"type":"signature_delta","signature":"sig-9"}}

event: content_block_start
data: {"type":"content_block_start","index":1,"content_block":{"type":"text","text":""}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"Searching "}}

event: content_block_delta
data: {"type":"content_block_delta","index":1,"delta":{"type":"text_delta","text":"now."}}

event: content_block_start
data: {"type":"content_block_start","index":2,"content_block":{"type":"tool_use","id":"tu_1","name":"search","input":{}}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"{\"query\":"}}

event: content_block_delta
data: {"type":"content_block_delta","index":2,"delta":{"type":"input_json_delta","partial_json":"\"Harbor\"}"}}

event: message_delta
data: {"type":"message_delta","delta":{"stop_reason":"tool_use"},"usage":{"output_tokens":7}}

event: message_stop
data: {"type":"message_stop"}

`

// A streamed Messages answer comes back together as the whole one would:
// its thinking (signed), its words as they came, and its tool call.
func TestAnthropicStream(t *testing.T) {
	var said, thought string
	raw, err := readAnthropicStream(strings.NewReader(anthropicSSE), func(d delta) { said += d.Text; thought += d.Thinking })
	if err != nil {
		t.Fatal(err)
	}
	ans, err := parseAnthropic(raw)
	if err != nil {
		t.Fatal(err)
	}
	if said != "Searching now." || thought != "Look it up." {
		t.Fatalf("deltas %q %q", said, thought)
	}
	if ans.Text != "Searching now." || ans.Thinking != "Look it up." || ans.Stop != "tool_use" || len(ans.Calls) != 1 || string(ans.Calls[0].Input) != `{"query":"Harbor"}` {
		t.Fatalf("answer %+v", ans)
	}
	if ans.Usage.Input != 10 || ans.Usage.Output != 7 || ans.Usage.CacheRead != 2 {
		t.Fatalf("usage %+v", ans.Usage)
	}
	var kept rawTurn
	_ = json.Unmarshal(ans.Raw, &kept)
	if !strings.Contains(string(kept.Content), `"signature":"sig-9"`) {
		t.Fatalf("the signature didn't stay: %s", kept.Content)
	}
}

// A streamed chat completion: words, and a call's arguments in pieces.
func TestOpenAIStream(t *testing.T) {
	sse := `data: {"choices":[{"delta":{"role":"assistant","content":"Hi "}}]}

data: {"choices":[{"delta":{"content":"there"}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"call_1","function":{"name":"search","arguments":"{\"query\""}}]}}]}

data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":\"x\"}"}}]},"finish_reason":"tool_calls"}]}

data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":3}}

data: [DONE]

`
	var said string
	raw, err := readOpenAIStream(strings.NewReader(sse), func(d delta) { said += d.Text })
	if err != nil {
		t.Fatal(err)
	}
	ans, err := parseOpenAI(raw)
	if err != nil {
		t.Fatal(err)
	}
	if said != "Hi there" || ans.Text != "Hi there" || ans.Stop != "tool_use" || len(ans.Calls) != 1 || string(ans.Calls[0].Input) != `{"query":"x"}` || ans.Usage.Output != 3 {
		t.Fatalf("answer %+v (%q)", ans, said)
	}
}

// A stream that breaks with an error says so in words.
func TestStreamError(t *testing.T) {
	_, err := readAnthropicStream(strings.NewReader("event: error\ndata: {\"type\":\"error\",\"error\":{\"type\":\"overloaded_error\",\"message\":\"Overloaded\"}}\n\n"), nil)
	if err == nil || !strings.Contains(err.Error(), "Overloaded") {
		t.Fatalf("error %v", err)
	}
}

// The history, rename, model pick, thumbs and delete.
func TestChatHistoryAndFeedback(t *testing.T) {
	p := &provider{answers: []string{anthropicAnswer("end_turn", `{"type":"text","text":"Hello."}`), anthropicAnswer("end_turn", `{"type":"text","text":"Hello again."}`)}}
	h, _ := setup(t, "anthropic", p, nil)
	id := chatID(t, h, "")
	got := readPanel(t, serve(t, h, "POST", "sendChat", map[string]string{"id": id}, `{"message":{"content":"Hi"}}`, nil))
	if got.Messages[len(got.Messages)-1].Content != "Hello." {
		t.Fatalf("answer %+v", got)
	}
	if res := serve(t, h, "GET", "chatHistory", nil, "", nil); !strings.Contains(res, `"title":"Hi"`) {
		t.Fatalf("history %s", res)
	}
	if res := serve(t, h, "PATCH", "updateChat", map[string]string{"id": id}, `{"chat":{"title":"Greetings","model":"fast"}}`, nil); !strings.Contains(res, `"title":"Greetings"`) || !strings.Contains(res, `"model":"fast"`) {
		t.Fatalf("rename %s", res)
	}
	var panel struct {
		LastAnswerID int64 `json:"last_answer_id"`
	}
	_ = json.Unmarshal([]byte(serve(t, h, "GET", "showChat", map[string]string{"id": id}, "", nil)), &panel)
	mid := itoa(panel.LastAnswerID)
	if res := serve(t, h, "POST", "rateMessage", map[string]string{"id": id, "message_id": mid}, `{"rating":"up"}`, nil); !strings.Contains(res, `"rating":"up"`) {
		t.Fatalf("rate %s", res)
	}
	if res := serve(t, h, "GET", "showChat", map[string]string{"id": id}, "", nil); !strings.Contains(res, `"feedback":"up"`) {
		t.Fatalf("feedback not shown: %s", res)
	}
	// Regenerating answers the same question again, with the fast model.
	got = readPanel(t, serve(t, h, "POST", "regenerateChat", map[string]string{"id": id}, "", nil))
	if got.Messages[len(got.Messages)-1].Content != "Hello again." || len(got.Messages) != 2 {
		t.Fatalf("regenerated %+v", got)
	}
	if p.bodies[1]["model"] != "claude-haiku-5-5" {
		t.Fatalf("the chat's model wasn't used: %v", p.bodies[1]["model"])
	}
	if res := serve(t, h, "DELETE", "deleteChat", map[string]string{"id": id}, "", nil); !strings.Contains(res, "Chat deleted") {
		t.Fatalf("delete %s", res)
	}
	if res := serve(t, h, "GET", "chatHistory", nil, "", nil); strings.Contains(res, "Greetings") {
		t.Fatalf("still in history %s", res)
	}
}

// Pictures go to a provider that reads them, as image blocks.
func TestImagesSent(t *testing.T) {
	p := &provider{answers: []string{anthropicAnswer("end_turn", `{"type":"text","text":"A red dot."}`)}}
	h, _ := setup(t, "anthropic", p, nil)
	id := chatID(t, h, "")
	png := "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="
	serve(t, h, "POST", "sendChat", map[string]string{"id": id}, `{"message":{"content":"What is this?","images":[{"media_type":"image/png","data":"`+png+`"}]}}`, nil)
	msgs := p.bodies[0]["messages"].([]any)
	blocks := msgs[0].(map[string]any)["content"].([]any)
	if blocks[0].(map[string]any)["type"] != "image" {
		t.Fatalf("no image block: %v", blocks)
	}
	if res := serve(t, h, "POST", "sendChat", map[string]string{"id": id}, `{"message":{"content":"x","images":[{"media_type":"text/html","data":"PGI+"}]}}`, nil); !strings.Contains(res, "PNG, JPEG") {
		t.Fatalf("a bad picture: %s", res)
	}
}

func itoa(n int64) string {
	b, _ := json.Marshal(n)
	return string(b)
}
