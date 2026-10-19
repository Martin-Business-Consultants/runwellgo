<script>
  // A secret shown once: monospace, selectable, with a copy button.
  import CopyButton from "./CopyButton.svelte";
  let { value, note = "" } = $props();
</script>

<div class="flex flex-col gap-2">
  {#if note}<p class="text-sm">{note}</p>{/if}
  <div class="flex items-center gap-2 bg-muted rounded-md border p-2">
    <code class="min-w-0 flex-1 select-all break-all font-mono text-xs">{value}</code>
    <CopyButton text={value} variant="default" />
  </div>
</div>
