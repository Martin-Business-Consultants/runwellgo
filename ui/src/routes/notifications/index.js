// What reaches a person and what's kept on any record: notifications (the
// bell), search everything, and the notes and documents tabs on a client's,
// an engagement's, a piece of work's and a request's page.
import NotificationsIndex from "./NotificationsIndex.svelte";
import NotificationOpen from "./NotificationOpen.svelte";
import SearchPage from "./SearchPage.svelte";
import NotesTab from "./NotesTab.svelte";
import DocumentsTab from "./DocumentsTab.svelte";
import { addTab } from "$lib/record-tabs.js";

export const routes = [
  ["/notifications", NotificationsIndex],
  ["/notifications/:id", NotificationOpen],
  ["/search", SearchPage],
];
export const publicRoutes = [];

for (const type of ["Client", "Engagement", "Todo", "Request", "ScopeItem"]) {
  addTab(type, { key: "notes", label: () => "Notes", count: (r) => r.notes?.length, component: NotesTab, order: 80 });
  addTab(type, { key: "documents", label: () => "Documents", count: (r) => r.documents?.length, component: DocumentsTab, order: 90 });
}
