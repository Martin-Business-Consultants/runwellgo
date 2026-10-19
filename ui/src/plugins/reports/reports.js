// What the Reports screens share: the plugin's routes, how health looks,
// places in words (a flag, "Portland, Maine"), the time there, money.
import { session } from "$lib/session.svelte.js";

export const X = "/x/reports";

// The plugin's route with a query: rp("/overview", {status: "all"}).
export function rp(path, query = {}) {
  const q = new URLSearchParams(Object.entries(query).filter(([, v]) => v !== "" && v != null)).toString();
  return X + path + (q ? "?" + q : "");
}

// Health bands: a word and a colour (a token), never colour alone.
export const BANDS = {
  on_track: { label: "On track", color: "var(--chart-2)", text: "text-emerald-700 dark:text-emerald-400", badge: "border-transparent bg-emerald-500/15 text-emerald-700 dark:text-emerald-400" },
  watch: { label: "Needs attention", color: "var(--chart-3)", text: "text-amber-700 dark:text-amber-400", badge: "border-transparent bg-amber-500/15 text-amber-700 dark:text-amber-400" },
  at_risk: { label: "At risk", color: "var(--destructive)", text: "text-destructive", badge: "border-transparent bg-destructive/12 text-destructive" },
  inactive: { label: "Inactive", color: "var(--muted-foreground)", text: "text-muted-foreground", badge: "border-transparent bg-muted text-muted-foreground" },
};
export const band = (b) => BANDS[b] || BANDS.inactive;

// A country's flag from its ISO code (regional indicator letters).
export function flag(code) {
  if (!code || code.length !== 2) return "";
  return String.fromCodePoint(...[...code.toUpperCase()].map((c) => 0x1f1e6 + c.charCodeAt(0) - 65));
}

// "Portland, Maine" or "around New York".
export function placeName(p, { country = false } = {}) {
  if (!p?.label) return "Not on the map yet";
  const parts = [p.label];
  if (p.region && p.region !== p.label) parts.push(p.region);
  if (country && p.country && p.country !== p.label) parts.push(p.country);
  return (p.approximate ? "Around " : "") + parts.join(", ");
}

// The time now in a zone: "2:14 PM".
export function timeIn(iana, now = new Date()) {
  try { return now.toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit", timeZone: iana }); } catch { return ""; }
}

// How far a zone is from here: "same time", "5h ahead", "3h behind".
export function offsetFromHere(iana, now = new Date()) {
  try {
    const there = new Date(now.toLocaleString("en-US", { timeZone: iana }));
    const here = new Date(now.toLocaleString("en-US"));
    const h = Math.round(((there - here) / 36e5) * 2) / 2;
    if (h === 0) return "same time as you";
    return `${Math.abs(h)}h ${h > 0 ? "ahead" : "behind"}`;
  } catch { return ""; }
}

// Whether it's working hours there (9 to 6, weekdays): the moon otherwise.
export function awake(iana, now = new Date()) {
  try {
    const parts = new Intl.DateTimeFormat("en-US", { timeZone: iana, hour: "numeric", hourCycle: "h23", weekday: "short" }).formatToParts(now);
    const hour = +parts.find((p) => p.type === "hour").value;
    const day = parts.find((p) => p.type === "weekday").value;
    return hour >= 9 && hour < 18 && day !== "Sat" && day !== "Sun";
  } catch { return true; }
}

// Money in whole units, compact on charts: $12,400 or $12.4k.
export const whole = (cents) => (cents == null ? "" : (cents / 100).toLocaleString(undefined, { style: "currency", currency: "USD", maximumFractionDigits: 0 }));
export const compact = (cents) => (cents / 100).toLocaleString(undefined, { style: "currency", currency: "USD", notation: "compact", maximumFractionDigits: 1 });

// An engagement type's word, from Settings › Names.
export const typeName = (label, n = 2) => session.label?.(label, n) || label.replace(/_/g, " ");

export const monthName = (ym, long = false) => new Date(ym + "-01T00:00:00Z").toLocaleDateString(undefined, { month: long ? "long" : "short", year: long ? "numeric" : undefined, timeZone: "UTC" });

export const prefersReducedMotion = () => typeof matchMedia !== "undefined" && matchMedia("(prefers-reduced-motion: reduce)").matches;

// Where a place came from, in words: set by hand (a search, a pin, an
// agent), their address, or their time zone.
export function placeSource(p) {
  if (!p?.label) return "";
  if (p.approximate || p.source === "zone") return "Approximate, from their time zone";
  if (p.source === "address") return "From their address";
  return p.source === "agent" ? "Set by hand (by an agent)" : "Set by hand";
}
export const setByHand = (p) => !!p?.label && !p.approximate && p.source !== "address" && p.source !== "zone";

// The map's view, remembered per browser: the contiguous United States
// (the default) or the whole world.
const VIEW = "runwell.reports.view";
export function savedView() {
  try { return localStorage.getItem(VIEW) === "world" ? "world" : "us"; } catch { return "us"; }
}
export function saveView(v) {
  try { localStorage.setItem(VIEW, v); } catch { /* private window: not remembered */ }
}
// The contiguous US, for fitting a map to it.
export const US = { south: 24.4, west: -125.0, north: 49.4, east: -66.9, lat: 39.5, lng: -98.4 };

// A token colour ("var(--chart-2)") as the CSS colour it resolves to, for
// what can't read tokens (a map's marker, a canvas).
export function cssColor(c) {
  const m = /^var\((--[\w-]+)\)$/.exec(c || "");
  if (!m || typeof document === "undefined") return c;
  return getComputedStyle(document.documentElement).getPropertyValue(m[1]).trim() || "#888";
}
