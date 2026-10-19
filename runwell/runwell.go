// Package runwell is the whole of Runwell as a library: the API, the app,
// the portal, the database (SQLite), the MCP server and the plugin host,
// and the command line that runs them. A binary is a main that hands it
// its version and its plugins (cmd/runwell is the official one):
//
//	func main() { os.Exit(runwell.Main(version, mine.Module())) }
//
// Its commands:
//
//	runwell serve [--data ./data] [--addr :8080] [--url https://runwell.example.com]
//	runwell user create --email you@example.com --name You --password - [--role owner]
//	runwell search reindex
//	runwell version
//	runwell login https://runwell.example.com, then runwell TOOL …
package runwell

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/admin"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/backup"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/cli"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/core"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/datalock"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/update"
	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// version is this build's, as Main was given it; plugins are the plugins
// it was built with.
var (
	version = "dev"
	plugins []*plugin.Module
)

// usageError is a command line that's wrong, which exits 2.
type usageError struct{ error }

func usagef(format string, args ...any) error { return usageError{fmt.Errorf(format, args...)} }

// Exit codes besides 0, 1 and 2 (usage), from BSD's sysexits.
const (
	exitDataErr  = 65 // the backup can't be imported (EX_DATAERR)
	exitTempFail = 75 // the data directory is in use; try later (EX_TEMPFAIL)
)

// Main runs the command line os.Args names with plugins compiled in, as
// release version (the binary's, set from the release tag: "dev" when
// empty), and answers the exit code: 0, 1 for an error, 2 for a command
// line that's wrong, 3 for a tool that failed, 65 for a backup that can't
// be imported, 75 for a data directory in use.
func Main(v string, mods ...*plugin.Module) int {
	if v != "" {
		version = v
	}
	plugins = mods
	// Where releases are asked for (GitHub's API unless set: a mirror, a test).
	if api := os.Getenv("RUNWELL_RELEASES_API"); api != "" {
		update.API = strings.TrimRight(api, "/")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, stop, os.Args[1:])
	stop()
	if err == nil && handOverTo != "" {
		// The update button: everything is closed; become the release.
		err = update.Exec(handOverTo)
	}
	if err == nil || errors.Is(err, flag.ErrHelp) {
		return 0
	}
	if !errors.Is(err, cli.ErrFailed) { // a tool's failure is already printed
		fmt.Fprintln(os.Stderr, "runwell:", err)
	}
	return exitCode(err)
}

func exitCode(err error) int {
	var usage usageError
	switch {
	case errors.As(err, &usage):
		return 2
	case errors.Is(err, cli.ErrFailed):
		return 3
	case errors.Is(err, datalock.ErrLocked):
		return exitTempFail
	case errors.Is(err, backup.ErrInvalid):
		return exitDataErr
	}
	return 1
}

func run(ctx context.Context, stop func(), args []string) error {
	cmd := "serve"
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		cmd, args = args[0], args[1:]
	}
	switch cmd {
	case "serve":
		return serve(ctx, stop, args)
	case "user":
		return user(ctx, args)
	case "search":
		return search(ctx, args)
	case "import":
		return importBackup(ctx, args)
	case "update":
		return selfUpdate(ctx, args)
	case "version", "--version":
		fmt.Println("runwell", version)
		return nil
	case "help", "-h", "--help":
		if len(args) > 0 { // help TOOL: one of an install's tools
			break
		}
		usage(os.Stdout)
		return nil
	}
	// Working on a site: login and kin, or one of its tools by name.
	err := cli.Run(ctx, cli.Env{Stdin: os.Stdin, Stdout: os.Stdout, Stderr: os.Stderr, Version: version}, cmd, args)
	if errors.Is(err, cli.ErrUnknown) {
		return usageError{fmt.Errorf("%w (runwell help lists the commands)", err)}
	}
	return err
}

