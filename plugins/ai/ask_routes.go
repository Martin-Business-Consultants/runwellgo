package ai

import (
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// What the Ask chat does beyond asking: its words streamed to the
// person's open tabs as they're written (the "live" grant, topic "ask"),
// Stop, regenerating the last answer and editing the last question, a
// model picked per chat, pictures sent with a question, thumbs up or down
// on an answer (ai_feedback), the chat history with rename and delete,
// and a card for a question handed to one of the agents.

// ---- streaming --------------------------------------------------------------------------

// streamer publishes a reply's words to the person as they come, at most
// every tenth of a second: the whole step so far each time (a tab that
// missed one is none the worse), or word to read again when it's too
// long to send.
type streamer struct {
	api    *plugin.API
	chatID int64
	to     int64

	mu       sync.Mutex
	step     int
	text     strings.Builder
	thinking strings.Builder
	last     time.Time
	pending  bool
	timer    *time.Timer
}

const streamEvery = 100 * time.Millisecond

func (s *streamer) begin() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.step++
	s.text.Reset()
	s.thinking.Reset()
	s.mu.Unlock()
	s.publish(map[string]any{"chat_id": s.chatID, "state": "thinking"})
}

func (s *streamer) add(d delta) {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.text.WriteString(d.Text)
	s.thinking.WriteString(d.Thinking)
	if time.Since(s.last) < streamEvery {
		if s.timer == nil {
			s.timer = time.AfterFunc(streamEvery, func() {
				s.mu.Lock()
				s.timer = nil
				s.mu.Unlock()
				s.flush()
			})
		}
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	s.flush()
}

func (s *streamer) flush() {
	s.mu.Lock()
	s.last = time.Now()
	text, thinking, step := s.text.String(), s.thinking.String(), s.step
	s.mu.Unlock()
	const room = 56 << 10
	if len(thinking) > 8<<10 {
		thinking = "…" + thinking[len(thinking)-8<<10:]
	}
	ev := map[string]any{"chat_id": s.chatID, "state": "writing", "step": step, "text": text, "thinking": thinking}
	if len(text) > room {
		ev = map[string]any{"chat_id": s.chatID, "state": "writing", "step": step, "stale": true}
	}
	s.publish(ev)
}

// partial is what the step had said, for a reply stopped part way.
func (s *streamer) partial() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return strings.TrimSpace(s.text.String())
}

// saved says a step is kept: the tab reads the chat again.
func (s *streamer) saved(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	s.publish(map[string]any{"chat_id": s.chatID, "state": "saved"})
}

// done says the reply has finished, however it did.
func (s *streamer) done(ctx context.Context) {
	if s == nil {
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.mu.Unlock()
	s.publish(map[string]any{"chat_id": s.chatID, "state": "done"})
}

func (s *streamer) publish(ev map[string]any) {
	if err := s.api.Publish(context.Background(), "ask", []int64{s.to}, ev); err != nil && !strings.Contains(err.Error(), "wasn't granted") {
		s.api.Log("ask stream: " + err.Error())
	}
}

// ---- stopping ---------------------------------------------------------------------------

type stopKey struct{}

// askReplies are the replies running in this process, by chat: Stop cancels
// one.
var askReplies sync.Map // chat id → *stopper

type stopper struct {
	cancel  context.CancelFunc
	stopped atomic.Bool
}

// stoppable is ctx, cancelled when the person presses Stop.
func stoppable(ctx context.Context, chatID int64) (context.Context, func()) {
	ctx, cancel := context.WithCancel(ctx)
	st := &stopper{cancel: cancel}
	ctx = context.WithValue(ctx, stopKey{}, st)
	askReplies.Store(chatID, st)
	return ctx, func() {
		askReplies.CompareAndDelete(chatID, st)
		cancel()
	}
}

// stopped is whether the person stopped the reply ctx belongs to.
func stopped(ctx context.Context) bool {
	st, _ := ctx.Value(stopKey{}).(*stopper)
	return st != nil && st.stopped.Load()
}

// saveStopped keeps what a stopped reply had written, saying it stopped.
func saveStopped(ctx context.Context, api *plugin.API, chatID int64, st *streamer) {
	text := st.partial()
	if text == "" {
		text = "(Stopped.)"
	} else {
		text += "\n\n(Stopped.)"
	}
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `INSERT INTO ai_messages (ai_chat_id, role, content, raw_content, finish_reason, created_at, updated_at) VALUES (?, 'assistant', ?, '', 'stopped', ?, ?)`,
		chatID, text, now, now); err != nil {
		api.Log("stop: " + err.Error())
	}
}

