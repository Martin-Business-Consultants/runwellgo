# Built-in plugins' own screens

A plugin compiled into Runwell (`plugins/<key>` in Go) may draw screens of its own here, in
`ui/src/plugins/<key>/`, when its manifest's described screens aren't enough. Each folder has an
`index.js` that exports:

```js
export const key = "my_day";                       // the plugin's key
export const routes = [["/my-day", DayPage]];       // its pages (staff), shown only while it's on
export const publicRoutes = [["/portal/x", Page]];  // optional: portal pages, open (each asks its portal route)
export function setup() {                           // optional, once: tabs on core records
  addTab("Client", { key: "my_day", label: () => "My day", component: Tab, when: () => isOn(key) });
}
```

Its screens talk only to its own routes (`/api/x/<key>/…`), hear its live events with
`subscribe(key, fn)` from `$lib/live.svelte.js` (when it was granted `live`), and use the app's shared pieces
(`$lib/…`, shadcn components). Nothing in the core imports a plugin's folder: `routes.js` picks
up every `index.js` here, and a plugin that's off shows nothing.
