// Package core is Runwell: who we work for and what we agreed (clients,
// contacts, engagements, agreement versions, approvals), what is happening
// against it (work, commitments, notes, events), what came in (requests) and
// what needs a person now (the briefing, questions, notifications). It serves
// the JSON API the app, the portal, the CLI and agents (MCP) all use.
package core

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/backup"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/ratelimit"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/update"
	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// Config is what an install is started with; everything else lives in the
// database.
type Config struct {
	DataDir string // the database, uploaded files, the secret key
	Addr    string
	// Runwell's own address, which links in email start with (APP_URL);
	// Settings › Address, else localhost, when empty.
	BaseURL string
	// SECRET_KEY: seals settings' secrets instead of DataDir/secret.key.
	SecretKey string
	// Version is this build's, "dev" when empty.
	Version string
	// HandOver stops serving and hands over to the release binary at bin
	// (the update button); nil where that can't be done.
	HandOver func(bin string)
	// UpdateCheck looks for a newer release once a day (UPDATE_CHECK=0 off).
	UpdateCheck bool
	// TrustProxy reads the client's address from X-Forwarded-For.
	TrustProxy bool
	Logger     *slog.Logger
	// Mailer sends email; SMTP (the environment's, else Settings › Email)
	// when nil.
	Mailer Mailer
	// UI serves the app (everything that isn't /api, /mcp or /up).
	UI http.Handler
	// NoDefaultPlugins doesn't install the default plugins (Module.Default)
	// at Start (tests).
	NoDefaultPlugins bool
	// Plugins are the plugins the binary was built with (runwell.Main's):
	// the host has these and no others.
	Plugins []*plugin.Module
}

// DatabaseFile is the database's name in the data directory: the Rails
// core's production database, so its volume opens here as it is.
const DatabaseFile = "production.sqlite3"

// App is one install: one business, one database, one process.
type App struct {
	cfg     Config
	log     *slog.Logger
	store   *store.DB
	secrets *secret.Key
	plugins *plugin.Host
	// live carries plugins' events to people's open tabs (live.go).
	live liveHub

	// Who hears of events (events.go).
	listeners []listener
	// The routes, and the agent tools declared on them (agent.go).
	mux   *http.ServeMux
	tools []*toolSpec
	// Settings › Address's URL, read a moment ago (BaseURL).
	baseURL atomic.Pointer[cachedURL]
	// The newest release, when it's newer than this build (UpdateCheck).
	newer atomic.Pointer[update.Release]

	// Sign-in: ten tries a minute from one address, and ten at one account.
	signIns *ratelimit.Limiter
	// Emailed links (password resets, sign-in links, portal links): five.
	mailed *ratelimit.Limiter
	// pluginCalls counts calls to plugins' public routes.
	pluginCalls *ratelimit.Limiter
	// agentCalls: what one token may call a minute.
	agentCalls *ratelimit.Limiter
	startedAt  time.Time

	// ctx lives as long as the app: Close cancels it, and everything
	// started in the background (spawn) derives from it.
	ctx    context.Context //nolint:containedctx // the app's own lifetime, not a request's
	cancel context.CancelFunc
	wg     sync.WaitGroup
	mu     sync.Mutex // guards closed, so nothing joins wg once Close waits
	closed bool
	once   sync.Once
	err    error
}

