# Plugins

A plugin is a Go module compiled into Runwell, in a repository of its own. It
builds a `*plugin.Module` (package `plugin`,
`github.com/Martin-Business-Consultants/runwellgo/v3/plugin`): a manifest saying what it is, what it needs and what it
adds, the handlers for its routes, the events it hears and the tasks it runs.
It reaches Runwell only through the `*plugin.API` it's handed on each call,
which holds the capabilities an owner granted it and nothing else. What it
shows in the app is described as data (screens: text, figures, details,
forms, tables) that the app draws with its own components: a plugin ships no
code into the browser.

This replaces the Rails contract (`runwellv2/docs/plugin-contract.md`), where
a plugin was a Rails engine installed from GitHub that rendered partials into
slots. The trade is the same one Libre Publish made: one language, one build,
no download or restart, and a plugin reviewed and built like the rest of the
code; but a plugin is added by building Runwell with it, and the grants are a
contract the host keeps, not a sandbox wall. A plugin that calls only its
`*plugin.API` can do no more than it was granted, but Go doesn't stop one that
imports `database/sql` or `os`, so review that a plugin doesn't.

## Adding one

Runwell is a library, the way PocketBase is: a binary is a `main` that hands
`runwell.Main` its version and its plugins, and has those and no others.

1. Write a Go module with a `Module() *plugin.Module`, in its own repository
   ([below](#writing-a-plugin-in-its-own-repository)).
   [runwellgo-plugin-template](https://github.com/Martin-Business-Consultants/runwellgo-plugin-template)
   (Hello) is the smallest;
   [runwellgo-time-tracking](https://github.com/Martin-Business-Consultants/runwellgo-time-tracking) is a real
   one, the Rails Time tracking plugin ported onto this API.
2. Pass its `Module()` to `runwell.Main` in a `main.go`: the official binary's is
   [cmd/runwell/main.go](../cmd/runwell/main.go), with its version in
   [cmd/runwell/go.mod](../cmd/runwell/go.mod).

The official plugins, each in its repository and tagged with its manifest's
version:

| Plugin | Key | Repository |
| --- | --- | --- |
| Runwell AI | `ai` | [runwellgo-ai](https://github.com/Martin-Business-Consultants/runwellgo-ai) |
| Chat | `chat` | [runwellgo-chat](https://github.com/Martin-Business-Consultants/runwellgo-chat) |
| Reports | `reports` | [runwellgo-reports](https://github.com/Martin-Business-Consultants/runwellgo-reports) |
| Runners | `runners` | [runwellgo-runners](https://github.com/Martin-Business-Consultants/runwellgo-runners) (the runner itself is `cmd/runwell-runner`, here) |
| Websites | `website_checklist` | [runwellgo-websites](https://github.com/Martin-Business-Consultants/runwellgo-websites) |
| My day | `my_day` | [runwellgo-my-day](https://github.com/Martin-Business-Consultants/runwellgo-my-day) |
| Time tracking | `time_tracking` | [runwellgo-time-tracking](https://github.com/Martin-Business-Consultants/runwellgo-time-tracking) |
| Hello (the example) | `hello` | [runwellgo-plugin-template](https://github.com/Martin-Business-Consultants/runwellgo-plugin-template) |

A build with it lists it under **Settings › Plugins › Add a plugin** until an
owner installs it. Installed, it starts off; switching it on grants what it
asks for (the card lists it, and each grant can be unticked). One with
`Default: true` is installed, granted and switched on at first run instead,
and isn't put back once someone removes it.

## The module

```go
func Module() *plugin.Module {
	return &plugin.Module{
		Manifest: plugin.Manifest{
			Key: "hello", Name: "Hello", Version: "1.0.0", Author: "Runwell",
			Description: "A note on every client's page, and a page of its own.",
			Permissions: plugin.Grants{Storage: true, RecordsRead: true},
			Hooks:       []string{"client.created"},
			Routes: []plugin.Route{
				{Method: "GET", Path: "/client", Export: "client"},
				{Method: "GET", Path: "/greeting", Export: "greeting"},
				{Method: "POST", Path: "/greeting", Export: "save"},
			},
			Screens: []plugin.Screen{
				{ID: "client", Title: "Hello", Source: "/client", Text: "text", Detail: &plugin.Detail{Items: "items"}},
				{ID: "greeting", Title: "Hello", Source: "/greeting", Text: "text",
					Form: &plugin.Form{Record: "greeting", Save: "/greeting", Fields: []plugin.Field{
						{Name: "message", Label: "Greeting", Required: true},
					}}},
			},
			Nav:    &plugin.NavPage{Label: "Hello", Page: "greeting", Icon: "hand"},
			Panels: []plugin.Panel{{Slot: "client_panel", Label: "Hello", Screen: "client"}},
		},
		Handlers: map[string]plugin.Handler{"client": client, "greeting": greeting, "save": save},
		OnEvent:  onEvent,
	}
}
```

- `Key`: lowercase letters, digits and `_`. It's the plugin's namespace (its
  storage, its routes, its permissions and events) and stays the same across
  versions.
- `Permissions`: what it asks for (below). `HTTP` names hosts
  (`api.example.com`), never `*` or URLs.
- `Capabilities`: permissions it adds, each `<key>:<name>` with a label and
  the roles that hold it (`owner`, `manager`, `member`; owners alone when it
  names none). They show on Settings › People with the core's while the
  plugin is on, and its routes can require them. The Rails
  `Runwell::Plugins.permission`.
- `Tasks`: `{Name, Every}` with `Every` a duration of a minute or more
  (`"15m"`, `"6h"`) or `"nightly"` (once a day, in the small hours of the
  install's time zone), run by `Module.Tasks[Name]`. The Rails
  `Runwell::Plugins.nightly`. A task's context ends after ten minutes; one
  plugin failing never stops another.
- `Author`, `Homepage`: shown on its card.

A module whose manifest doesn't hold up (a route naming a handler it doesn't
have, a screen naming a route it doesn't serve, a panel in a slot there isn't)
stops Runwell at start: that's a bug in the build, not the install.

## Routes

A route is served at `/api/x/<key><path>`, by the handler its `Export` names:

```go
func save(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error)
```

`req` has `Method`, `Path`, `Query`, `Headers` (Content-Type, Accept,
User-Agent, Referer, Origin, X-Requested-With), `Body` (up to the route's
limit), `IP` and `Actor`: who's calling, `{ID, Name, Role, Agent,
Capabilities}` with `ID` the person's user id (an agent's person, for an
agent) and `Capabilities` the permissions their role holds
(`req.Actor.Can("time_tracking:view_everyones_time")`).

A route needs someone on staff signed in (a session or a token), holding its
`Capability` when it names one (a `Machine` route instead needs one of the plugin's own machine
tokens: Machines, below) (one of its own, or a core permission such as
`manage_settings`); a read-only connection only reads. A `Public` route
answers anyone (a form on another site): 30 calls a minute from one address,
from any origin. A route takes a body of up to 1 MB unless it sets `MaxBody`
(at most 10 MB).

It answers a `*plugin.Response`: `plugin.JSON(status, v)`, `plugin.Invalid(
map[string][]string{"field": {"can't be blank"}})` for a 422 the app shows
beside the field, or `plugin.Answer(err)` for an error the caller should see
(a write's validation, `plugin.ErrNoRecord`); nil from `Answer` means the
error is the plugin failing, so return it. An error answers 502 and is
logged. `Set-Cookie` is dropped and every answer is served with
`Content-Security-Policy: sandbox`. Each call's context ends after 10
seconds, and a call that panics answers an error rather than taking Runwell
down.

### Agent tools

A route with a `Tool` is an agent tool while the plugin is on: in `tools/list`
over MCP and the CLI, called through the same route as the caller, with the
caller's role, its answer in the agent envelope (a `summary` key in the answer
becomes the envelope's summary). Name it as the Rails plugin named its
`agent_tool`; a core tool's name always wins.

```go
{Method: "POST", Path: "/entries", Export: "create", Tool: &plugin.Tool{
	Name: "log_time", Title: "Log time",
	Description: `record: "Type:id" for a Client, Engagement or Todo. duration like 1:30, 90 or 1.5h.`,
	Wrap: "entry", Body: `{duration (required), worked_on: YYYY-MM-DD, note}`,
	Params: []plugin.ToolParam{{Name: "record", Description: `"Client:12", "Engagement:5" or "Todo:40".`}},
}},
```

`Wrap`/`Body` is the JSON object the route takes and how to fill it; `Params`
are top-level body fields beside it; `Query` are query parameters.

## Events

`OnEvent(ctx, api, kind, data)` hears each event the manifest's `Hooks`
name, with `data` the event as JSON: `{id, subject_type, subject_id, kind,
payload, actor_user_id, actor, source, occurred_at}`. Events are delivered in
the background once their write commits, so a slow or failing plugin never
holds up a save; there is no caller (`api` writes as the system). The kinds
are the Rails core's, with the same payloads:

`client.created`, `engagement.created`, `engagement.closed` (`outcome`,
`reason`), `engagement.reopened`, `engagement.erased`, `todo.created`,
`todo.status` (`from`, `to`), `todo.converted`, `agreement.sent`,
`agreement.emailed`, `agreement.approved`, `agreement.changes_requested`,
`commitment.added`, `commitment.done`, `commitment.missed`,
`commitment.dropped`, `request.received`, `request.replied`,
`request.promoted`, `request.dismissed`, `question.answered`, `note.added`,
`note.deleted`, `document.added`, `client.updated` (`changed`: the attributes and `custom_fields.<key>` a save set; heard by plugins and webhooks, not kept in the client's history); `mention.created` (to the plugin whose mentionable a note
mentions, below); `user.removed` (below); and other plugins' own (`<key>.<what>`, `api.Emit`), by name or any plugin's
with `*.<what>`.

`user.removed` is the one to hook if a plugin keeps anything per person. Settings › People ›
Remove hands everything someone was responsible for to a successor and deletes them; the core
moves its own columns in one transaction, then this event (subject `User:<id>`) tells plugins to
move theirs. Its payload is `{id, name, email_address, agent_id?, successor_id, successor_name,
moved, removed}` (`moved` and `removed` are the counts the preview showed). By the time it's heard
the user row and their agent are gone (`Records` no longer finds them), so keep the `name` wherever
your history needs it (show "Name (removed)"), and hand what they owned or were responsible for to
`successor_id`, an active person. Drop their settings and running state; keep history readable.
Handle it idempotently: it may be heard twice.

## The host's capabilities

`*plugin.API`'s methods. One that wasn't granted answers an error
(`storage wasn't granted to this plugin`) and does nothing.

| Method | Needs | Does |
| --- | --- | --- |
| `Log(msg)`, `Key()` | always | writes to the server log under the plugin's key; its key |
| `Site(ctx)` | always | `plugin.Site{Name, URL, Locale, TimeZone, ContactEmail}` |
| `Records(ctx, plugin.Query)` | `records:read` | records of one type, as the agent tools show them (below) |
| `Record(ctx, "Client:12")` | `records:read` | one record by name; `plugin.ErrNoRecord` when there's none |
| `AddNote(ctx, plugin.Note{Record, Body, Kind})` | `records:write` | a note on a Client, Engagement, ScopeItem, Todo or Request: `note.added` |
| `AddWork(ctx, plugin.Work{Ref, Title, Description, Status, OwnerID, DueOn, ClientVisible})` | `records:write` | work on an engagement (by ref): `todo.created` |
| `AddCommitment(ctx, plugin.Commitment{Record, Description, DueOn, OwnerKind, UserID, ContactID})` | `records:write` | a commitment on a client or engagement: `commitment.added` |
| `RecordEvent(ctx, plugin.Event{Record, Kind, Payload})` | `records:write` | an event of its own (`<key>.<what>`) in a record's history, which webhooks, listeners and other plugins hear |
| `Metrics(ctx, plugin.MetricsQuery)` | `metrics` | the install's numbers for reports, read-only (below, "Numbers for reports") |
| `KVGet`, `KVSet`, `KVDelete` | `storage` | a value (≤ 1 MB) under a key, ≤ 100,000 keys |
| `KVList(ctx, prefix, after, limit)`, `KVAll`, `KVCount` | `storage` | keys under a prefix, in order, a page (≤ 1000) at a time; all; how many |
| `KVValues(ctx, prefix)` | `storage` | keys under a prefix with their values, in order (≤ 10,000): a set of records in one read |
| `Mail(ctx, mail.Message)` | `mail` | through Settings › Email (its sender where blank); ≤ 200 messages an hour |
| `HTTP()` | `http` | an `*http.Client` that reaches only the granted hosts (redirects too) and reads ≤ 4 MB of an answer |

**Reading records.** `plugin.Query{Type, ID, Ref, ClientID, EngagementID,
AgreementVersionID, OwnerID, Status, Open, IDs, Limit}`: `Type` is one of
`Client`, `Contact`, `Engagement`, `AgreementVersion`, `ScopeItem`, `Todo`,
`Commitment`, `Request` or `CustomField` (the install's fields: `key`, `label`, `kind`,
`model_type`; `Status` narrows them to a model's); a `Client` carries its `custom_fields` by key; the filters that make sense for the type apply
(`Open`: engagements not closed, work not done, commitments not resolved,
requests still open; `OwnerID`: work someone leads or is on, commitments
someone owns). At most `Limit` (100 by default, 500 at most). Each record
starts with its agent ref (`type`, `id`, `record` "Client:12", `ref`,
`display_name`, `url`) and has the keys its Rails jbuilder partial has
(`clients/_client`, `todos/_todo`, …), parents as refs.

**Writing records** goes through the core: the same validation (a 422 with
the Rails messages, through `plugin.Answer`), the same event, search
reindexed, all in one write, recorded as whoever called the route (the system
for an event or a task) with `"plugin": "<key>"` in the event's payload. A
plugin never writes core tables any other way.

Storage is the plugin's own (`plugin_kv`, keyed by its key): other plugins
can't see it, it survives upgrades, and removing the plugin keeps it unless
the removal purges it.

## Where it shows

All placed by the manifest, each pointing at one of its screens. Someone sees
a place only when they may call that screen's source route, so a plugin
capability decides who sees what.

| Manifest | Where | The Rails call |
| --- | --- | --- |
| `Nav: &plugin.NavPage{Label, Page, Icon, Badge}` | the sidebar, under Plugins, at `/p/<key>/<page>`; each person orders or hides it (`users.nav_plugins` {order, hidden}). `Icon` is a lucide name (clock, history, hand, calendar, chart-bar, users, message-square, …). `Badge` names a GET route of its own answering `{"count": n}` for the person: the number beside it (unread), asked each time the sidebar refreshes (2 seconds at most; none at 0) | `nav` |
| `Panels: []plugin.Panel{{Slot, Label, Screen}}` | `client_panel`, `engagement_panel`, `todo_panel`: a tab on each client, engagement or piece of work, its screen called with `record=<Type>:<id>`; `home_top`: a card above home's briefing | `slot` |
| `Home: []plugin.HomeSection{{Title, Screen}}` | a card in home's briefing | `briefing` |
| `Settings: &plugin.SettingsPage{Label, Screen}` | Settings › Install, at `/settings/plugins/<key>`, for those who may manage settings (the gear on its card) | `settings` |
| `QuickActions: []plugin.QuickAction{{Label, Icon, Types, Screen}}` | Quick actions: a form screen in a dialog, for the record on screen when it's one of `Types` (Client, Engagement, Todo; its `record=` is passed) | `quick_action` |
| `Capabilities` | Settings › People, with the core's permissions | `permission` |
| `Tasks` | the scheduler | `nightly` |
| `Route.Tool` | MCP and the CLI | `agent_tool` |

Settings › Plugins lists all of these on the plugin's card ("Client panel",
"Nav: Time", "Agent tool: log_time"), as the Rails `additions` did.

The Rails slots with no Go counterpart yet (`approval_page`, `email_inbound`; `nav_actions`,
`portal_actions`, the asides and row slots are the Svelte slots below), `stylesheet`,
`shortcut`, `agent_brief`, `agent_workflow` and `ai_prompt` aren't part of
this contract: ask for one when a plugin needs it.

## Screens, as data

A screen is `{ID, Title, Source}` and one or more parts, drawn in this order:

- `Text`: the key of a paragraph in the source's answer.
- `Stats`: the key of `[{label, value, hint}]`, drawn as a row of figures.
- `Detail{Items}`: the key of `[{label, value, href, badge, date, time,
  multiline}]`, a list of labelled values.
- `Form{Record, Save, Method, Submit, Fields, Reset}`: starts from the answer's
  `Record` (empty when blank), posts `{"<record>": {field: value}}` to `Save`
  (POST unless `Method` says PATCH or PUT) with the screen's query on the
  address, marks the fields a 422's `errors` name, shows the answer's
  `message`, then draws the screen again (emptied first when `Reset`). Fields:
  `{Name, Label, Type, Placeholder, Help, Required, Options, OptionsFrom,
  Default}` with `Type` one of text, textarea, number, date, email, url,
  select, checkbox, password; a date's `Default: "today"`. Only someone who
  may call `Save` sees the form.
- `Table{Rows, Key, Columns, RowActions, Empty}`: the key of the rows; each
  column `{Key, Label, Link{Screen, Params}, Href, Sub, Num, Badge, Date,
  Time}` (`Href` the key of a path in the row; `Params` `{"id": "id"}` takes
  query params from the row).

`Filters: [{Param, Label, Options | OptionsFrom}]` set a query parameter and
draw the screen again (on a page, in the address; options from the answer
can depend on who's looking, and a filter with none is hidden). `Actions`
(header) and `RowActions` are `{Label, Screen, Params}` (another screen) or
`{Label, Post, Method, Params, Body, Confirm, Danger, ShowIf}` (a call to a
route, after a confirmation when `Confirm` says what happens; `ShowIf` names a
row key that must be true, e.g. `mine`). An action is offered only to those
who may call its route. The answer's `message` is shown after a call.

The source's answer is also what an agent gets from that route, so shape it
for both: the keys the screen draws beside the ones the Rails jbuilder view
had (`summary` first).

## Its own tables, files and screens

What a plugin with more to keep than key-value storage, or more to show than described screens,
reaches. Each is generic: the core knows nothing of any plugin, and a plugin that's off (or
removed) changes nothing.

- **`tables`**: its manifest's `Schema` makes its own tables (`CREATE TABLE IF NOT EXISTS` and
  `CREATE [UNIQUE] INDEX IF NOT EXISTS`, every name `<key>_…`) each time it loads switched on;
  `ALTER TABLE <key>_… ADD COLUMN …` adds a column a newer version needs to a table made before it
  (skipped where the column is already there). It
  reaches them with `api.Exec`, `api.Query`, `api.QueryRow` and `api.Tx`, plain SQL with `?`
  arguments. Every statement is read first: one naming any other table, changing the schema, or
  holding more than one statement is refused. A plugin ported from Rails keeps its tables' names
  and columns, so an imported Rails database's rows are its rows. Core records are never read or
  written by SQL: `Records` and the write verbs are the way.
- **`files`**: `api.Attach(ctx, "WebsiteChecklist::Item", id, "files", name, type, r)`,
  `api.Attachments`, `api.OpenFile`, `api.Detach`. Files go in Runwell's store (Active Storage's
  layout, so a Rails plugin's files carry over), attached only to record types in the plugin's
  namespace: the CamelCase of its key and `::`.
- **`records:write` › `ChangeWork`**: a piece of work's status, due date, description, lead and
  others on it, through the core's own rules (the Work pages' `changeTodo`, `todo.status` events).
- **`http:public`**: `api.PublicHTTP()` reaches any public internet address (a client's own site),
  never this server's network: the address dialed is checked, redirects included; 4 MB per answer.
- **`api.Background(name, fn)`**: work that outlasts a call (up to five minutes, four at a time per
  plugin), with the same API and caller.
- **Who's calling**: `req.Actor` is the person (`ID`), and for an agent also `AgentID` (the agent
  user) and `Via` (the app, "Claude").
- **People**: `Records` reads `User` (people on staff: name, email, role, active).
- **Words**: `api.Site(ctx).Terms` are the install's words for clients, engagements, scope items,
  work and commitments.
- **`client.deleted`**: heard (with the client's name) once a client is deleted, so a plugin can
  clear what it kept on it.
- **Its own screens**: an official plugin may draw pages and record tabs in Svelte, in its
  repository's `ui/`, which the app's build copies into `ui/src/plugins/<key>/` (see
  [its README](../ui/src/plugins/README.md)), when described screens aren't enough. `Nav.Path` puts
  such a page in the sidebar (`"/my-day"`). They're shown only while the plugin is on, and talk
  only to its routes.

## Acting as the caller, the portal, your own network, secrets and the app's chrome

Generic capabilities a plugin asks for like any other (the AI plugin, [runwellgo-ai](https://github.com/Martin-Business-Consultants/runwellgo-ai), is the one that
uses them all); with the plugin off or removed, none of them does anything.

- **`tools`**: `api.Tools(ctx, req.Actor)` is the agent tools (the MCP catalogue, `api_request`
  aside) the caller's role is offered: `plugin.ToolInfo{Name, Title, Description, InputSchema,
  ReadOnly, Confirm}`. `api.RunTool(ctx, req.Actor, name, args)` runs one as that person, through
  the same path as their own agent over MCP (a token for the one call), so their role, the
  validation and the events are the API's, and history reads "Ted's agent via <plugin name>". It
  answers the tool's envelope (`{status, summary, data, …}`). The caller must be the route's own
  caller. A tool that only reads runs any time, in `api.Background` too; one that changes
  anything runs only inside that person's own non-GET request (not an agent's token, not in the
  background), otherwise `plugin.ErrNeedsApproval`: the plugin holds a proposed change and runs it
  when the person approves it. A change that reaches a client is confirmed (the person approved
  it). `api.PortalTools(ctx, req.Contact)` / `api.RunPortalTool` are the same for a portal contact,
  with the tools their portal's own agent has (`/portal/mcp`'s).
- **`portal`**: a route with `Portal: true` is served at `/api/x/<key>/…` to a client's contact
  signed in to their portal (the `rw_portal` session, with its CSRF token on writes, or their
  portal token), never to staff; `req.Contact` is `{ID, Name, Email, ClientID, ClientName,
  CanApprove, Agent}`. It can't be public, need a capability or be an agent tool.
- **`http:private`**: `api.PrivateHTTP()` reaches any address, this server and its local network
  included (an owner's model server). Switching a plugin on never grants it (the Settings ›
  Plugins box starts unticked); an owner ticks "Reach this server's network". Answers are read to
  4 MB; it waits as long as the context allows (copy the client to set `Timeout`).
- **`secrets`**: `api.Seal(plain)` / `api.Unseal(sealed)`: a value the plugin must use again (an
  API key) sealed with the install's key, like Runwell's own settings' secrets; a Rails install's
  encrypted values open when Runwell runs with that install's keys. Never send one back to the app.
- `api.Grants()` is what was granted, so a plugin can say what it can't do yet rather than fail;
  `Site.Location` is the install's zone, to work out dates in.
- **The app's chrome and record pages** (official plugins' own Svelte, their `ui/`):
  `addSlot(name, {key, component, order})` from `$lib/plugin-slots.js` draws a component in a place
  the app's screens hold for plugins, while the plugin is on: `header` (beside Search and Add, on
  every page), `home_top`, `client_aside`, `request_aside`, `scope_item_aside`, `draft_version`,
  `commitment_row`, `note_row`, `portal_header` (the portal's header) and `portal_nav` (the
  portal's page links); the portal ones aren't behind the staff session, so their component asks
  its own portal route whether to show. A plugin's `index.js` may export `publicRoutes` too: portal
  pages (`/portal/<page>`, in `PortalLayout`), open like the portal's own, each asking its plugin's
  portal route whether it shows. `addSettingsPage({…,
  when: () => isOn(key)})` puts a settings page of its own in Settings' nav while it's on.

## Live updates, notifications and rich text

For a plugin whose screens change as other people act (a chat): generic grants, like the rest,
that do nothing while the plugin is off. [runwellgo-chat](https://github.com/Martin-Business-Consultants/runwellgo-chat) uses them all.

- **`live`**: `api.Publish(ctx, topic, userIDs, data)` pushes `data` (JSON, 64 KB at most) to the
  open tabs of the people named, as an event on `topic` (lowercase, `message`, `typing`).
  `api.Online(userIDs)` says which of them have a tab open now. The core keeps one stream per
  tab, `GET /api/live` (server-sent events, for someone on staff signed in; `404` while no plugin
  that's on was granted live), carrying only what's addressed to the person: `data: {plugin,
  topic, data}` with an `id`, a comment every 25 seconds, and a fresh stream every few minutes. A
  tab that reconnects with `Last-Event-ID` (or `?after=`) gets the events it missed while they're
  still kept (the last 512, in memory), else a `resync` event: read again. Events are best
  effort, never stored; a person with no tab open misses them, so the plugin's routes stay the
  truth. The core sends its own (`_core` `nav`) when a person's bell changed. In the app,
  `subscribe(key, (topic, data) => …)` from `$lib/live.svelte.js` opens the stream with the first
  subscriber and closes it with the last; it hands each plugin only its own events, passes
  `resync` on, and falls back to a `resync` every 20 seconds where the stream can't stay open.
- **`notify`**: `api.Notify(ctx, plugin.Notification{UserID, FromID, Title, Body, Path, Source,
  Subject, Group, Email})` puts a notification in a person's bell and on `/notifications`, beside
  the core's mentions: `Title` and `Body` are the words, `Path` the app path it opens (reading
  it), `Source`/`Subject` what it's about ("Chat", "#general"), `FromID` whose avatar shows.
  Unread ones of the same `Group` collapse into one (its count goes up, its words are the
  latest). `Email` sends it by email too (Settings › Email), the first time only. They're kept
  in `plugin_notifications` and show only while their plugin is on.
- **`api.RichText(html)`** (always): HTML cleaned the core's way (`sanitizeRichText`), its plain
  text and the people it @mentions (`<span data-mention-id="12">`).
- **`Site.MaxUpload`**: the largest file one upload may be (`RUNWELL_MAX_UPLOAD_MB`); a route
  still takes 10 MB at most.

## Numbers for reports

A generic, read-only grant for a plugin that reports on the business ([runwellgo-reports](https://github.com/Martin-Business-Consultants/runwellgo-reports)
uses it); with the plugin off, nothing reads it.

- **`metrics`**: `api.Metrics(ctx, plugin.MetricsQuery{Since, ClientID})` answers the install's
  numbers in a few grouped reads, where `Records` (100 at a time, a record each) would be slow:
  `Clients` (each client today: open, late, due-soon, blocked and in-review work; requests to
  triage and the oldest; open commitments, ours and theirs overdue, due this week; agreements
  waiting on them and since when; contacts and portal contacts; the last touch, a note, work done,
  a decision or a request, and its kind), `Engagements` (each one's derived state, agreed value,
  what's sent and waiting or in draft, its cadence, its work open, done and late, when it was
  approved and closed) and `Facts` (dated, from `Since`, a year back by default and three at most,
  oldest first, 50,000 at most: `client.created`, `engagement.created`, `agreement.sent`,
  `agreement.approved` / `agreement.changes_requested` with `Start` the sending, `todo.created`,
  `todo.done` with `Due`, `request.received`, `request.triaged` with `Start`, `note.added`,
  `commitment.done` / `missed` / `dropped`), each with its client and engagement. `ClientID`
  narrows it to one client. `Prices` is false (and every `*Cents` 0) where the install hides prices.

## Content from other sites

A generic grant for an official plugin whose own Svelte screens load something from another site
(a map provider's script, tiles and fonts; [runwellgo-reports](https://github.com/Martin-Business-Consultants/runwellgo-reports) loads Google
Maps); with the plugin off or the grant unticked, the app's pages load only from this server.

- **`content:external`** ("Load content from other sites"): the manifest's `Content` names the
  sources by Content-Security-Policy directive, `{"script-src": ["https://maps.googleapis.com"],
  "worker-src": ["blob:"]}`. While the plugin is on and granted it, the core adds them to the
  policy the app's pages are served with (`plugin.Host.ContentSources`, `admin.WithSources`); the
  next page load after switching it off has them gone. Directives: `script-src`, `style-src`,
  `img-src`, `font-src`, `connect-src`, `frame-src`, `worker-src`. Sources: a host
  (`maps.gstatic.com`) or a wildcard under a domain (`*.gstatic.com`), optionally `https://`;
  `blob:` and `data:` where they make sense, `'unsafe-eval'` in `script-src`. Never `*`, `http:`
  or `'unsafe-inline'`. The plugin's card lists the hosts ("Loads content from: …").

## Mentions, events of its own and approvals

Generic grants, like the rest, that do nothing while the plugin is off. [runwellgo-ai](https://github.com/Martin-Business-Consultants/runwellgo-ai)
offers its agents as mentionables; [runwellgo-chat](https://github.com/Martin-Business-Consultants/runwellgo-chat) relays the mentions in its
messages.

- **`mentionables`**: `Module.Mentionables(ctx, api)` answers names people can @mention that
  aren't people: `plugin.Mentionable{ID, Handle, Name, Avatar, Description, Kind, Roles,
  Paused}` (`Roles` may mention it; everyone when empty). They're in every @ picker (the core's
  rich text editor: notes, chat) beside the people while the plugin is on, as
  `<span data-mentionable="<key>:<id>">@Name</span>`; the host asks for them at most once a
  minute, and `api.MentionablesChanged()` says they changed. A note on a record that mentions
  one (by someone whose role may) is heard as **`mention.created`** by its plugin alone, data
  `plugin.MentionEvent{ID, Mentionable, Source: "note", Record, NoteID, Author, Text, HTML, Path,
  Subject, Context}`, and that call's `api.Caller()` is the person who wrote it: the plugin reads
  as them through the tools (`api.Background` keeps them), and a change still runs only in their
  own request. `api.Reply(ctx, plugin.Reply{Mention, Body, Replace})` answers where it was
  mentioned: a note on the same record written as the mentionable, for the person
  (`notes.written_as`; it reads "Researcher · via AI for Ted", they may delete it, and with the
  plugin off it reads as theirs with that byline), or, for a relayed mention, a reply in the
  relaying plugin's thread. `Replace` (an earlier reply's id) changes that reply instead ("On
  it…" becoming the answer). Mentions are kept in `plugin_mentions`.
- **`mentions:relay`**: `api.RelayMention(ctx, plugin.Mention{Mentionable, Text, HTML, Path,
  Subject, Context, Ref})`, from a route's call, passes a mention in the plugin's own content
  (`api.RichText(html).Mentionables` finds them) to the plugin offering it, as the caller; its
  replies come to `Module.OnMentionReply(ctx, api, plugin.MentionReply{Ref, As, Via, For, HTML,
  Text, Replace})`, which answers the reply's own id.
- **`api.Emit(ctx, "<key>.<what>", data)`** (always): an event of the plugin's own, with no
  record and no caller, heard by plugins that hook it by name, or with **`*.<what>`** any
  plugin's (`*.approval.requested`; never the core's). Chat emits `chat.message.mentioned`
  (`conversation_id`, `conversation`, `message_id`, `thread_id`, `author`, `text`,
  `mentionable`) when a message mentions a mentionable that's on.

### Asking a person to approve

A plugin whose work must wait for a person before it counts (a runner's pull request, an
import, a payment) asks for it with an event, and needs to know nothing of who keeps the
inbox. With the AI plugin on, the item lands in **Approvals** (`/ai/approvals`), beside the
changes Ask and the agents propose; the approvers are notified and their open pages updated.

```go
api.Emit(ctx, "runners.approval.requested", map[string]any{
	"id":               "job-42",                 // your own id for it (required); the same id updates it
	"kind":             "pull_request",           // what it is, for filtering
	"title":            "Merge the fix for Todo:18",
	"summary":          "The runner finished: 3 files changed, tests pass.",
	"record":           "Todo:18",                // the record it's on, "Type:id"
	"link":             "https://github.com/acme/app/pull/9", // a web address or an app path
	"details_markdown": "```diff\n- old\n+ new\n```", // markdown; a diff block is tinted
	"requested_by":     req.Actor.ID,             // a user id
	"approvers":        []int64{req.Actor.ID},    // user ids, or a role ("manager": managers and owners), or a permission ("<key>:merge")
	"approve":          map[string]any{"tool": "merge_runner_job", "args": map[string]any{"id": "job-42"}},
	"decline":          map[string]any{"tool": "close_runner_job", "args": map[string]any{"id": "job-42"}},
})
// It isn't wanted any more (closed elsewhere):
api.Emit(ctx, "runners.approval.withdrawn", map[string]any{"id": "job-42"})
```

Approving runs `approve.tool` with `args` through the tools grant, as the person deciding,
inside their own request (so the "changes only in a person's request" guarantee holds: make
the tool one of your routes with a `Tool`); declining runs `decline.tool` when it names one.
Who may decide: the user ids named, the role named or above, or those holding the permission
named; with no approvers, the person who asked. The item keeps the outcome: approved or
declined, by whom, their optional note, and done or failed with the tool's own words. With no
approvers given, `requested_by` decides. Without the AI plugin the events go nowhere: keep
your own state as the truth and treat the inbox as where a person is asked.

## Machines

For a plugin that works with machines rather than people ([runwellgo-runners](https://github.com/Martin-Business-Consultants/runwellgo-runners), a
runner on a server of the owner's that does work Runwell hands it): a generic grant, like the rest,
that does nothing while the plugin is off. See [runners.md](runners.md).

- **`machines`**: `api.IssueToken(ctx, label)` makes a token for a machine and answers it (shown
  once: the core keeps only its digest, in `plugin_machine_tokens` under the plugin's key) and its
  id; `api.RevokeToken(ctx, id)` stops it working at once; `api.Tokens(ctx)` lists them (never the
  tokens). A route with `Machine: true` answers only `Authorization: Bearer rwm_…` holding one of
  the plugin's own live tokens: no session, no CSRF, never a person's token, and the token reaches
  nothing else (not the API, MCP, or another plugin's routes). The handler gets `req.Machine`
  (`{TokenID, Label}`) and no `req.Actor`; writes it makes are the system's. A machine route can't
  be public, a portal route, need a capability or be an agent tool. Switching the plugin off stops
  its machines' calls (`404`); removing it revokes every token it issued.

## Installing, switching, removing

The Rails Settings › Plugins routes and agent tools, under `manage_settings`:

```
GET    /api/settings/plugins                 list_plugins
GET    /api/settings/plugins/<key>           show_plugin
PATCH  /api/settings/plugins/<key>           switch_plugin    {"plugin": {"enabled": true, "granted": {…}}}
DELETE /api/settings/plugins/<key>[?purge_data=1]   remove_plugin
POST   /api/settings/plugins/installations   install_plugin   {"repo": "time_tracking"}
POST   /api/settings/plugins/check           check_plugin_updates
POST   /api/settings/plugins/<key>/upgrade   update_plugin
```

`list_plugins` answers the installed `plugins` (with what each asks for, was
granted and adds), `failed_to_load` (installed but not in this build: kept,
off, its data kept), `available` (in this build, not installed) and `changes`
(the last installs and removals). `install_plugin` takes a key or a
repository's name (`Martin-Business-Consultants/runwell-time-tracking` is
`time_tracking`) and records the plugin, off. `switch_plugin` switches it, with
`granted` the grants (never wider than asked); switching on without `granted`
grants what it asks for. Switching is recorded as `settings.plugin`
`{plugin, on}` on the settings, as the Rails core did; installs, grants,
upgrades and removals as `plugin.installed`, `plugin.granted`,
`plugin.upgraded` and `plugin.removed`. Plugins update with Runwell, so
`check_plugin_updates` and `update_plugin` say so.

A new build whose plugin's manifest changed upgrades it in place at start: it
keeps the grants it still asks for, and if it asks for more it's off until an
owner grants the rest.

The app reads `GET /api/plugin_extensions` (where the plugins that are on show
for the person) once someone signs in, and `GET /api/plugin_screens/<key>`
(the screens they may open and the routes they may write to) to draw a
plugin's screens.

## The API is a contract

What a plugin may use of Runwell is two packages, versioned with Runwell's
module (`github.com/Martin-Business-Consultants/runwellgo/v3`):

- **`plugin`**: `Module`, `Manifest`, `Grants`, `API`, `Request`, `Response`
  and the rest above, and the core's types a plugin meets, named here so it
  needs nothing internal: `plugin.Time` (a nullable timestamp, the core's
  column type), `plugin.Now()`, `plugin.TimeOf(t)`, `plugin.MailMessage` (what
  `Env.SendMail` sends) and `plugin.DB` (the database a `Host` runs on).
- **`plugintest`**, for its tests: `plugintest.Setup(t, env, Module())` is a
  host on a fresh database (Runwell's own tables, made as the core makes them)
  with the plugin installed, granted and on; `plugintest.Env` is a stand-in for
  the core that answers just enough (no records, no tools, mail that goes
  nowhere), to embed in a type of your own that answers what a test needs;
  `DB`, `Open`, `Host`, `Enable`, `Inline` (a `Spawn` that runs at once) and
  `UserRemoved` are its parts.

Within v3, both only grow: nothing is removed or renamed, and a new field or
method never changes what an old one does, so a plugin built against v3.1
builds against every later v3. A change that has to break them is Runwell v4
(and a `/v4` module path). Everything under `internal/` is Runwell's own and
can change in any release: Go won't let another module import it.

Keys, table names, manifests and grants are the other half of the contract:
an install keeps a plugin's data and grants under its key, so a plugin keeps
its key and tables for good (a newer version adds tables and columns, as
`Schema` allows).

## Writing a plugin in its own repository

Start from [runwellgo-plugin-template](https://github.com/Martin-Business-Consultants/runwellgo-plugin-template)
(**Use this template** on GitHub): Hello, a `go.mod` that requires
`github.com/Martin-Business-Consultants/runwellgo/v3`, a test on `plugintest`,
the same `.golangci.yml` and a CI workflow (vet, race-checked tests,
golangci-lint). Give it your own module path and key, and tag releases with
the manifest's version (`v1.2.0`).

```go
func TestGreeting(t *testing.T) {
	h, _ := plugintest.Setup(t, env{}, Module()) // env embeds plugintest.Env
	res, err := h.Serve(t.Context(), "hello", "client",
		&plugin.Request{Method: "GET", Query: url.Values{"record": {"Client:7"}}, Actor: &plugin.Caller{ID: 1, Role: "owner"}}, nil)
	…
}
```

**Building Runwell with it** is a `main.go` of your own, in a repository (or
a directory) of its own, listing every plugin your Runwell has, the official
ones you want included:

```go
package main

import (
	"os"

	ai "github.com/Martin-Business-Consultants/runwellgo-ai"
	"github.com/Martin-Business-Consultants/runwellgo/v3/runwell"
	mine "github.com/you/runwell-mine"
)

var version = "dev" // go build -ldflags "-X main.version=1.0.0"

func main() {
	os.Exit(runwell.Main(version, ai.Module(), mine.Module()))
}
```

`go mod init`, `go get` each module, `go build -o runwell .`: that's the whole
of Runwell (`serve`, `user`, `import`, `update`, the CLI) with your plugins.
`runwell import` makes the tables of the plugins it was built with for a data
export's rows, and the update button installs official releases, which carry
the official plugins only: a binary of your own updates by being rebuilt.

**Deploying it to Hoster** is the same as deploying Runwell: copy
[Dockerfile](../Dockerfile), [docker/runwell](../docker/runwell) and
[config/deploy.yml](../config/deploy.yml) into that repository, and point the
Dockerfile's build at your main:

```dockerfile
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=
RUN V="${VERSION:-$(cat VERSION 2>/dev/null || echo dev)}" \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w -X main.version=${V#v}" -o /out/runwell .
```

Then in Hoster: a new app from your repository, its volume at `/data`, as
[going-live.html](going-live.html) says for Runwell's own.

**Its screens**: the app your binary serves is the one built into the
runwellgo module, which carries the official plugins' Svelte screens. A
plugin of your own draws its screens through its manifest (screens, panels,
nav, settings, tabs: above), which needs no build of the app. One that needs
Svelte screens of its own puts them in its `ui/` (an `index.js` exporting
`key`, `routes` and `setup`, as the official ones do) and is added to the
official binary: `cmd/runwell/go.mod` requires it and `make plugin-ui` copies
its `ui/` into the app.

**Working on one beside Runwell**: check it out next to runwellgo and run
`go work use ../runwell-mine` there (don't commit that line of `go.work`):
`make build`, `make run` and `make plugin-ui` then use your checkout.

## Porting a Rails plugin

- Its tables become storage: a record per key (`entry:<id>` as JSON), read in
  one go with `KVValues`. Point at core records by their name (`Todo:40`),
  and keep what a list needs (the label, the engagement and client ids) in the
  record, as the Rails plugin kept them in columns.
- Its controllers become routes, each action's `agent_tool` a `Tool` of the
  same name; `can?` becomes `req.Actor.Can`, `Current.user` `req.Actor`.
- Its partials become screens; a slot's partial becomes a panel; its
  subscribers become `Hooks` and `OnEvent`; its nightly job a task.
- Model verbs on core records become the `records:write` verbs; ask for a new
  verb when a plugin needs one, rather than writing around the core.
