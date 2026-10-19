// What the work screens share: the statuses and their colors (the board's),
// a short date, copying a brief for a local AI, and a bulk call.
import { api, get } from "$lib/api.js";
import { toast, toastError } from "$lib/toast.svelte.js";

// in_review: finished by whoever did it (often an agent), waiting for a
// person to check it and mark it done.
export const STATUSES = ["planned", "in_progress", "in_review", "blocked", "done"];

export const statusDot = {
  planned: "bg-muted-foreground/40",
  in_progress: "bg-sky-500",
  in_review: "bg-violet-500",
  blocked: "bg-red-500",
  done: "bg-emerald-500",
};

// shortDate("2026-10-20") → "Oct 20" (Rails' :short date).
export function shortDate(ymd) {
  if (!ymd) return "";
  const [y, m, d] = String(ymd).slice(0, 10).split("-").map(Number);
  return new Date(y, m - 1, d).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

// inDays(7) → YYYY-MM-DD a week from today.
export function inDays(n) {
  const d = new Date();
  d.setDate(d.getDate() + n);
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`;
}

// Everyone on a todo, the lead first.
export const ownersOf = (todo) => [todo.owner, ...(todo.other_owners || [])].filter(Boolean);

// copyBrief puts a todo's brief on the clipboard, for a local AI.
export async function copyBrief(todo) {
  try {
    const { brief } = await get(`/work/${todo.id}/brief`);
    await navigator.clipboard.writeText(brief);
    toast("Copied a brief for your AI.");
  } catch (e) { toastError(e); }
}

// bulk runs a bulk action and says what it did; true when anything changed.
export async function bulk(method, path, body) {
  try {
    const r = await api(method, path, body);
    toast(r.summary);
    return true;
  } catch (e) { toastError(e); return false; }
}
