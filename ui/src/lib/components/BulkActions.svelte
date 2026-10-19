<script>
  // A list's "Bulk actions" select and Apply, as WordPress lays it out. The
  // chosen action runs on the ticked rows; a destructive one asks first.
  import Button from "./Button.svelte";
  import Select from "./Select.svelte";
  import { confirm } from "../confirm.svelte.js";
  let { actions = [], count = 0, noun = "item", onapply } = $props();
  let choice = $state(null);
  let busy = $state(false);
  async function apply() {
    const action = actions.find((a) => a.value === choice);
    if (!action || !count) return;
    if (action.confirm && !(await confirm({ title: `${action.label} ${count} ${count === 1 ? noun : noun + "s"}?`, message: action.confirm, confirmLabel: action.label, destructive: action.destructive }))) return;
    busy = true;
    try { await onapply(choice); choice = null; } finally { busy = false; }
  }
</script>

<div class="flex items-center gap-2">
  <Select class="w-44" placeholder="Bulk actions" options={actions} bind:value={choice} aria-label="Bulk actions" />
  <Button onclick={apply} disabled={!choice || !count} loading={busy}>Apply</Button>
  {#if count}<span class="text-muted-foreground text-xs">{count} selected</span>{/if}
</div>
