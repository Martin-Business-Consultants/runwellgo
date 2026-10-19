// A long page's tabs, kept in the address (?tab=) the way Home keeps its
// own: the first tab has no parameter, a link can open any other, and
// switching replaces the address so Back leaves the page rather than
// stepping through its tabs. The router keeps the screen on a query change,
// so switching doesn't flicker.
//
//   const tabs = pageTabs(["profile", "security", "preferences"], { hashes: { "two-factor": "security" } });
//   <Tabs.Root value={tabs.value} onValueChange={tabs.set}>…
//
// keys is a list, or a function answering one (tabs that come and go).
// hashes opens a tab for an old anchor (/settings/account#two-factor) and
// scrolls to it once it's drawn (call tabs.scrollToHash() after loading):
// a map of anchor to tab, or a function answering the tab for an anchor.
import { tick } from "svelte";
import { router, go, withQuery } from "./router.svelte.js";
import { focusInvalid } from "./forms.js";

export function pageTabs(keys, { param = "tab", hashes = {} } = {}) {
  const list = () => (typeof keys === "function" ? keys() : keys);
  const hash = decodeURIComponent(location.hash.slice(1));
  const hashTab = hash ? (typeof hashes === "function" ? hashes(hash) : hashes[hash]) || null : null;
  let fromHash = $state(hashTab);
  // Its methods don't lean on this, so each can be handed on (onValueChange={tabs.set}).
  const tabs = {
    get value() {
      const q = router.query.get(param);
      if (list().includes(q)) return q;
      return (fromHash && list().includes(fromHash) && fromHash) || list()[0];
    },
    set(v) {
      // A tab picked by hand outranks the anchor that opened the page, and
      // the anchor leaves the address so a reload keeps the tab picked.
      if (fromHash || location.hash) history.replaceState(history.state, "", location.pathname + location.search);
      fromHash = null;
      go(withQuery({ [param]: v === list()[0] ? null : v }), undefined, { replace: true });
    },
    // Switches to a tab and waits until it's drawn: a validation error on a
    // tab that isn't showing brings its tab up, then focuses the field.
    async reveal(v) {
      fromHash = null;
      go(withQuery({ [param]: v === list()[0] ? null : v }), undefined, { replace: true, transition: false });
      await new Promise((r) => queueMicrotask(r));
      await tick();
    },
    async invalid(v) {
      if (tabs.value !== v) await tabs.reveal(v);
      focusInvalid();
    },
    async scrollToHash() {
      if (!hashTab) return;
      await tick();
      document.getElementById(hash)?.scrollIntoView({ block: "start" });
    },
  };
  return tabs;
}
