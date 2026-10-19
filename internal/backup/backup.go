// Package backup restores a Runwell backup: the .zip the Rails core's
// Settings › Export › "Back up for Runwell Go" makes (Export kind backup).
// It holds manifest.json, the database as production.sqlite3 (a consistent
// copy) and every stored file at storage/<key[0,2]>/<key[2,2]>/<key>, Active
// Storage's own layout, so restoring is putting them where they were: the
// database opens here as it is.
//
// It happens in two steps, so nothing changes until the backup has checked
// out and the database isn't open:
//
//	Stage    reads the .zip, checks it (its manifest, its paths, the
//	         database's integrity, that it has people to sign in) and unpacks
//	         it into DATA_DIR/import-<time>/, marking it pending.
//	Apply    with nothing using the data directory (at the start of serve, or
//	         the import command), moves what's there aside into
//	         DATA_DIR/replaced-<time>/ and the staged backup into place.
//
// Nothing is deleted: the install that was replaced stays in replaced-<time>/.
package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Format is what a backup's manifest says it is.
const Format = "runwell-backup"

// Database is the database's name in a backup and in the data directory.
const Database = "production.sqlite3"

// pendingFile, in the data directory, names the staged import to apply.
const pendingFile = "import-pending"

// ErrInvalid is a backup that can't be restored: not a Runwell backup, or
// damaged.
var ErrInvalid = errors.New("this isn't a Runwell backup that can be restored")

// Manifest is a backup's manifest.json.
type Manifest struct {
	Format         string `json:"format"`
	FormatVersion  int    `json:"format_version"`
	RunwellVersion string `json:"runwell_version"`
	CreatedAt      string `json:"created_at"`
	Host           string `json:"host"`
	By             string `json:"by"`
	Tables         int    `json:"tables"`
	Rows           int    `json:"rows"`
	Files          int    `json:"files"`
}

// Report is what a staged backup holds, as checked here.
type Report struct {
	// Kind is backup (the database itself) or data (a data export rebuilt
	// into one: no passwords, so everyone got the one given).
	Kind     string   `json:"kind"`
	Manifest Manifest `json:"manifest"`
	People   int      `json:"people"`
	Clients  int      `json:"clients"`
	Files    int      `json:"files"`
	Bytes    int64    `json:"bytes"`
	// Skipped are a data export's tables this Runwell doesn't have (a Rails
	// plugin's), left out.
	Skipped []string `json:"skipped,omitempty"`
}

// Options are what an import may need besides the file.
type Options struct {
	// Password is everyone's in a data export, which leaves passwords out.
	Password string
	// PluginTables makes the built-in plugins' own tables in a data
	// export's new database, so their rows (a Rails plugin's, kept under the
	// same names) come in with the core's; nil leaves them out.
	PluginTables func(ctx context.Context, db *store.DB) error
}

// ErrNeedsPassword is a data export imported without a password for the
// people in it.
var ErrNeedsPassword = errors.New("this is a data export, which leaves passwords out: give the password everyone in it signs in with")

// storagePath is where a stored file sits: storage/ab/cd/abcd…
var storagePath = regexp.MustCompile(`^storage/([a-z0-9]{2})/([a-z0-9]{2})/([a-z0-9]{4,})$`)

// blobDir is one of Active Storage's first-level folders in a data directory.
var blobDir = regexp.MustCompile(`^[a-z0-9]{2}$`)