// NewApp opens an install: the database (migrated), the key, the defaults
// a new one needs. Nothing runs in the background until Start.
func NewApp(cfg Config) (*App, error) {
	if cfg.Logger == nil {
		cfg.Logger = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	if cfg.Version == "" {
		cfg.Version = "dev"
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, err
	}
	db, err := store.Open(filepath.Join(cfg.DataDir, DatabaseFile))
	if err != nil {
		return nil, err
	}
	secrets, err := secret.Load(cfg.DataDir, cfg.SecretKey)
	if err != nil {
		return nil, errors.Join(err, db.Close())
	}
	a := &App{cfg: cfg, log: cfg.Logger, store: db, secrets: secrets, startedAt: time.Now(),
		signIns: ratelimit.New(10), mailed: ratelimit.New(5), pluginCalls: ratelimit.New(30), agentCalls: ratelimit.New(agentRateLimit())}
	a.ctx, a.cancel = context.WithCancel(context.Background())
	a.plugins = plugin.New(pluginEnv{a}, db, a.log, a.spawn, a.cfg.Plugins...)
	a.listen()
	if err := a.seed(context.Background()); err != nil {
		a.cancel()
		return nil, errors.Join(fmt.Errorf("seed: %w", err), db.Close())
	}
	return a, nil
}

// Start loads the plugins (installing the ones the binary ships) and starts
// what runs in the background. One-off commands open an App without it.
func (a *App) Start(ctx context.Context) error {
	if err := a.plugins.LoadAll(ctx); err != nil {
		a.log.Warn("plugins", "err", err)
	}
	if !a.cfg.NoDefaultPlugins {
		a.plugins.EnsureDefaults(ctx)
	}
	if !a.spawn(a.ctx, "scheduler", a.scheduler) {
		return errors.New("the app is closed")
	}
	if a.cfg.UpdateCheck {
		a.spawn(a.ctx, "update check", a.checkForUpdates)
	}
	// An imported data export comes without a search index: rebuilt now.
	if marker := filepath.Join(a.cfg.DataDir, backup.ReindexFile); fileExists(marker) {
		a.spawn(a.ctx, "search reindex", func(ctx context.Context) {
			if err := a.ReindexAll(ctx); err != nil {
				a.log.Warn("search reindex after an import", "err", err)
				return
			}
			_ = os.Remove(marker)
			a.log.Info("rebuilt the search index after an import")
		})
	}
	return nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// Store is the database, for one-off commands.
func (a *App) Store() *store.DB { return a.store }

// checkForUpdates asks for the newest release a minute after starting, then
// daily, and keeps it when it's newer than this build.
func (a *App) checkForUpdates(ctx context.Context) {
	wait := time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		wait = 24 * time.Hour
		r, err := update.Latest(ctx)
		if err != nil {
			a.log.Debug("update check", "err", err)
			continue
		}
		if update.Newer(r.Version, a.cfg.Version) {
			a.newer.Store(&r)
			a.log.Info("a new version is available", "version", r.Version, "running", a.cfg.Version, "notes", r.URL)
		}
	}
}

// Close stops what runs in the background, waits for it, then closes the
// database.
func (a *App) Close() error {
	a.once.Do(func() {
		a.mu.Lock()
		a.closed = true
		a.mu.Unlock()
		a.cancel()
		a.wg.Wait()
		a.err = a.store.Close()
	})
	return a.err
}

// BaseURL is Runwell's own address, which emailed links start with, as the
// Rails core chose it (Runwell.url): the one an owner set in Settings ›
// Address, else APP_URL (or APP_HOST), else the address people were last
// seen using, else localhost and the port it listens on.
func (a *App) BaseURL() string {
	// Read once a few seconds, not once per record an answer links to.
	if c := a.baseURL.Load(); c != nil && time.Since(c.at) < 5*time.Second {
		return c.url
	}
	var host, proto, seen string
	_ = a.store.QueryRowContext(context.Background(), `SELECT COALESCE(app_host, ''), COALESCE(app_protocol, ''), COALESCE(seen_host, '') FROM settings ORDER BY id LIMIT 1`).Scan(&host, &proto, &seen)
	url := a.localURL()
	switch {
	case host != "":
		url = strOr(proto, "https") + "://" + host
	case a.cfg.BaseURL != "":
		url = strings.TrimRight(a.cfg.BaseURL, "/")
	case seen != "":
		url = strOr(proto, "https") + "://" + seen
	}
	a.baseURL.Store(&cachedURL{url, time.Now()})
	return url
}

// sawAddress remembers the address a person reached Runwell at (seen_host),
// for links when none was set. Not localhost: that's no use to a client.
func (a *App) sawAddress(r *http.Request) {
	host := r.Host
	if host == "" || strings.HasPrefix(host, "localhost") || strings.HasPrefix(host, "127.") || strings.HasPrefix(host, "[::1]") {
		return
	}
	if _, err := a.store.ExecContext(r.Context(), `UPDATE settings SET seen_host = ? WHERE COALESCE(seen_host, '') != ?`, host, host); err != nil {
		a.log.Warn("seen host", "err", err)
	}
}

type cachedURL struct {
	url string
	at  time.Time
}

// localURL is the address with nothing set: localhost and the port.
func (a *App) localURL() string {
	host := a.cfg.Addr
	if host == "" || strings.HasPrefix(host, ":") {
		host = "localhost" + host
	}
	return "http://" + host
}

// Host is BaseURL's host, for addresses made from it (no-reply@host).
func (a *App) Host() string {
	h := strings.TrimPrefix(strings.TrimPrefix(a.BaseURL(), "https://"), "http://")
	if i := strings.IndexAny(h, ":/"); i >= 0 {
		h = h[:i]
	}
	return h
}

// spawn runs fn in the background with ctx's values but the app's lifetime:
// it isn't cancelled when a request ends, only when the app closes, and
// Close waits for it. A panic is logged, not fatal. False once the app is
// closed (fn doesn't run).
func (a *App) spawn(ctx context.Context, name string, fn func(context.Context)) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.closed {
		return false
	}
	a.wg.Go(func() {
		ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
		defer cancel()
		defer context.AfterFunc(a.ctx, cancel)()
		defer a.recovered(name)
		fn(ctx)
	})
	return true
}

// later runs fn in the background, as Solid Queue's jobs did: mail, events
// to plugins and webhooks, search indexing. Nothing a request does waits on
// SMTP or a slow plugin.
func (a *App) later(name string, fn func(context.Context)) {
	if !a.spawn(a.ctx, name, fn) {
		a.log.Warn("background work dropped: the app is closing", "task", name)
	}
}

// recovered logs a panic in background work, where nothing else would.
func (a *App) recovered(name string) {
	if rec := recover(); rec != nil {
		a.log.Error("panic in the background", "task", name, "panic", rec, "stack", string(debug.Stack()))
	}
}
