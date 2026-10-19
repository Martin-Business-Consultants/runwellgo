<script>
  // A command or address to copy: monospace, with a copy button.
  import CopyButton from "$lib/components/CopyButton.svelte";
  let { text } = $props();
</script>

<div class="bg-muted flex items-start gap-2 rounded-md border p-2">
  <pre class="min-w-0 flex-1 overflow-x-auto font-mono text-xs whitespace-pre-wrap break-all select-all">{text}</pre>
  <CopyButton {text} />
</div>
