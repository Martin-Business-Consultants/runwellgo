<script>
  // The grip a sortable row is dragged by (see sortable.js); from the
  // keyboard, the up and down arrows move it: onmove(-1 or 1).
  import Icon from "./Icon.svelte";
  let { onmove } = $props();
</script>

<button
  type="button"
  data-drag-handle
  class="text-muted-foreground hover:text-foreground focus-visible:ring-ring/50 cursor-grab touch-none rounded-sm p-0.5 outline-none focus-visible:ring-3 active:cursor-grabbing"
  aria-label="Drag to reorder (or press the up and down arrows)"
  onclick={(e) => e.preventDefault()}
  onkeydown={(e) => { if (e.key === "ArrowUp" || e.key === "ArrowDown") { e.preventDefault(); onmove(e.key === "ArrowUp" ? -1 : 1); } }}
><Icon name="grip" /></button>
