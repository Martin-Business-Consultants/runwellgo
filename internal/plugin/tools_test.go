package plugin

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// fakeEnv offers two tools, one that looks and one that changes, and keeps
// what ran.
type fakeEnv struct {
	ran []string
}

func (e *fakeEnv) Setting(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}
func (e *fakeEnv) ReplaceSetting(context.Context, string, map[string]any) error { return nil }
func (e *fakeEnv) Site(context.Context) (Site, error)                           { return Site{Name: "Runwell"}, nil }
func (e *fakeEnv) SendMail(context.Context, mail.Message) error                 { return nil }
func (e *fakeEnv) Records(context.Context, Query) ([]map[string]any, error)     { return nil, nil }
func (e *fakeEnv) Write(context.Context, Actor, string, any) (map[string]any, error) {
	return nil, nil
}
func (e *fakeEnv) Audit(context.Context, Actor, string, int64, string, map[string]any) {}
func (e *fakeEnv) AttachFile(context.Context, string, int64, string, string, string, io.Reader) (*File, error) {
	return nil, nil
}
func (e *fakeEnv) Attachments(context.Context, string, int64, string) ([]File, error) {
	return nil, nil
}
func (e *fakeEnv) OpenFile(context.Context, string, int64, int64) (io.ReadCloser, *File, error) {
	return nil, nil, nil
}
func (e *fakeEnv) DetachFile(context.Context, string, int64, int64) error { return nil }
func (e *fakeEnv) Tools(_ context.Context, actor Actor) ([]ToolInfo, error) {
	if actor == nil {
		return nil, errors.New("no one")
	}
	return []ToolInfo{{Name: "search", ReadOnly: true}, {Name: "add_note"}}, nil
}
func (e *fakeEnv) RunTool(_ context.Context, _ Actor, via, name string, _ map[string]any) (map[string]any, error) {
	e.ran = append(e.ran, name+" via "+via)
	return map[string]any{"status": "ok"}, nil
}
func (e *fakeEnv) Seal(plain string) string { return "sealed:" + plain }
func (e *fakeEnv) Unseal(sealed string) (string, bool) {
	return strings.TrimPrefix(sealed, "sealed:"), strings.HasPrefix(sealed, "sealed:")
}

