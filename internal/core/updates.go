package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
	"github.com/Martin-Business-Consultants/runwellgo/internal/update"
)

// Settings › Updates: the version this install runs, the newest release,
// and the button that updates to it in place: the database is backed up
// into the data directory, the release's binary put there (checked against
// its checksums and run once), and the running process hands over to it
// (Config.HandOver), which confirms itself once it's serving. If it
// doesn't come up, the next start stays on the version before and this
// page says so. Past updates and how each went are listed below.

func init() {
	routeSets = append(routeSets, (*App).updateRoutes)
	every("release check", 24*time.Hour, func(a *App, ctx context.Context) error {
		if !a.cfg.UpdateCheck {
			return nil
		}
		return a.checkRelease(ctx)
	})
	every("updates reconcile", 5*time.Minute, func(a *App, ctx context.Context) error { return a.reconcileUpgrades(ctx) })
}

const (
	upgradeTimeout   = 45 * time.Minute
	keptUpdateBackup = 3
)

func (a *App) updateRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	a.handle(mux, "GET /api/settings/updates", a.showUpdates, ms, tool("show_updates", "Show this install's version, the newest release and past updates"))
	a.handle(mux, "POST /api/settings/updates", a.startUpgrade, ms, tool("update_runwell", "Update Runwell to the newest release",
		note("Starts updating this install to the newest release that show_updates lists. It backs up the database, swaps in the new version and restarts; show_updates says how it went."),
		confirm("The whole install updates to the newest release and restarts: it's unavailable for a minute or two.")))
	a.handle(mux, "POST /api/settings/updates/check", a.checkUpdatesNow, ms, tool("check_for_updates", "Check for a newer Runwell release now",
		note("Asks GitHub from a job; show_updates has the answer a few seconds later.")))
	a.handle(mux, "DELETE /api/settings/updates/failures", a.clearFailedUpdates, ms, tool("clear_failed_updates", "Clear failed updates from this install's past updates"))
}

// releaseState is what the settings row keeps of the newest release.
type releaseState struct {
	Release     map[string]any
	CheckedAt   store.Time
	RequestedAt store.Time
	CheckError  string
}

func (a *App) releaseState(ctx context.Context) (*releaseState, error) {
	var r releaseState
	var raw store.JSON
	var errText *string
	err := a.store.QueryRowContext(ctx, `SELECT latest_release, release_checked_at, release_check_requested_at, release_check_error FROM settings ORDER BY id LIMIT 1`).
		Scan(&raw, &r.CheckedAt, &r.RequestedAt, &errText)
	if err != nil {
		return nil, err
	}
	r.Release = raw.Map()
	if errText != nil {
		r.CheckError = *errText
	}
	return &r, nil
}

// latestVersion is the newest release's version, "" when unchecked.
func (r *releaseState) latestVersion() string { return str(r.Release["version"]) }

// pending is whether a check asked for is under way (one that hasn't come
// back in five minutes isn't).
func (r *releaseState) pending() bool {
	return r.RequestedAt.Valid && time.Since(r.RequestedAt.Time) < 5*time.Minute && r.CheckError == "" &&
		(!r.CheckedAt.Valid || r.CheckedAt.Before(r.RequestedAt.Time))
}

// checkRelease asks GitHub for the newest release and keeps it.
func (a *App) checkRelease(ctx context.Context) error {
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	r, err := update.Latest(ctx)
	if err != nil {
		_, uerr := a.store.ExecContext(ctx, `UPDATE settings SET release_check_error = ? WHERE id = ?`, truncate(err.Error(), 250), s.ID)
		return uerr
	}
	if update.Newer(r.Version, a.cfg.Version) {
		a.newer.Store(&r)
	}
	var published any
	if !r.PublishedAt.IsZero() {
		published = r.PublishedAt.UTC().Format(time.RFC3339)
	}
	data := map[string]any{"version": r.Version, "url": r.URL, "notes": r.Notes, "published_at": published}
	_, err = a.store.ExecContext(ctx, `UPDATE settings SET latest_release = ?, release_checked_at = ?, release_check_error = NULL WHERE id = ?`,
		string(store.MustJSON(data)), store.Now().DB(), s.ID)
	return err
}

func (a *App) updatesBy() string {
	if a.cfg.HandOver != nil {
		return "in_place"
	}
	return "manual"
}

