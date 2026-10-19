package core

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// A note that @mentions a plugin's mentionable reaches that plugin as
// mention.created, with the note's author as the caller; its reply is a
// note on the same record written as the mentionable, for that person. A
// mention relayed from another plugin's content comes back to that plugin.
// Someone whose role may not mention it reaches nothing.
func TestPluginMentionsAndReplies(t *testing.T) {
	heard := make(chan plugin.MentionEvent, 4)
	callers := make(chan *plugin.Caller, 4)
	bots := &plugin.Module{
		Manifest: plugin.Manifest{Key: "bots", Name: "Bots", Version: "1", Permissions: plugin.Grants{Mentionables: true}, Hooks: []string{"mention.created"}},
		Mentionables: func(context.Context, *plugin.API) ([]plugin.Mentionable, error) {
			return []plugin.Mentionable{{ID: "1", Handle: "helper", Name: "Helper"}, {ID: "2", Handle: "boss", Name: "Boss", Roles: []string{"owner"}}}, nil
		},
		OnEvent: func(ctx context.Context, api *plugin.API, event string, data json.RawMessage) error {
			var ev plugin.MentionEvent
			if err := json.Unmarshal(data, &ev); err != nil {
				return err
			}
			callers <- api.Caller()
			ack, err := api.Reply(ctx, plugin.Reply{Mention: ev.ID, Body: "On it…"})
			if err != nil {
				return err
			}
			if _, err := api.Reply(ctx, plugin.Reply{Mention: ev.ID, Body: "**Done**: Client:1 looks fine.", Replace: ack}); err != nil {
				return err
			}
			heard <- ev
			return nil
		},
	}
	replies := make(chan plugin.MentionReply, 2)
	talk := &plugin.Module{
		Manifest: plugin.Manifest{Key: "talk", Name: "Talk", Version: "1", Permissions: plugin.Grants{MentionsRelay: true},
			Routes: []plugin.Route{{Method: "POST", Path: "/say", Export: "say"}}},
		Handlers: map[string]plugin.Handler{"say": func(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
			rt := api.RichText(string(req.Body))
			for _, m := range rt.Mentionables {
				if _, err := api.RelayMention(ctx, plugin.Mention{Mentionable: m, Text: rt.Text, HTML: rt.HTML, Ref: "77", Subject: "#general"}); err != nil {
					return nil, err
				}
			}
			return plugin.JSON(200, map[string]any{"mentionables": rt.Mentionables}), nil
		}},
		OnMentionReply: func(_ context.Context, _ *plugin.API, r plugin.MentionReply) (string, error) {
			replies <- r
			return "reply-1", nil
		},
	}
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Plugins: []*plugin.Module{bots, talk}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	a.Handler()
	ctx := t.Context()
	for _, key := range []string{"bots", "talk"} {
		if _, err := a.plugins.Install(ctx, nil, key); err != nil {
			t.Fatal(err)
		}
		on := true
		if _, err := a.plugins.Configure(ctx, nil, key, nil, &on); err != nil {
			t.Fatal(err)
		}
	}
	person := func(email, name, role string) *Actor {
		res, err := a.store.ExecContext(ctx, `INSERT INTO users (email_address, password_digest, name, role, created_at, updated_at) VALUES (?, 'x', ?, ?, datetime('now'), datetime('now'))`, email, name, role)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		u, err := a.loadUser(ctx, id)
		if err != nil {
			t.Fatal(err)
		}
		return &Actor{User: u}
	}
	ted, mia := person("ted@example.com", "Ted Martin", "owner"), person("mia@example.com", "Mia", "member")
	if _, err := a.store.ExecContext(ctx, `INSERT INTO clients (id, name, created_at, updated_at) VALUES (1, 'Harbor', datetime('now'), datetime('now'))`); err != nil {
		t.Fatal(err)
	}

	// The pickers list them, and a member doesn't see the owners' one.
	if got := a.mentionablesFor(ctx, "member"); len(got) != 1 || !strings.Contains(string(must(json.Marshal(got[0]))), `"bots:1"`) {
		t.Fatalf("a member's mentionables: %v", got)
	}
	if got := a.mentionablesFor(ctx, "owner"); len(got) != 2 {
		t.Fatalf("an owner's mentionables: %v", got)
	}

	note := func(actor *Actor, html string) {
		t.Helper()
		html = sanitizeRichText(html)
		if !strings.Contains(html, `data-mentionable="bots:`) {
			t.Fatalf("the mention didn't survive cleaning: %s", html)
		}
		err := a.write(ctx, actor, func(tx *Tx) error {
			id, err := tx.Insert(`INSERT INTO notes (subject_type, subject_id, body, kind, occurred_at, author_id, source, created_at, updated_at) VALUES ('Client', 1, ?, 'internal', datetime('now'), ?, 'app', datetime('now'), datetime('now'))`, html, actor.User.ID)
			if err != nil {
				return err
			}
			tx.relayMentions("Client", 1, id, html)
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	note(ted, `<p>Hey <span data-type="mention" data-mentionable="bots:1">@Helper</span> look at this</p>`)
	select {
	case ev := <-heard:
		if ev.Source != "note" || ev.Record != "Client:1" || ev.Author.ID != ted.User.ID || !strings.Contains(ev.Text, "look at this") || ev.Mentionable != "1" {
			t.Fatalf("heard %+v", ev)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the plugin never heard the mention")
	}
	if c := <-callers; c == nil || c.ID != ted.User.ID || c.Role != "owner" {
		t.Fatalf("the caller was %+v", c)
	}
	// The ack became the answer: one note, written as the mentionable.
	var body, source, writtenAs string
	var author int64
	var n int
	if err := a.store.QueryRowContext(ctx, `SELECT count(*), max(body), max(source), max(written_as), max(author_id) FROM notes WHERE written_as IS NOT NULL`).Scan(&n, &body, &source, &writtenAs, &author); err != nil {
		t.Fatal(err)
	}
	if n != 1 || !strings.Contains(body, "Done") || writtenAs != "bots:1" || author != ted.User.ID || source != "Helper · via Bots for Ted" {
		t.Fatalf("the reply: %d %q %q %q %d", n, body, source, writtenAs, author)
	}
	notes, err := a.notesJSON(ctx, "Client", 1, 0)
	if err != nil {
		t.Fatal(err)
	}
	var reply Obj
	for _, x := range notes {
		if b, _ := json.Marshal(x); strings.Contains(string(b), `"bots:1"`) {
			reply = x
		}
	}
	if b, _ := json.Marshal(reply); !strings.Contains(string(b), `"name":"Helper"`) || !strings.Contains(string(b), `"source":"via Bots for Ted"`) {
		t.Fatalf("the reply as the app sees it: %s", b)
	}

	// A member mentioning the owners' one reaches nothing.
	note(mia, `<p><span data-mentionable="bots:2">@Boss</span> hello</p>`)
	select {
	case ev := <-heard:
		t.Fatalf("the plugin heard a mention it shouldn't: %+v", ev)
	case <-time.After(300 * time.Millisecond):
	}

	// Relayed from talk's own content, the reply goes back to talk.
	req := &plugin.Request{Method: "POST", Path: "/say", Body: []byte(`<p><span data-mentionable="bots:1">@Helper</span> summarise this thread</p>`), Actor: a.pluginCallerFor(mia.User, "")}
	if _, err := a.plugins.Serve(ctx, "talk", "say", req, mia); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-replies:
		if r.Ref != "77" || r.As.ID != "bots:1" || r.For.ID != mia.User.ID || r.Via != "Bots" || !strings.Contains(r.HTML, "On it") {
			t.Fatalf("talk's reply: %+v", r)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("talk never got the reply")
	}
	if c := <-callers; c == nil || c.ID != mia.User.ID {
		t.Fatalf("the relayed mention's caller was %+v", c)
	}
}

// A plugin's own event reaches the plugins that hook it by name, or with
// "*." any plugin's; never the core's.
func TestPluginEmitAndWildcardHooks(t *testing.T) {
	got := make(chan string, 4)
	hear := func(key string, hooks ...string) *plugin.Module {
		return &plugin.Module{Manifest: plugin.Manifest{Key: key, Name: key, Version: "1", Hooks: hooks},
			OnEvent: func(_ context.Context, _ *plugin.API, event string, _ json.RawMessage) error {
				got <- key + " " + event
				return nil
			}}
	}
	sender := &plugin.Module{Manifest: plugin.Manifest{Key: "sender", Name: "Sender", Version: "1",
		Routes: []plugin.Route{{Method: "POST", Path: "/go", Export: "go"}}},
		Handlers: map[string]plugin.Handler{"go": func(ctx context.Context, api *plugin.API, _ *plugin.Request) (*plugin.Response, error) {
			if err := api.Emit(ctx, "other.approval.requested", nil); err == nil {
				t.Error("a plugin emitted another's event")
			}
			return plugin.JSON(200, map[string]any{}), api.Emit(ctx, "sender.approval.requested", map[string]any{"id": "1"})
		}}}
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Plugins: []*plugin.Module{sender, hear("byname", "sender.approval.requested"), hear("anyone", "*.approval.requested"), hear("deaf", "*.approval.withdrawn")}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	a.Handler()
	ctx := t.Context()
	for _, key := range []string{"sender", "byname", "anyone", "deaf"} {
		if _, err := a.plugins.Install(ctx, nil, key); err != nil {
			t.Fatal(err)
		}
		on := true
		if _, err := a.plugins.Configure(ctx, nil, key, nil, &on); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := a.plugins.Serve(ctx, "sender", "go", &plugin.Request{Method: "POST"}, nil); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for range 2 {
		select {
		case s := <-got:
			seen[s] = true
		case <-time.After(5 * time.Second):
			t.Fatalf("heard only %v", seen)
		}
	}
	if !seen["byname sender.approval.requested"] || !seen["anyone sender.approval.requested"] {
		t.Fatalf("heard %v", seen)
	}
	select {
	case s := <-got:
		t.Fatalf("also heard %s", s)
	case <-time.After(200 * time.Millisecond):
	}
}

func must(b []byte, _ error) []byte { return b }
