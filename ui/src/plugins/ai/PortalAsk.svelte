<script>
  // The portal's Ask (the portal_header slot): a client's contact asks
  // about their work, while the owner lets clients ask and AI is on for
  // them. Their portal's own route says whether it shows.
  import * as Sheet from "$lib/components/ui/sheet/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import AskPanel from "./AskPanel.svelte";
  import { portal } from "./ai.svelte.js";

  let available = $state(false), open = $state(false), opened = $state(0);
  portal.get("/status").then((s) => (available = s.available)).catch(() => {});
</script>

{#if available}
  <Button variant="outline" size="sm" onclick={() => { opened++; open = true; }}><Sparkles />Ask</Button>
  <Sheet.Root bind:open>
    <Sheet.Content side="right" showCloseButton={false} class="gap-0 p-0 data-[side=right]:w-full data-[side=right]:sm:max-w-md" aria-describedby={undefined}>
      <Sheet.Title class="sr-only">Ask</Sheet.Title>
      {#key opened}<AskPanel mode="portal" onclose={() => (open = false)} />{/key}
    </Sheet.Content>
  </Sheet.Root>
{/if}
