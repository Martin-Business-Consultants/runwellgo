// Every screen, by address: each area of the app lists its own in
// routes/<area>/index.js. Staff screens need someone on staff signed in;
// public ones (the portal, approval links, emailed sign-in links) don't.
import * as home from "./routes/home/index.js";
import * as clients from "./routes/clients/index.js";
import * as engagements from "./routes/engagements/index.js";
import * as work from "./routes/work/index.js";
import * as commitments from "./routes/commitments/index.js";
import * as requests from "./routes/requests/index.js";
import * as notifications from "./routes/notifications/index.js";
import * as settings from "./routes/settings/index.js";
import * as plugins from "./routes/plugins/index.js";
import * as portal from "./routes/portal/index.js";
import * as approvals from "./routes/approvals/index.js";
import * as agents from "./routes/agents/index.js";

const areas = [home, clients, engagements, work, commitments, requests, notifications, settings, plugins, agents, portal, approvals];

// The plugins' own screens (plugins/<key>/index.js, copied in from each
// plugin's module by make plugin-ui), each behind its
// plugin being on.
import PluginGate from "./plugins/PluginGate.svelte";
const pluginUIs = Object.values(import.meta.glob("./plugins/*/index.js", { eager: true }));
for (const p of pluginUIs) p.setup?.();
const pluginRoutes = pluginUIs.flatMap((p) => (p.routes || []).map(([pattern, screen]) => [pattern, PluginGate, { plugin: p.key, screen }]));

export const routes = [...areas.flatMap((a) => a.routes), ...pluginRoutes];
// A plugin's portal pages (publicRoutes) are open like the portal's own:
// each asks its plugin's portal route whether it shows.
export const publicRoutes = [...areas.flatMap((a) => a.publicRoutes), ...pluginUIs.flatMap((p) => p.publicRoutes || [])];
