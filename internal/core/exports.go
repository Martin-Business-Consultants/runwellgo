package core

import (
	"archive/zip"
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Settings › Export: everything this install holds as one .zip, for
// owners: every table, the core's and the plugins', as JSON and CSV, and
// every stored file. Secrets stay out: sign-in sessions, tokens, OAuth
// grants and the search index are skipped, and columns holding passwords,
// secrets, tokens or keys are left out of every table. Built in the
// background, emailed when ready, kept for a week.

func init() {
	routeSets = append(routeSets, (*App).exportRoutes)
	every("exports expiry", 24*time.Hour, func(a *App, ctx context.Context) error { return a.expireExports(ctx) })
}

const exportKeepFor = 7 * 24 * time.Hour

var exportSkipped = []string{"schema_migrations", "ar_internal_metadata", "sessions", "access_tokens", "oauth_clients", "oauth_grants", "portal_sessions",
	"approval_links", "invitations", "identities", "sign_in_providers", "agent_calls", "agent_idempotency_keys",
	"searchable_documents", "active_storage_variant_records", "action_mailbox_inbound_emails", "plugin_changes", "upgrades", "exports",
	"account_tokens", "plugin_kv"}

var exportSecretColumns = regexp.MustCompile(`(?i)password|secret|token|_digest$|^otp_|credential|api_key|private_key|recovery`)

func (a *App) exportRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	a.handle(mux, "GET /api/settings/exports", a.listExports, ms, tool("list_exports", "List exports",
		note("Exports of everything (a .zip of every table as JSON and CSV, and every file), with their state and a download address when ready.")))
	a.handle(mux, "POST /api/settings/exports", a.createExport, ms, tool("create_export", "Export everything",
		note("Starts an export of all data and files as one .zip. It's built in the background: list_exports shows when it's ready."), next("list_exports")))
	a.handle(mux, "GET /api/settings/exports/{id}", a.showExport, ms, tool("download_export", "Download an export",
		note("Answers with the address of a ready export's .zip.")))
	a.handle(mux, "GET /api/settings/exports/{id}/download", a.downloadExport, ms)
}

func (a *App) listExports(c *Ctx) error {
	ctx := c.R.Context()
	rows, err := a.store.QueryContext(ctx, `SELECT x.id, x.state, x.tables_count, x.rows_count, x.files_count, x.error, x.created_at, x.completed_at,
		COALESCE(NULLIF(u.name, ''), u.email_address, '') FROM exports x LEFT JOIN users u ON u.id = x.user_id ORDER BY x.created_at DESC`)
	if err != nil {
		return err
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var state, by string
		var tables, rowsN, files store.NullInt
		var errText *string
		var created, completed store.Time
		if err := r.Scan(&id, &state, &tables, &rowsN, &files, &errText, &created, &completed, &by); err != nil {
			return nil, err
		}
		return O("id", id, "state", state, "tables_count", tables, "rows_count", rowsN, "files_count", files, "error", errText,
			"created_at", created, "completed_at", completed, "by", by), nil
	})
	if err != nil {
		return err
	}
	if list == nil {
		list = []Obj{}
	}
	for i, x := range list {
		var url, size any
		if x.Get("state") == "ready" {
			if b, err := a.attachedBlob(ctx, a.store, "Export", x.Get("id").(int64), "archive"); err != nil {
				return err
			} else if b != nil {
				url = fmt.Sprintf("%s/api/settings/exports/%d/download", a.BaseURL(), x.Get("id"))
				size = b.ByteSize
			}
		}
		list[i] = x.Set("download_url", url).Set("byte_size", size)
	}
	c.Summary = "No exports yet: create_export makes one."
	if len(list) > 0 {
		c.Summary = pluralize(len(list), "export") + ", newest first"
	}
	return c.JSON(200, O("summary", c.Summary, "exports", list))
}

func (a *App) createExport(c *Ctx) error {
	ctx := c.R.Context()
	me := c.Actor.User.Person()
	var id int64
	err := a.write(ctx, c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		var err error
		if id, err = t.Insert(`INSERT INTO exports (user_id, state, created_at, updated_at) VALUES (?, 'queued', ?, ?)`, me.ID, now, now); err != nil {
			return err
		}
		return t.Event("User", me.ID, "user.exported", map[string]any{"export": id})
	})
	if err != nil {
		return err
	}
	a.later("export", func(ctx context.Context) {
		if err := a.expireExports(ctx); err != nil {
			a.log.Warn("exports expiry", "err", err)
		}
		if err := a.buildExport(ctx, id); err != nil {
			a.log.Warn("export", "export", id, "err", err)
		}
	})
	c.Summary = "Export started. It takes a minute or so; we’ll email " + me.Email + " when it’s ready."
	return c.JSON(201, O("summary", c.Summary, "id", id))
}

