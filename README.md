# Runwell

The core of project management, for a business, a team, or anything someone runs (a home, a
wedding, a trip): who we work for, what we agreed, what is happening against it, what came in,
and what needs a person now. One file: the API, the app, the client portal, SQLite, the MCP
server and its plugins, compiled into a single static binary for Linux, macOS and Windows.

```sh
curl -fsSL https://raw.githubusercontent.com/Martin-Business-Consultants/runwellgo/main/install.sh | sh
runwell serve --data ./data
# open http://localhost:8080 and set up the first owner
```

This is the Go core of Runwell, a rewrite of the Rails core
([runwellv2](https://github.com/Martin-Business-Consultants/runwellv2), the reference for what
Runwell does). It opens a Rails install's data directory as it is: its database
(`production.sqlite3`, same tables and columns), its uploaded files (Active Storage's layout), its
passwords. Its agent tools keep the Rails names and answers. `AGENTS.md` is the guide for working
on it; `docs/plugins.md` is how to extend it, and `docs/runners.md` how to hand code to a machine
of yours with the Runners plugin and `runwell-runner`.

Runwell is a library with a public plugin API (`plugin`, `plugintest`), and the official binary
is a small `main` ([cmd/runwell](cmd/runwell/main.go)) that hands `runwell.Main` the official
plugins, each in its own repository: [AI](https://github.com/Martin-Business-Consultants/runwellgo-ai),
[Chat](https://github.com/Martin-Business-Consultants/runwellgo-chat),
[Reports](https://github.com/Martin-Business-Consultants/runwellgo-reports),
[Runners](https://github.com/Martin-Business-Consultants/runwellgo-runners),
[Websites](https://github.com/Martin-Business-Consultants/runwellgo-websites),
[My day](https://github.com/Martin-Business-Consultants/runwellgo-my-day),
[Time tracking](https://github.com/Martin-Business-Consultants/runwellgo-time-tracking) and
[Hello](https://github.com/Martin-Business-Consultants/runwellgo-plugin-template), the template for a
new one. A Runwell with plugins of your own is a ten-line `main.go` (docs/plugins.md).

## Running it

From source (Go 1.26 and Node; `mise install` sets them up):

```sh
make run      # go run …serve --data ./data, on :8080
make ui       # the app (ui/: Svelte, shadcn-svelte, the plugins' screens) into internal/admin/dist, which the binary embeds
make ui-dev   # the app with hot reload on :5173, against make run
make build    # dist/runwell (and dist/runwell-runner) for this machine
make test     # vet + tests, both modules (the core, and cmd/runwell with the plugins)
make dist     # every platform, with checksums.txt
```

| Flag | Variable | What |
| --- | --- | --- |
| `--data` | `RUNWELL_DATA_DIR` (or `DATA_DIR`) | Everything the install keeps: `production.sqlite3`, uploaded files, `secret.key`. Back it up and you've backed up the install. |
| `--addr` | `ADDR` / `PORT` | Where to listen (`:8080`). |
| `--url` | `APP_URL` | Runwell's address for emailed links, when Settings › Address has none. |
| | `SITE_KEY` | The install's name (Hoster: one per app). The first start records it in the data directory, and a different one is refused, so two installs can't share a volume by mistake. |
| | `EPHEMERAL_DATA=1` | In a container, start without a persistent volume (a throwaway trial only). |
| | `SECRET_KEY` | Instead of `DATA_DIR/secret.key`: seals settings' secrets. |
| | `TIME_ZONE` | The install's zone when Settings › Address sets none (Eastern by default). |
| | `SMTP_ADDRESS`, `SMTP_PORT`, `SMTP_USERNAME`, `SMTP_PASSWORD`, `SMTP_TLS`, `SMTP_STARTTLS` | A mail server, over Settings › Email's. |
| | `MAIL_FROM` | The sender when Settings › Email names none. |
| | `RUNWELL_MAX_UPLOAD_MB` | The largest file one upload may be (100). |
| | `TRUST_PROXY=1` | Behind Caddy, nginx or a load balancer: read the client's address from `X-Forwarded-For`. |
| | `UPDATE_CHECK=0` | Don't look for new releases. |
| | `LOG_LEVEL`, `LOG_FORMAT` | `debug`/`info`/`warn`/`error`; `text` or `json`. |

Put TLS in front of it (Caddy is one line: `runwell.example.com { reverse_proxy :8080 }`). The
`Dockerfile` builds the same binary into a small image that listens on port 80, logs JSON, and
answers `GET /up` for health checks; mount a named volume at `/data`, one per install.
On Hoster (or any Kamal host) the repository's `config/deploy.yml` sets the port, health check,
proxy and volume: `docs/going-live.html` walks through going live, and updating from the app.

## Moving from the Rails core

To copy a Rails install here (to try it out, or to move): in its Settings › Export, press **Make
a backup** under "Back up for Runwell Go", download the .zip it emails you about, and import it
in Settings › Import here (Runwell restarts onto it), or with Runwell stopped:

```sh
runwell import runwell-backup-2026-10-18-1400.zip --data ./data --force   # --dry-run only checks it
```

A data export (**Make an export**, the JSON and CSV kind, in any version) imports too, for a
quick look: it leaves passwords out, so everyone in it signs in with one you choose
(`--password -`, or the field in Settings › Import), and Rails plugins' tables are left out.

It replaces everything in the data directory; what was there is kept in `replaced-<time>/`. The
backup's Settings › Address comes with it, so on a local copy change it (or emails link to the live
install). Or, on the same server: stop the Rails install, point `runwell serve --data` at its data
directory (`RUNWELL_DATA_DIR`, `storage/` by default), and start it. The database is brought up to date in place (a few
columns and tables the Go core adds; nothing the Rails core reads is changed), files are where
Active Storage left them, and everyone signs in again with their password. Two-factor sign-in set
up under Rails keeps working when the Rails install's `ACTIVE_RECORD_ENCRYPTION_PRIMARY_KEY`,
`ACTIVE_RECORD_ENCRYPTION_KEY_DERIVATION_SALT` and `SECRET_KEY_BASE` are in the environment.
Plugins were Rails engines; they're Go modules compiled in now, each in its own repository
([docs/plugins.md](docs/plugins.md)), so an install's
plugins come back by being switched on in Settings › Plugins. A Rails plugin's tables are left
as they were, but a Go plugin keeps its own storage, so what a Rails plugin recorded (time
entries, say) doesn't carry over.

## Working on an install: the command line and AI agents

The same binary works on an install, from a terminal, a script or an AI app:

```sh
runwell login https://runwell.example.com   # signs in through the browser (or --token rw_…)
runwell briefing                            # what needs a person now
runwell tools                               # every tool your role is offered
runwell show_engagement --ref WO-12
runwell mcp setup                           # how to add Runwell to Claude, Cursor, …
```

AI apps can also connect straight to `https://runwell.example.com/mcp` (OAuth 2.1, or a personal
token from Settings › Connected apps). A client's own agent connects to `/portal/mcp`.

## Users

`runwell user create --email you@example.com --name You --password - [--role owner]`,
`runwell user password --email … --password -`, `runwell user disable-2fa --email …`.

## License

Functional Source License (FSL-1.1-MIT, `LICENSE.md`): use it, change it and run it for yourself
or your clients, but not to offer a competing product; each release becomes MIT two years after
it's published.
