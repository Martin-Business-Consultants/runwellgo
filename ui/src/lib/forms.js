// After a validation error: focus the first field marked invalid, so the
// person lands on what needs fixing and hears its error.
import { tick } from "svelte";

export async function focusInvalid() {
  await tick();
  const el = document.querySelector('[aria-invalid="true"]');
  if (!el) return;
  el.focus({ preventScroll: true });
  el.scrollIntoView({ block: "center" });
}