func (a *App) readyExport(c *Ctx) (*Blob, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, err
	}
	var state string
	if err := a.store.QueryRowContext(c.R.Context(), `SELECT state FROM exports WHERE id = ?`, id).Scan(&state); err != nil {
		if isNoRows(err) {
			return nil, notFound("export")
		}
		return nil, err
	}
	b, err := a.attachedBlob(c.R.Context(), a.store, "Export", id, "archive")
	if err != nil {
		return nil, err
	}
	if state != "ready" || b == nil {
		return nil, &apierr.Error{Status: 422, Code: "not_ready", Message: "That export isn’t ready."}
	}
	return b, nil
}

func (a *App) showExport(c *Ctx) error {
	b, err := a.readyExport(c)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/settings/exports/%s/download", a.BaseURL(), c.Param("id"))
	c.Summary = "Download it at " + url + " (signed in, or with your token)."
	return c.JSON(200, O("summary", c.Summary, "download_url", url, "filename", b.Filename, "byte_size", b.ByteSize))
}

func (a *App) downloadExport(c *Ctx) error {
	b, err := a.readyExport(c)
	if err != nil {
		return err
	}
	a.serveBlob(c.W, c.R, b, true)
	return nil
}

// expireExports removes exports older than a week, with their archives.
func (a *App) expireExports(ctx context.Context) error {
	ids, err := store.Int64s(ctx, a.store, `SELECT id FROM exports WHERE created_at < ?`, store.TimeOf(time.Now().Add(-exportKeepFor)).DB())
	if err != nil || len(ids) == 0 {
		return err
	}
	return a.write(ctx, nil, func(t *Tx) error {
		for _, id := range ids {
			if err := t.detach("Export", id, "archive"); err != nil {
				return err
			}
			if _, err := t.Exec(`DELETE FROM exports WHERE id = ?`, id); err != nil {
				return err
			}
		}
		return nil
	})
}

