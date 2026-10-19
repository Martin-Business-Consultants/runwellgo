package ai

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"
)

// The provider layer: one small client per call, raw HTTP, two wire
// protocols. Anthropic's Messages API for Claude (and for Claude and Qwen
// through OpenCode Zen, as Zen serves them), and OpenAI's Chat Completions
// for everyone else: OpenAI, Gemini (through its OpenAI-compatible
// address), OpenRouter, OpenCode Zen's other models and Ollama. A call is
// one turn: the system prompt, the tools, the conversation so far, and,
// for a suggestion, the JSON schema the answer must take. The agentic loop
// (chat.go) runs the tools between turns.

// providerInfo is one provider the owner can choose (AiSetting::PROVIDERS):
// its name, its recommended models, whether it needs a key, and where it
// is.
type providerInfo struct {
	Key      string `json:"key"`
	Name     string `json:"name"`
	Model    string `json:"model"`
	Fast     string `json:"fast"`
	NeedsKey bool   `json:"needs_key"`
	Base     string `json:"base"`
}

var providers = []providerInfo{
	{"anthropic", "Anthropic (Claude)", "claude-opus-5-5", "claude-haiku-5-5", true, "https://api.anthropic.com/v1"},
	{"openai", "OpenAI", "gpt-5.6", "gpt-5.4-mini", true, "https://api.openai.com/v1"},
	{"gemini", "Google Gemini", "gemini-pro-latest", "gemini-flash-latest", true, "https://generativelanguage.googleapis.com/v1beta/openai"},
	{"openrouter", "OpenRouter (any model)", "anthropic/claude-sonnet-5", "anthropic/claude-haiku-4-5", true, "https://openrouter.ai/api/v1"},
	{"opencode", "OpenCode Zen (Claude, GPT, Gemini and open models on one key)", "claude-sonnet-5", "claude-haiku-4-5", true, "https://opencode.ai/zen/v1"},
	{"ollama", "Ollama (your own server, nothing leaves it)", "llama4", "llama4", false, "http://localhost:11434/v1"},
}

func providerFor(key string) *providerInfo {
	for i := range providers {
		if providers[i].Key == key {
			return &providers[i]
		}
	}
	return nil
}

// shortName is a provider's name without what's in brackets.
func (p *providerInfo) shortName() string {
	name, _, _ := strings.Cut(p.Name, " (")
	return name
}

// hosts are the cloud providers' own: reached through the declared hosts.
var hosts = []string{"api.anthropic.com", "api.openai.com", "generativelanguage.googleapis.com", "openrouter.ai", "opencode.ai"}

// ---- the neutral conversation ----------------------------------------------------------

// turn is one message of the conversation, as the loop keeps it.
type turn struct {
	Role string // user or assistant
	// Text is the person's words (with the page's context) or the reply's.
	Text string
	// Raw is an assistant turn exactly as its provider answered it
	// ({"provider", "content"}), sent back unchanged to the same protocol.
	Raw json.RawMessage
	// Calls are an assistant turn's tool calls; Results a user turn's
	// answers to the previous turn's calls, every one of them.
	Calls   []toolCall
	Results []toolResult
	// Images go with a user turn's text (base64), for a provider that
	// takes them.
	Images []image
}

// image is a picture sent with a question.
type image struct {
	MediaType string `json:"media_type"`
	Data      string `json:"data"` // base64
}

type toolCall struct {
	ID    string
	Name  string
	Input json.RawMessage
}

type toolResult struct {
	ID      string
	Name    string
	Content string
	IsError bool
}

type toolDef struct {
	Name        string
	Description string
	Schema      map[string]any
}

// ask is one call: what's sent.
type ask struct {
	System    string
	Tools     []toolDef
	Turns     []turn
	Schema    map[string]any // a structured answer's JSON schema
	Name      string         // its name
	MaxTokens int
}

// answer is the provider's reply.
type answer struct {
	Text string
	// Thinking is what the model said it was thinking, where it says.
	Thinking string
	Calls    []toolCall
	// Stop is tool_use, end, max_tokens or refusal.
	Stop  string
	Raw   json.RawMessage
	Usage usage
}

type usage struct {
	Input, Output, CacheRead, CacheWrite int64
}

// llm is a model reached through a provider, with the install's key.
type llm struct {
	provider string
	protocol string // anthropic or openai
	base     string // up to /v1 (Anthropic's without it)
	key      string
	model    string
	client   *http.Client
}

