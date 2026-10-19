<script>
  // Searching messages, beside the conversation: every word must be in a
  // message, in any conversation you can see; picking one opens it (in its
  // thread, for a reply).
  import * as InputGroup from "$lib/components/ui/input-group/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import X from "@lucide/svelte/icons/x";
  import Search from "@lucide/svelte/icons/search";
  import ChatAvatar from "./ChatAvatar.svelte";
  import { chat, BASE, clockTime } from "./chat.svelte.js";
  import { get } from "$lib/api.js";
  import { go } from "$lib/router.svelte.js";

  let { onclose } = $props();
  let q = $state("");
  let results = $state(null);
  let error = $state("");
  let timer;
  let input = $state(null);

  $effect(() => { input?.focus(); });

  function typed() {
    clearTimeout(timer);
    timer = setTimeout(run, 250);
  }

  async function run() {
    const words = q.trim();
    if (words.length < 2) { results = null; error = ""; return; }
    try {
      results = (await get(`${BASE}/search?q=${encodeURIComponent(words)}`, { quiet: true })).messages;
      error = "";
    } catch (e) { error = e.message; }
  }

  function open(m) {
    go("/chat", { c: m.conversation_id, thread: m.parent_id || undefined, panel: m.parent_id ? undefined : "search" }, { transition: false });
  }
</script>

<div class="flex h-14 shrink-0 items-center gap-2 border-b px-4">
  <h2 class="min-w-0 flex-1 truncate text-base font-semibold">Search messages</h2>
  <Button variant="ghost" size="icon" class="size-8" onclick={onclose} aria-label="Close search" title="Close (Esc)"><X /></Button>
</div>
<div class="border-b p-3">
  <InputGroup.Root>
    <InputGroup.Addon><Search /></InputGroup.Addon>
    <InputGroup.Input bind:ref={input} type="search" bind:value={q} oninput={typed} placeholder="Words in a message" aria-label="Search messages" />
  </InputGroup.Root>
</div>
<div class="min-h-0 flex-1 overflow-y-auto overscroll-contain" aria-live="polite">
  {#if error}
    <p class="text-muted-foreground p-4 text-sm">{error}</p>
  {:else if results === null}
    <p class="text-muted-foreground p-4 text-sm">Type two letters or more to search everything you can see: your conversations and public channels.</p>
  {:else if !results.length}
    <p class="text-muted-foreground p-4 text-sm">No messages have all of “{q.trim()}”. Try fewer words.</p>
  {:else}
    <ul class="divide-y">
      {#each results as m (m.id)}
        <li>
          <button type="button" class="hover:bg-muted/50 focus-visible:ring-ring/50 flex w-full gap-3 px-4 py-3 text-start outline-none focus-visible:ring-[3px] focus-visible:ring-inset" onclick={() => open(m)}>
            <ChatAvatar person={chat.byId[m.user.id] || m.user} size="size-8" />
            <span class="min-w-0 flex-1">
              <span class="text-muted-foreground flex items-baseline gap-2 text-xs">
                <span class="text-foreground truncate font-medium">{m.user.name}</span>
                <span class="truncate">{m.conversation?.name}{m.parent_id ? " · in a thread" : ""}</span>
                <span class="ms-auto shrink-0 tabular-nums">{new Date(m.created_at).toLocaleDateString(undefined, { month: "short", day: "numeric" })} {clockTime(m.created_at)}</span>
              </span>
              <span class="line-clamp-3 text-sm">{m.text || "A file"}</span>
            </span>
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</div>
