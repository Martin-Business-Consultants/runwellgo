// Live updates: events the server pushes to this tab (a plugin granted
// "live" publishing to the person signed in). One stream per tab, opened
// with the first subscriber and closed with the last:
//
//   const stop = subscribe("chat", (topic, data) => { … });
//
// A plugin's screens get its own events; a "resync" topic means the tab
// may have missed some (it was asleep, or the stream fell back to polling)
// and should read again. The core's own ("_core" "nav": the sidebar's
// counts or the bell changed) refresh the sidebar. Where the stream can't
// be kept open (a proxy that buffers it), the tab falls back to polling:
// a "resync" every 20 seconds.
import { nav } from "./nav.svelte.js";

const handlers = new Map(); // plugin key → Set of fn(topic, data)
let source = null;
let lastId = "";
let failures = 0;
let retry = null;
let poll = null;

class Live {
  // connected: the stream is open (else updates come by polling, if at all).
  connected = $state(false);
}
export const live = new Live();

const count = () => [...handlers.values()].reduce((n, s) => n + s.size, 0);

function dispatch(plugin, topic, data) {
  for (const fn of handlers.get(plugin) || []) {
    try { fn(topic, data); } catch (e) { console.error(e); }
  }
}

function resyncAll() {
  for (const plugin of handlers.keys()) dispatch(plugin, "resync", {});
}

function startPolling() {
  if (poll) return;
  poll = setInterval(() => { resyncAll(); nav.refresh(); }, 20000);
}

function stopPolling() {
  clearInterval(poll);
  poll = null;
}

function connect() {
  if (source || retry || typeof window === "undefined") return;
  if (typeof EventSource === "undefined") return startPolling();
  const es = new EventSource("/api/live" + (lastId ? `?after=${encodeURIComponent(lastId)}` : ""));
  source = es;
  let wasDown = failures > 0;
  es.onopen = () => {
    live.connected = true;
    failures = 0;
    stopPolling();
  };
  es.onmessage = (e) => {
    if (e.lastEventId) lastId = e.lastEventId;
    let ev;
    try { ev = JSON.parse(e.data); } catch { return; }
    if (ev.plugin === "_core") {
      if (ev.topic === "nav") nav.refresh();
      return;
    }
    dispatch(ev.plugin, ev.topic, ev.data);
  };
  es.addEventListener("hello", (e) => {
    if (e.lastEventId) lastId = e.lastEventId;
    // Back after being cut off: what was missed was replayed, unless the
    // server says to read again; polling may have left screens behind.
    if (wasDown) { wasDown = false; resyncAll(); nav.refresh(); }
  });
  es.addEventListener("resync", () => { resyncAll(); nav.refresh(); });
  es.onerror = () => {
    live.connected = false;
    // The browser retries a dropped stream by itself; one it gave up on
    // (an error answer) is retried here, slower each time.
    if (es.readyState !== EventSource.CLOSED) return;
    es.close();
    source = null;
    failures++;
    if (failures >= 3) startPolling();
    if (!count()) return;
    const wait = Math.min(60000, 2000 * 2 ** Math.min(failures, 5));
    retry = setTimeout(() => { retry = null; if (count()) connect(); }, wait);
  };
}

function disconnect() {
  source?.close();
  source = null;
  clearTimeout(retry);
  retry = null;
  stopPolling();
  live.connected = false;
}

// subscribe hands fn(topic, data) the plugin's events while it's
// subscribed; it answers the function that stops it.
export function subscribe(plugin, fn) {
  if (!handlers.has(plugin)) handlers.set(plugin, new Set());
  handlers.get(plugin).add(fn);
  connect();
  return () => {
    handlers.get(plugin)?.delete(fn);
    if (!count()) disconnect();
  };
}