// route is how a model is reached: OpenCode Zen serves Claude and most Qwen
// as Anthropic's Messages, the rest as chat completions (AiSetting#route).
func route(provider, model string) string {
	switch {
	case provider == "anthropic":
		return "anthropic"
	case provider == "opencode" && (strings.HasPrefix(model, "claude") || strings.HasPrefix(model, "qwen")) && !strings.HasPrefix(model, "qwen3.8-max"):
		return "anthropic"
	}
	return "openai"
}

// routeName is how a model is reached, in words, for errors and the test.
func routeName(provider, model string) string {
	if provider != "opencode" {
		return model
	}
	endpoint := "chat completions"
	if route(provider, model) == "anthropic" {
		endpoint = "Anthropic Messages"
	}
	return model + " through OpenCode Zen’s " + endpoint + " endpoint"
}

// ---- errors ------------------------------------------------------------------------------

// providerError is a call that failed, in words a person can act on.
type providerError struct {
	Status  int
	Message string
}

func (e *providerError) Error() string { return e.Message }

func errorFor(status int, body []byte, host, model string) error {
	detail := providerMessage(body)
	if detail != "" {
		detail = ": " + truncate(detail, 200)
	}
	var msg string
	switch {
	case status == 401 || status == 403:
		msg = fmt.Sprintf("The provider refused the API key (%d)%s. Check the key in Settings › AI.", status, detail)
	case status == 404:
		msg = fmt.Sprintf("The provider doesn’t know the model “%s” or the address (404)%s. Choose another model, or check the address, in Settings › AI.", model, detail)
	case status == 429:
		msg = fmt.Sprintf("The provider is limiting how often we ask (429)%s. Try again in a minute.", detail)
	case status == 529 || status >= 500:
		msg = fmt.Sprintf("The provider is having trouble (%d)%s. Try again shortly.", status, detail)
	default:
		msg = fmt.Sprintf("The provider refused the request (%d)%s.", status, detail)
	}
	return &providerError{Status: status, Message: msg}
}

// providerMessage is the error message an answer carries, if any.
func providerMessage(body []byte) string {
	var b struct {
		Error   any    `json:"error"`
		Message string `json:"message"`
	}
	if json.Unmarshal(body, &b) != nil {
		return strings.TrimSpace(truncate(string(body), 200))
	}
	switch e := b.Error.(type) {
	case map[string]any:
		if m, ok := e["message"].(string); ok {
			return m
		}
	case string:
		return e
	}
	return b.Message
}

// send posts body to path (GET when body is nil) with one retry on a rate
// limit, an overloaded provider or a dropped connection.
func (l *llm) send(ctx context.Context, method, path string, body any) ([]byte, error) {
	var raw []byte
	if body != nil {
		var err error
		if raw, err = json.Marshal(body); err != nil {
			return nil, err
		}
	}
	host := l.base
	if u, err := url.Parse(l.base); err == nil {
		host = u.Host
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, method, l.base+path, bytes.NewReader(raw))
		if err != nil {
			return nil, err
		}
		if body == nil {
			req.Body = http.NoBody
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")
		l.headers(req)
		res, err := l.client.Do(req)
		if err != nil {
			if attempt == 0 && ctx.Err() == nil && retryable(err) {
				wait(ctx, 2*time.Second)
				continue
			}
			return nil, &providerError{Message: fmt.Sprintf("Couldn’t reach %s: %s. Check the address in Settings › AI, and that the server is up.", host, plainNetError(err))}
		}
		out, err := io.ReadAll(res.Body)
		res.Body.Close()
		if err != nil {
			return nil, &providerError{Message: fmt.Sprintf("The answer from %s broke off: %s.", host, err)}
		}
		if res.StatusCode >= 300 {
			if attempt == 0 && (res.StatusCode == 429 || res.StatusCode >= 500) {
				d := 2 * time.Second
				if s, err := strconv.Atoi(res.Header.Get("Retry-After")); err == nil && s > 0 && s <= 20 {
					d = time.Duration(s) * time.Second
				}
				wait(ctx, d)
				continue
			}
			return nil, errorFor(res.StatusCode, out, host, l.model)
		}
		return out, nil
	}
}

func retryable(err error) bool {
	var ne net.Error
	return !strings.Contains(err.Error(), "wasn't granted") && !strings.Contains(err.Error(), "isn't a public address") && (errors.As(err, &ne) || errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "connection reset"))
}

func plainNetError(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	return err.Error()
}

