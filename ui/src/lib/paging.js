// A list's page in the address (?page=), how many a page holds for its
// view (a table 25 rows, a cards grid 24, rows of 2 or 3), its sort and its
// search (?q). A board shows everything, so it asks for no pages. Spread it
// into the list's query:
// get(href("/clients", { status, ...pageQuery(view) })).
import { router } from "./router.svelte.js";

export const PER = { table: 25, cards: 24 };

export function pageQuery(view) {
  const per = PER[view];
  if (!per) return {};
  return { per, page: router.query.get("page"), sort: router.query.get("sort"), direction: router.query.get("direction"), q: router.query.get("q") };
}
