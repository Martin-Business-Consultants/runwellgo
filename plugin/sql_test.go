package plugin

import (
	"path/filepath"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

func TestSchemaAddsColumns(t *testing.T) {
	if err := checkSchema("demo", `ALTER TABLE "users" ADD COLUMN "x" text`); err == nil {
		t.Fatal("altered another table")
	}
	if err := checkSchema("demo", `DROP TABLE "demo_items"`); err == nil {
		t.Fatal("dropped a table")
	}
	db, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite3"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	// An older version's table, then the newer schema, twice (each load).
	if _, err := db.ExecContext(t.Context(), `CREATE TABLE "demo_items" ("id" integer PRIMARY KEY)`); err != nil {
		t.Fatal(err)
	}
	schema := `CREATE TABLE IF NOT EXISTS "demo_items" ("id" integer PRIMARY KEY, "note" text);
ALTER TABLE "demo_items" ADD COLUMN "note" text;`
	for range 2 {
		if err := applySchema(t.Context(), db, "demo", schema); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := db.ExecContext(t.Context(), `INSERT INTO demo_items (note) VALUES ('ok')`); err != nil {
		t.Fatal(err)
	}
}
