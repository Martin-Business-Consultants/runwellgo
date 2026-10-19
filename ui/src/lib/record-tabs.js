// The tabs on a record's page (a client's, an engagement's, a piece of
// work's). Each area adds the tabs it keeps, from its routes/<area>/index.js:
//
//   addTab("Client", { key: "engagements", label: () => session.term("engagement", 2),
//     count: (record) => record.engagements?.length, component: ClientEngagements, order: 10 });
//
// The page shows them in order; the tab's component gets {record, reload}.
// A tab with when: () => bool shows only while that holds (a plugin's, while
// it's on).
const tabs = {};

export function addTab(type, tab) {
  const list = (tabs[type] ||= []);
  const i = list.findIndex((t) => t.key === tab.key);
  if (i >= 0) list[i] = tab; else list.push(tab);
  list.sort((a, b) => (a.order ?? 50) - (b.order ?? 50));
}

export const tabsFor = (type) => (tabs[type] || []).filter((t) => !t.when || t.when());

// removeTab takes a tab back (a plugin switched off).
export function removeTab(type, key) {
  const list = tabs[type] || [];
  const i = list.findIndex((t) => t.key === key);
  if (i >= 0) list.splice(i, 1);
}
