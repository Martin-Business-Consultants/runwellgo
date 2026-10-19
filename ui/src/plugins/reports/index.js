// Reports (plugins/reports): the business at a glance on a map, at
// /reports; "Where they are" and "Their progress report" in each client's
// side column; Settings › Reports; the owner's preview of a client's report;
// and, in the portal, "Your progress" (the portal_nav link and the page,
// each asking the plugin's portal route whether it shows). Staff screens
// show only while the plugin is on.
import { addSlot } from "$lib/plugin-slots.js";
import { addSettingsPage } from "$lib/settings-nav.js";
import { isOn } from "../../routes/plugins/extensions.svelte.js";
import ReportsPage from "./ReportsPage.svelte";
import ReportsSettings from "./ReportsSettings.svelte";
import ClientReportPreview from "./ClientReportPreview.svelte";
import ClientPlace from "./ClientPlace.svelte";
import ClientReportAside from "./ClientReportAside.svelte";
import PortalNavLink from "./PortalNavLink.svelte";
import PortalProgress from "./PortalProgress.svelte";

export const key = "reports";
export const routes = [
  ["/reports", ReportsPage],
  ["/reports/clients/:id/client_report", ClientReportPreview],
  ["/settings/reports", ReportsSettings],
];
export const publicRoutes = [["/portal/progress", PortalProgress]];

export function setup() {
  addSlot("client_aside", { key, component: ClientPlace, order: 30 });
  addSlot("client_aside", { key, component: ClientReportAside, order: 31 });
  addSlot("portal_nav", { key, component: PortalNavLink });
  addSettingsPage({ href: "/settings/reports", label: "Reports", group: "Plugins and AI", perm: "manage_settings", order: 47, when: () => isOn(key) });
}