// buildExport writes the .zip, attaches it, and mails whoever asked.
func (a *App) buildExport(ctx context.Context, id int64) (err error) {
	setState := func(fields map[string]any) error {
		return a.write(ctx, nil, func(t *Tx) error { return t.update("exports", id, fields) })
	}
	defer func() {
		if err != nil {
			_ = setState(map[string]any{"state": "failed", "error": truncate(err.Error(), 250)})
		}
	}()
	if err := setState(map[string]any{"state": "working"}); err != nil {
		return err
	}
	var userID int64
	var created store.Time
	if err := a.store.QueryRowContext(ctx, `SELECT user_id, created_at FROM exports WHERE id = ?`, id).Scan(&userID, &created); err != nil {
		return err
	}
	user, err := a.loadUser(ctx, userID)
	if err != nil || user == nil {
		return fmt.Errorf("the export's owner is gone: %w", err)
	}
	f, err := os.CreateTemp(a.cfg.DataDir, ".export-*.zip")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	tables, rows, files, err := a.writeExportZip(ctx, f, user)
	if err != nil {
		return err
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	filename := "runwell-export-" + created.In(a.zone(ctx)).Format("2006-01-02-1504") + ".zip"
	err = a.write(ctx, nil, func(t *Tx) error {
		b, err := t.putBlob(filename, "application/zip", f)
		if err != nil {
			return err
		}
		if err := t.attach("Export", id, "archive", b); err != nil {
			return err
		}
		return t.update("exports", id, map[string]any{"state": "ready", "completed_at": store.Now().DB(),
			"tables_count": tables, "rows_count": rows, "files_count": files})
	})
	if err != nil {
		return err
	}
	link := a.BaseURL() + "/settings/exports"
	fileWord := pluralize(files, "file")
	what := fmt.Sprintf("%s records from %d tables, and %s, as one .zip.", withDelimiter(rows), tables, fileWord)
	a.sendAccountMail(ctx, accountMail{
		To: user.Email, Subject: "Your Runwell export is ready", Preheader: "Everything in Runwell, as one .zip. The link works for a week.",
		Heading: "Your export is ready", Paragraphs: []string{what}, Button: "Download it", URL: link,
		Small: "It’s kept for a week. Signing in is needed to download it.", Reason: "You asked for an export in Settings > Export.",
		Text: "Your export is ready: " + what + "\n\nDownload it (sign-in needed): " + link + "\n\nIt's kept for a week.",
	})
	return nil
}

// withDelimiter is 12345 as "12,345".
func withDelimiter(n int) string {
	s := fmt.Sprint(n)
	var out []string
	for len(s) > 3 {
		out = append([]string{s[len(s)-3:]}, out...)
		s = s[:len(s)-3]
	}
	return strings.Join(append([]string{s}, out...), ",")
}

func (a *App) writeExportZip(ctx context.Context, w io.Writer, user *User) (tables, rows, files int, err error) {
	names, err := store.Strings(ctx, a.store, `SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return 0, 0, 0, err
	}
	var kept []string
	for _, n := range names {
		if slices.Contains(exportSkipped, n) || strings.HasPrefix(n, "searchable_documents") {
			continue
		}
		kept = append(kept, n)
	}
	zw := zip.NewWriter(w)
	readme, err := zw.Create("README.txt")
	if err != nil {
		return 0, 0, 0, err
	}
	fmt.Fprintf(readme, `%s: everything in Runwell, exported %s by %s.

data/   one table per file, as JSON (an array of rows) and CSV (a header row, then rows).
        Rows keep their ids, so records point at each other the way they do in Runwell:
        engagements.client_id is a clients.id, todos.engagement_id an engagements.id, and so on.
        Tables named after a plugin are that plugin's.
files/  every stored file (documents, images in rich text, the logo), named <blob id>-<name>.
        active_storage_attachments says which record each blob belongs to.

Left out on purpose: sign-in sessions, tokens and OAuth grants, invitations, the search index,
and every password, secret, token or key column. Times are UTC.

Tables: %s
`, a.BrandName(ctx), time.Now().In(a.zone(ctx)).Format("January 2, 2006 15:04"), user.DisplayName(), strings.Join(kept, ", "))
	for _, table := range kept {
		n, err := a.exportTable(ctx, zw, table)
		if err != nil {
			return 0, 0, 0, fmt.Errorf("%s: %w", table, err)
		}
		rows += n
	}
	brows, err := a.store.QueryContext(ctx, `SELECT DISTINCT b.id, b.key, b.filename FROM active_storage_blobs b
		JOIN active_storage_attachments x ON x.blob_id = b.id WHERE x.record_type != 'Export' ORDER BY b.id`)
	if err != nil {
		return 0, 0, 0, err
	}
	type blob struct {
		id            int64
		key, filename string
	}
	blobs, err := store.ScanAll(brows, func(r *sqlRows) (blob, error) {
		var b blob
		return b, r.Scan(&b.id, &b.key, &b.filename)
	})
	if err != nil {
		return 0, 0, 0, err
	}
	for _, b := range blobs {
		src, err := os.Open(a.blobPath(b.key))
		if err != nil {
			continue // a file that's gone from disk
		}
		dst, err := zw.Create(fmt.Sprintf("files/%d-%s", b.id, sanitizeFilename(b.filename)))
		if err == nil {
			_, err = io.Copy(dst, src)
		}
		src.Close()
		if err != nil {
			return 0, 0, 0, err
		}
		files++
	}
	return len(kept), rows, files, zw.Close()
}

var unsafeFilename = regexp.MustCompile(`[^\w.\- ]+`)

func sanitizeFilename(name string) string {
	name = unsafeFilename.ReplaceAllString(name, "_")
	if name == "" {
		return "file"
	}
	return name
}

// exportTable writes a table as data/<table>.json and .csv, its secret
// columns left out: how many rows.
func (a *App) exportTable(ctx context.Context, zw *zip.Writer, table string) (int, error) {
	cols, err := store.Strings(ctx, a.store, `SELECT name FROM pragma_table_info(?) ORDER BY cid`, table)
	if err != nil {
		return 0, err
	}
	var kept []string
	for _, c := range cols {
		if !exportSecretColumns.MatchString(c) {
			kept = append(kept, c)
		}
	}
	if len(kept) == 0 {
		return 0, nil
	}
	quoted := make([]string, len(kept))
	for i, c := range kept {
		quoted[i] = `"` + strings.ReplaceAll(c, `"`, `""`) + `"`
	}
	rows, err := a.store.QueryContext(ctx, `SELECT `+strings.Join(quoted, ", ")+` FROM "`+strings.ReplaceAll(table, `"`, `""`)+`"`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var records []Obj
	var lines [][]string
	for rows.Next() {
		vals := make([]any, len(kept))
		ptrs := make([]any, len(kept))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return 0, err
		}
		rec := make(Obj, 0, len(kept))
		line := make([]string, len(kept))
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			rec = append(rec, kv{kept[i], v})
			if v != nil {
				line[i] = fmt.Sprint(v)
			}
		}
		records = append(records, rec)
		lines = append(lines, line)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	if records == nil {
		records = []Obj{}
	}
	jw, err := zw.Create("data/" + table + ".json")
	if err != nil {
		return 0, err
	}
	raw, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return 0, err
	}
	if _, err := jw.Write(raw); err != nil {
		return 0, err
	}
	cw, err := zw.Create("data/" + table + ".csv")
	if err != nil {
		return 0, err
	}
	w := csv.NewWriter(cw)
	if err := w.Write(kept); err != nil {
		return 0, err
	}
	if err := w.WriteAll(lines); err != nil {
		return 0, err
	}
	return len(records), nil
}
