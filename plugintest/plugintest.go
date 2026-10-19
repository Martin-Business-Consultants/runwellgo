// Package plugintest runs a plugin in a test the way Runwell runs it: a
// host (plugin.Host) on a real, freshly made database, with the core
// played by Env, a stand-in that answers just enough. Embed Env in a type
// of your own to answer what a test needs (a record, a tool, a setting);
// add Publish, Online, Notify and RichText (plugin.LiveEnv) or Metrics
// (plugin.MetricsEnv) to it when the plugin uses them.
//
//	h, db := plugintest.Setup(t, plugintest.Env{}, mine.Module())
//	res, err := h.Serve(t.Context(), mine.Key, "list", &plugin.Request{Method: "GET", Actor: someone}, nil)
//
// Like package plugin, this only grows within a major version of Runwell.
package plugintest

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// Env is just enough Runwell to install a plugin, switch it on and hand it
// events: no settings kept, no records, no tools, no files, mail that goes
// nowhere, and secrets "sealed" as they are.
type Env struct{}

var _ plugin.Env = Env{}

// Setting answers an empty setting.
func (Env) Setting(context.Context, string) (map[string]any, error) {
	return map[string]any{}, nil
}

// ReplaceSetting keeps nothing.
func (Env) ReplaceSetting(context.Context, string, map[string]any) error { return nil }

// Site is an install called Runwell.
func (Env) Site(context.Context) (plugin.Site, error) {
	return plugin.Site{Name: "Runwell"}, nil
}

// SendMail sends nothing.
func (Env) SendMail(context.Context, plugin.MailMessage) error { return nil }

// Records finds none.
func (Env) Records(context.Context, plugin.Query) ([]map[string]any, error) {
	return nil, nil
}

// Write does nothing, and says it worked.
func (Env) Write(context.Context, plugin.Actor, string, any) (map[string]any, error) {
	return map[string]any{}, nil
}

// Audit records nothing.
func (Env) Audit(context.Context, plugin.Actor, string, int64, string, map[string]any) {}

// AttachFile refuses.
func (Env) AttachFile(context.Context, string, int64, string, string, string, io.Reader) (*plugin.File, error) {
	return nil, errors.New("plugintest: no files")
}

// Attachments are none.
func (Env) Attachments(context.Context, string, int64, string) ([]plugin.File, error) {
	return nil, nil
}

// OpenFile finds none.
func (Env) OpenFile(context.Context, string, int64, int64) (io.ReadCloser, *plugin.File, error) {
	return nil, nil, errors.New("plugintest: no files")
}

// DetachFile does nothing.
func (Env) DetachFile(context.Context, string, int64, int64) error { return nil }

// Tools are none.
func (Env) Tools(context.Context, plugin.Actor) ([]plugin.ToolInfo, error) { return nil, nil }

// RunTool refuses.
func (Env) RunTool(context.Context, plugin.Actor, string, string, map[string]any) (map[string]any, error) {
	return nil, errors.New("plugintest: no tools")
}

// Seal keeps plain as it is.
func (Env) Seal(plain string) string { return plain }

// Unseal answers sealed as it is.
func (Env) Unseal(sealed string) (string, bool) { return sealed, true }

// Open opens (making it when needed) a database at path, with Runwell's
// tables, as the core opens its own.
func Open(path string) (*plugin.DB, error) { return store.Open(path) }

// DB is a new database in the test's temporary directory, closed when the
// test ends.
func DB(tb testing.TB) *plugin.DB {
	tb.Helper()
	db, err := Open(filepath.Join(tb.TempDir(), "test.sqlite3"))
	if err != nil {
		tb.Fatal(err)
	}
	tb.Cleanup(func() { db.Close() })
	return db
}

// Inline is a plugin.Spawn that runs fn at once, so what a plugin does in
// the background (an event's hook, a task) is done when the call returns.
func Inline(ctx context.Context, _ string, fn func(context.Context)) bool {
	fn(ctx)
	return true
}

// Host is a host with modules on db, logging nowhere and running
// background work inline, with what's installed loaded.
func Host(tb testing.TB, env plugin.Env, db *plugin.DB, modules ...*plugin.Module) *plugin.Host {
	tb.Helper()
	h := plugin.New(env, db, slog.New(slog.NewTextHandler(io.Discard, nil)), Inline, modules...)
	if err := h.LoadAll(tb.Context()); err != nil {
		tb.Fatal(err)
	}
	return h
}

// Enable installs m on h, grants it everything its manifest asks for and
// switches it on, as an owner does in Settings › Plugins.
func Enable(tb testing.TB, h *plugin.Host, m *plugin.Module) {
	tb.Helper()
	if _, err := h.Install(tb.Context(), nil, m.Manifest.Key); err != nil {
		tb.Fatal(err)
	}
	grants, on := m.Manifest.Permissions, true
	if _, err := h.Configure(tb.Context(), nil, m.Manifest.Key, &grants, &on); err != nil {
		tb.Fatal(err)
	}
}

// Setup is a host with m installed, granted and on, on a new database:
// Host and Enable in one.
func Setup(tb testing.TB, env plugin.Env, m *plugin.Module) (*plugin.Host, *plugin.DB) {
	tb.Helper()
	db := DB(tb)
	h := Host(tb, env, db, m)
	Enable(tb, h, m)
	return h, db
}

// UserRemoved delivers the core's user.removed for id (named name),
// handing what was theirs to successor, as the core does after commit.
func UserRemoved(h *plugin.Host, id, successor int64, name, successorName string) {
	h.Emit(context.Background(), "user.removed", map[string]any{"id": 99, "subject_type": "User", "subject_id": id, "kind": "user.removed",
		"payload": map[string]any{"id": id, "name": name, "successor_id": successor, "successor_name": successorName}, "source": "app"})
}
