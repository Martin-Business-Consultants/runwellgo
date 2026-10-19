// Plugins: Settings › Plugins (install, switch on, grant, remove), each
// plugin's settings page, its own pages (/p/<key>/<page>), and what the
// plugins that are on add elsewhere: record tabs, settings pages, home's
// cards and quick actions (extensions.svelte.js), loaded once someone is
// signed in, before the first screen draws.
import PluginsSettings from "./PluginsSettings.svelte";
import PluginSettingsPage from "./PluginSettingsPage.svelte";
import PluginPage from "./PluginPage.svelte";
import { addSettingsPage } from "$lib/settings-nav.js";
import { sessionHooks } from "$lib/session.svelte.js";
import { extensions } from "./extensions.svelte.js";

addSettingsPage({ href: "/settings/plugins", label: "Plugins", group: "Plugins and AI", perm: "manage_settings", order: 10 });
sessionHooks.push(() => extensions.load());

export const routes = [
  ["/settings/plugins", PluginsSettings],
  ["/settings/plugins/:key", PluginSettingsPage],
  ["/p/:key/:page", PluginPage],
];
export const publicRoutes = [];
