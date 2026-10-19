package backup

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// A data export's plugin tables come in when the importer is told how to
// make them, and are skipped (as before) when it isn't.
func TestDataExportBringsPluginTables(t *testing.T) {
	zipPath := filepath.Join(t.TempDir(), "runwell-export.zip")
	f, err := os.Create(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	put := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	put("README.txt", "Acme: everything in Runwell, exported October 18, 2026 14:00 by Ted.\n")
	put("data/users.json", `[{"id":1,"email_address":"ted@example.com","name":"Ted","role":"owner","created_at":"2026-10-18 14:00:00","updated_at":"2026-10-18 14:00:00"}]`)
	put("data/demo_sites.json", `[{"id":7,"name":"Acme site"}]`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}

	makeTables := func(ctx context.Context, db *store.DB) error {
		_, err := db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS demo_sites (id integer PRIMARY KEY, name varchar)`)
		return err
	}
	for _, tc := range []struct {
		name  string
		opts  Options
		sites int
	}{
		{"with plugin tables", Options{Password: "password1", PluginTables: makeTables}, 1},
		{"without", Options{Password: "password1"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rep, err := Stage(t.Context(), t.TempDir(), zipPath, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			skipped := slices.Contains(rep.Skipped, "demo_sites")
			if skipped != (tc.sites == 0) {
				t.Fatalf("demo_sites skipped: %v (skipped %v)", skipped, rep.Skipped)
			}
			if tc.sites > 0 && rep.Manifest.Rows != 2 {
				t.Fatalf("rows %d, want 2 (a person and a site)", rep.Manifest.Rows)
			}
		})
	}
}