func usage(w io.Writer) {
	fmt.Fprint(w, `runwell: the core of project management, in one file. It runs an install, and works on one.

Working on an install (from a terminal, a script, or an AI app):

  login        sign in to an install        https://runwell.example.com [--name NAME] [--read-only]
               through the browser; or paste a personal token: --paste (or --token TOKEN)
  TOOL         run one of its tools         e.g. briefing, list_clients, show_engagement --ref WO-12,
               create_client --client.name "Bloom & Vine"; names work where ids do
  tools        what its tools are           [NAME] [--json]; help TOOL says the same of one
  whoami       you, your role and what it can do
  doctor       whether all that works
  mcp          a local MCP server for AI apps; mcp setup says how to add it
  profiles, logout                          --profile NAME picks an install; --json prints answers raw

Running an install:

  serve          run it (the default)       --data DIR --addr :8080 --url https://runwell.example.com
  user create    add a person               --email --name --password [--role owner|manager|member]
  user password  a new password             --email --password
  user disable-2fa  turn two-factor off     --email
  search reindex rebuild the search index
  import         replace the install with a backup   BACKUP.zip [--dry-run] [--force] [--password -]
                 (Rails' Settings › Export: a backup, or a data export with --password for everyone
                 in it; runwell serve mustn't be running)
  update         install the newest release over this binary   [--check]
  version

The data directory holds production.sqlite3 (the Rails core's database opens
as it is), uploaded files and secret.key. Back it up and you've backed up
the install.

--password - reads the password from the first line of stdin, and with no
--password it comes from RUNWELL_PASSWORD: either keeps it out of the
process list and shell history.

Every flag has an environment variable: RUNWELL_DATA_DIR (or DATA_DIR), ADDR
(or PORT), APP_URL, SECRET_KEY, and TRUST_PROXY=1 behind a proxy (Caddy,
nginx, a load balancer) so client addresses come from X-Forwarded-For.
SMTP_ADDRESS, SMTP_PORT, SMTP_USERNAME, SMTP_PASSWORD, SMTP_TLS and
SMTP_STARTTLS name a mail server (else Settings › Email); MAIL_FROM the
sender. LOG_LEVEL (debug, info, warn, error) and LOG_FORMAT (text, json)
shape the log.
`)
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// parse reads the flags every command shares, then the command's own, and
// the environment; a command takes no arguments besides flags.
func parse(name string, args []string, extra func(*flag.FlagSet)) (core.Config, error) {
	cfg, rest, err := parseArgs(name, args, extra)
	if err == nil && len(rest) > 0 {
		return core.Config{}, usagef("%s takes no arguments besides flags: %q", name, rest)
	}
	return cfg, err
}

// parseArgs is parse for a command that takes arguments too, before or
// after its flags.
func parseArgs(name string, args []string, extra func(*flag.FlagSet)) (core.Config, []string, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	addr := env("ADDR", "")
	if addr == "" {
		addr = ":" + env("PORT", "8080")
	}
	data := fs.String("data", env("RUNWELL_DATA_DIR", env("DATA_DIR", "./data")), "where the database and files live")
	listen := fs.String("addr", addr, "address to listen on")
	url := fs.String("url", env("APP_URL", ""), "Runwell's own address, which emailed links start with")
	if extra != nil {
		extra(fs)
	}
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			if errors.Is(err, flag.ErrHelp) {
				return core.Config{}, nil, err
			}
			return core.Config{}, nil, usageError{err}
		}
		if fs.NArg() == 0 {
			break
		}
		rest, args = append(rest, fs.Arg(0)), fs.Args()[1:]
	}
	// Unset, the address is Settings › Address's (core.App.BaseURL).
	base := strings.TrimRight(*url, "/")
	logger, err := newLogger(name)
	if err != nil {
		return core.Config{}, nil, err
	}
	trust := os.Getenv("TRUST_PROXY") == "1" || os.Getenv("TRUST_PROXY") == "true"
	return core.Config{DataDir: *data, Addr: *listen, BaseURL: base, SecretKey: os.Getenv("SECRET_KEY"),
		Version: version, Plugins: plugins, UpdateCheck: os.Getenv("UPDATE_CHECK") != "0" && os.Getenv("UPDATE_CHECK") != "false", TrustProxy: trust, Logger: logger}, rest, nil
}

// newLogger is LOG_LEVEL and LOG_FORMAT's log, on stderr. serve logs
// from info up, the one-off commands warnings and errors.
func newLogger(cmd string) (*slog.Logger, error) {
	level := slog.LevelWarn
	if cmd == "serve" {
		level = slog.LevelInfo
	}
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		if err := level.UnmarshalText([]byte(v)); err != nil {
			return nil, usagef("LOG_LEVEL is debug, info, warn or error, not %q", v)
		}
	}
	opts := &slog.HandlerOptions{Level: level}
	switch f := env("LOG_FORMAT", "text"); f {
	case "text":
		return slog.New(slog.NewTextHandler(os.Stderr, opts)), nil
	case "json":
		return slog.New(slog.NewJSONHandler(os.Stderr, opts)), nil
	default:
		return nil, usagef("LOG_FORMAT is text or json, not %q", f)
	}
}

