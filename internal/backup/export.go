package backup

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// A data export is the Rails core's other kind of export (Settings › Export
// › Make an export): every table as JSON, rows as the database holds them,
// and every stored file as files/<blob id>-<name>. It leaves out what a
// sign-in needs (passwords, sessions, tokens) and the search index, so it's
// rebuilt into a database here: a fresh one with this Runwell's schema, each
// table's rows put back with their ids, everyone given the password the
// import was given, and the search index rebuilt once Runwell starts
// (ReindexFile). Good for trying a live install out on a laptop; a backup
// moves one for real.

// ReindexFile is the file in the data directory that asks the next start
// to rebuild the search index (an imported data export has none).
const ReindexFile = "reindex-pending"

// exportFile is a stored file in a data export: files/<blob id>-<name>.
var exportFile = regexp.MustCompile(`^files/(\d+)-`)

// exportTable is a table in a data export: data/<table>.json.
var exportTable = regexp.MustCompile(`^data/([a-z0-9_]+)\.json$`)

func stageExport(ctx context.Context, dataDir string, zr *zip.Reader, opts Options) (*Report, error) {
	if len(opts.Password) < 8 {
		if opts.Password == "" {
			return nil, ErrNeedsPassword
		}
		return nil, fmt.Errorf("%w: the password needs at least 8 characters", ErrNeedsPassword)
	}
	digest, err := secret.HashPassword(opts.Password)
	if err != nil {
		return nil, err
	}
	stage := filepath.Join(dataDir, "import-"+time.Now().UTC().Format("20060102-150405"))
	if err := os.MkdirAll(stage, 0o750); err != nil {
		return nil, err
	}
	ok := false
	defer func() {
		if !ok {
			_ = os.RemoveAll(stage)
		}
	}()

	rep := &Report{Kind: "data", Manifest: Manifest{Format: "runwell-export", FormatVersion: 1}}
	readme(zr, &rep.Manifest)
	db, err := store.Open(filepath.Join(stage, Database))
	if err != nil {
		return nil, err
	}
	known := store.Tables()
	if opts.PluginTables != nil {
		if err := opts.PluginTables(ctx, db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("making the plugins' tables: %w", err)
		}
		plugins, err := pluginTableNames(ctx, db, known)
		if err != nil {
			_ = db.Close()
			return nil, err
		}
		known = append(slices.Clone(known), plugins...)
	}
	blobKeys := map[string]string{}
	err = db.Tx(ctx, func(tx *sql.Tx) error {
		for _, f := range zr.File {
			m := exportTable.FindStringSubmatch(f.Name)
			if m == nil {
				continue
			}
			table := m[1]
			if !slices.Contains(known, table) {
				rep.Skipped = append(rep.Skipped, table)
				continue
			}
			var rows []map[string]any
			if err := readRows(f, &rows); err != nil {
				return fmt.Errorf("%w: %s: %w", ErrInvalid, f.Name, err)
			}
			if err := insertRows(ctx, tx, table, rows, digest); err != nil {
				return fmt.Errorf("%s: %w", table, err)
			}
			rep.Manifest.Rows += len(rows)
			rep.Manifest.Tables++
			if table == "active_storage_blobs" {
				for _, r := range rows {
					blobKeys[fmt.Sprint(r["id"])] = fmt.Sprint(r["key"])
				}
			}
		}
		return nil
	})
	if cerr := db.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return nil, err
	}
	// The files, where their blob's key says.
	for _, f := range zr.File {
		m := exportFile.FindStringSubmatch(f.Name)
		if m == nil {
			continue
		}
		key := blobKeys[m[1]]
		if len(key) < 4 || !regexp.MustCompile(`^[a-z0-9]+$`).MatchString(key) {
			continue // a file with no blob row is no use
		}
		n, err := extract(f, filepath.Join(stage, "storage", key[0:2], key[2:4], key))
		if err != nil {
			return nil, err
		}
		rep.Files++
		rep.Bytes += n
	}
	rep.Manifest.Files = rep.Files
	if err := check(ctx, filepath.Join(stage, Database), rep); err != nil {
		return nil, err
	}
	if err := markPending(dataDir, stage, rep); err != nil {
		return nil, err
	}
	ok = true
	return rep, nil
}

