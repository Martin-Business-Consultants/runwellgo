// Settings' own sidebar (the Rails section_nav): its pages in groups. Each
// area adds the settings pages it keeps, from its routes/<area>/index.js:
//
//   addSettingsPage({ href: "/settings/plugins", label: "Plugins", group: "Plugins and AI", perm: "manage_settings", order: 10 });
//
// perm hides a page from people whose role lacks it; when: () => bool shows
// it only while that holds (a plugin's, while it's on). Groups show in the
// order of GROUPS: yourself, then the workspace (how Runwell reads and
// looks), who's in it and how they get in, how it talks to the outside,
// what extends it, its data, and help.
export const GROUPS = ["You", "Workspace", "Team and access", "Connections", "Plugins and AI", "Data", "Help"];
// Groups pages were filed under before (a plugin's "Install" page lands
// with the plugins).
const ALIASES = { Install: "Plugins and AI", People: "Team and access", "Plugins & AI": "Plugins and AI" };
const pages = [];

export function addSettingsPage(p) {
  p = { ...p, group: ALIASES[p.group] || p.group };
  const i = pages.findIndex((x) => x.href === p.href);
  if (i >= 0) pages[i] = p; else pages.push(p);
}

export function settingsPages(can) {
  return GROUPS.map((g) => [g, pages.filter((p) => p.group === g && (!p.perm || can(p.perm)) && (!p.when || p.when())).sort((a, b) => (a.order ?? 50) - (b.order ?? 50))])
    .filter(([, list]) => list.length);
}
