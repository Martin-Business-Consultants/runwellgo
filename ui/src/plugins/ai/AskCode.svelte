<script>
  // A code block in an answer: its language, the code (scrolling sideways
  // rather than the page), and Copy.
  import { Button } from "$lib/components/ui/button/index.js";
  import Copy from "@lucide/svelte/icons/copy";
  import Check from "@lucide/svelte/icons/check";
  import { copyText } from "./ai.svelte.js";
  let { code = "", lang = "" } = $props();
  let copied = $state(false);
  async function copy() {
    if (await copyText(code)) { copied = true; setTimeout(() => (copied = false), 1500); }
  }
</script>

<div class="bg-muted/40 min-w-0 overflow-hidden rounded-md border">
  <div class="text-muted-foreground flex items-center justify-between border-b px-2.5 py-1 text-xs">
    <span class="font-mono">{lang || "code"}</span>
    <Button variant="ghost" size="sm" class="h-6 gap-1 px-1.5 text-xs" onclick={copy} aria-label="Copy the code">
      {#if copied}<Check class="size-3.5" />Copied{:else}<Copy class="size-3.5" />Copy{/if}
    </Button>
  </div>
  <pre class="overflow-x-auto p-2.5 font-mono text-[12.5px] leading-relaxed"><code>{code}</code></pre>
</div>