// pickedModel is the model a chat answers with: its pick, else the main one.
func pickedModel(s *setting, pick string) string {
	switch pick {
	case "", "main":
		return s.modelFor(false)
	case "fast":
		return s.modelFor(true)
	}
	return strings.TrimPrefix(pick, "opencode/")
}

// ---- routes ----------------------------------------------------------------------------

// chatHistory is the person's chats, latest first, for the history list.
func chatHistory(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return res, nil
	}
	rows, err := api.Query(ctx, `SELECT `+chatColumns+` FROM ai_chats WHERE user_id = ? AND purpose = 'ask' AND title IS NOT NULL ORDER BY updated_at DESC LIMIT 200`, caller.ID)
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

// updateChat renames a chat, or picks its model.
func updateChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	in := wrapped(params(req), "chat")
	if v, ok := in["title"]; ok {
		title := truncate(strings.Join(strings.Fields(str(v)), " "), 80)
		if title == "" {
			return plugin.Invalid(map[string][]string{"title": {"can't be blank"}}), nil
		}
		if _, err := api.Exec(ctx, `UPDATE ai_chats SET title = ? WHERE id = ?`, title, c.ID); err != nil {
			return nil, err
		}
	}
	if v, ok := in["model"]; ok {
		pick := strings.TrimSpace(str(v))
		if len(pick) > 120 {
			return plugin.Invalid(map[string][]string{"model": {"is too long"}}), nil
		}
		if _, err := api.Exec(ctx, `UPDATE ai_chats SET model = ? WHERE id = ?`, nullStr(pick), c.ID); err != nil {
			return nil, err
		}
	}
	c, err = loadChat(ctx, api, c.ID)
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, "", req.Actor)
}

