// The agents screens: Settings › Connected apps (the AI apps and tokens
// acting as you), and the OAuth consent an app sends a person to, open
// signed out (it signs them in first, or hands a client to the portal's).
import { addSettingsPage } from "$lib/settings-nav.js";
import ConnectedApps from "./ConnectedApps.svelte";
import OauthAuthorize from "./OauthAuthorize.svelte";

addSettingsPage({ href: "/settings/connected_apps", label: "Connected apps", group: "Connections", order: 10 });

export const routes = [["/settings/connected_apps", ConnectedApps]];
export const publicRoutes = [["/oauth/authorize", OauthAuthorize]];
