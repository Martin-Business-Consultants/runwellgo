<script>
  // A list of short strings as chips: type and press Enter or comma to add,
  // Backspace on an empty input to take the last off.
  import { Badge } from "$lib/components/ui/badge/index.js";
  import Icon from "./Icon.svelte";
  let { value = $bindable([]), placeholder = "Add and press Enter", id = undefined, invalid = false } = $props();
  let draft = $state("");
  const list = $derived(Array.isArray(value) ? value : []);
  function add() {
    const v = draft.trim().replace(/,$/, "").trim();
    if (v && !list.includes(v)) value = [...list, v];
    draft = "";
  }
  function key(e) {
    if (e.key === "Enter" || e.key === ",") { e.preventDefault(); add(); }
    else if (e.key === "Backspace" && !draft && list.length) value = list.slice(0, -1);
  }
</script>

<div class="border-input dark:bg-input/30 focus-within:border-ring focus-within:ring-ring/50 flex min-h-9 w-full flex-wrap items-center gap-1 rounded-md border bg-transparent px-1.5 py-1 shadow-xs focus-within:ring-3 {invalid ? 'border-destructive' : ''}">
  {#each list as v, i (v)}
    <Badge variant="secondary" class="pr-0.5">
      {v}
      <button type="button" class="hover:bg-muted-foreground/20 rounded-sm" onclick={() => (value = list.filter((_, j) => j !== i))} aria-label="Remove {v}"><Icon name="close" size="xs" /></button>
    </Badge>
  {/each}
  <input {id} bind:value={draft} onkeydown={key} onblur={add} class="placeholder:text-muted-foreground min-w-24 flex-1 bg-transparent px-1 text-sm outline-none" placeholder={list.length ? "" : placeholder} />
</div>