func wait(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

func (l *llm) headers(req *http.Request) {
	if l.protocol == "anthropic" {
		req.Header.Set("anthropic-version", "2023-06-01")
		if l.key != "" {
			req.Header.Set("x-api-key", l.key)
		}
		if l.fallbacks() {
			req.Header.Set("anthropic-beta", "server-side-fallback-2026-07-01")
		}
		return
	}
	if l.key != "" {
		req.Header.Set("Authorization", "Bearer "+l.key)
	}
	if l.provider == "openrouter" {
		req.Header.Set("X-Title", "Runwell")
	}
}

// fallbacks is whether a refused request is re-served by the API's
// fallback model: Claude Opus 5.5 at Anthropic's own address.
func (l *llm) fallbacks() bool {
	return l.provider == "anthropic" && strings.HasPrefix(l.model, "claude-opus-5-5") && strings.HasPrefix(l.base, "https://api.anthropic.com")
}

// ---- one turn ----------------------------------------------------------------------------

func (l *llm) turn(ctx context.Context, a ask) (*answer, error) {
	if a.MaxTokens == 0 {
		a.MaxTokens = 16000
	}
	if l.protocol == "anthropic" {
		return l.anthropicTurn(ctx, a)
	}
	return l.openaiTurn(ctx, a)
}

// raw is an assistant turn as kept: which protocol, and its own content.
type rawTurn struct {
	Protocol string          `json:"protocol"`
	Content  json.RawMessage `json:"content"`
}

// anthropicTurn is POST /v1/messages: adaptive thinking (but on Haiku
// 4.5), tool_choice auto, the system prompt and the last tool marked for
// caching (both stay byte for byte the same from call to call), and
// earlier assistant turns sent back exactly as they came, thinking blocks
// and all.
func (l *llm) anthropicTurn(ctx context.Context, a ask) (*answer, error) {
	raw, err := l.send(ctx, "POST", "/messages", l.anthropicBody(a))
	if err != nil {
		return nil, err
	}
	return parseAnthropic(raw)
}

// anthropicBody is a Messages request for a.
func (l *llm) anthropicBody(a ask) map[string]any {
	body := map[string]any{
		"model":      l.model,
		"max_tokens": a.MaxTokens,
		"system":     []any{map[string]any{"type": "text", "text": a.System, "cache_control": map[string]any{"type": "ephemeral"}}},
		"messages":   anthropicMessages(a.Turns),
	}
	if !strings.Contains(l.model, "haiku-4-5") && !strings.Contains(l.model, "-3-") {
		// Summarized, so the panel can show what it's thinking.
		body["thinking"] = map[string]any{"type": "adaptive", "display": "summarized"}
	}
	if len(a.Tools) > 0 {
		tools := []any{}
		for i, t := range a.Tools {
			def := map[string]any{"name": t.Name, "description": t.Description, "input_schema": t.Schema}
			if i == len(a.Tools)-1 {
				def["cache_control"] = map[string]any{"type": "ephemeral"}
			}
			tools = append(tools, def)
		}
		body["tools"] = tools
		body["tool_choice"] = map[string]any{"type": "auto"}
	}
	if a.Schema != nil {
		body["output_config"] = map[string]any{"format": map[string]any{"type": "json_schema", "schema": a.Schema}}
	}
	if l.fallbacks() {
		body["fallbacks"] = "default"
	}
	return body
}

// parseAnthropic reads a Messages answer.
func parseAnthropic(raw []byte) (*answer, error) {
	var res struct {
		Content    json.RawMessage `json:"content"`
		StopReason string          `json:"stop_reason"`
		Usage      struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, &providerError{Message: "The provider answered with something that isn’t a message."}
	}
	var blocks []struct {
		Type     string          `json:"type"`
		Text     string          `json:"text"`
		Thinking string          `json:"thinking"`
		ID       string          `json:"id"`
		Name     string          `json:"name"`
		Input    json.RawMessage `json:"input"`
	}
	_ = json.Unmarshal(res.Content, &blocks)
	out := &answer{Usage: usage{res.Usage.Input, res.Usage.Output, res.Usage.CacheRead, res.Usage.CacheWrite}}
	var texts, thoughts []string
	for _, b := range blocks {
		switch b.Type {
		case "thinking":
			if strings.TrimSpace(b.Thinking) != "" {
				thoughts = append(thoughts, b.Thinking)
			}
		case "text":
			texts = append(texts, b.Text)
		case "tool_use":
			out.Calls = append(out.Calls, toolCall{ID: b.ID, Name: b.Name, Input: b.Input})
		}
	}
	out.Text = strings.TrimSpace(strings.Join(texts, "\n\n"))
	out.Thinking = strings.TrimSpace(strings.Join(thoughts, "\n\n"))
	switch res.StopReason {
	case "tool_use":
		out.Stop = "tool_use"
	case "max_tokens":
		out.Stop = "max_tokens"
	case "refusal":
		out.Stop = "refusal"
	default:
		out.Stop = "end"
	}
	content := res.Content
	if len(content) == 0 {
		content = json.RawMessage("[]")
	}
	out.Raw, _ = json.Marshal(rawTurn{Protocol: "anthropic", Content: content})
	return out, nil
}

// anthropicMessages is the conversation as Messages takes it: tool results
// first in a user turn, an assistant turn as it came (or rebuilt, when it
// came from another protocol), turns of the same role run together.
func anthropicMessages(turns []turn) []any {
	type message struct {
		role    string
		content []any
	}
	var out []message
	for _, t := range turns {
		var blocks []any
		if t.Role == "assistant" {
			var r rawTurn
			if json.Unmarshal(t.Raw, &r) == nil && r.Protocol == "anthropic" && len(r.Content) > 0 {
				// Exactly as it came, byte for byte: thinking blocks too.
				var kept []json.RawMessage
				if json.Unmarshal(r.Content, &kept) == nil {
					for _, b := range kept {
						blocks = append(blocks, b)
					}
				}
			}
			if blocks == nil {
				if t.Text != "" {
					blocks = append(blocks, map[string]any{"type": "text", "text": t.Text})
				}
				for _, c := range t.Calls {
					blocks = append(blocks, map[string]any{"type": "tool_use", "id": c.ID, "name": c.Name, "input": inputOf(c.Input)})
				}
			}
		} else {
			for _, r := range t.Results {
				res := map[string]any{"type": "tool_result", "tool_use_id": r.ID, "content": r.Content}
				if r.IsError {
					res["is_error"] = true
				}
				blocks = append(blocks, res)
			}
			for _, im := range t.Images {
				blocks = append(blocks, map[string]any{"type": "image", "source": map[string]any{"type": "base64", "media_type": im.MediaType, "data": im.Data}})
			}
			if t.Text != "" {
				blocks = append(blocks, map[string]any{"type": "text", "text": t.Text})
			}
		}
		if len(blocks) == 0 {
			continue
		}
		if n := len(out); n > 0 && out[n-1].role == t.Role {
			out[n-1].content = append(out[n-1].content, blocks...)
			continue
		}
		out = append(out, message{t.Role, blocks})
	}
	list := []any{}
	for _, m := range out {
		list = append(list, map[string]any{"role": m.role, "content": m.content})
	}
	return list
}

func inputOf(raw json.RawMessage) any {
	var v any
	if json.Unmarshal(raw, &v) != nil || v == nil {
		return map[string]any{}
	}
	return v
}

// openaiTurn is POST /chat/completions with function calling.
func (l *llm) openaiTurn(ctx context.Context, a ask) (*answer, error) {
	raw, err := l.send(ctx, "POST", "/chat/completions", l.openaiBody(a))
	if err != nil {
		return nil, err
	}
	return parseOpenAI(raw)
}

// openaiBody is a chat completions request for a.
func (l *llm) openaiBody(a ask) map[string]any {
	msgs := []any{map[string]any{"role": "system", "content": a.System}}
	for _, t := range a.Turns {
		if t.Role == "assistant" {
			m := map[string]any{"role": "assistant"}
			var r rawTurn
			if json.Unmarshal(t.Raw, &r) == nil && r.Protocol == "openai" && len(r.Content) > 0 {
				var kept map[string]any
				if json.Unmarshal(r.Content, &kept) == nil {
					m = kept
					m["role"] = "assistant"
				}
			} else {
				m["content"] = t.Text
				if len(t.Calls) > 0 {
					calls := []any{}
					for _, c := range t.Calls {
						args := string(c.Input)
						if args == "" {
							args = "{}"
						}
						calls = append(calls, map[string]any{"id": c.ID, "type": "function", "function": map[string]any{"name": c.Name, "arguments": args}})
					}
					m["tool_calls"] = calls
				}
			}
			msgs = append(msgs, m)
			continue
		}
		for _, r := range t.Results {
			content := r.Content
			if r.IsError {
				content = "Error: " + content
			}
			msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": r.ID, "content": content})
		}
		if len(t.Images) > 0 {
			parts := []any{}
			for _, im := range t.Images {
				parts = append(parts, map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:" + im.MediaType + ";base64," + im.Data}})
			}
			if t.Text != "" {
				parts = append(parts, map[string]any{"type": "text", "text": t.Text})
			}
			msgs = append(msgs, map[string]any{"role": "user", "content": parts})
		} else if t.Text != "" {
			msgs = append(msgs, map[string]any{"role": "user", "content": t.Text})
		}
	}
	body := map[string]any{"model": l.model, "messages": msgs}
	if len(a.Tools) > 0 {
		tools := []any{}
		for _, t := range a.Tools {
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Schema}})
		}
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	if a.Schema != nil {
		body["response_format"] = map[string]any{"type": "json_schema", "json_schema": map[string]any{"name": a.Name, "schema": a.Schema, "strict": true}}
	}
	return body
}