// reconcileUpgrades settles running updates: done once this install runs
// the new version, failed when the new version didn't come up, or went
// quiet.
func (a *App) reconcileUpgrades(ctx context.Context) error {
	rows, err := a.store.QueryContext(ctx, `SELECT id, to_version, from_version, created_at FROM upgrades WHERE status = 'running'`)
	if err != nil {
		return err
	}
	type running struct {
		id       int64
		to, from string
		at       store.Time
	}
	list, err := store.ScanAll(rows, func(r *sqlRows) (running, error) {
		var u running
		return u, r.Scan(&u.id, &u.to, &u.from, &u.at)
	})
	if err != nil {
		return err
	}
	failed, _ := update.Failed(a.cfg.DataDir)
	for _, u := range list {
		set := map[string]any{}
		switch {
		case !update.Newer(u.to, a.cfg.Version) && a.cfg.Version != "dev":
			set = map[string]any{"status": "succeeded", "finished_at": store.Now().DB(), "message": nil}
		case failed == strings.TrimPrefix(u.to, "v"):
			set = map[string]any{"status": "failed", "finished_at": store.Now().DB(),
				"message": "Runwell " + u.to + " didn't come up, so this install stayed on " + a.cfg.Version + ". The server's log says why."}
		case time.Since(u.at.Time) > upgradeTimeout:
			set = map[string]any{"status": "failed", "finished_at": store.Now().DB(), "message": "No word after 45m. The server's log says what happened."}
		default:
			continue
		}
		if err := a.write(ctx, nil, func(t *Tx) error { return t.update("upgrades", u.id, set) }); err != nil {
			return err
		}
	}
	return nil
}

func (a *App) showUpdates(c *Ctx) error {
	ctx := c.R.Context()
	if err := a.reconcileUpgrades(ctx); err != nil {
		return err
	}
	rs, err := a.releaseState(ctx)
	if err != nil {
		return err
	}
	rows, err := a.store.QueryContext(ctx, `SELECT g.id, g.from_version, g.to_version, g.status, g.via, g.requested_by_id, COALESCE(NULLIF(u.name, ''), u.email_address, ''),
		g.created_at, g.finished_at, g.external_url, g.message FROM upgrades g LEFT JOIN users u ON u.id = g.requested_by_id ORDER BY g.created_at DESC LIMIT 10`)
	if err != nil {
		return err
	}
	var current any
	upgrades, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id, by int64
		var from, to, status, via, name string
		var started, finished store.Time
		var logURL, msg *string
		if err := r.Scan(&id, &from, &to, &status, &via, &by, &name, &started, &finished, &logURL, &msg); err != nil {
			return nil, err
		}
		return O("id", id, "from_version", from, "to_version", to, "status", status, "via", via, "requested_by", name,
			"requested_by_id", by, "started_at", started, "finished_at", finished, "log_url", logURL, "message", msg), nil
	})
	if err != nil {
		return err
	}
	if upgrades == nil {
		upgrades = []Obj{}
	}
	for _, u := range upgrades {
		if u.Get("status") == "running" && current == nil {
			current = u
		}
	}
	cur, _ := current.(Obj)
	failedCount, err := store.Count(ctx, a.store, `SELECT count(*) FROM upgrades WHERE status = 'failed'`)
	if err != nil {
		return err
	}
	var latest any
	newer := false
	if v := rs.latestVersion(); v != "" {
		newer = update.Newer(v, a.cfg.Version)
		latest = O("version", v, "newer", newer, "url", rs.Release["url"], "published_at", rs.Release["published_at"], "notes", rs.Release["notes"])
	}
	available := a.cfg.HandOver != nil
	var summary string
	switch {
	case cur != nil:
		summary = fmt.Sprintf("Updating from %s to %s", cur.Get("from_version"), cur.Get("to_version"))
	case newer:
		summary = "Runwell " + a.cfg.Version + "; " + rs.latestVersion() + " is out"
		if available {
			summary += " (update_runwell installs it)"
		} else {
			summary += ", but updating from here isn't set up"
		}
	default:
		summary = "Runwell " + a.cfg.Version + ", the newest release"
	}
	by := "Hand: updating from here isn’t set up"
	if available {
		by = "Itself: it downloads the release and restarts on it"
	}
	manual := "runwell update"
	if update.InContainer() {
		manual = "docker pull ghcr.io/martin-business-consultants/runwellgo:" + rs.latestVersion() + "  # then recreate the container with the same volume"
	}
	var failedRelease any
	if v, at := update.Failed(a.cfg.DataDir); v != "" {
		failedRelease = O("version", v, "at", at.UTC().Format(time.RFC3339))
	}
	c.Summary = summary
	return c.JSON(200, O("summary", summary, "version", a.cfg.Version, "updates_by", a.updatesBy(), "updates_by_description", by,
		"available", available, "checking_nightly", a.cfg.UpdateCheck, "checked_at", rs.CheckedAt, "checking", rs.pending(),
		"check_error", nullable(rs.CheckError), "latest_release", latest, "updating", current, "failed_count", failedCount,
		"failed_release", failedRelease, "manual_command", manual, "updates", upgrades))
}

