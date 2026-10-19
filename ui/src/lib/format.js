// Small formatting helpers the screens share.
const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto" });
const steps = [["year", 31536000], ["month", 2592000], ["week", 604800], ["day", 86400], ["hour", 3600], ["minute", 60]];

export function timeAgo(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  if (isNaN(d)) return iso;
  const s = (d.getTime() - Date.now()) / 1000;
  for (const [unit, n] of steps) if (Math.abs(s) >= n) return rtf.format(Math.round(s / n), unit);
  return "just now";
}

export function dateTime(iso) {
  if (!iso) return "—";
  const d = new Date(iso);
  return isNaN(d) ? iso : d.toLocaleString(undefined, { dateStyle: "medium", timeStyle: "short" });
}

export function bytes(n) {
  if (n == null) return "";
  if (n < 1024) return `${n} B`;
  if (n < 1048576) return `${Math.round(n / 1024)} KB`;
  const [v, unit] = n < 1073741824 ? [n / 1048576, "MB"] : [n / 1073741824, "GB"];
  return `${v.toFixed(1).replace(/\.0$/, "")} ${unit}`;
}

export function slugify(s) {
  return s.toLowerCase().normalize("NFKD").replace(/[̀-ͯ]/g, "").replace(/[^a-z0-9]+/g, "-").replace(/^-+|-+$/g, "").slice(0, 80);
}

export const plural = (n, word, many = word + "s") => `${n} ${n === 1 ? word : many}`;

// capitalize("portal") → "Portal": free text that starts a line or a cell.
export const capitalize = (s) => (s ? String(s).replace(/^./, (c) => c.toUpperCase()) : "");

export const humanize = (s) => (s ? String(s).replace(/_/g, " ").replace(/^./, (c) => c.toUpperCase()) : "");

// The Rails admin's status tones (ApplicationHelper::STATUS_TONES).
const TONES = {
  success: ["published", "active", "approved", "paid", "accepted", "delivered", "succeeded", "success", "enabled", "ready", "verified", "on", "passes", "ok"],
  danger: ["failed", "rejected", "void", "overdue", "canceled", "failure", "revoked", "off", "not verified", "fails"],
  warning: ["draft", "pending", "scheduled", "queued", "sent", "open", "yours_to_judge"],
  info: ["running", "in progress"],
};
export function tone(status) {
  const s = String(status || "").toLowerCase();
  for (const [t, list] of Object.entries(TONES)) if (list.includes(s)) return t;
  return "neutral";
}

// initials("Ted Martin") → "TM", for an avatar with no picture.
export const initials = (name) => (name || "?").split(/\s+/).filter(Boolean).slice(0, 2).map((w) => w[0].toUpperCase()).join("");

// money(12345) → "$123.45": integer cents, shown as dollars.
export const money = (cents) => (cents == null ? "" : (cents / 100).toLocaleString(undefined, { style: "currency", currency: "USD" }));

// date("2026-10-20") → "Oct 20, 2026": a date with no time, read as it is
// (not shifted by the browser's zone).
export function date(ymd) {
  if (!ymd) return "";
  const [y, m, d] = String(ymd).slice(0, 10).split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { month: "short", day: "numeric", year: "numeric" });
}
