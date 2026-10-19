import { tick, untrack } from "svelte";
import { readsSettled } from "./api.js";

// The app's router, on real addresses (/clients/12?tab=work), so a link
// mailed out (/approve/<token>, /portal/session/<token>) opens the right
// screen. Routes are patterns with :params and a trailing *rest; the first
// that matches wins. Clicks on same-origin links are routed in the page.
class Router {
  path = $state("/");
  query = $state(new URLSearchParams());

  constructor() {
    const read = () => {
      this.path = location.pathname.replace(/\/+$/, "") || "/";
      this.query = new URLSearchParams(location.search);
    };
    read();
    addEventListener("popstate", () => this.change(read));
    this.read = read;
    document.addEventListener("click", (e) => {
      if (e.defaultPrevented || e.button !== 0 || e.metaKey || e.ctrlKey || e.shiftKey || e.altKey) return;
      const a = e.target.closest?.("a[href]");
      if (!a || a.target === "_blank" || a.hasAttribute("download") || a.dataset.external !== undefined) return;
      const url = new URL(a.href, location.href);
      if (url.origin !== location.origin || url.pathname.startsWith("/api/") || url.pathname.startsWith("/files/")) return;
      if (url.pathname === location.pathname && url.search === location.search && url.hash) return;
      e.preventDefault();
      this.navigate(url.pathname + url.search + url.hash);
    });
  }

  // transition: false for a move that shouldn't ease (typing in a search
  // box moves the address with each pause).
  navigate(to, { replace = false, transition = true } = {}) {
    if (to === location.pathname + location.search) return;
    history[replace ? "replaceState" : "pushState"]({}, "", to);
    // Next tick: a screen may redirect while it's being made, and the
    // router's state can't change in the middle of drawing.
    queueMicrotask(() => (transition ? this.change(this.read) : this.read()));
  }

  // change moves to what the address now says inside the browser's own view
  // transition, where it has one: the screen as it was stays up while the
  // new one reads its data (up to WAIT), then one fades into the other. No
  // transition for reduced motion, or while one is already running.
  change(read) {
    if (!document.startViewTransition || matchMedia("(prefers-reduced-motion: reduce)").matches || this.transitioning) return read();
    this.transitioning = true;
    const t = document.startViewTransition(async () => {
      read();
      await tick();
      await Promise.race([readsSettled(), new Promise((r) => setTimeout(r, WAIT))]);
      await tick();
    });
    t.finished.finally(() => (this.transitioning = false));
  }

  match(routes) {
    // A route may carry props for its component as a third element.
    for (const [pattern, component, props] of routes) {
      const params = matchPattern(pattern, this.path);
      if (params) return { component, params, props: props || {} };
    }
    return null;
  }
}

function matchPattern(pattern, path) {
  const a = pattern.split("/").filter(Boolean);
  const b = path.split("/").filter(Boolean);
  const params = {};
  for (let i = 0; i < a.length; i++) {
    if (a[i] === "*rest") { params.rest = b.slice(i).map(decodeURIComponent).join("/"); return params.rest ? params : null; }
    if (b[i] === undefined) return null;
    if (a[i].startsWith(":")) params[a[i].slice(1)] = decodeURIComponent(b[i]);
    else if (a[i] !== b[i]) return null;
  }
  return a.length === b.length ? params : null;
}

// The longest a move waits for its screen's data before showing it anyway.
const WAIT = 800;

export const router = new Router();

// onQuery(load) runs load now and again whenever the address's query
// changes on the same screen (a filter, a sort, a page, a search): the
// screen stays as it is and only its data is read again, so nothing
// flickers. Call it while a component is being made, in place of load().
export function onQuery(fn) {
  const path = router.path;
  $effect(() => {
    router.query;
    if (router.path === path) untrack(fn);
  });
}

// href("/clients", {status: "active"}) → "/clients?status=active"
export function href(path, query) {
  const q = query ? new URLSearchParams(Object.entries(query).filter(([, v]) => v !== "" && v != null)).toString() : "";
  return path + (q ? "?" + q : "");
}

// go("/clients/12") moves to a screen; replace keeps it out of history.
export function go(path, query, opts) { router.navigate(href(path, query), opts); }

// The current address with query keys changed (a filter link). A change
// other than the page itself goes back to the first page.
export function withQuery(changes) {
  const q = new URLSearchParams(router.query);
  for (const [k, v] of Object.entries(changes)) {
    if (v === "" || v == null) q.delete(k); else q.set(k, v);
  }
  if (!("page" in changes)) q.delete("page");
  const s = q.toString();
  return router.path + (s ? "?" + s : "");
}