// pluginTableNames are the tables in db beyond the core's: the plugins'.
func pluginTableNames(ctx context.Context, db *store.DB, core []string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%'`)
	if err != nil {
		return nil, err
	}
	names, err := store.ScanAll(rows, func(r *sql.Rows) (string, error) {
		var n string
		return n, r.Scan(&n)
	})
	if err != nil {
		return nil, err
	}
	return slices.DeleteFunc(names, func(n string) bool { return slices.Contains(core, n) }), nil
}

// readme reads what the export's README.txt says of it: whose it is and
// when it was made ("Acme: everything in Runwell, exported October 18,
// 2026 14:00 by Ted Martin.").
func readme(zr *zip.Reader, m *Manifest) {
	for _, f := range zr.File {
		if f.Name != "README.txt" {
			continue
		}
		r, err := f.Open()
		if err != nil {
			return
		}
		defer r.Close()
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		first, _, _ := strings.Cut(buf.String(), "\n")
		if name, rest, ok := strings.Cut(first, ": everything in Runwell, exported "); ok {
			m.Host = name
			when, by, _ := strings.Cut(strings.TrimSuffix(rest, "."), " by ")
			m.CreatedAt, m.By = when, by
		}
		return
	}
}

func readRows(f *zip.File, rows *[]map[string]any) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	dec := json.NewDecoder(r)
	dec.UseNumber()
	return dec.Decode(rows)
}

// column is one of a table's columns, as PRAGMA table_info has it.
type column struct {
	name     string
	notNull  bool
	hasDflt  bool
	isPKey   bool
	declType string
}

// insertRows puts a table's rows back, with their ids. Only the columns
// both sides have go in. A column the export left out (a secret) that can't
// be empty gets something: people's password_digest the import's password,
// anything else a fresh random value (a webhook's signing secret).
func insertRows(ctx context.Context, tx *sql.Tx, table string, rows []map[string]any, digest string) error {
	if len(rows) == 0 {
		return nil
	}
	rs, err := tx.QueryContext(ctx, `SELECT name, type, "notnull", dflt_value IS NOT NULL, pk FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	cols, err := store.ScanAll(rs, func(r *sql.Rows) (column, error) {
		var c column
		var pk int
		err := r.Scan(&c.name, &c.declType, &c.notNull, &c.hasDflt, &pk)
		c.isPKey = pk > 0
		return c, err
	})
	if err != nil {
		return err
	}
	for _, row := range rows {
		var names []string
		var args []any
		for _, c := range cols {
			v, given := row[c.name]
			switch {
			case given:
				args = append(args, value(v))
			case c.name == "password_digest":
				args = append(args, digest)
			case c.notNull && !c.hasDflt && !c.isPKey:
				args = append(args, secret.Token(24))
			default:
				continue
			}
			names = append(names, `"`+c.name+`"`)
		}
		if len(names) == 0 {
			continue
		}
		q := `INSERT INTO "` + table + `" (` + strings.Join(names, ", ") + `) VALUES (` + store.Placeholders(len(names)) + `)` //nolint:gosec // table is one of store.Tables, its columns from its PRAGMA
		if _, err := tx.ExecContext(ctx, q, args...); err != nil {
			return fmt.Errorf("row %v: %w", row["id"], err)
		}
	}
	return nil
}

// value is a JSON value as a column takes it: numbers as numbers, objects
// and lists (a json column read back) as their JSON text.
func value(v any) any {
	switch x := v.(type) {
	case json.Number:
		if n, err := strconv.ParseInt(x.String(), 10, 64); err == nil {
			return n
		}
		f, _ := x.Float64()
		return f
	case bool:
		if x {
			return 1
		}
		return 0
	case map[string]any, []any:
		raw, _ := json.Marshal(x)
		return string(raw)
	}
	return v
}
