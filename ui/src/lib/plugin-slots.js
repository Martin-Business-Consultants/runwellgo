// Places in the app's own screens where an official plugin's Svelte screens
// (its ui/, copied into ui/src/plugins/<key>) draw a component of theirs, from their index.js:
//
//   addSlot("client_aside", { key: "ai", component: ClientAside, order: 10 });
//
// The screen renders <PluginSlot name="client_aside" record={client} />,
// which draws each one while its plugin is on (and nothing otherwise),
// with the props it was given. The slots:
//
//   header          the app's header, beside Search and Add (on every page)
//   home_top        home, above the briefing
//   client_aside    a client's side column ({record})
//   request_aside   a request's side column ({record})
//   scope_item_aside  a scope item's side column ({record})
//   draft_version   under a draft agreement ({version, engagement})
//   commitment_row  each commitment in a list ({commitment})
//   note_row        each note in a list ({note, subjectType, subjectId})
//   portal_header   the client portal's header: not behind isOn (the portal
//                   has no staff session to ask), so its component asks its
//                   own portal route whether to show
//   portal_nav      the portal's page links (a link to a plugin's portal page,
//                   from its index.js publicRoutes); like portal_header, it
//                   asks its own portal route whether to show
const slots = {};

export function addSlot(name, entry) {
  const list = (slots[name] ||= []);
  const i = list.findIndex((e) => e.key === entry.key && e.component === entry.component);
  if (i >= 0) list[i] = entry; else list.push(entry);
  list.sort((a, b) => (a.order ?? 50) - (b.order ?? 50));
}

export const slotsFor = (name) => slots[name] || [];
