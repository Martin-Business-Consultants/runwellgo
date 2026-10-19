// Engagements: what we agreed with a client (projects, work orders,
// services), their agreement versions and scope items. Adds the
// Engagements tab to a client's page.
import EngagementsIndex from "./EngagementsIndex.svelte";
import EngagementShow from "./EngagementShow.svelte";
import EngagementForm from "./EngagementForm.svelte";
import ScopeItemShow from "./ScopeItemShow.svelte";
import ClientEngagements from "./ClientEngagements.svelte";
import { addTab } from "$lib/record-tabs.js";
import { session } from "$lib/session.svelte.js";

addTab("Client", {
  key: "engagements",
  label: () => session.term("engagement", 2),
  count: (record) => record.engagements?.length,
  component: ClientEngagements,
  order: 10,
});

export const routes = [
  ["/engagements", EngagementsIndex],
  ["/engagements/new", EngagementForm],
  ["/engagements/:ref/edit", EngagementForm],
  ["/engagements/:ref", EngagementShow],
  ["/scope_items/:id", ScopeItemShow],
];
export const publicRoutes = [];
