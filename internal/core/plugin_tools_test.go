package core

import (
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// A plugin's tools are its caller's: a member is never offered, and can't
// run, an owner's tool through a plugin, and what runs is recorded as the
// person's agent via the plugin.
func TestPluginToolsAreTheCallers(t *testing.T) {
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	a.Handler()
	ctx := t.Context()
	person := func(email, role string) *Actor {
		res, err := a.store.ExecContext(ctx, `INSERT INTO users (email_address, password_digest, name, role, created_at, updated_at) VALUES (?, 'x', ?, ?, datetime('now'), datetime('now'))`, email, email, role)
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		u, err := a.loadUser(ctx, id)
		if err != nil || u == nil {
			t.Fatal(err)
		}
		return &Actor{User: u}
	}
	owner, member := person("o@example.com", "owner"), person("m@example.com", "member")
	env := pluginEnv{a}
	names := func(actor *Actor) []string {
		tools, err := env.Tools(ctx, actor)
		if err != nil {
			t.Fatal(err)
		}
		var out []string
		for _, tl := range tools {
			out = append(out, tl.Name)
		}
		return out
	}
	if !slices.Contains(names(owner), "switch_plugin") {
		t.Fatal("the owner isn't offered switch_plugin")
	}
	if slices.Contains(names(member), "switch_plugin") || slices.Contains(names(member), "api_request") {
		t.Fatal("a member is offered an owner's tool")
	}
	if _, err := env.RunTool(ctx, member, "AI", "list_plugins", nil); err == nil || !strings.Contains(err.Error(), "no tool list_plugins") {
		t.Fatalf("a member ran list_plugins: %v", err)
	}
	out, err := env.RunTool(ctx, owner, "AI", "list_plugins", nil)
	if err != nil || out["status"] != "ok" {
		t.Fatalf("the owner's list_plugins: %v %v", out, err)
	}
	out, err = env.RunTool(ctx, member, "AI", "create_client", map[string]any{"client": map[string]any{"name": "Acme"}})
	if err != nil || out["status"] != "ok" {
		t.Fatalf("the member's create_client: %v %v", out, err)
	}
	var actor, source string
	if err := a.store.QueryRowContext(ctx, `SELECT actor, source FROM events WHERE kind = 'client.created' ORDER BY id DESC LIMIT 1`).Scan(&actor, &source); err != nil {
		t.Fatal(err)
	}
	if actor != "AI" || source != "agent" {
		t.Fatalf("recorded as %q from %q", actor, source)
	}
	var live int
	if err := a.store.QueryRowContext(ctx, `SELECT COUNT(*) FROM access_tokens WHERE kind = 'assistant' AND revoked_at IS NULL`).Scan(&live); err != nil || live != 0 {
		t.Fatalf("%d assistant tokens left live (%v)", live, err)
	}
	// The plugin API sees the same: a Caller's tools come through Env.
	var _ plugin.Env = env
}
