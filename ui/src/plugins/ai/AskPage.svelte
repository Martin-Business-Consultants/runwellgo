<script>
  // Ask on a page of its own (/ai): your chats down the left, grouped by
  // when, and the one you picked (?c=12) or a new one beside them. On a
  // phone the list is a button away. i puts you in the box to ask.
  import { MediaQuery } from "svelte/reactivity";
  import { Button } from "$lib/components/ui/button/index.js";
  import * as Sheet from "$lib/components/ui/sheet/index.js";
  import History from "@lucide/svelte/icons/history";
  import Plus from "@lucide/svelte/icons/plus";
  import AskChat from "./AskChat.svelte";
  import AskHistory from "./AskHistory.svelte";
  import { page } from "$lib/page.svelte.js";
  import { router, go } from "$lib/router.svelte.js";

  page.set("Ask");
  const wide = new MediaQuery("min-width: 768px");
  const cid = $derived(Number(router.query.get("c")) || null);
  let chat = $state(null), box = $state(null), listOpen = $state(false), version = $state(0);
  // svelte-ignore state_referenced_locally
  let shown = $state(cid), key = $state(0);

  // The address says which chat; moving back or forward opens that one.
  $effect(() => {
    if (cid && cid !== chat?.id && cid !== shown) { shown = cid; key++; }
  });
  function onchat(c) {
    chat = c;
    page.set(c?.title || "Ask");
    if (c && c.id !== cid) go("/ai", { c: c.id }, { transition: false, replace: true });
    version++;
  }
  function open(id) { listOpen = false; shown = id; key++; go("/ai", { c: id }, { transition: false }); }
  function newChat() { listOpen = false; shown = null; chat = null; key++; queueMicrotask(() => box?.fresh()); }

  const typing = (e) => ["INPUT", "TEXTAREA", "SELECT"].includes(e.target.tagName) || e.target.isContentEditable;
  function onkeydown(e) {
    if (e.key === "i" && !e.metaKey && !e.ctrlKey && !e.altKey && !typing(e) && !document.querySelector("[role=dialog]")) {
      e.preventDefault();
      box?.focus();
    }
  }
</script>

<svelte:window {onkeydown} />

<h1 class="sr-only">Ask</h1>
<div class="bg-background relative -mx-4 -my-6 flex h-[calc(100dvh-3rem)] min-h-0 overflow-hidden md:h-[calc(100dvh-4rem)] lg:-mx-6">
  {#if wide.current}
    <aside class="bg-muted/30 flex w-64 shrink-0 flex-col border-e" aria-label="Your chats">
      <AskHistory current={chat?.id} onopen={open} onnew={newChat} {version} />
    </aside>
  {/if}
  <section class="flex min-w-0 flex-1 flex-col" aria-label="Chat">
    {#if !wide.current}
      <div class="flex items-center gap-1 border-b px-3 py-2">
        <Button variant="ghost" size="sm" onclick={() => (listOpen = true)}><History />Your chats</Button>
        <span class="flex-1"></span>
        <Button variant="ghost" size="icon-sm" onclick={newChat} aria-label="Start a new chat" title="Start a new chat"><Plus /></Button>
      </div>
    {/if}
    {#key key}
      <AskChat bind:this={box} chatId={shown} {onchat} page="/ai" />
    {/key}
  </section>
</div>

{#if !wide.current}
  <Sheet.Root bind:open={listOpen}>
    <Sheet.Content side="left" class="w-72 gap-0 p-0" aria-describedby={undefined}>
      <Sheet.Title class="px-4 pt-4 text-base">Your chats</Sheet.Title>
      <AskHistory current={chat?.id} onopen={open} onnew={newChat} {version} />
    </Sheet.Content>
  </Sheet.Root>
{/if}
