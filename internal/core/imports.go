package core

import (
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/backup"
)

// Settings › Import: an owner uploads a backup from the Rails core (its
// Settings › Export › "Back up for Runwell Go"), or its data export with a
// password for everyone in it (a data export has none); it's checked and staged
// (internal/backup), then Runwell restarts and puts it in place before it
// opens the database. It replaces everything here, people included: they
// sign in with the passwords they had there. A person's call, so no agent
// tool. Where Runwell can't restart itself, it says to restart it.

func init() { routeSets = append(routeSets, (*App).importRoutes) }

func (a *App) importRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/settings/import", a.showImport, need("manage_settings"))
	a.handle(mux, "POST /api/settings/import", a.createImport, need("manage_settings"))
	a.handle(mux, "DELETE /api/settings/import", a.discardImport, need("manage_settings"))
}

// importMaxBytes is the largest backup the app takes: IMPORT_MAX_BYTES, 10
// GB unless set. A bigger one goes through `runwell import`.
func importMaxBytes() int64 {
	if n, err := strconv.ParseInt(os.Getenv("IMPORT_MAX_BYTES"), 10, 64); err == nil && n > 0 {
		return n
	}
	return 10 << 30
}

func (a *App) showImport(c *Ctx) error {
	pending, err := backup.Pending(a.cfg.DataDir)
	if err != nil {
		return err
	}
	return c.JSON(200, O("pending", pending, "can_restart", a.cfg.HandOver != nil, "max_bytes", importMaxBytes()))
}

// createImport takes the backup (multipart, "file"), streamed to the data
// directory, checks and stages it, and restarts Runwell to apply it.
func (a *App) createImport(c *Ctx) error {
	if c.Actor.Agent() {
		return &apierr.Error{Status: 403, Code: "forbidden", Message: "Importing replaces everything: a person does it, in Settings › Import."}
	}
	mr, err := c.R.MultipartReader()
	if err != nil {
		return apierr.BadRequest("Send the backup as multipart/form-data, in a field named file.")
	}
	var upload, password string
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		} else if err != nil {
			return apierr.BadRequest("Couldn't read the upload: " + err.Error())
		}
		if part.FormName() == "password" {
			raw, _ := io.ReadAll(io.LimitReader(part, 1024))
			password = string(raw)
			continue
		}
		if part.FormName() != "file" || upload != "" {
			continue
		}
		f, err := os.CreateTemp(a.cfg.DataDir, ".import-upload-*.zip")
		if err != nil {
			return err
		}
		upload = f.Name()
		defer os.Remove(upload)
		n, err := io.Copy(f, io.LimitReader(part, importMaxBytes()+1))
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			return apierr.BadRequest("The upload stopped partway: " + err.Error())
		}
		if n > importMaxBytes() {
			return apierr.TooLarge(importMaxBytes())
		}
	}
	if upload == "" {
		return apierr.BadRequest("Choose the backup's .zip to import.")
	}
	if err := backup.Discard(a.cfg.DataDir); err != nil { // one waiting at a time
		return err
	}
	rep, err := backup.Stage(c.R.Context(), a.cfg.DataDir, upload, backup.Options{Password: password, PluginTables: PluginTables})
	if errors.Is(err, backup.ErrNeedsPassword) {
		return &apierr.Error{Status: 422, Code: "needs_password", Message: err.Error(), Errors: map[string][]string{"password": {"is needed: a data export has no passwords"}}}
	}
	if errors.Is(err, backup.ErrInvalid) {
		return &apierr.Error{Status: 422, Code: "invalid_backup", Message: err.Error()}
	} else if err != nil {
		return err
	}
	a.log.Info("import staged", "by", c.Actor.Label(), "from", rep.Manifest.Host, "made", rep.Manifest.CreatedAt, "people", rep.People, "files", rep.Files)
	restarting := a.cfg.HandOver != nil
	if restarting {
		exe, err := os.Executable()
		if err == nil {
			exe, err = filepath.EvalSymlinks(exe)
		}
		if err != nil {
			restarting = false
		} else {
			// Once this answer is on its way: serving stops, then this
			// binary starts again on the same data and applies the backup.
			defer func() { time.AfterFunc(500*time.Millisecond, func() { a.cfg.HandOver(exe) }) }()
		}
	}
	c.Summary = "The backup checked out."
	if restarting {
		c.Summary += " Runwell is restarting to put it in place; sign in again in a moment."
	} else {
		c.Summary += " Restart Runwell to put it in place."
	}
	return c.JSON(202, O("summary", c.Summary, "report", rep, "restarting", restarting))
}

func (a *App) discardImport(c *Ctx) error {
	if err := backup.Discard(a.cfg.DataDir); err != nil {
		return err
	}
	return c.JSON(200, O("summary", "The staged backup is discarded.", "pending", nil))
}
