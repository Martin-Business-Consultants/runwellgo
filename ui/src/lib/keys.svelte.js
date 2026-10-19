// The app's keyboard, vim's way, on every staff screen:
//
//   g then h c r e m w n d s   go to Home, Clients, Requests, Engagements,
//                              Commitments, Work, Notifications, My day,
//                              Settings
//   j / k                      the next / previous item on the screen
//   gg / G                     the first / last item
//   Enter or o                 open the item (O: in a new tab)
//   x                          pick the item (its checkbox), for the bulk bar
//   h / l                      the previous / next tab or filter tab
//   [ / ]                      the previous / next page of a list
//   /                          the screen's search box (else search everything)
//   n                          New (the screen's New button)
//   e                          Edit (the screen's Edit button)
//   Escape                     let go of the item
//   ?                          these keys
//
// Search (s, ⌘K), Add (a) and Ask (i) are their own components'. A screen's
// own keys win: a handler that takes a key calls preventDefault, and these
// run after it. Items are what a screen marks data-nav-item, else its
// table's rows, its cards that are links and its list items with a link
// or a button, innermost first, in the order they're on screen.
import { go } from "./router.svelte.js";

export const GOTO = [
  ["h", "/", "Home"],
  ["c", "/clients", "Clients"],
  ["r", "/requests", "Requests"],
  ["e", "/engagements", "Engagements"],
  ["m", "/commitments", "Commitments"],
  ["w", "/work", "Work"],
  ["n", "/notifications", "Notifications"],
  ["d", "/my-day", "My day"],
  ["s", "/settings", "Settings"],
];

const ITEMS = "[data-nav-item], tbody tr, a:has(> [data-slot=card]), li:has(a[href], button)";
const SKIP = "nav, [role=tablist], [role=menu], [role=listbox], [data-nav-skip], header";

class Keys {
  help = $state(false);
  #current = null; // the item the cursor is on
  #pending = ""; // "g", waiting for the next key
  #timer;

  constructor() {
    if (typeof window === "undefined") return;
    // After the event has been to everyone else: a screen's own key wins.
    addEventListener("keydown", (e) => setTimeout(() => this.#key(e)));
    addEventListener("popstate", () => this.#drop());
  }

  // items are the screen's items, in order, visible ones only.
  items() {
    const main = document.getElementById("main");
    if (!main) return [];
    const all = [...main.querySelectorAll(ITEMS)].filter((el) => (el.matches("[data-nav-item]") || !el.parentElement.closest(SKIP)) && el.getClientRects().length);
    // Innermost: drop an item that holds another.
    return all.filter((el) => !all.some((o) => o !== el && el.contains(o)));
  }

  // current is the item the cursor is on, if it's still on screen.
  current() { return this.#here(); }

  #drop() {
    this.#current?.removeAttribute("data-nav-current");
    this.#current = null;
  }

  #move(to) {
    const list = this.items();
    if (!list.length) return;
    let i = list.indexOf(this.#current);
    i = to === "first" ? 0 : to === "last" ? list.length - 1 : i < 0 ? (to > 0 ? 0 : list.length - 1) : Math.max(0, Math.min(list.length - 1, i + to));
    this.#drop();
    this.#current = list[i];
    this.#current.setAttribute("data-nav-current", "");
    this.#current.scrollIntoView({ block: "nearest", behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth" });
  }

  #here() {
    if (this.#current && !this.#current.isConnected) this.#drop();
    return this.#current;
  }

  #open(newTab) {
    const el = this.#here();
    if (!el) return false;
    const target = el.querySelector("[data-nav-open]") || (el.matches("a[href]") ? el : el.querySelector("a[href]")) || el.querySelector("button:not([role=checkbox])");
    if (!target) return false;
    if (newTab && target.href) window.open(target.href, "_blank", "noopener");
    else target.click();
    return true;
  }

  #pick() {
    const box = this.#here()?.querySelector("[data-nav-select], button[role=checkbox], input[type=checkbox]");
    box?.click();
    return !!box;
  }

  // tab steps the screen's tabs (shadcn's, or an index's tab links).
  #tab(step) {
    const main = document.getElementById("main");
    const list = main?.querySelector("[role=tablist]");
    if (list) {
      const tabs = [...list.querySelectorAll("[role=tab]:not([disabled])")];
      const at = tabs.findIndex((t) => t.getAttribute("aria-selected") === "true" || t.dataset.state === "active");
      const to = tabs[(at + step + tabs.length) % tabs.length];
      to?.focus();
      to?.click();
      return tabs.length > 0;
    }
    const links = [...(main?.querySelectorAll("nav[aria-label=Show] a") || [])];
    if (!links.length) return false;
    const at = links.findIndex((a) => a.getAttribute("aria-current"));
    links[(at + step + links.length) % links.length].click();
    return true;
  }

  #page(step) {
    const a = document.querySelector(`#main nav[aria-label=Pages] a[aria-label="${step > 0 ? "Next page" : "Previous page"}"]:not([aria-disabled=true])`);
    a?.click();
    return !!a;
  }

  #button(selector, words) {
    const main = document.getElementById("main");
    // Marked, else a button (never a link in a list) whose words say it.
    const el = main?.querySelector(selector) || [...(main?.querySelectorAll("[data-slot=button]") || [])].find((b) => words.test(b.textContent.trim()));
    el?.click();
    return !!el;
  }

  #key(e) {
    if (e.defaultPrevented || e.metaKey || e.ctrlKey || e.altKey) return;
    const t = e.target;
    if (t?.closest?.("input, textarea, select, [contenteditable=true], [contenteditable='']") || t?.isContentEditable) return;
    if (e.key === "?") { this.help = !this.help; return; }
    if (document.querySelector("[role=dialog], [role=menu], [role=listbox]")) return;

    if (this.#pending === "g") {
      this.#pending = "";
      clearTimeout(this.#timer);
      if (e.key === "g") return this.#move("first");
      const to = GOTO.find(([k]) => k === e.key);
      if (to) go(to[1]);
      return;
    }
    switch (e.key) {
      case "g": this.#pending = "g"; this.#timer = setTimeout(() => (this.#pending = ""), 1000); return;
      case "j": return this.#move(1);
      case "k": return this.#move(-1);
      case "G": return this.#move("last");
      case "Enter":
        // Enter on a focused link or button is the browser's.
        if (t?.closest?.("a[href], button, [role=button], [role=tab]")) return;
        return this.#open(false);
      case "o": return this.#open(false);
      case "O": return this.#open(true);
      case "x": return this.#pick();
      case "h": return this.#tab(-1);
      case "l": return this.#tab(1);
      case "[": return this.#page(-1);
      case "]": return this.#page(1);
      case "n": return this.#button("[data-nav-new]", /^New\b|^Add\b/);
      case "e": return this.#button("[data-nav-edit], a[href$='/edit']", /^Edit$/);
      case "Escape": return this.#drop();
      case "/": {
        const box = document.querySelector("#main input[type=search]");
        if (box) box.focus();
        else dispatchEvent(new Event("opensearch"));
        return;
      }
    }
  }
}

export const keys = new Keys();