// serve runs until a signal, then lets requests in flight finish (a second
// signal quits at once) before closing the app.
func serve(ctx context.Context, stop func(), args []string) (err error) {
	cfg, err := parse("serve", args, nil)
	if err != nil {
		return err
	}
	cfg.UI = admin.Handler()
	if err := checkPersistent(cfg); err != nil {
		return err
	}
	// A release the update button put in this data directory, newer than
	// this binary, is what runs (update.Choose).
	if bin, ok := update.Choose(cfg.DataDir, version); ok {
		cfg.Logger.Info("handing over to the release in the data directory", "binary", bin, "this", version)
		if err := update.Exec(bin); err != nil {
			cfg.Logger.Warn("couldn't hand over; running this version", "err", err)
		}
	}
	handover := make(chan string, 1)
	cfg.HandOver = func(bin string) {
		select {
		case handover <- bin:
		default:
		}
	}
	// No import under a server (servers may overlap, as a deploy does).
	lock, err := datalock.AcquireShared(cfg.DataDir)
	switch {
	case errors.Is(err, datalock.ErrLocked):
		return fmt.Errorf("%w: is an import replacing %s?", err, cfg.DataDir)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%w: the user runwell runs as (uid %d) can't write to %s; give it that directory", err, os.Getuid(), cfg.DataDir)
	case err != nil:
		return err
	}
	defer func() { err = errors.Join(err, lock.Release()) }()
	// A backup Settings › Import staged goes in place now, before anything
	// opens the database.
	if aside, err := backup.Apply(cfg.DataDir); err != nil {
		return fmt.Errorf("applying the imported backup: %w", err)
	} else if aside != "" {
		cfg.Logger.Info("imported a backup; the install it replaced is kept", "kept_in", aside)
	}
	// One install per data directory (SITE_KEY): a volume shared by mistake isn't.
	if err := datalock.ClaimForSite(cfg.DataDir, os.Getenv("SITE_KEY")); err != nil {
		return err
	}
	app, err := core.NewApp(cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, app.Close()) }()
	// Bound first, so a taken port says so at once; requests wait in the
	// backlog until the plugins are loaded.
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", cfg.Addr)
	if err != nil {
		return err
	}
	if err := app.Start(ctx); err != nil {
		ln.Close()
		return err
	}
	srv := &http.Server{
		Handler:           app.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       5 * time.Minute,
		WriteTimeout:      5 * time.Minute,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(cfg.Logger.Handler(), slog.LevelWarn),
	}
	// Live streams (open tabs) end at once rather than hold up a shutdown.
	srv.RegisterOnShutdown(app.EndStreams)
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	cfg.Logger.Info("runwell is up", "version", version, "url", app.BaseURL(), "data", cfg.DataDir, "addr", ln.Addr().String())
	// Serving: a release being tried has come up (update.Confirm).
	update.Confirm(cfg.DataDir, version)

	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	case bin := <-handover:
		handOverTo = bin
		cfg.Logger.Info("updating: finishing requests in flight, then handing over", "binary", bin)
	}
	stop()
	cfg.Logger.Info("shutting down: finishing requests in flight (a second signal quits now)")
	shut, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	if err := srv.Shutdown(shut); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// password is --password, read from stdin when it's "-", or
// RUNWELL_PASSWORD when it wasn't given.
func password(flagValue string) (string, error) {
	switch flagValue {
	case "-":
		line, err := bufio.NewReader(os.Stdin).ReadString('\n')
		if err != nil && (!errors.Is(err, io.EOF) || line == "") {
			return "", fmt.Errorf("reading the password from stdin: %w", err)
		}
		return strings.TrimRight(line, "\r\n"), nil
	case "":
		return os.Getenv("RUNWELL_PASSWORD"), nil
	}
	return flagValue, nil
}

// required says which of the flags named weren't given.
func required(flags ...[2]string) error {
	for _, f := range flags {
		if f[1] == "" {
			return usagef("--%s is required", f[0])
		}
	}
	return nil
}

