package core

import (
	"io"
	"log/slog"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Records reads the install's custom fields, and a client's values by key
// (what Reports reads an address from).
func TestPluginRecordsCustomFields(t *testing.T) {
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	ctx := t.Context()
	exec := func(q string, args ...any) int64 {
		res, err := a.store.ExecContext(ctx, q, args...)
		if err != nil {
			t.Fatalf("%s: %v", q, err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	c := exec(`INSERT INTO clients (name, status, created_at, updated_at) VALUES ('Acme', 'active', datetime('now'), datetime('now'))`)
	f := exec(`INSERT INTO custom_fields (model_type, label, key, kind, choices, position, listed, client_visible, created_at, updated_at) VALUES ('Client', 'Address', 'address', 'long_text', '[]', 1, 0, 0, datetime('now'), datetime('now'))`)
	exec(`INSERT INTO custom_fields (model_type, label, key, kind, choices, position, listed, client_visible, created_at, updated_at) VALUES ('Todo', 'Size', 'size', 'text', '[]', 1, 0, 0, datetime('now'), datetime('now'))`)
	exec(`INSERT INTO custom_values (custom_field_id, subject_type, subject_id, value, created_at, updated_at) VALUES (?, 'Client', ?, '1 Main St', datetime('now'), datetime('now'))`, f, c)

	env := pluginEnv{a}
	fields, err := env.Records(ctx, plugin.Query{Type: "CustomField", Status: "Client", Limit: 50})
	if err != nil || len(fields) != 1 || fields[0]["key"] != "address" || fields[0]["label"] != "Address" || fields[0]["kind"] != "long_text" {
		t.Fatalf("fields %v %v", fields, err)
	}
	clients, err := env.Records(ctx, plugin.Query{Type: "Client", ID: c, Limit: 1})
	if err != nil || len(clients) != 1 {
		t.Fatalf("clients %v %v", clients, err)
	}
	if cf, _ := clients[0]["custom_fields"].(map[string]any); cf["address"] != "1 Main St" {
		t.Fatalf("custom fields %#v", clients[0]["custom_fields"])
	}
}
