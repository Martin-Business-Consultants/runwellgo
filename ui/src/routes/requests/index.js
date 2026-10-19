// Requests: what came in from clients, waiting for triage.
import RequestsIndex from "./RequestsIndex.svelte";
import RequestShow from "./RequestShow.svelte";
import RequestForm from "./RequestForm.svelte";

export const routes = [
  ["/requests", RequestsIndex],
  ["/requests/new", RequestForm],
  ["/requests/:id/edit", RequestForm],
  ["/requests/:id", RequestShow],
];
export const publicRoutes = [];
