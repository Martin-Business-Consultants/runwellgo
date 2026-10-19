// Flash messages (svelte-sonner, drawn by <Toasts />); an error stays a
// little longer.
import { toast as sonner } from "svelte-sonner";

export const toast = (m) => sonner.success(m);
export const toastError = (e) => sonner.error(errorMessage(e), { duration: 7000 });

// What went wrong, in words: a refused role says who can change it; a 422
// whose fields aren't on screen names the first problem ("Title can't be
// blank.") rather than "Validation failed".
export function errorMessage(e) {
  if (e?.status === 403) return "Your role doesn't allow that. An owner can change roles in Settings › People.";
  const first = e?.status === 422 && Object.entries(e.errors || {})[0];
  if (first) {
    const [key, [msg] = []] = first;
    const field = key === "base" ? "" : key.split(".").pop().replace(/_id$/, "").replaceAll("_", " ");
    const text = field ? `${field} ${msg}` : msg || e.message;
    return text.charAt(0).toUpperCase() + text.slice(1) + (/[.!?]$/.test(text) ? "" : ".");
  }
  return e?.message || String(e);
}
// toasts.show(message, tone) for a tone other than success: info or danger.
export const toasts = {
  show(m, tone = "info") { tone === "danger" ? sonner.error(m, { duration: 7000 }) : tone === "success" ? sonner.success(m) : sonner.info(m); },
};

// undoable says what happened with an Undo button for a while: the
// reversible actions (trash, a status change) ask no question first.
export function undoable(message, undo, { done = "Undone." } = {}) {
  if (!undo) return toast(message);
  sonner.success(message, {
    duration: 8000,
    action: { label: "Undo", onClick: async () => { try { await undo(); sonner.success(done); } catch (e) { toastError(e); } } },
  });
}
