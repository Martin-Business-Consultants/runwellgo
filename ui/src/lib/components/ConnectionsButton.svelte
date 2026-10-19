<script>
  // Connections: a button on a client, an engagement or a piece of work
  // that opens the map of everything its client's records tie together,
  // centred on this one.
  //   record: "Client:7", "Engagement:P-1" or "Todo:18"; name: what it's called
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Network from "@lucide/svelte/icons/network";
  import ConnectionsMap from "./connections/ConnectionsMap.svelte";

  let { record, name = "" } = $props();
  let open = $state(false);
</script>

<Button variant="outline" size="sm" onclick={() => (open = true)}><Network />Connections</Button>
<Dialog.Root bind:open>
  <Dialog.Content class="flex h-[85svh] w-[min(1280px,calc(100vw-2rem))] max-w-none flex-col gap-3 sm:max-w-none">
    <Dialog.Header>
      <Dialog.Title>Connections</Dialog.Title>
      <Dialog.Description>How {name || "this"} connects: who it's for, what was agreed, and what's happening. Select a record to open it.</Dialog.Description>
    </Dialog.Header>
    {#if open}<div class="min-h-0 flex-1"><ConnectionsMap {record} onopen={() => (open = false)} /></div>{/if}
  </Dialog.Content>
</Dialog.Root>