func user(ctx context.Context, args []string) error {
	if len(args) > 0 && (args[0] == "password" || args[0] == "disable-2fa") {
		return userAccount(ctx, args[0], args[1:])
	}
	if len(args) == 0 || args[0] != "create" {
		return usagef("usage: runwell user create|password|disable-2fa --email … [--name … --password … --role owner]")
	}
	var email, name, pass, role string
	cfg, err := parse("user create", args[1:], func(fs *flag.FlagSet) {
		fs.StringVar(&email, "email", "", "their email, which they sign in with")
		fs.StringVar(&name, "name", "", "their name")
		fs.StringVar(&pass, "password", "", "at least 10 characters; - reads it from stdin, unset uses RUNWELL_PASSWORD")
		fs.StringVar(&role, "role", "owner", "owner, manager or member")
	})
	if err != nil {
		return err
	}
	if pass, err = password(pass); err != nil {
		return err
	}
	if err := required([2]string{"email", email}, [2]string{"password", pass}); err != nil {
		return err
	}
	return withApp(cfg, func(app *core.App) error {
		id, err := app.CreateUser(ctx, name, email, pass, role)
		if err != nil {
			return err
		}
		fmt.Printf("Created user %d (%s), %s. Sign in at %s\n", id, email, role, app.BaseURL())
		return nil
	})
}

// userAccount: a new password for someone locked out, or a second factor
// turned off for an account the Rails core set it up on.
func userAccount(ctx context.Context, cmd string, args []string) error {
	var email, pass string
	cfg, err := parse("user "+cmd, args, func(fs *flag.FlagSet) {
		fs.StringVar(&email, "email", "", "whose account")
		if cmd == "password" {
			fs.StringVar(&pass, "password", "", "the new password, at least 10 characters; - reads it from stdin, unset uses RUNWELL_PASSWORD")
		}
	})
	if err != nil {
		return err
	}
	need := [][2]string{{"email", email}}
	if cmd == "password" {
		if pass, err = password(pass); err != nil {
			return err
		}
		need = append(need, [2]string{"password", pass})
	}
	if err := required(need...); err != nil {
		return err
	}
	return withApp(cfg, func(app *core.App) error {
		if cmd == "password" {
			if err := app.SetPassword(ctx, email, pass); err != nil {
				return err
			}
			fmt.Println("Password changed; their sessions were signed out.")
			return nil
		}
		if err := app.DisableTwoFactor(ctx, email); err != nil {
			return err
		}
		fmt.Println("Two-factor sign-in is off for", email+". They sign in with their password alone now.")
		return nil
	})
}

func search(ctx context.Context, args []string) error {
	if len(args) == 0 || args[0] != "reindex" {
		return usagef("usage: runwell search reindex [--data DIR]")
	}
	cfg, err := parse("search reindex", args[1:], nil)
	if err != nil {
		return err
	}
	return withApp(cfg, func(app *core.App) error {
		if err := app.ReindexAll(ctx); err != nil {
			return err
		}
		fmt.Println("Rebuilt the search index.")
		return nil
	})
}

// withApp opens the install for a one-off command (nothing runs in the
// background) and closes it after.
func withApp(cfg core.Config, fn func(*core.App) error) (err error) {
	app, err := core.NewApp(cfg)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, app.Close()) }()
	return fn(app)
}