// Stage checks the backup in the .zip at archive and unpacks it into the
// data directory, ready for Apply. Nothing in use is touched.
func Stage(ctx context.Context, dataDir, archive string, opts Options) (*Report, error) {
	zr, err := zip.OpenReader(archive)
	if err != nil {
		return nil, fmt.Errorf("%w: it isn't a .zip (%w)", ErrInvalid, err)
	}
	defer zr.Close()

	var manifest *zip.File
	var database *zip.File
	isExport := false
	for _, f := range zr.File {
		switch f.Name {
		case "manifest.json":
			manifest = f
		case Database:
			database = f
		case "data/users.json":
			isExport = true
		}
	}
	if (manifest == nil || database == nil) && isExport {
		return stageExport(ctx, dataDir, &zr.Reader, opts)
	}
	if manifest == nil || database == nil {
		return nil, fmt.Errorf("%w: it has neither a backup's manifest.json and %s nor a data export's data/ folder", ErrInvalid, Database)
	}
	var m Manifest
	if err := readJSON(manifest, &m); err != nil {
		return nil, fmt.Errorf("%w: manifest.json: %w", ErrInvalid, err)
	}
	if m.Format != Format || m.FormatVersion != 1 {
		return nil, fmt.Errorf("%w: it says it's %q version %d, not %s version 1", ErrInvalid, m.Format, m.FormatVersion, Format)
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

	rep := &Report{Kind: "backup", Manifest: m}
	for _, f := range zr.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		name := f.Name
		var to string
		switch {
		case name == Database:
			to = filepath.Join(stage, Database)
		case storagePath.MatchString(name):
			parts := storagePath.FindStringSubmatch(name)
			if !strings.HasPrefix(parts[3], parts[1]+parts[2]) {
				return nil, fmt.Errorf("%w: %s isn't where its key says", ErrInvalid, name)
			}
			to = filepath.Join(stage, "storage", parts[1], parts[2], parts[3])
			rep.Files++
		default:
			// The manifest (read above), the README, directories, and
			// anything that's no part of a backup: skipped, never written.
			continue
		}
		n, err := extract(f, to)
		if err != nil {
			return nil, err
		}
		rep.Bytes += n
	}
	if err := check(ctx, filepath.Join(stage, Database), rep); err != nil {
		return nil, err
	}
	if err := markPending(dataDir, stage, rep); err != nil {
		return nil, err
	}
	ok = true
	return rep, nil
}

// markPending says which staged import Apply puts in place.
func markPending(dataDir, stage string, rep *Report) error {
	raw, _ := json.Marshal(map[string]any{"stage": filepath.Base(stage), "report": rep})
	return os.WriteFile(filepath.Join(dataDir, pendingFile), raw, 0o640) //nolint:gosec // 0640, as the database is: never world-readable
}

func readJSON(f *zip.File, v any) error {
	r, err := f.Open()
	if err != nil {
		return err
	}
	defer r.Close()
	return json.NewDecoder(io.LimitReader(r, 1<<20)).Decode(v)
}

// extract writes one entry to path, answering its size.
func extract(f *zip.File, to string) (int64, error) {
	if err := os.MkdirAll(filepath.Dir(to), 0o750); err != nil {
		return 0, err
	}
	r, err := f.Open()
	if err != nil {
		return 0, fmt.Errorf("%w: %s: %w", ErrInvalid, f.Name, err)
	}
	defer r.Close()
	w, err := os.OpenFile(to, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o640) //nolint:gosec // 0640, as the database and blobs are
	if err != nil {
		return 0, err
	}
	n, err := io.Copy(w, r) //nolint:gosec // the operator's own backup, every byte of it wanted
	if cerr := w.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return n, fmt.Errorf("%w: %s: %w", ErrInvalid, f.Name, err)
	}
	return n, nil
}

// check opens the staged database: it must be whole, and Runwell's, with
// someone to sign in.
func check(ctx context.Context, path string, rep *Report) error {
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return err
	}
	defer db.Close()
	var integrity string
	if err := db.QueryRowContext(ctx, `PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		return fmt.Errorf("%w: the database is damaged (%s%v)", ErrInvalid, integrity, errOr(err))
	}
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM users WHERE agent_owner_id IS NULL AND deactivated_at IS NULL`).Scan(&rep.People); err != nil {
		return fmt.Errorf("%w: the database isn't Runwell's (%w)", ErrInvalid, err)
	}
	if rep.People == 0 {
		return fmt.Errorf("%w: it has nobody who could sign in", ErrInvalid)
	}
	_ = db.QueryRowContext(ctx, `SELECT count(*) FROM clients`).Scan(&rep.Clients)
	return nil
}

