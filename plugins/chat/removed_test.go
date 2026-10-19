package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// env is Runwell as chat sees it: the people on staff (who can change),
// and live and notify that go nowhere.
type env struct {
	mu     sync.Mutex
	people []map[string]any
}

func (e *env) Setting(context.Context, string) (map[string]any, error)      { return map[string]any{}, nil }
func (e *env) ReplaceSetting(context.Context, string, map[string]any) error { return nil }
func (e *env) Site(context.Context) (plugin.Site, error)                    { return plugin.Site{Name: "Runwell"}, nil }
func (e *env) SendMail(context.Context, mail.Message) error                 { return nil }
func (e *env) Records(_ context.Context, q plugin.Query) ([]map[string]any, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if q.Type == "User" {
		return append([]map[string]any{}, e.people...), nil
	}
	return nil, nil
}
func (e *env) Write(context.Context, plugin.Actor, string, any) (map[string]any, error) {
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
	return nil, nil
}
func (e *env) RunTool(context.Context, plugin.Actor, string, string, map[string]any) (map[string]any, error) {
	return nil, errors.New("no")
}
func (e *env) Seal(plain string) string                                  { return plain }
func (e *env) Unseal(sealed string) (string, bool)                       { return sealed, true }
func (e *env) Publish(string, string, []int64, json.RawMessage)          {}
func (e *env) Online([]int64) map[int64]bool                             { return map[int64]bool{} }
func (e *env) Notify(context.Context, string, plugin.Notification) error { return nil }
func (e *env) RichText(html string) plugin.RichText                      { return plugin.RichText{HTML: html, Text: html} }

func setup(t *testing.T) (*plugin.Host, *env, *sql.DB) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	e := &env{people: []map[string]any{
		{"type": "User", "id": int64(1), "name": "Ted", "role": "owner"},
		{"type": "User", "id": int64(2), "name": "Sarah", "role": "manager"},
		{"type": "User", "id": int64(3), "name": "Marcus", "role": "member"},
	}}
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
	return h, e, db.DB
}

var (
	sarah  = &plugin.Caller{ID: 2, Name: "Sarah", Role: "manager"}
	marcus = &plugin.Caller{ID: 3, Name: "Marcus", Role: "member"}
)

func call(t *testing.T, h *plugin.Host, who *plugin.Caller, method, export string, params map[string]string, body string) map[string]any {
	t.Helper()
	req := &plugin.Request{Method: method, Params: params, Query: map[string][]string{}, Body: []byte(body), Actor: who}
	res, err := h.Serve(t.Context(), Key, export, req, who.Name)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(res.Body, &out); err != nil {
		t.Fatalf("%v: %s", err, res.Body)
	}
	if res.Status >= 300 {
		t.Fatalf("%s %s: %d %s", method, export, res.Status, res.Body)
	}
	return out
}

func convID(t *testing.T, out map[string]any) string {
	t.Helper()
	c, _ := out["conversation"].(map[string]any)
	id, _ := c["id"].(float64)
	if id == 0 {
		t.Fatalf("no conversation: %v", out)
	}
	return strconv.FormatInt(int64(id), 10)
}

// Someone removed: their messages stay under "Name (removed)", their
// channel goes to the successor, they leave channels but not their direct
// messages, and their chat settings go; hearing it twice changes nothing.
func TestUserRemoved(t *testing.T) {
	h, e, db := setup(t)
	ch := convID(t, call(t, h, sarah, "POST", "createConversation", nil, `{"conversation":{"kind":"channel","name":"design","private":true,"member_ids":[1]}}`))
	dm := convID(t, call(t, h, sarah, "POST", "createConversation", nil, `{"conversation":{"kind":"dm","member_ids":[3]}}`))
	call(t, h, sarah, "POST", "sendMessage", map[string]string{"id": dm}, `{"message":{"body":"Over to you"}}`)
	call(t, h, sarah, "POST", "sendMessage", map[string]string{"id": ch}, `{"message":{"body":"Plans"}}`)
	call(t, h, sarah, "PATCH", "updateSettings", nil, `{"settings":{"status_text":"Away"}}`)

	e.mu.Lock()
	e.people = e.people[:1:1]
	e.people = append(e.people, map[string]any{"type": "User", "id": int64(3), "name": "Marcus", "role": "member"})
	e.mu.Unlock()
	ev := map[string]any{"kind": "user.removed", "subject_type": "User", "subject_id": 2,
		"payload": map[string]any{"id": 2, "name": "Sarah", "successor_id": 3, "successor_name": "Marcus"}}
	for range 2 {
		h.Emit(t.Context(), "user.removed", ev)
	}

	var creator, members, profiles, sarahIn int
	_ = db.QueryRowContext(t.Context(), `SELECT created_by_id FROM chat_conversations WHERE id = ?`, ch).Scan(&creator)
	_ = db.QueryRowContext(t.Context(), `SELECT count(*) FROM chat_members WHERE conversation_id = ? AND user_id = 3`, ch).Scan(&members)
	_ = db.QueryRowContext(t.Context(), `SELECT count(*) FROM chat_members WHERE user_id = 2 AND conversation_id = ?`, ch).Scan(&sarahIn)
	_ = db.QueryRowContext(t.Context(), `SELECT count(*) FROM chat_profiles WHERE user_id = 2`).Scan(&profiles)
	if creator != 3 || members != 1 || sarahIn != 0 || profiles != 0 {
		t.Fatalf("creator %d, marcus in %d, sarah in %d, profiles %d", creator, members, sarahIn, profiles)
	}

	// Marcus still has the direct message, named for her, and reads it.
	boot := call(t, h, marcus, "GET", "bootstrap", nil, "")
	raw, _ := json.Marshal(boot)
	if !strings.Contains(string(raw), `"name":"Sarah (removed)"`) || !strings.Contains(string(raw), `"removed":true`) {
		t.Fatalf("bootstrap: %s", raw)
	}
	msgs := call(t, h, marcus, "GET", "listMessages", map[string]string{"id": dm}, "")
	raw, _ = json.Marshal(msgs)
	if !strings.Contains(string(raw), "Over to you") || !strings.Contains(string(raw), `"name":"Sarah (removed)"`) {
		t.Fatalf("messages: %s", raw)
	}
	// And the channel, which he now runs.
	msgs = call(t, h, marcus, "GET", "listMessages", map[string]string{"id": ch}, "")
	raw, _ = json.Marshal(msgs)
	if !strings.Contains(string(raw), "Plans") || !strings.Contains(string(raw), "Sarah (removed)") {
		t.Fatalf("channel: %s", raw)
	}
}
