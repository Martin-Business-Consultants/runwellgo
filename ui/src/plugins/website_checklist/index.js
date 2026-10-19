// Websites (the website_checklist plugin's own screens): every client's
// websites at /websites, a site's go-live checklist at /websites/:id, its
// history, and a Websites tab on each client while the plugin is on.
import { addTab } from "$lib/record-tabs.js";
import { isOn } from "../../routes/plugins/extensions.svelte.js";
import SitesIndex from "./SitesIndex.svelte";
import SiteShow from "./SiteShow.svelte";
import SiteForm from "./SiteForm.svelte";
import HistoryPage from "./HistoryPage.svelte";
import ClientWebsites from "./ClientWebsites.svelte";

export const key = "website_checklist";

export const routes = [
  ["/websites", SitesIndex],
  ["/websites/new", SiteForm],
  ["/websites/history", HistoryPage],
  ["/websites/:id/edit", SiteForm],
  ["/websites/:id", SiteShow],
];

export function setup() {
  addTab("Client", { key, label: () => "Websites", component: ClientWebsites, order: 85, when: () => isOn(key) });
}
