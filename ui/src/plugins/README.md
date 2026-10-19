# The plugins' own screens

An official plugin (a Go module in its own repository, compiled in through `cmd/runwell`) may draw
screens of its own, when its manifest's described screens aren't enough. They live in its
repository's `ui/` folder, and `make plugin-ui` ([scripts/plugin-ui.js](../../scripts/plugin-ui.js),
run by `make ui`, `make ui-dev` and CI) copies each one here, into `ui/src/plugins/<key>/`: it asks
Go where each plugin module `cmd/runwell/go.mod` requires is (`go list -m`; a checkout you've added
to `go.work` wins) and copies its `ui/` when it has one. The copies are gitignored: change a
plugin's screens in its own repository (or your checkout of it), then run `make plugin-ui` again.
Its `sources.css` names the copies for Tailwind, which skips gitignored files otherwise.

Each folder has an `index.js` that exports:

```js
export const key = "my_day";                       // the plugin's key (the folder it's copied to)
export const routes = [["/my-day", DayPage]];       // its pages (staff), shown only while it's on
export const publicRoutes = [["/portal/x", Page]];  // optional: portal pages, open (each asks its portal route)
export function setup() {                           // optional, once: tabs on core records
  addTab("Client", { key: "my_day", label: () => "My day", component: Tab, when: () => isOn(key) });
}
```

Its screens talk only to its own routes (`/api/x/<key>/…`), hear its live events with
`subscribe(key, fn)` from `$lib/live.svelte.js` (when it was granted `live`), and use the app's shared
pieces (`$lib/…`, shadcn components, and the app's own modules by their path from here, as
`../../routes/plugins/extensions.svelte.js`). Nothing in the core imports a plugin's folder:
`routes.js` picks up every `index.js` here, wrapped in `PluginGate.svelte`, and a plugin that's off
shows nothing. `PluginGate.svelte` and this README stay here; everything else in this folder is a
copy.
