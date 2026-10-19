// Clients: who we work for, and their contacts.
import ClientsIndex from "./ClientsIndex.svelte";
import ClientShow from "./ClientShow.svelte";
import ClientForm from "./ClientForm.svelte";

export const routes = [
  ["/clients", ClientsIndex],
  ["/clients/new", ClientForm],
  ["/clients/:id/edit", ClientForm],
  ["/clients/:id", ClientShow],
];
export const publicRoutes = [];
