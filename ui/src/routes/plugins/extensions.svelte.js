// Where the plugins that are on show, for the person signed in
// (/api/plugin_extensions): each record panel becomes a tab on its
// record's page (addTab), a settings page joins Settings' nav
// (addSettingsPage), and home's cards and the quick actions are kept here
// for <PluginHome> and <PluginQuickActions>. It loads before the first
// screen draws (session.signedIn), and again when a plugin is switched.
import { addTab, removeTab } from "$lib/record-tabs.js";
import { addSettingsPage } from "$lib/settings-nav.js";
import PluginPanel from "./PluginPanel.svelte";

// Which record type each slot puts a tab on.
const SLOT_TYPES = { client_panel: "Client", engagement_panel: "Engagement", todo_panel: "Todo" };

class Extensions {
  enabled = $state([]); // the keys of the plugins that are on
  home = $state([]); // [{key, plugin, title, screen}], home's briefing cards
  top = $state([]); // [{key, plugin, label, screen}], above the briefing (home_top)
  quickActions = $state([]); // [{key, label, icon, types, screen}]
  loaded = false;
  tabs = []; // [type, tab key] of the tabs added, to take back
  settings = []; // hrefs of the settings pages added

  async load() {
    const { get } = await import("$lib/api.js");
    const x = await get("/plugin_extensions", { quiet: true });
    // A plugin switched off takes its tabs and pages with it.
    for (const [type, key] of this.tabs) removeTab(type, key);
    for (const href of this.settings) addSettingsPage({ href, label: "", group: "Plugins and AI", perm: "plugin-off" });
    this.tabs = [];
    this.settings = [];
    x.panels.forEach((p, i) => {
      const type = SLOT_TYPES[p.slot];
      if (!type) return;
      const key = `plugin-${p.key}-${p.screen}`;
      // A tab's component gets {record, reload}; this one is told which
      // plugin's screen it draws too.
      const component = (anchor, props) => PluginPanel(anchor, withProps(props, { plugin: p.key, screen: p.screen }));
      addTab(type, { key, label: () => p.label, component, order: 80 + i });
      this.tabs.push([type, key]);
    });
    x.settings.forEach((s, i) => {
      addSettingsPage({ href: s.href, label: s.label, group: "Plugins and AI", perm: "manage_settings", order: 41 + i });
      this.settings.push(s.href);
    });
    this.enabled = x.enabled || [];
    this.home = x.home;
    this.top = x.panels.filter((p) => p.slot === "home_top");
    this.quickActions = x.quick_actions;
    this.loaded = true;
  }
}

// withProps is props with a few more, read live.
function withProps(props, more) {
  return new Proxy(props, {
    get: (t, k) => (k in more ? more[k] : t[k]),
    has: (t, k) => k in more || k in t,
  });
}

export const extensions = new Extensions();

// isOn says whether a plugin is on: its own screens and tabs show only then.
export const isOn = (key) => extensions.enabled.includes(key);