// deleteChat takes a chat out of the person's history: its words go, what
// it cost stays in the month's spend.
func deleteChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	if c.replying() {
		return refuse(http.StatusConflict, "Stop the answer first, then delete the chat."), nil
	}
	err = api.Tx(ctx, func(q plugin.Querier) error {
		if _, err := q.Exec(ctx, `UPDATE ai_usages SET ai_message_id = NULL WHERE ai_chat_id = ?`, c.ID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM ai_feedback WHERE ai_message_id IN (SELECT id FROM ai_messages WHERE ai_chat_id = ?)`, c.ID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM ai_tool_calls WHERE ai_message_id IN (SELECT id FROM ai_messages WHERE ai_chat_id = ?)`, c.ID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM ai_messages WHERE ai_chat_id = ?`, c.ID); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `UPDATE ai_chats SET purpose = 'deleted', title = NULL WHERE id = ?`, c.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	_, _ = api.Exec(ctx, `UPDATE ai_approvals SET status = 'withdrawn' WHERE ai_chat_id = ? AND status = 'waiting'`, c.ID)
	return plugin.JSON(200, map[string]any{"message": "Chat deleted.", "deleted": true}), nil
}

const (
	maxImages     = 4
	maxImageBytes = 3 << 20
)

var imageTypes = map[string]bool{"image/png": true, "image/jpeg": true, "image/gif": true, "image/webp": true}

// sendChat asks a question, with pictures when the provider reads them.
func sendChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	msg := wrapped(params(req), "message")
	text := strings.TrimSpace(str(msg["content"]))
	var images []image
	if list, _ := msg["images"].([]any); len(list) > 0 {
		s, err := loadSetting(ctx, api)
		if err != nil {
			return nil, err
		}
		if !takesImages(s.Provider) {
			return refuse(http.StatusUnprocessableEntity, s.shortName()+" doesn’t read pictures here: send the question as words."), nil
		}
		if len(list) > maxImages {
			return refuse(http.StatusUnprocessableEntity, fmt.Sprintf("Send %d pictures at most with one question.", maxImages)), nil
		}
		for _, v := range list {
			m, _ := v.(map[string]any)
			typ, data := str(m["media_type"]), str(m["data"])
			raw, err := base64.StdEncoding.DecodeString(data)
			switch {
			case !imageTypes[typ]:
				return refuse(http.StatusUnprocessableEntity, "Pictures are PNG, JPEG, GIF or WebP."), nil
			case err != nil || len(raw) == 0:
				return refuse(http.StatusUnprocessableEntity, "A picture couldn’t be read: attach it again."), nil
			case len(raw) > maxImageBytes:
				return refuse(http.StatusUnprocessableEntity, "A picture is larger than 3 MB: send a smaller one."), nil
			}
			images = append(images, image{MediaType: typ, Data: data})
		}
	}
	caller := req.Actor
	if err := askSoonWith(ctx, api, c, text, images, func(ctx context.Context, api *plugin.API) { reply(ctx, api, c.ID, caller, nil) }); err != nil {
		return failed(err)
	}
	c, err = loadChat(ctx, api, c.ID)
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, "", req.Actor)
}

// stopChat stops the answer being written; what it had written stays.
func stopChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	if v, ok := askReplies.Load(c.ID); ok {
		st := v.(*stopper)
		st.stopped.Store(true)
		st.cancel()
	} else if c.replying() {
		// Not in this process (a restart): it isn't coming.
		_, _ = api.Exec(ctx, `UPDATE ai_chats SET replying_since = NULL WHERE id = ?`, c.ID)
	}
	// The reply notices and saves what it had; give it a moment.
	for i := 0; i < 20; i++ {
		if c, err = loadChat(ctx, api, c.ID); err != nil || !c.replying() {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, "", req.Actor)
}

// lastQuestion is the chat's last question, and whether anything since
// waits on the person.
func lastQuestion(ctx context.Context, api *plugin.API, c *chat) (int64, string, error) {
	var id int64
	var raw string
	err := api.QueryRow(ctx, `SELECT id, COALESCE(raw_content, '') FROM ai_messages WHERE ai_chat_id = ? AND role = 'user' ORDER BY id DESC LIMIT 1`, c.ID).Scan(&id, &raw)
	return id, raw, err
}

// rewind deletes everything after message id, the cost staying counted.
func rewind(ctx context.Context, api *plugin.API, c *chat, after int64) error {
	_, _ = api.Exec(ctx, `UPDATE ai_approvals SET status = 'withdrawn' WHERE ai_chat_id = ? AND status = 'waiting'`, c.ID)
	return api.Tx(ctx, func(q plugin.Querier) error {
		sub := `SELECT id FROM ai_messages WHERE ai_chat_id = ? AND id > ?`
		if _, err := q.Exec(ctx, `UPDATE ai_usages SET ai_message_id = NULL WHERE ai_message_id IN (`+sub+`)`, c.ID, after); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM ai_feedback WHERE ai_message_id IN (`+sub+`)`, c.ID, after); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM ai_tool_calls WHERE ai_message_id IN (`+sub+`)`, c.ID, after); err != nil {
			return err
		}
		_, err := q.Exec(ctx, `DELETE FROM ai_messages WHERE ai_chat_id = ? AND id > ?`, c.ID, after)
		return err
	})
}

// again answers the chat's last question afresh: after regenerating, or
// editing it.
func again(ctx context.Context, api *plugin.API, req *plugin.Request, edit bool) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	if c.replying() {
		return refuse(http.StatusConflict, "Wait for the answer, or stop it, first."), nil
	}
	id, raw, err := lastQuestion(ctx, api, c)
	if errors.Is(err, sql.ErrNoRows) {
		return refuse(http.StatusConflict, "There's no question to answer again yet."), nil
	} else if err != nil {
		return nil, err
	}
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	if reason, err := available(ctx, api, s); err != nil {
		return nil, err
	} else if reason != "" {
		return refuse(http.StatusUnprocessableEntity, reason), nil
	}
	if edit {
		text := truncate(strings.TrimSpace(str(wrapped(params(req), "message")["content"])), 8000)
		if text == "" {
			return plugin.Invalid(map[string][]string{"content": {"can't be blank"}}), nil
		}
		var kept map[string]any
		_ = json.Unmarshal([]byte(raw), &kept)
		if kept == nil {
			kept = map[string]any{}
		}
		kept["text"] = contextLine(ctx, api, c) + "\n\n" + text
		b, _ := json.Marshal(kept)
		if _, err := api.Exec(ctx, `UPDATE ai_messages SET content = ?, raw_content = ?, updated_at = ? WHERE id = ?`, text, string(b), store.Now().DB(), id); err != nil {
			return nil, err
		}
	}
	if err := rewind(ctx, api, c, id); err != nil {
		return nil, err
	}
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `UPDATE ai_chats SET replying_since = ?, updated_at = ? WHERE id = ?`, now, now, c.ID); err != nil {
		return nil, err
	}
	caller := req.Actor
	if err := api.Background("reply", func(ctx context.Context, api *plugin.API) { reply(ctx, api, c.ID, caller, nil) }); err != nil {
		_, _ = api.Exec(ctx, `UPDATE ai_chats SET replying_since = NULL WHERE id = ?`, c.ID)
		return refuse(http.StatusConflict, err.Error()), nil
	}
	c, err = loadChat(ctx, api, c.ID)
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, "", req.Actor)
}

func regenerateChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return again(ctx, api, req, false)
}

func editChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return again(ctx, api, req, true)
}

// chatMessage is one of the chat's messages, by the route's message_id.
func chatMessage(ctx context.Context, api *plugin.API, c *chat, req *plugin.Request) (int64, string, string, error) {
	var id int64
	var role, raw string
	err := api.QueryRow(ctx, `SELECT id, role, COALESCE(raw_content, '') FROM ai_messages WHERE id = ? AND ai_chat_id = ?`, pathID(req, "message_id"), c.ID).Scan(&id, &role, &raw)
	return id, role, raw, err
}

// rateMessage keeps the person's thumbs up or down on an answer.
func rateMessage(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	id, role, _, err := chatMessage(ctx, api, c, req)
	if errors.Is(err, sql.ErrNoRows) || role != "assistant" {
		return missing("answer"), nil
	} else if err != nil {
		return nil, err
	}
	rating := map[string]int{"up": 1, "down": -1}[str(params(req)["rating"])]
	now := store.Now().DB()
	if rating == 0 {
		_, err = api.Exec(ctx, `DELETE FROM ai_feedback WHERE ai_message_id = ? AND user_id = ?`, id, req.Actor.ID)
	} else {
		_, err = api.Exec(ctx, `INSERT INTO ai_feedback (ai_message_id, user_id, rating, created_at, updated_at) VALUES (?, ?, ?, ?, ?)
			ON CONFLICT (ai_message_id, user_id) DO UPDATE SET rating = excluded.rating, updated_at = excluded.updated_at`, id, req.Actor.ID, rating, now, now)
	}
	if err != nil {
		return nil, err
	}
	msg := map[int]string{1: "Thanks: marked helpful.", -1: "Thanks: marked not helpful.", 0: "Feedback taken back."}[rating]
	return plugin.JSON(200, map[string]any{"message": msg, "rating": map[int]any{1: "up", -1: "down", 0: nil}[rating]}), nil
}

// messageImage is a picture sent with a question.
func messageImage(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	_, _, raw, err := chatMessage(ctx, api, c, req)
	if errors.Is(err, sql.ErrNoRows) {
		return missing("picture"), nil
	} else if err != nil {
		return nil, err
	}
	var kept struct {
		Images []image `json:"images"`
	}
	_ = json.Unmarshal([]byte(raw), &kept)
	n, _ := strconv.Atoi(req.Params["n"])
	if n < 0 || n >= len(kept.Images) {
		return missing("picture"), nil
	}
	data, err := base64.StdEncoding.DecodeString(kept.Images[n].Data)
	if err != nil {
		return missing("picture"), nil
	}
	return &plugin.Response{Status: 200, Body: data, Headers: map[string]string{"Content-Type": kept.Images[n].MediaType, "Cache-Control": "private, max-age=86400"}}, nil
}

// agentCard keeps, in the chat, that a question went to one of the agents
// (it started the run itself): a card linking the run, never sent to the
// model.
func agentCard(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := ownChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	p := wrapped(params(req), "card")
	runID := intOf(p["run_id"])
	var userID int64
	var handle string
	err = api.QueryRow(ctx, `SELECT r.user_id, a.handle FROM ai_runs r JOIN ai_agents a ON a.id = r.agent_id WHERE r.id = ?`, runID).Scan(&userID, &handle)
	if errors.Is(err, sql.ErrNoRows) || (err == nil && userID != req.Actor.ID) {
		return missing("run"), nil
	} else if err != nil {
		return nil, err
	}
	prompt := truncate(strings.TrimSpace(str(p["prompt"])), 2000)
	raw, _ := json.Marshal(map[string]any{"run_id": runID, "handle": handle})
	now := store.Now().DB()
	if _, err := api.Exec(ctx, `INSERT INTO ai_messages (ai_chat_id, role, content, raw_content, finish_reason, created_at, updated_at) VALUES (?, 'assistant', ?, ?, 'agent', ?, ?)`,
		c.ID, prompt, string(raw), now, now); err != nil {
		return nil, err
	}
	title := c.Title
	if title == "" {
		title = truncate("@"+handle+": "+strings.Join(strings.Fields(prompt), " "), 80)
	}
	if _, err := api.Exec(ctx, `UPDATE ai_chats SET title = ?, updated_at = ? WHERE id = ?`, title, now, c.ID); err != nil {
		return nil, err
	}
	c, err = loadChat(ctx, api, c.ID)
	if err != nil {
		return nil, err
	}
	return chatAnswer(ctx, api, c, "", req.Actor)
}