func (a *App) startUpgrade(c *Ctx) error {
	ctx := c.R.Context()
	refused := func(msg string) error { return &apierr.Error{Status: 422, Code: "refused", Message: msg} }
	if a.cfg.HandOver == nil {
		return refused("Updating from here isn't set up on this install: see Settings > Updates.")
	}
	rs, err := a.releaseState(ctx)
	if err != nil {
		return err
	}
	to := rs.latestVersion()
	if to == "" || !update.Newer(to, a.cfg.Version) {
		return refused("There's no newer release to update to.")
	}
	var running string
	if err := a.store.QueryRowContext(ctx, `SELECT to_version FROM upgrades WHERE status = 'running' ORDER BY created_at DESC LIMIT 1`).Scan(&running); err == nil {
		return refused("An update to " + running + " is already running.")
	} else if !isNoRows(err) {
		return err
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		var err error
		id, err = t.Insert(`INSERT INTO upgrades (from_version, to_version, via, status, requested_by_id, created_at, updated_at) VALUES (?, ?, 'in_place', 'running', ?, ?, ?)`,
			a.cfg.Version, to, c.Actor.User.Person().ID, now, now)
		return err
	})
	if err != nil {
		return err
	}
	a.later("update", func(ctx context.Context) { a.runUpgrade(ctx, id, to) })
	c.Summary = "Updating to " + to + ". Runwell restarts when it's done; this page shows how it went."
	return c.JSON(202, O("summary", c.Summary, "updating_to", to, "from", a.cfg.Version))
}

// runUpgrade backs up, stages the release, and hands over to it; a failure
// is recorded on the update and this version keeps running.
func (a *App) runUpgrade(ctx context.Context, id int64, to string) {
	fail := func(err error) {
		a.log.Warn("update", "to", to, "err", err)
		_ = a.write(ctx, nil, func(t *Tx) error {
			return t.update("upgrades", id, map[string]any{"status": "failed", "finished_at": store.Now().DB(), "message": err.Error()})
		})
	}
	r, err := update.Latest(ctx)
	if err != nil {
		fail(fmt.Errorf("couldn't ask GitHub for %s: %w", to, err))
		return
	}
	if !update.Newer(r.Version, a.cfg.Version) {
		fail(fmt.Errorf("there's no newer release than %s to update to", a.cfg.Version))
		return
	}
	backup, err := a.backupBeforeUpdate(ctx, r.Version)
	if err != nil {
		fail(fmt.Errorf("backing up before the update: %w", err))
		return
	}
	bin, err := update.Stage(ctx, r, a.cfg.DataDir)
	if err != nil {
		fail(fmt.Errorf("couldn't get %s ready, so nothing changed: %w", r.Version, err))
		return
	}
	if err := update.Try(a.cfg.DataDir, r.Version); err != nil {
		fail(err)
		return
	}
	_ = a.write(ctx, nil, func(t *Tx) error {
		return t.update("upgrades", id, map[string]any{"to_version": r.Version, "message": "Backed up to " + backup + "; restarting on " + r.Version + "."})
	})
	a.log.Info("updating", "from", a.cfg.Version, "to", r.Version, "backup", backup)
	time.Sleep(500 * time.Millisecond)
	a.cfg.HandOver(bin)
}

// backupBeforeUpdate copies the database into DATA_DIR/backups, keeping
// the newest few.
func (a *App) backupBeforeUpdate(ctx context.Context, to string) (string, error) {
	dir := filepath.Join(a.cfg.DataDir, "backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	name := fmt.Sprintf("before-%s-%s.sqlite3", to, time.Now().UTC().Format("20060102-150405"))
	if _, err := a.store.ExecContext(ctx, `VACUUM INTO ?`, filepath.Join(dir, name)); err != nil {
		return "", err
	}
	if entries, err := os.ReadDir(dir); err == nil {
		var before []string
		for _, e := range entries {
			if strings.HasPrefix(e.Name(), "before-") && strings.HasSuffix(e.Name(), ".sqlite3") {
				before = append(before, e.Name())
			}
		}
		stamp := func(n string) string {
			n = strings.TrimSuffix(n, ".sqlite3")
			if i := strings.LastIndex(n, "-"); i > 0 {
				if j := strings.LastIndex(n[:i], "-"); j > 0 {
					return n[j+1:]
				}
			}
			return n
		}
		sort.Slice(before, func(i, j int) bool { return stamp(before[i]) < stamp(before[j]) })
		for len(before) > keptUpdateBackup {
			_ = os.Remove(filepath.Join(dir, before[0]))
			before = before[1:]
		}
	}
	return filepath.Join("backups", name), nil
}

func (a *App) checkUpdatesNow(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	if _, err := a.store.ExecContext(ctx, `UPDATE settings SET release_check_requested_at = ?, release_check_error = NULL WHERE id = ?`, store.Now().DB(), s.ID); err != nil {
		return err
	}
	a.later("release check", func(ctx context.Context) {
		if err := a.checkRelease(ctx); err != nil {
			a.log.Warn("release check", "err", err)
		}
	})
	c.Summary = "Checking GitHub for a newer release."
	return c.JSON(200, O("summary", c.Summary))
}

func (a *App) clearFailedUpdates(c *Ctx) error {
	var n int64
	err := a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		res, err := t.Exec(`DELETE FROM upgrades WHERE status = 'failed'`)
		if err != nil {
			return err
		}
		n, _ = res.RowsAffected()
		return nil
	})
	if err != nil {
		return err
	}
	c.Summary = "There were no failed updates to clear."
	if n > 0 {
		c.Summary = "Cleared " + pluralize(int(n), "failed update") + "."
	}
	return c.JSON(200, O("summary", c.Summary))
}
