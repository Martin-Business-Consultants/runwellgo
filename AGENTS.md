# Runwell (Go) — Project Guide

Runwell rewritten as one Go binary, the way `../librepublishgo` rewrote Libre Publish: the API,
the app (Svelte + shadcn-svelte, embedded), SQLite, the MCP server and Go plugins, in one file.
`../runwellv2` (Rails) is the reference: **its behavior, rules, words and JSON shapes are the
spec**. Read the Rails model, controller, jbuilder view and ERB template for anything you port,
and port what they do, not a guess at it. Its `AGENTS.md` explains the domain.

## Layout

| Path | What |
| --- | --- |
| `cmd/runwell` | The binary: `serve`, `user …`, `search reindex`, `update`, and the CLI (`internal/cli`) |
| `internal/core` | Runwell itself: one package, one file (or a few) per area of the app |
| `internal/store` | SQLite: `schema.sql` (the Rails tables, so a Rails database opens as it is, plus the Go core's own), migrations, column types (`store.Time`, `store.JSON`, `store.NullInt`) |
| `internal/plugin` | The plugin host: plugins are Go packages compiled in (`plugins/<key>`) |
| `internal/{apierr,mail,secret,ratelimit,update,webhook,outbound,datalock,jsonv,cli}` | Infrastructure, from librepublishgo |
| `internal/admin` | Embeds `ui/` built into `internal/admin/dist` and serves it for every path that isn't `/api`, `/mcp` or `/up` |
| `ui/` | The app: Svelte 5, Tailwind 4, shadcn-svelte, lucide icons |

`make build`, `make run` (`:8080`, data in `./data`), `cd ui && npm run check && npm run build`.
Go and Node come from mise: run Go as `mise exec -- go …` in this directory.

## Backend conventions (`internal/core`)

- **Routes.** Each area file registers its own in an `init`:
  `func init() { routeSets = append(routeSets, (*App).clientRoutes) }`, then
  `a.handle(mux, "GET /api/clients/{id}", a.showClient, tool(…))`. Paths are the Rails paths under
  `/api` (`/api/clients`, `/api/engagements/{ref}`, `/api/work/{id}`, `/api/settings/names`).
  Options: `open` (no sign-in), `portal` (a client's contact, not staff), `need("send_agreements")`
  (a permission; staff only by default). A handler that decides per record calls `c.Need(perm)` /
  `c.Can(perm)`. Read-only tokens can't write (checked for you).
- **Agent tools.** Every Rails `agent_tool` becomes `tool(name, title, note(description), body("client", "…", true), query(…))`
  on its route, **same name**, same params (the body wraps them the way Rails params nest them:
  `{"client": {"name": …}}`). Rails `agent_exempt` actions get no tool. Answers can set
  `c.Summary` and `c.Crumbs = []Crumb{step("Add work", "create_todo", "ref", ref)}` (next steps).
- **JSON shapes match the Rails jbuilder views** (`app/views/**/*.json.jbuilder`): same keys,
  same nesting, ids, `*_cents` beside formatted money. Build answers with `O("id", id, "name", …)`
  (keeps key order). Lists answer `{"clients": [...]}` the way the jbuilder index does. The app uses
  the same answers; add a key the screen needs rather than a second endpoint.
- **Writes** go through `a.write(ctx, c.Actor, func(t *Tx) error { … })`: one transaction; record
  every write of consequence with `t.Event("Engagement", id, "engagement.closed", payload)` (the
  Rails `record_event!` kinds and payloads, exactly), queue search with `t.Reindex("Client", id)`,
  and do slow things after commit with `t.AfterCommit(func(ctx) { … })` (mail: `a.mailLater(m)`).
  Events reach plugins and listeners (`onEvent(func(a, ctx, ev))`) after commit.
- **Validation** errors: `inv := apierr.Invalid{}; inv.Add("name", "can't be blank"); return inv.Err()`
  (422, `{"errors": {"name": ["can't be blank"]}}`, the Rails messages). Not found:
  `notFound("client")`. Others: `apierr.BadRequest`, `apierr.Conflict`, `&apierr.Error{Status, Code, Message}`.
- **Domain rules live in Go functions on the area's types** (the Rails model verbs: `send!`,
  `decide!`, `resolve!`, `promote_to_change!` → `sendVersion`, `decide`, …), called by handlers,
  bulk actions and agents alike. State is derived, never stored (`Engagement#state`).
- **Words.** `s, _ := a.settings(ctx)`, `s.Term("client", 1)`, `s.LabelName`, `s.LabelPrefix`.
  Never "agency" or one industry's jargon in anything people or agents read.
- **Money** is integer cents everywhere. **Dates** are `YYYY-MM-DD` strings (date columns);
  timestamps are `store.Time` (UTC in the database, ISO 8601 in JSON).
- **Rich text** is HTML in a text column: clean it on the way in with `sanitizeRichText`, answer it
  as stored (the app shows it with `{@html}` after the server cleaned it). Mentions:
  `mentionedUserIDs(html)`. Search text: `plainText(html)`.
- **Search**: a searchable type registers `searchable("Client", func(ctx, db, id) (SearchDoc, bool, error))`
  in an init (Rails `Searchable::MODELS`: Client Contact Engagement Request Todo Commitment Note Document).
- **Files** are Active Storage's, so a Rails data directory's files open as they are (`blobs.go`):
  `t.putBlob(filename, contentType, r)`, `t.attach("Document", id, "file", blob)`, `t.detach(…)`,
  `a.attachedBlob(ctx, db, "User", id, "avatar")`, `a.serveBlob(w, r, blob, download)`.
- **Record show answers** are assembled from sections: `addSection("Client", "engagements", fn)` in
  the area that keeps them; the show handler calls `a.withSections`. Deleting a record runs
  `t.cascade(typ, id)`, which runs what areas registered with `onDelete(typ, fn)`.
- **Background**: `a.later(name, fn)` for one-off work; `every(name, interval, fn)` in an init for
  recurring work (Rails' recurring jobs and the nightly run).
- **Sidebar counts**: `navCounts["requests"] = func(a, ctx, actor) (int, error)` in an init.
- **Who**: `c.Actor.User` (staff, maybe a person's agent: `IsAgent()`, `Person()`), `c.Actor.Contact`
  (portal), `c.Actor.Token`. `c.Actor.UserID()`, `Label()`, `Source()` (app / agent / portal).
- Queries are plain SQL with `?` args on `a.store` (reads) or `t` (in a write). Preload what a list
  shows with one query per association (`IN (…)`), never a query per row.
- Comments say why, in the house voice (short, plain, present tense), like the copied code.
- Don't write tests unless asked. `go vet ./...` and `go build ./...` must pass.

## Frontend conventions (`ui/`)

### Design

The look is Runwell's own on shadcn-svelte's components (tokens in `ui/src/app.css`; Settings ›
Appearance overrides them through `lib/appearance.svelte.js`):

- **Color.** Runwell Indigo `#5D63CD` means "act here": primary buttons, links, focus, the current
  place. Never decoration. Ink `#1B1E2E` on white, slate `#5B6175` for quieter text, a cool mist
  sidebar `#F5F6FA`, lines the ink at 10% (they sit as quietly on a tint as on white); red `#C2362B` only for late or irreversible. Dark mode keeps
  the hues on blue-black `#12141C`.
- **Type.** Manrope (bundled), body 14px; headings 650–700 weight, slightly tight, sentence case;
  dates, counts and money in tabular figures. Scale: 12 / 13 / 14 / 16 / 20 / 24 / 30.
- **One loud place.** Home's greeting and how much needs the person; everything else is quiet.
- **Status is a word and a color**, never color alone. Borders over shadows (only the faint
  `shadow-xs/5` and 1px inner highlight the components carry). No all-caps eyebrows,
  no arrows tacked onto buttons, no badge that repeats what the list already says.
- **Quality floor.** Works at 390px wide with no sideways scroll (tab rows scroll themselves);
  every page has one `<main>` and one `<h1>` (indexes: `sr-only`, from the toolbar); every control
  and editor has a name; focus is always visible; motion respects reduced-motion; text meets
  WCAG AA. The audit sweep (axe-core at desktop, phone and dark) should come back clean.
- **Three things and their parts.** Clients (who we work for; requests come from them), engagements
  (what we agreed; commitments are made on them) and work (what's happening). The sidebar nests each
  part under its thing (`lib/nav.svelte.js`); notifications are the header's bell. A record shows how
  it connects: `ConnectionsButton` opens the client's whole map (`/api/connections`, Svelte Flow).
- **No flicker.** A screen is made afresh only for a new path. A new query (a filter, sort, page or
  search) keeps the screen: read it with `$derived(router.query…)` and load with `onQuery(load)`,
  never a bare `load()` that reads the query. Moves ease with the browser's view transition, which
  waits briefly for the new screen's reads (`router.change`); pass `transition: false` for moves made
  while typing.
- **Keyboard first.** `lib/keys.svelte.js` gives every screen vim keys (`?` lists them): `g` + a
  letter to go somewhere, `j`/`k`/`gg`/`G` through the screen's items, Enter/`o` to open, `x` to pick,
  `h`/`l` tabs, `[`/`]` pages, `/` search, `n` New, `e` Edit. Items are table rows, cards that are
  links and list items with a link or button; mark anything else `data-nav-item` (and the thing to
  open `data-nav-open`, New `data-nav-new`). A screen's own key handler wins by calling
  `preventDefault`.
- **Tabs switch views; filters narrow a list.** Choosing which records a list shows (Open / Done /
  All, a state, a client) is a filter, never a row of tabs or toggle buttons: `FilterMenu` (a
  dropdown, or a searchable combobox past five choices), through `IndexToolbar`'s `filters` on an
  index and `QueryFilter` (`?param=`) for a list inside a page, such as a record's tab. (An index's
  `tabs` row, engagements' types or requests' sources, is the one place a filter is drawn as tabs.)
- **Words.** The person's words (`session.term`), plain verbs, sentence case; an error says what
  happened and what to do; an empty list says where its records come from,
  with `EmptyState` (messenger's: always an icon on a stack of cards, a title, a line, actions, in a
  dashed box; `variant` "card" on its own, in a frame of its own unless it's already in a card,
  frame or panel (`section` titles that frame), "inline" in a tab; parts in `$lib/components/ui/empty`).

- **Frames.** The surfaces are messenger's (`../messenger`, coss ui): a section is a frame, a
  tinted rim (`bg-muted/72 p-1 rounded-2xl`) holding its header and footer, its body a white panel.
  A `Card.Root` with a `Card.Content` or a `Table.Root` straight inside draws as a frame by itself
  (`card-framed`, in `app.css`); a card of only a header and footer (a link in a grid) stays one
  solid surface. A table straight in a frame or card draws as cards (its head on the tint, its body
  a panel); `DataTable` is a frame. A new section is one component, `$lib/components/Frame.svelte`
  (`<Frame title description>` with `actions`/`footer` snippets, `flush` for a list edge to edge,
  `bare` for a Table or several panels); `$lib/components/ui/frame` holds its parts (`Frame.Root`,
  `.Panel`, `.Header`, `.Title`, `.Footer`) for layouts it doesn't cover.
- **shadcn-svelte, in messenger's finish.** Build screens from `$lib/components/ui/*` (Card, Table, Button,
  Badge, Tabs, Dialog, Sheet, DropdownMenu, Field, Input, Select, Textarea, Checkbox, Switch,
  Popover + Calendar for dates, Command, Tooltip, Alert, Skeleton, Avatar, Breadcrumb…) and
  lucide icons (`import Plus from "@lucide/svelte/icons/plus"`), as the shadcn blocks and examples
  do. Tailwind utilities only; no custom CSS beyond `app.css`. Missing a shadcn component? Add it:
  `npx shadcn-svelte@1.7.0 add <name>` (don't overwrite existing ones), then give it messenger's
  classes (its `app/frontend/components/ui/*.jsx`, Base UI's states mapped to bits-ui's). The older helpers in
  `$lib/components/*.svelte` (DataTable, EmptyState, Time, Badge…) may be used where they fit.
- **Layout.** The app is shadcn's sidebar layout (`lib/layout/AppSidebar.svelte`, `SiteHeader.svelte`).
  A screen sets its header with `page.set(title, [{label, href}])` (breadcrumbs: up a level, then
  the screen) and renders its content in `<main>`'s column: an index is a toolbar (filters left, view
  toggle and New right) over a Table or a card grid; a record is a two-column page
  (`lg:grid-cols-3`: the working column `lg:col-span-2` with Tabs, a side column of Cards: Details,
  contacts, history, destructive actions), like the Rails `settings--split` pages.
- **Record page tabs** come from `lib/record-tabs.js`: an area adds its tab to another area's record
  page (`addTab("Client", {key, label, count, component, order})`) from its own `index.js`; the
  record page renders `tabsFor(type)`. Likewise **settings pages** register in `lib/settings-nav.js`
  (`addSettingsPage({href, label, group, perm, order})`) and wrap their content in
  `lib/layout/SettingsLayout.svelte`.
- **Routes.** Each area lists its screens in `src/routes/<area>/index.js`
  (`export const routes = [["/clients/:id", ClientShow]]`, and `publicRoutes` for screens open signed
  out: the portal, approval links, emailed links). Components live beside it. Real paths
  (`/clients/12`), not hashes: plain `<a href>` links route in the page; `go(path)` navigates.
- **Data.** `get/post/patch/del` from `$lib/api.js` (paths without `/api`). Errors: `ApiError`
  with `.errors` per field; show with `Field.Error`, toasts with `toast` / `toastError` from
  `$lib/toast.svelte.js`; confirm destructive actions with `confirm({title, message, destructive})`.
- **Words**: `session.term("client", 2)`, `session.label("work_order")`; permissions
  `session.can("send_agreements")`; the team `session.me.team`. Money: `money(cents)`, dates
  `date("2026-10-20")`, `timeAgo(iso)` from `$lib/format.js`.
- Keyboard: `s`/⌘K search (done). Keep `data-keys`-style shortcuts out unless the area needs them.
- `npm run check` must pass with 0 errors; `npm run build` writes `internal/admin/dist`.

## Areas

`internal/core` is one package, a file (or a few) per area: clients/contacts (`clients.go`),
engagements and agreements (`engagements*.go`, `agreements.go`, `scope_items.go`, `approvals.go`),
work and commitments (`todos*.go`, `board.go`, `assignments.go`, `commitments.go`, `bulk_*.go`),
requests and home (`requests*.go`, `mailbox*.go`, `questions.go`, `briefing*.go`, `setup.go`,
`quick_actions.go`), notes and what reaches people (`notes.go`, `documents.go`, `mentions.go`,
`notifications.go`, `avatars.go`, `api_search.go`), settings and people (`settings_*.go`,
`account*.go`, `people.go`, `invitations.go`, `passwords.go`, `two_factor*.go`, `sign_in*.go`,
`webhooks.go`, `exports.go`, `updates.go`, `appearance.go`, `audit.go`), agents and the portal
(`agent.go`, `mcp*.go`, `oauth.go`, `tokens.go`, `connected_apps.go`, `changes.go`, `resolve.go`,
`portal.go`, `cli_install.go`), plugins (`api_plugins.go`, `plugin_env.go`, `plugins.go`), and
`wiring.go` for what ties two areas together. The app mirrors it: `ui/src/routes/<area>/`.