// importBackup replaces the install in --data with a backup from the Rails
// core (Settings › Export › Back up for Runwell Go): checked, staged, then
// put in place, the install it replaces kept in DATA_DIR/replaced-<time>/.
// --dry-run only checks it; replacing an install with people in it needs
// --force. Nothing may be serving the data directory.
func importBackup(ctx context.Context, args []string) (err error) {
	var dryRun, force bool
	var pass string
	cfg, rest, err := parseArgs("import", args, func(fs *flag.FlagSet) {
		fs.BoolVar(&dryRun, "dry-run", false, "check the backup, change nothing")
		fs.BoolVar(&force, "force", false, "replace an install that has people in it")
		fs.StringVar(&pass, "password", "", "a data export's: everyone in it signs in with it; - reads it from stdin, unset uses RUNWELL_PASSWORD")
	})
	if err != nil {
		return err
	}
	if len(rest) != 1 {
		return usagef("usage: runwell import BACKUP.zip [--data DIR] [--dry-run] [--force]")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return err
	}
	lock, err := datalock.Acquire(cfg.DataDir)
	if err != nil {
		return fmt.Errorf("%w: stop runwell serve on %s first, or import from Settings › Import", err, cfg.DataDir)
	}
	defer func() { err = errors.Join(err, lock.Release()) }()
	if !dryRun && !force && backup.HasPeople(cfg.DataDir) {
		return usagef("%s already has people in it; add --force to replace it (it's kept in %s/replaced-<time>/)", cfg.DataDir, cfg.DataDir)
	}
	if err := backup.Discard(cfg.DataDir); err != nil {
		return err
	}
	fmt.Fprintln(os.Stderr, "Reading", rest[0], "…")
	if pass, err = password(pass); err != nil {
		return err
	}
	rep, err := backup.Stage(ctx, cfg.DataDir, rest[0], backup.Options{Password: pass, PluginTables: core.PluginTables(plugins)})
	if errors.Is(err, backup.ErrNeedsPassword) {
		return usagef("%v: add --password - (or set RUNWELL_PASSWORD)", err)
	} else if err != nil {
		return err
	}
	m := rep.Manifest
	if rep.Kind == "data" {
		fmt.Printf("A data export of %s, made %s by %s: %d rows in %d tables, %d people who can sign in (with the password given), %d clients, %d files.\n",
			m.Host, m.CreatedAt, m.By, m.Rows, m.Tables, rep.People, rep.Clients, rep.Files)
		if len(rep.Skipped) > 0 {
			fmt.Printf("Left out (tables this Runwell doesn't have, a Rails plugin's): %s\n", strings.Join(rep.Skipped, ", "))
		}
	} else {
		fmt.Printf("A backup of %s, made %s by %s (Runwell %s): %d people who can sign in, %d clients, %d files.\n",
			m.Host, m.CreatedAt, m.By, m.RunwellVersion, rep.People, rep.Clients, rep.Files)
	}
	if dryRun {
		fmt.Fprintln(os.Stderr, "Dry run: nothing was changed.")
		return backup.Discard(cfg.DataDir)
	}
	aside, err := backup.Apply(cfg.DataDir)
	if err != nil {
		return err
	}
	if aside != "" {
		how := "a password from the backup"
		if rep.Kind == "data" {
			how = "the password you gave"
		}
		fmt.Printf("Done. What was there is kept in %s. Start runwell serve and sign in with %s.\n", aside, how)
	}
	return nil
}

// selfUpdate installs the newest release over this binary, or with --check
// only says whether there's one.
func selfUpdate(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("update", flag.ContinueOnError)
	check := fs.Bool("check", false, "only say whether a newer release is out")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return err
		}
		return usageError{err}
	}
	if fs.NArg() > 0 {
		return usagef("update takes no arguments")
	}
	r, err := update.Latest(ctx)
	if err != nil {
		return err
	}
	if !update.Newer(r.Version, version) {
		fmt.Printf("runwell %s is the newest (the latest release is %s).\n", version, r.Version)
		return nil
	}
	fmt.Printf("runwell %s is out (this is %s): %s\n", r.Version, version, r.URL)
	if *check {
		return nil
	}
	if update.InContainer() {
		return fmt.Errorf("this runs in a container: deploy ghcr.io/martin-business-consultants/runwellgo:%s on your host instead", r.Version)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	if err := update.Install(ctx, r, exe); err != nil {
		return err
	}
	fmt.Printf("Installed %s at %s. Restart the service to run it (systemctl restart runwell, or however it's started).\n", r.Version, exe)
	return nil
}

// handOverTo is the release serve stopped to hand over to (the update
// button), which main becomes once everything is closed.
var handOverTo string

// checkPersistent refuses, in a container, a data directory the next
// deploy would lose (no volume, or an anonymous one), unless EPHEMERAL_DATA=1
// says that's meant (a trial).
func checkPersistent(cfg core.Config) error {
	if !update.InContainer() {
		return nil
	}
	if os.Getenv("EPHEMERAL_DATA") == "1" {
		cfg.Logger.Warn("EPHEMERAL_DATA=1: the data directory may be lost with this container", "data", cfg.DataDir)
		return nil
	}
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		cfg.Logger.Warn("can't tell whether the data directory is a volume", "err", err)
		return nil
	}
	defer f.Close()
	return datalock.CheckPersistent(cfg.DataDir, f)
}