// toolsHost is a host with one plugin whose routes run a tool: name from
// the query, at once (/run) or in the background (/later).
func toolsHost(t *testing.T, grants Grants) (*Host, *fakeEnv) {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	run := func(ctx context.Context, api *API, req *Request) (*Response, error) {
		if _, err := api.RunTool(ctx, req.Actor, req.Query.Get("tool"), nil); err != nil {
			return JSON(422, map[string]any{"message": err.Error()}), nil //nolint:nilerr // answered as a 422
		}
		return JSON(200, map[string]any{}), nil
	}
	var later error
	m := &Module{
		Manifest: Manifest{Key: "toolish", Name: "Toolish", Version: "1", Permissions: Grants{Tools: true, Secrets: true, PrivateHTTP: true},
			Routes: []Route{{Method: "POST", Path: "/run", Export: "run"}, {Method: "GET", Path: "/run", Export: "run"}, {Method: "POST", Path: "/later", Export: "later"}}},
		Handlers: map[string]Handler{"run": run, "later": func(ctx context.Context, api *API, req *Request) (*Response, error) {
			err := api.Background("later", func(ctx context.Context, api *API) {
				_, later = api.RunTool(ctx, req.Actor, "add_note", nil)
			})
			if err != nil {
				return nil, err
			}
			if later != nil {
				return JSON(422, map[string]any{"message": later.Error()}), nil //nolint:nilerr // answered as a 422
			}
			return JSON(200, map[string]any{}), nil
		}},
	}
	env := &fakeEnv{}
	spawn := func(ctx context.Context, _ string, fn func(context.Context)) bool { fn(ctx); return true }
	h := New(env, db, slog.New(slog.NewTextHandler(io.Discard, nil)), spawn, m)
	if err := h.LoadAll(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err := h.Install(t.Context(), nil, "toolish"); err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := h.Configure(t.Context(), nil, "toolish", &grants, &on); err != nil {
		t.Fatal(err)
	}
	return h, env
}

func call(t *testing.T, h *Host, method, export, tool string, caller *Caller) string {
	t.Helper()
	req := &Request{Method: method, Path: "/" + export, Actor: caller}
	req.Query = map[string][]string{"tool": {tool}}
	res, err := h.Serve(t.Context(), "toolish", export, req, "the person")
	if err != nil {
		t.Fatal(err)
	}
	return string(res.Body)
}

// A tool that looks runs as the caller whenever; a change only in the
// person's own write request, never in the background, a GET or an agent's
// call; and nothing without the grant.
func TestRunToolNeedsThePersonsRequest(t *testing.T) {
	t.Parallel()
	ted := &Caller{ID: 1, Name: "Ted", Role: "owner"}
	h, env := toolsHost(t, Grants{Tools: true})
	if got := call(t, h, "GET", "run", "search", ted); got != "{}" {
		t.Fatalf("a read in a GET: %s", got)
	}
	if got := call(t, h, "GET", "run", "add_note", ted); !strings.Contains(got, "approves it") {
		t.Fatalf("a change in a GET ran: %s", got)
	}
	if got := call(t, h, "POST", "later", "", ted); !strings.Contains(got, "approves it") {
		t.Fatalf("a change in the background ran: %s", got)
	}
	agent := &Caller{ID: 1, Name: "Ted", Role: "owner", Agent: true}
	if got := call(t, h, "POST", "run", "add_note", agent); !strings.Contains(got, "approves it") {
		t.Fatalf("an agent's change ran: %s", got)
	}
	if got := call(t, h, "POST", "run", "add_note", ted); got != "{}" {
		t.Fatalf("the person's change: %s", got)
	}
	if got := call(t, h, "POST", "run", "delete_everything", ted); !strings.Contains(got, "no tool delete_everything") {
		t.Fatalf("a tool not offered: %s", got)
	}
	if strings.Join(env.ran, ",") != "search via Toolish,add_note via Toolish" {
		t.Fatalf("ran %v", env.ran)
	}

	h2, env2 := toolsHost(t, Grants{})
	if got := call(t, h2, "POST", "run", "search", ted); !strings.Contains(got, "tools wasn't granted") {
		t.Fatalf("ungranted: %s", got)
	}
	if len(env2.ran) > 0 {
		t.Fatalf("ran %v without the grant", env2.ran)
	}
}

// Tools run only as the route's own caller, never someone the plugin names.
func TestRunToolOnlyAsTheCaller(t *testing.T) {
	t.Parallel()
	h, env := toolsHost(t, Grants{Tools: true})
	p, api := h.enabled("toolish", "the person")
	if p == nil {
		t.Fatal("not on")
	}
	api.caller = &Caller{ID: 1}
	if _, err := api.RunTool(t.Context(), &Caller{ID: 2, Role: "owner"}, "search", nil); err == nil || !strings.Contains(err.Error(), "only as the person") {
		t.Fatalf("someone else: %v", err)
	}
	contact := &Contact{ID: 5}
	if _, err := api.RunPortalTool(t.Context(), contact, "search", nil); err == nil {
		t.Fatal("a staff route ran a contact's tool")
	}
	if len(env.ran) > 0 {
		t.Fatalf("ran %v", env.ran)
	}
}

// Switching a plugin on grants what it asks for, but never this server's
// network: an owner ticks that.
func TestSwitchingOnNeverGrantsPrivateHTTP(t *testing.T) {
	t.Parallel()
	h, _ := toolsHost(t, Grants{})
	off := false
	if _, err := h.Configure(t.Context(), nil, "toolish", nil, &off); err != nil {
		t.Fatal(err)
	}
	on := true
	p, err := h.Configure(t.Context(), nil, "toolish", nil, &on)
	if err != nil {
		t.Fatal(err)
	}
	if !p.Granted.Tools || !p.Granted.Secrets || p.Granted.PrivateHTTP {
		t.Fatalf("granted %+v", p.Granted)
	}
	_, api := h.enabled("toolish", nil)
	if _, err := api.PrivateHTTP(); err == nil {
		t.Fatal("private HTTP without the grant")
	}
	sealed, err := api.Seal("sk-1")
	if err != nil || sealed != "sealed:sk-1" {
		t.Fatalf("seal %q %v", sealed, err)
	}
	if plain, err := api.Unseal(sealed); err != nil || plain != "sk-1" {
		t.Fatalf("unseal %q %v", plain, err)
	}
	if _, err := (&API{}).Seal("x"); err == nil {
		t.Fatal("sealed without the grant")
	}
}

// A portal route needs the portal grant, and can't be public or a tool.
func TestPortalRouteValidation(t *testing.T) {
	t.Parallel()
	h := func(context.Context, *API, *Request) (*Response, error) { return nil, nil }
	m := &Module{Manifest: Manifest{Key: "porty", Name: "P", Version: "1", Routes: []Route{{Method: "GET", Path: "/a", Export: "a", Portal: true}}},
		Handlers: map[string]Handler{"a": h}}
	if err := m.validate(); err == nil || !strings.Contains(err.Error(), "permissions.portal") {
		t.Fatalf("err %v", err)
	}
	m.Manifest.Permissions.Portal = true
	m.Manifest.Routes[0].Public = true
	if err := m.validate(); err == nil || !strings.Contains(err.Error(), "can't be public") {
		t.Fatalf("err %v", err)
	}
	m.Manifest.Routes[0].Public = false
	if err := m.validate(); err != nil {
		t.Fatal(err)
	}
}
