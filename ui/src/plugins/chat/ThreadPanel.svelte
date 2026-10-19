<script>
  // A message's thread, beside the conversation: the message, its replies
  // as they come, and a composer to reply. Escape closes it.
  import { tick, onDestroy } from "svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import X from "@lucide/svelte/icons/x";
  import MessageItem from "./MessageItem.svelte";
  import Composer from "./Composer.svelte";
  import { chat, BASE, grouped, typingWords } from "./chat.svelte.js";
  import { get } from "$lib/api.js";
  import { toastError } from "$lib/toast.svelte.js";

  let { parentId, onclose } = $props();

  let parent = $state(null);
  let replies = $state([]);
  let conversation = $state(null);
  let scroller = $state(null);
  let editingId = $state(null);
  let failed = $state(false);

  const member = $derived(conversation && !!chat.conversation(conversation.id));
  const typers = $derived(conversation ? chat.typers(conversation.id, parentId) : []);

  async function load() {
    try {
      const r = await get(`${BASE}/messages/${parentId}/thread`);
      parent = r.message;
      replies = r.replies;
      conversation = r.conversation;
      await tick();
      if (scroller) scroller.scrollTop = scroller.scrollHeight;
    } catch (e) { failed = true; toastError(e); }
  }
  load();

  async function add(m) {
    const near = scroller && scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 80;
    const i = replies.findIndex((x) => x.id === m.id);
    if (i >= 0) replies[i] = m; else replies.push(m);
    await tick();
    if ((near || m.mine) && scroller) scroller.scrollTop = scroller.scrollHeight;
  }

  function replace(m) {
    if (m.id === parentId) parent = m;
    const i = replies.findIndex((x) => x.id === m.id);
    if (i >= 0) replies[i] = m;
  }

  const stop = chat.listen((topic, data) => {
    if (topic === "resync") return load();
    const m = data.message;
    if (!m) return;
    if (data.stale && (m.id === parentId || m.parent_id === parentId)) return load();
    if (topic === "message" && m.parent_id === parentId) add(m);
    else if (topic === "message_updated") replace(m);
  });
  onDestroy(stop);

  function editLast() {
    const m = replies.findLast((x) => x.mine && !x.deleted);
    if (m) editingId = m.id;
  }
</script>

<div class="flex h-14 shrink-0 items-center gap-2 border-b px-4">
  <h2 class="min-w-0 flex-1 truncate text-base font-semibold">Thread{#if conversation}<span class="text-muted-foreground ms-2 text-sm font-normal">{conversation.name}</span>{/if}</h2>
  <Button variant="ghost" size="icon" class="size-8" onclick={onclose} aria-label="Close thread" title="Close (Esc)"><X /></Button>
</div>
<div bind:this={scroller} class="min-h-0 flex-1 overflow-y-auto overscroll-contain py-2" role="log" aria-label="Replies" aria-live="polite">
  {#if failed}
    <p class="text-muted-foreground p-4 text-sm">This thread couldn't be opened: it may have been deleted, or you can't see its conversation.</p>
  {:else if !parent}
    <div class="flex gap-3 p-4"><Skeleton class="size-9 rounded-md" /><div class="flex flex-1 flex-col gap-2"><Skeleton class="h-4 w-32" /><Skeleton class="h-4 w-2/3" /></div></div>
  {:else}
    <MessageItem m={parent} inThread onchanged={replace} />
    <div class="my-2 flex items-center gap-2 px-4">
      <span class="text-muted-foreground text-xs">{replies.length === 1 ? "1 reply" : `${replies.length} replies`}</span>
      <div class="flex-1 border-t"></div>
    </div>
    {#each replies as m, i (m.id)}
      <MessageItem {m} inThread grouped={grouped(replies[i - 1], m)} onchanged={replace}
        bind:editing={() => editingId === m.id, (v) => (editingId = v ? m.id : null)} />
    {/each}
  {/if}
</div>
<div class="text-muted-foreground h-5 shrink-0 truncate px-4 text-xs" aria-live="polite">{typingWords(typers)}</div>
{#if parent && conversation && member && !conversation.archived}
  <div class="shrink-0 px-3 pb-3">
    <Composer conversationId={conversation.id} {parentId} label="Reply in thread" placeholder="Reply…" onsent={add} oneditlast={editLast} />
  </div>
{/if}
