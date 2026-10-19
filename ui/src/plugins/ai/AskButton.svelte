<script>
  // Ask (the header slot, key i): a button beside Search that opens the
  // person's chat about the record on screen in a panel at the side, while
  // AI is set up here (Ai.ready?). Esc closes it. On Ask's own page (/ai)
  // the page has the chat, so the button stands aside.
  import * as Sheet from "$lib/components/ui/sheet/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Kbd } from "$lib/components/ui/kbd/index.js";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import AskPanel from "./AskPanel.svelte";
  import { status, recordFrom } from "./ai.svelte.js";
  import { page } from "$lib/page.svelte.js";
  import { router } from "$lib/router.svelte.js";

  let open = $state(false), opened = $state(0);
  status.load();
  const record = $derived(recordFrom(page.record, router.path));

  function show() {
    opened++;
    open = true;
  }
  const typing = (e) => ["INPUT", "TEXTAREA", "SELECT"].includes(e.target.tagName) || e.target.isContentEditable;
  function onkeydown(e) {
    if (e.key !== "i" || e.metaKey || e.ctrlKey || e.altKey || typing(e) || document.querySelector("[role=dialog]") || !status.ready || router.path === "/ai") return;
    e.preventDefault();
    show();
  }
</script>

<svelte:window {onkeydown} />

{#if status.ready && router.path !== "/ai"}
  <Button variant="outline" size="sm" class="w-9 px-0 sm:w-auto sm:px-2.5" onclick={show} aria-label="Ask AI" title="Ask AI (i)">
    <Sparkles /><span class="hidden sm:inline">Ask</span><Kbd class="hidden sm:inline-flex">I</Kbd>
  </Button>
  <Sheet.Root bind:open>
    <Sheet.Content side="right" showCloseButton={false} class="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-lg" aria-describedby={undefined}>
      <Sheet.Title class="sr-only">Ask AI</Sheet.Title>
      {#key opened}<AskPanel {record} page={router.path} onclose={() => (open = false)} />{/key}
    </Sheet.Content>
  </Sheet.Root>
{/if}
