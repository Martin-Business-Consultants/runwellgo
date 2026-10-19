// What My day's screens share: the statuses in words and colors, the due
// pill's words, the greeting's date, and Copy for AI (one brief, or one
// prompt for several) from the core's brief.
import { api } from "$lib/api.js";
import { toast, toastError } from "$lib/toast.svelte.js";

export const BASE = "/x/my_day";

// call is one of the plugin's routes; it says what happened (the Rails
// notice, or its alert as an error) and answers true when it went through.
export async function call(method, path, body) {
  try {
    const r = await api(method, BASE + path, body);
    if (r?.message) (r.alert ? toastError({ message: r.message }) : toast(r.message));
    return true;
  } catch (e) { toastError(e); return false; }
}

export const STATUS_LABELS = { planned: "Planned", in_progress: "In progress", in_review: "In review", blocked: "Blocked", done: "Done" };
export const STATUSES = Object.keys(STATUS_LABELS);
export const WAITING = ["in_review", "blocked"];

export const statusDot = {
  planned: "bg-muted-foreground/40",
  in_progress: "bg-sky-500",
  in_review: "bg-violet-500",
  blocked: "bg-red-500",
  done: "bg-emerald-500",
};

// The tabs (Day::TABS), "open" first.
export const TABS = [["open", "Open"], ["today", "Today"], ["week", "Week"], ["done", "Done"], ["all", "All"]];

const parse = (ymd) => { const [y, m, d] = String(ymd).slice(0, 10).split("-").map(Number); return new Date(y, m - 1, d); };
const days = (a, b) => Math.round((parse(a) - parse(b)) / 86400000);

// due is a todo's due pill (my_day_due): its words, and whether it's late or
// today; null when it has no date.
export function due(todo, today) {
  if (!todo.due_on) return null;
  const n = days(todo.due_on, today);
  const d = parse(todo.due_on);
  const text = n === 0 ? "Today" : n === 1 ? "Tomorrow" : n === -1 ? "Yesterday" : n >= 2 && n <= 6
    ? d.toLocaleDateString(undefined, { weekday: "short" })
    : d.toLocaleDateString(undefined, { day: "numeric", month: "short" });
  const late = todo.status !== "done" && n < 0;
  return { text: late ? `Late · ${text}` : text, late, today: !late && n === 0 };
}

// "Sunday, October 18", in the reader's own date order like the rest of
// the app.
export function greetingDate(today) {
  return parse(today).toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric" });
}

// copyText puts text that's still loading on the clipboard: inside a
// ClipboardItem, so Safari keeps the click's permission while it loads.
export async function copyText(loading, message) {
  try {
    if (window.ClipboardItem && navigator.clipboard.write) {
      await navigator.clipboard.write([new ClipboardItem({ "text/plain": loading.then((t) => new Blob([t], { type: "text/plain" })) })]);
    } else {
      await navigator.clipboard.writeText(await loading);
    }
    toast(message);
    return true;
  } catch (e) { toastError(e); return false; }
}

async function brief(id) {
  const res = await fetch(`/api/work/${id}/brief?format=text`, { headers: { Accept: "text/plain" }, credentials: "same-origin" });
  if (!res.ok) throw new Error(res.statusText || "Couldn't get the brief.");
  return res.text();
}

// copyBrief copies one todo's brief, for a local AI (the core's todos/ai_brief).
export const copyBrief = (id) => copyText(brief(id), "Copied a brief for your AI.");

// LIMIT is the most one prompt takes (BriefsController::LIMIT).
const LIMIT = 25;

// prompt is one prompt for a local AI to work through several todos, one at
// a time, in order (BriefsController#index): what to do with them, then
// each one's brief.
async function prompt(ids) {
  const picked = [...new Set(ids)].slice(0, LIMIT);
  const texts = await Promise.all(picked.map(brief));
  const n = texts.length;
  const briefs = texts.map((t, i) => `---\n\n## ${i + 1} of ${n}\n\n` + t.replace(/^# /, "### "));
  return `# Work through ${n} ${n === 1 ? "piece" : "pieces"} of work, one at a time

Below are ${n} pieces of work from Runwell, each with its own brief. Do them in this order, one at a time:

1. Read the brief, then do the work it describes.
2. Report back as the brief says (its status, and a note of what you did) before you start the next.
3. If one can't be done (it needs a person, access you don't have, or a decision), mark it blocked with a note saying why, and move on to the next.
4. When you've been through them all, give a short summary: what's done, what's in review, what's blocked and why.

` + briefs.join("\n");
}

export const copyPrompt = (ids) => copyText(prompt(ids), "Copied a prompt for your AI.");
