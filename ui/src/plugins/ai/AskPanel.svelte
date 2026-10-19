<script>
  // The Ask panel (the header's Ask, and the portal's): the chat about the
  // record on screen, with your earlier chats a click away (grouped by
  // when, to open, rename or delete), a new chat, and the whole page at
  // /ai. A client in their portal gets the chat alone.
  import { Button } from "$lib/components/ui/button/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import History from "@lucide/svelte/icons/history";
  import Maximize from "@lucide/svelte/icons/maximize-2";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import X from "@lucide/svelte/icons/x";
  import AskChat from "./AskChat.svelte";
  import AskHistory from "./AskHistory.svelte";

  let { record = "", page = "", mode = "staff", onclose = null } = $props();
  // svelte-ignore state_referenced_locally
  const inPortal = mode === "portal";

  let chat = $state(null), view = $state("chat"), opened = $state(null), key = $state(0), box = $state(null);

  function open(id) { opened = id; key++; view = "chat"; }
  function fresh() { opened = null; view = "chat"; box?.fresh(); }
</script>

<div class="flex min-h-0 flex-1 flex-col">
  <div class="flex items-center gap-1 border-b px-3 py-2.5">
    {#if view === "history"}
      <Button variant="ghost" size="icon-sm" onclick={() => (view = "chat")} aria-label="Back to the chat" title="Back to the chat"><ArrowLeft /></Button>
      <h2 class="min-w-0 flex-1 truncate font-semibold">Your chats</h2>
    {:else}
      <h2 class="min-w-0 flex-1 truncate ps-1 font-semibold">{chat?.title || chat?.heading || (inPortal ? "Ask" : "Ask anything")}</h2>
      {#if !inPortal}
        <Button variant="ghost" size="icon-sm" onclick={() => (view = "history")} aria-label="Your chats" title="Your chats"><History /></Button>
        <Button variant="ghost" size="icon-sm" href={chat ? `/ai?c=${chat.id}` : "/ai"} onclick={() => onclose?.()} aria-label="Open on its own page" title="Open on its own page"><Maximize /></Button>
      {/if}
      <Button variant="ghost" size="icon-sm" onclick={fresh} aria-label="Start a new chat" title="Start a new chat"><Plus /></Button>
    {/if}
    {#if onclose}
      <Button variant="ghost" size="icon-sm" onclick={() => onclose()} aria-label="Close" title="Close (Esc)"><X /></Button>
    {/if}
  </div>
  {#if view === "history"}
    <AskHistory current={chat?.id} onopen={open} onnew={fresh} />
  {:else}
    {#key key}
      <AskChat bind:this={box} {record} {page} {mode} chatId={opened} onchat={(c) => (chat = c)} />
    {/key}
  {/if}
</div>