func errOr(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

// Pending is the staged backup waiting to be applied, nil when there's none.
func Pending(dataDir string) (*Report, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, pendingFile)) //nolint:gosec // the data directory's own pending file
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	var p struct {
		Report *Report `json:"report"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, err
	}
	return p.Report, nil
}

// Discard drops a staged backup that hasn't been applied.
func Discard(dataDir string) error {
	raw, err := os.ReadFile(filepath.Join(dataDir, pendingFile)) //nolint:gosec // the data directory's own pending file
	if errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	var p struct{ Stage string }
	_ = json.Unmarshal(raw, &p)
	if p.Stage != "" && !strings.ContainsAny(p.Stage, `/\`) {
		_ = os.RemoveAll(filepath.Join(dataDir, p.Stage))
	}
	return os.Remove(filepath.Join(dataDir, pendingFile))
}

// Apply puts the staged backup in place, when there is one: the database
// and the stored files there now move to DATA_DIR/replaced-<time>/, the
// backup's take their place. Nothing may have the database open. It answers
// where the replaced install went, "" when nothing was pending.
func Apply(dataDir string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(dataDir, pendingFile)) //nolint:gosec // the data directory's own pending file
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	} else if err != nil {
		return "", err
	}
	var p struct {
		Stage  string
		Report *Report
	}
	if err := json.Unmarshal(raw, &p); err != nil || p.Stage == "" || strings.ContainsAny(p.Stage, `/\`) {
		return "", fmt.Errorf("the pending import (%s) is unreadable: remove it to start as before", pendingFile)
	}
	stage := filepath.Join(dataDir, p.Stage)
	if _, err := os.Stat(filepath.Join(stage, Database)); err != nil {
		return "", fmt.Errorf("the staged backup %s is incomplete: remove %s to start as before", p.Stage, pendingFile)
	}
	aside := filepath.Join(dataDir, "replaced-"+time.Now().UTC().Format("20060102-150405"))
	if err := os.MkdirAll(aside, 0o750); err != nil {
		return "", err
	}
	// The install there now: its database (with SQLite's journal files) and
	// its stored files' folders.
	entries, err := os.ReadDir(dataDir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, Database) || (e.IsDir() && blobDir.MatchString(name)) {
			if err := os.Rename(filepath.Join(dataDir, name), filepath.Join(aside, name)); err != nil {
				return "", fmt.Errorf("moving %s aside: %w", name, err)
			}
		}
	}
	// The backup's.
	if err := os.Rename(filepath.Join(stage, Database), filepath.Join(dataDir, Database)); err != nil {
		return "", fmt.Errorf("putting the database in place: %w (the old one is in %s)", err, aside)
	}
	folders, err := os.ReadDir(filepath.Join(stage, "storage"))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	for _, f := range folders {
		if f.IsDir() && blobDir.MatchString(f.Name()) {
			if err := os.Rename(filepath.Join(stage, "storage", f.Name()), filepath.Join(dataDir, f.Name())); err != nil {
				return "", fmt.Errorf("putting files in place: %w", err)
			}
		}
	}
	_ = os.RemoveAll(stage)
	if err := os.Remove(filepath.Join(dataDir, pendingFile)); err != nil {
		return "", err
	}
	// The search index is the database's own in a backup; a data export
	// leaves it out, so it's rebuilt once Runwell starts.
	if p.Report != nil && p.Report.Kind == "data" {
		if err := os.WriteFile(filepath.Join(dataDir, ReindexFile), nil, 0o640); err != nil { //nolint:gosec // 0640, as the database is
			return "", err
		}
	}
	return aside, nil
}

// HasPeople is whether the data directory's database has anyone in it: an
// install an import would replace.
func HasPeople(dataDir string) bool {
	path := filepath.Join(dataDir, Database)
	if _, err := os.Stat(path); err != nil {
		return false
	}
	db, err := sql.Open("sqlite", "file:"+path+"?mode=ro")
	if err != nil {
		return false
	}
	defer db.Close()
	var n int
	_ = db.QueryRowContext(context.Background(), `SELECT count(*) FROM users`).Scan(&n) // unreadable is no one
	return n > 0
}
