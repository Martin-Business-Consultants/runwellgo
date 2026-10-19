// How a commitment reads: "Rosa will send the logo", by when or how it
// ended, and the two lanes every list shows (what the other side owes you,
// what you've promised them).
import { shortDate } from "../work/work.js";

export const RESOLUTIONS = ["done", "missed", "dropped"];

// The promise itself, read after "will": its first letter lowered unless
// it starts an acronym.
export const action = (c) => c.description.replace(/^\p{Lu}(?=\p{Ll})/u, (ch) => ch.toLowerCase());

export function when(c) {
  if (c.open) return c.overdue ? `overdue since ${shortDate(c.due_on)}` : `by ${shortDate(c.due_on)}`;
  return `${c.resolution} ${shortDate(c.resolved_at)}`;
}

// details: by when and where it was promised, with the client and
// engagement when the list spans them.
export const details = (c, withContext = false) =>
  [withContext && c.client?.display_name, withContext && c.engagement?.ref, when(c), c.source].filter(Boolean).join(" · ");

// lanes: [theirs, ours].
export const lanes = (list) => [list.filter((c) => c.owner_kind === "client"), list.filter((c) => c.owner_kind !== "client")];

export const statusOf = (c) => (c.overdue ? "overdue" : c.resolution || "open");
