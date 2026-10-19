// What the Websites screens share: the plugin's routes, and how a step's
// state and a checklist's count look.
export const X = "/x/website_checklist";

// The plugin's route, with a query: wc("/websites", {show: "all"}) → "/x/website_checklist/websites?show=all".
export function wc(path, query = {}) {
  const q = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== "" && v != null)).toString();
  return X + path + (q ? "?" + q : "");
}

// A step's state, tinted like the core's status tags: confirmed green, to do
// amber, not applicable grey.
export const STATE_CLASSES = {
  confirmed: "border-transparent bg-emerald-500/15 text-emerald-700 dark:text-emerald-400",
  unconfirmed: "border-transparent bg-amber-500/15 text-amber-700 dark:text-amber-400",
  not_applicable: "border-transparent bg-muted text-muted-foreground",
};

// A checklist's count: green once every step that applies is confirmed.
export const summaryClass = (checklist) => (checklist?.ready ? STATE_CLASSES.confirmed : STATE_CLASSES.unconfirmed);

// The steps by group, in the page's order (the server answers them so).
export function groupsOf(items) {
  const out = [];
  for (const i of items) {
    let g = out.find((x) => x.group === i.group);
    if (!g) out.push((g = { group: i.group, items: [] }));
    g.items.push(i);
  }
  return out;
}

// "3 of 4 done", beside a group's heading; done when every step that
// applies is confirmed.
export function groupSummary(items) {
  const applicable = items.filter((i) => i.state !== "not_applicable");
  const confirmed = applicable.filter((i) => i.state === "confirmed").length;
  return { text: `${confirmed} of ${applicable.length} done`, done: applicable.length > 0 && confirmed === applicable.length };
}