// parseOpenAI reads a chat completions answer.
func parseOpenAI(raw []byte) (*answer, error) {
	var res struct {
		Choices []struct {
			Message      json.RawMessage `json:"message"`
			FinishReason string          `json:"finish_reason"`
		} `json:"choices"`
		Usage struct {
			Prompt     int64 `json:"prompt_tokens"`
			Completion int64 `json:"completion_tokens"`
			Details    struct {
				Cached int64 `json:"cached_tokens"`
			} `json:"prompt_tokens_details"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &res); err != nil || len(res.Choices) == 0 {
		return nil, &providerError{Message: "The provider answered with something that isn’t a message."}
	}
	var msg struct {
		Content   *string `json:"content"`
		Reasoning string  `json:"reasoning_content"`
		Refusal   *string `json:"refusal"`
		ToolCalls []struct {
			ID       string `json:"id"`
			Function struct {
				Name      string `json:"name"`
				Arguments string `json:"arguments"`
			} `json:"function"`
		} `json:"tool_calls"`
	}
	_ = json.Unmarshal(res.Choices[0].Message, &msg)
	out := &answer{Usage: usage{Input: res.Usage.Prompt - res.Usage.Details.Cached, Output: res.Usage.Completion, CacheRead: res.Usage.Details.Cached}}
	if msg.Content != nil {
		out.Text = strings.TrimSpace(*msg.Content)
	}
	out.Thinking = strings.TrimSpace(msg.Reasoning)
	for i, c := range msg.ToolCalls {
		id := c.ID
		if id == "" {
			id = fmt.Sprintf("call_%d_%d", time.Now().UnixNano(), i)
		}
		input := json.RawMessage(c.Function.Arguments)
		if !json.Valid(input) {
			input = json.RawMessage("{}")
		}
		out.Calls = append(out.Calls, toolCall{ID: id, Name: c.Function.Name, Input: input})
	}
	switch {
	case msg.Refusal != nil && *msg.Refusal != "", res.Choices[0].FinishReason == "content_filter":
		out.Stop = "refusal"
	case res.Choices[0].FinishReason == "length":
		out.Stop = "max_tokens"
	case len(out.Calls) > 0:
		out.Stop = "tool_use"
	default:
		out.Stop = "end"
	}
	// Kept as sent back: the message, with any call ids filled in.
	kept := map[string]any{"role": "assistant", "content": out.Text}
	if len(out.Calls) > 0 {
		calls := []any{}
		for _, c := range out.Calls {
			calls = append(calls, map[string]any{"id": c.ID, "type": "function", "function": map[string]any{"name": c.Name, "arguments": string(c.Input)}})
		}
		kept["tool_calls"] = calls
	}
	content, _ := json.Marshal(kept)
	out.Raw, _ = json.Marshal(rawTurn{Protocol: "openai", Content: content})
	return out, nil
}

// ---- the provider's models ------------------------------------------------------------

// models is the provider's chat models, newest first where it says.
func (l *llm) models(ctx context.Context) ([]string, error) {
	path := "/models"
	if l.protocol == "anthropic" {
		path = "/models?limit=1000"
	}
	raw, err := l.send(ctx, "GET", path, nil)
	if err != nil {
		return nil, err
	}
	var res struct {
		Data []struct {
			ID        string `json:"id"`
			Type      string `json:"type"`
			Created   int64  `json:"created"`
			CreatedAt string `json:"created_at"`
		} `json:"data"`
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, &providerError{Message: "The provider’s list of models couldn’t be read."}
	}
	type m struct {
		id string
		at int64
		i  int
	}
	var list []m
	for i, d := range res.Data {
		id := strings.TrimPrefix(d.ID, "models/")
		if id == "" || !chatModel(id) {
			continue
		}
		at := d.Created
		if t, err := time.Parse(time.RFC3339, d.CreatedAt); err == nil {
			at = t.Unix()
		}
		list = append(list, m{id, at, i})
	}
	slices.SortStableFunc(list, func(a, b m) int {
		if a.at != b.at {
			return int(b.at - a.at)
		}
		return a.i - b.i
	})
	out := []string{}
	for _, x := range list {
		if !slices.Contains(out, x.id) {
			out = append(out, x.id)
		}
	}
	return out, nil
}

// chatModel leaves out what can't hold a conversation (embeddings,
// speech, images, moderation).
func chatModel(id string) bool {
	id = strings.ToLower(id)
	for _, not := range []string{"embed", "whisper", "tts", "dall-e", "moderation", "image", "audio", "realtime", "transcribe", "speech", "imagen", "veo", "aqa"} {
		if strings.Contains(id, not) {
			return false
		}
	}
	return true
}

// ---- what it costs ---------------------------------------------------------------------

// price is dollars per million tokens: input, output, and reading from and
// writing to the cache.
type price struct{ in, out, cacheRead, cacheWrite float64 }

// prices are the models' list prices, by the start of their id. A model
// not here costs what its provider's usual model does (estimated); one on
// the owner's own Ollama costs nothing.
var prices = []struct {
	prefix string
	p      price
}{
	{"claude-fable-5", price{10, 50, 1, 12.5}},
	{"claude-mythos-5", price{10, 50, 1, 12.5}},
	{"claude-opus-5-5", price{4, 20, 0.2, 5}},
	{"claude-opus-5", price{5, 25, 0.5, 6.25}},
	{"claude-opus-4", price{5, 25, 0.5, 6.25}},
	{"claude-sonnet-5", price{2, 10, 0.2, 2.5}},
	{"claude-sonnet-4", price{3, 15, 0.3, 3.75}},
	{"claude-haiku-5-5", price{0.1, 0.5, 0.01, 0.125}},
	{"claude-haiku-4-5", price{1, 5, 0.1, 1.25}},
	{"gpt-5.4-mini", price{0.25, 2, 0.025, 0}},
	{"gpt-5", price{1.25, 10, 0.125, 0}},
	{"gemini-flash", price{0.3, 2.5, 0.03, 0}},
	{"gemini-pro", price{1.25, 10, 0.125, 0}},
}

// cost is what usage of model cost, in dollars.
func cost(provider, model string, u usage) float64 {
	if provider == "ollama" {
		return 0
	}
	id := model
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	p := price{2, 10, 0.2, 2.5} // a usual model, when this one isn't known
	for _, x := range prices {
		if strings.HasPrefix(id, x.prefix) {
			p = x.p
			break
		}
	}
	return (float64(u.Input)*p.in + float64(u.Output)*p.out + float64(u.CacheRead)*p.cacheRead + float64(u.CacheWrite)*p.cacheWrite) / 1e6
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "…"
	}
	return s
}

// ---- streaming ---------------------------------------------------------------------------

// delta is what a streamed turn has said so far: its words and its
// thinking, as they come.
type delta struct {
	Text     string
	Thinking string
}

// streamTurn is a turn whose words arrive as they're written: onDelta
// hears each piece (in the order they come), and the answer is the same
// as turn's once it's done.
func (l *llm) streamTurn(ctx context.Context, a ask, onDelta func(delta)) (*answer, error) {
	if a.MaxTokens == 0 {
		a.MaxTokens = 16000
	}
	if l.protocol == "anthropic" {
		body := l.anthropicBody(a)
		body["stream"] = true
		res, sse, err := l.open(ctx, "/messages", body)
		if err != nil {
			return nil, err
		}
		defer res.Close()
		if !sse {
			return whole(res, parseAnthropic, onDelta)
		}
		raw, err := readAnthropicStream(res, onDelta)
		if err != nil {
			return nil, err
		}
		return parseAnthropic(raw)
	}
	body := l.openaiBody(a)
	body["stream"] = true
	body["stream_options"] = map[string]any{"include_usage": true}
	res, sse, err := l.open(ctx, "/chat/completions", body)
	if err != nil {
		return nil, err
	}
	defer res.Close()
	if !sse {
		return whole(res, parseOpenAI, onDelta)
	}
	raw, err := readOpenAIStream(res, onDelta)
	if err != nil {
		return nil, err
	}
	return parseOpenAI(raw)
}

// whole reads an answer that came whole though a stream was asked for (a
// server that doesn't stream), telling onDelta all of it at once.
func whole(r io.Reader, parse func([]byte) (*answer, error), onDelta func(delta)) (*answer, error) {
	raw, err := io.ReadAll(r)
	if err != nil {
		return nil, &providerError{Message: "The answer broke off: " + err.Error() + "."}
	}
	ans, err := parse(raw)
	if err == nil && onDelta != nil && (ans.Text != "" || ans.Thinking != "") {
		onDelta(delta{Text: ans.Text, Thinking: ans.Thinking})
	}
	return ans, err
}

// open posts body and answers the response's body to read as it comes
// (and whether it's a stream of events), with send's one retry before
// anything was read.
func (l *llm) open(ctx context.Context, path string, body any) (io.ReadCloser, bool, error) {
	raw, err := json.Marshal(body)
	if err != nil {
		return nil, false, err
	}
	host := l.base
	if u, err := url.Parse(l.base); err == nil {
		host = u.Host
	}
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, "POST", l.base+path, bytes.NewReader(raw))
		if err != nil {
			return nil, false, err
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "text/event-stream")
		l.headers(req)
		res, err := l.client.Do(req)
		if err != nil {
			if attempt == 0 && ctx.Err() == nil && retryable(err) {
				wait(ctx, 2*time.Second)
				continue
			}
			return nil, false, &providerError{Message: fmt.Sprintf("Couldn’t reach %s: %s. Check the address in Settings › AI, and that the server is up.", host, plainNetError(err))}
		}
		if res.StatusCode >= 300 {
			out, _ := io.ReadAll(res.Body)
			res.Body.Close()
			if attempt == 0 && (res.StatusCode == 429 || res.StatusCode >= 500) {
				wait(ctx, 2*time.Second)
				continue
			}
			return nil, false, errorFor(res.StatusCode, out, host, l.model)
		}
		return res.Body, strings.Contains(res.Header.Get("Content-Type"), "event-stream"), nil
	}
}

// sseEvents calls fn with each server-sent event's data, in order.
func sseEvents(r io.Reader, fn func(event string, data []byte) error) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64<<10), 8<<20)
	event := ""
	var data []byte
	flush := func() error {
		if len(data) == 0 {
			event = ""
			return nil
		}
		err := fn(event, data)
		event, data = "", nil
		return err
	}
	for sc.Scan() {
		line := sc.Text()
		switch {
		case line == "":
			if err := flush(); err != nil {
				return err
			}
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(line[6:])
		case strings.HasPrefix(line, "data:"):
			if len(data) > 0 {
				data = append(data, '\n')
			}
			data = append(data, strings.TrimPrefix(line[5:], " ")...)
		}
	}
	if err := sc.Err(); err != nil {
		return &providerError{Message: "The answer broke off: " + err.Error() + "."}
	}
	return flush()
}

// readAnthropicStream puts a streamed Messages answer back together as
// the whole answer would have come (the same blocks, thinking signatures
// and all), telling onDelta of words and thinking as they arrive.
func readAnthropicStream(r io.Reader, onDelta func(delta)) ([]byte, error) {
	type block struct {
		start   map[string]any
		text    strings.Builder
		think   strings.Builder
		sig     string
		partial strings.Builder
	}
	var blocks []*block
	stop := ""
	u := map[string]any{}
	at := func(i int) *block {
		for len(blocks) <= i {
			blocks = append(blocks, &block{start: map[string]any{}})
		}
		return blocks[i]
	}
	err := sseEvents(r, func(_ string, data []byte) error {
		var ev struct {
			Type    string          `json:"type"`
			Index   int             `json:"index"`
			Message json.RawMessage `json:"message"`
			Block   map[string]any  `json:"content_block"`
			Delta   struct {
				Type        string `json:"type"`
				Text        string `json:"text"`
				Thinking    string `json:"thinking"`
				Signature   string `json:"signature"`
				PartialJSON string `json:"partial_json"`
				StopReason  string `json:"stop_reason"`
			} `json:"delta"`
			Usage map[string]any `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &ev) != nil {
			return nil
		}
		switch ev.Type {
		case "message_start":
			var m struct {
				Usage map[string]any `json:"usage"`
			}
			_ = json.Unmarshal(ev.Message, &m)
			for k, v := range m.Usage {
				u[k] = v
			}
		case "content_block_start":
			b := at(ev.Index)
			b.start = ev.Block
			if t, _ := ev.Block["text"].(string); t != "" {
				b.text.WriteString(t)
			}
		case "content_block_delta":
			b := at(ev.Index)
			switch ev.Delta.Type {
			case "text_delta":
				b.text.WriteString(ev.Delta.Text)
				if onDelta != nil {
					onDelta(delta{Text: ev.Delta.Text})
				}
			case "thinking_delta":
				b.think.WriteString(ev.Delta.Thinking)
				if onDelta != nil {
					onDelta(delta{Thinking: ev.Delta.Thinking})
				}
			case "signature_delta":
				b.sig += ev.Delta.Signature
			case "input_json_delta":
				b.partial.WriteString(ev.Delta.PartialJSON)
			}
		case "message_delta":
			if ev.Delta.StopReason != "" {
				stop = ev.Delta.StopReason
			}
			for k, v := range ev.Usage {
				u[k] = v
			}
		case "error":
			msg := ev.Error.Message
			if msg == "" {
				msg = "it stopped answering"
			}
			return &providerError{Message: "The provider broke off its answer: " + msg + ". Try again shortly."}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	content := []any{}
	for _, b := range blocks {
		out := map[string]any{}
		for k, v := range b.start {
			out[k] = v
		}
		switch out["type"] {
		case "text":
			out["text"] = b.text.String()
		case "thinking":
			out["thinking"] = b.think.String()
			if b.sig != "" {
				out["signature"] = b.sig
			}
		case "tool_use", "server_tool_use":
			var input any = map[string]any{}
			if p := strings.TrimSpace(b.partial.String()); p != "" {
				if json.Unmarshal([]byte(p), &input) != nil {
					input = map[string]any{}
				}
			}
			out["input"] = input
		}
		content = append(content, out)
	}
	return json.Marshal(map[string]any{"content": content, "stop_reason": stop, "usage": u})
}

// readOpenAIStream puts a streamed chat completion back together as one
// answer, telling onDelta of words (and reasoning, where it's sent).
func readOpenAIStream(r io.Reader, onDelta func(delta)) ([]byte, error) {
	var text, reasoning, refusal strings.Builder
	type call struct {
		ID, Name string
		Args     strings.Builder
	}
	var calls []*call
	finish := ""
	var usage json.RawMessage
	err := sseEvents(r, func(_ string, data []byte) error {
		if strings.TrimSpace(string(data)) == "[DONE]" {
			return nil
		}
		var ch struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					Reasoning string `json:"reasoning_content"`
					Refusal   string `json:"refusal"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage json.RawMessage `json:"usage"`
			Error any             `json:"error"`
		}
		if json.Unmarshal(data, &ch) != nil {
			return nil
		}
		if ch.Error != nil {
			return &providerError{Message: "The provider broke off its answer: " + providerMessage(data) + ". Try again shortly."}
		}
		if len(ch.Usage) > 0 && string(ch.Usage) != "null" {
			usage = ch.Usage
		}
		for _, c := range ch.Choices {
			d := c.Delta
			if d.Content != "" {
				text.WriteString(d.Content)
				if onDelta != nil {
					onDelta(delta{Text: d.Content})
				}
			}
			if d.Reasoning != "" {
				reasoning.WriteString(d.Reasoning)
				if onDelta != nil {
					onDelta(delta{Thinking: d.Reasoning})
				}
			}
			refusal.WriteString(d.Refusal)
			for _, tc := range d.ToolCalls {
				for len(calls) <= tc.Index {
					calls = append(calls, &call{})
				}
				x := calls[tc.Index]
				if tc.ID != "" {
					x.ID = tc.ID
				}
				if tc.Function.Name != "" {
					x.Name = tc.Function.Name
				}
				x.Args.WriteString(tc.Function.Arguments)
			}
			if c.FinishReason != "" {
				finish = c.FinishReason
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	msg := map[string]any{"role": "assistant", "content": text.String()}
	if reasoning.Len() > 0 {
		msg["reasoning_content"] = reasoning.String()
	}
	if refusal.Len() > 0 {
		msg["refusal"] = refusal.String()
	}
	if len(calls) > 0 {
		list := []any{}
		for _, c := range calls {
			list = append(list, map[string]any{"id": c.ID, "type": "function", "function": map[string]any{"name": c.Name, "arguments": c.Args.String()}})
		}
		msg["tool_calls"] = list
	}
	out := map[string]any{"choices": []any{map[string]any{"message": msg, "finish_reason": finish}}}
	if len(usage) > 0 {
		out["usage"] = usage
	}
	return json.Marshal(out)
}

// takesImages is whether a provider reads pictures sent with a question.
func takesImages(provider string) bool {
	switch provider {
	case "anthropic", "openai", "gemini", "openrouter":
		return true
	}
	return false
}
