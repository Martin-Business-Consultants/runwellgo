// What the sidebar lists: Home, then Runwell's three things, each with
// what belongs to it: who we work for (clients, and the requests that come
// from them), what we agreed (engagements, and the commitments made on
// them) and what's happening (work). Named in this install's words, then
// each plugin's pages. Notifications aren't a section: they're the bell in
// the header. Plugins' items come from /api/nav.
import House from "@lucide/svelte/icons/house";
import Users from "@lucide/svelte/icons/users";
import Briefcase from "@lucide/svelte/icons/briefcase";
import ListChecks from "@lucide/svelte/icons/list-checks";
import Handshake from "@lucide/svelte/icons/handshake";
import Inbox from "@lucide/svelte/icons/inbox";
import { pluginIcon } from "../routes/plugins/icons.js";
import { session } from "./session.svelte.js";

export function coreNav() {
  return [
    { href: "/", label: "Home", icon: House, exact: true, key: "g h" },
    { href: "/clients", label: session.term("client", 2), icon: Users, key: "g c", children: [
      { href: "/requests", label: "Requests", icon: Inbox, key: "g r", badge: "requests" },
    ] },
    { href: "/engagements", label: session.term("engagement", 2), icon: Briefcase, key: "g e", children: [
      { href: "/commitments", label: session.term("commitment", 2), icon: Handshake, key: "g m" },
    ] },
    { href: "/work", label: session.term("work", 2), icon: ListChecks, key: "g w" },
  ];
}

// The plugins' pages and the counts beside items (open requests, unread
// notifications), refreshed as the person moves around.
class Nav {
  plugins = $state([]);
  counts = $state({});

  async refresh() {
    const { get } = await import("./api.js");
    try {
      const n = await get("/nav", { quiet: true });
      this.plugins = (n.plugins || []).map((p) => ({ ...p, icon: pluginIcon(p.icon) }));
      this.counts = n.counts || {};
    } catch {
      // The sidebar keeps what it had.
    }
  }
}

export const nav = new Nav();
