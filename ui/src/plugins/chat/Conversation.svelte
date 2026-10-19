<script>
  // One conversation: its header (name, topic, members, details), its
  // messages by day with a "New" line above what you hadn't read (older
  // ones load as you scroll up), who's typing, and the composer. Messages
  // you're looking at are marked read. Drop files anywhere on it to attach.
  import { tick, onDestroy } from "svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ArrowLeft from "@lucide/svelte/icons/arrow-left";
  import Hash from "@lucide/svelte/icons/hash";
  import Lock from "@lucide/svelte/icons/lock";
  import Users from "@lucide/svelte/icons/users";
  import Info from "@lucide/svelte/icons/info";
  import BellOff from "@lucide/svelte/icons/bell-off";
  import Upload from "@lucide/svelte/icons/upload";
  import ChatAvatar from "./ChatAvatar.svelte";
  import MessageItem from "./MessageItem.svelte";
  import Composer from "./Composer.svelte";
  import { chat, BASE, dayLabel, sameDay, grouped, typingWords, PRESENCE } from "./chat.svelte.js";
  import { get, post, patch } from "$lib/api.js";
  import { nav } from "$lib/nav.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { id, onthread, onpanel, panel = "" } = $props();

  let info = $state(null); // the conversation, as the messages' answer has it
  let messages = $state([]);
  let more = $state(false);
  let loading = $state(true);
  let loadingOlder = $state(false);
  let marker = $state(0); // where you'd read up to when you opened it
  let editingId = $state(null);
  let scroller = $state(null);
  let composer = $state(null);
  let dragging = $state(0);
  let atBottom = true;

  const conv = $derived(chat.conversation(id) || info);
  const other = $derived(conv?.kind === "dm" ? chat.person(conv.dm_user_id) : null);
  const member = $derived(!!chat.conversation(id));
  const typers = $derived(chat.typers(id));
  const firstNew = $derived(messages.find((m) => m.id > marker && !m.mine)?.id);

  async function load() {
    try {
      const r = await get(`${BASE}/conversations/${id}/messages`);
      info = r.conversation;
      marker = r.conversation.last_read_id || 0;
      messages = r.messages;
      more = r.has_more;
    } catch (e) {
      toastError(e);
      if (e.status === 404) go("/chat", undefined, { replace: true });
      return;
    } finally {
      loading = false;
    }
    await tick();
    const line = scroller?.querySelector("[data-new-line]");
    // Where you left off: the "New" line near the top, unless it's on the last screen anyway.
    bottom();
    if (line && scroller.scrollTop > 0 && line.getBoundingClientRect().top < scroller.getBoundingClientRect().top + 40) {
      atBottom = false;
      line.scrollIntoView({ block: "start" });
      scroller.scrollTop -= 40;
    }
    markRead();
  }
  load();

  // since reads what came after the last message here (a reconnect, or a
  // message too long to push).
  async function since() {
    const last = messages.at(-1)?.id;
    if (!last) return load();
    try {
      const r = await get(`${BASE}/conversations/${id}/messages?after=${last}&limit=100`);
      if (r.has_more) return load();
      add(r.messages);
    } catch { /* the next event tries again */ }
  }

  async function older() {
    if (!more || loadingOlder || !messages.length) return;
    loadingOlder = true;
    const before = scroller.scrollHeight;
    try {
      const r = await get(`${BASE}/conversations/${id}/messages?before=${messages[0].id}`);
      messages = [...r.messages, ...messages];
      more = r.has_more;
      await tick();
      scroller.scrollTop += scroller.scrollHeight - before;
    } catch (e) { toastError(e); } finally { loadingOlder = false; }
  }

  function bottom() {
    if (scroller) scroller.scrollTop = scroller.scrollHeight;
  }

  // At the bottom stays at the bottom while things around it settle (the
  // composer drawing itself, images arriving, the window resizing).
  $effect(() => {
    if (!scroller) return;
    const keep = new ResizeObserver(() => { if (atBottom) bottom(); });
    keep.observe(scroller);
    for (const el of scroller.children) keep.observe(el);
    const watch = new MutationObserver(() => { for (const el of scroller.children) keep.observe(el); });
    watch.observe(scroller, { childList: true });
    return () => { keep.disconnect(); watch.disconnect(); };
  });

  // Only scrolling up lets go of the bottom: the page moving under it (the
  // composer drawing itself) doesn't.
  let lastTop = 0;
  function scrolled() {
    const near = scroller.scrollHeight - scroller.scrollTop - scroller.clientHeight < 80;
    if (near) atBottom = true;
    else if (scroller.scrollTop < lastTop - 2) atBottom = false;
    lastTop = scroller.scrollTop;
    if (scroller.scrollTop < 300) older();
    if (atBottom) markRead();
  }

  async function add(list) {
    const stick = atBottom;
    let changed = false;
    for (const m of list) {
      if (m.parent_id || m.conversation_id !== id) continue;
      // Seen as it arrives: no "New" line for it.
      if (m.mine || (stick && chat.looking(id))) marker = Math.max(marker, m.id);
      const i = messages.findIndex((x) => x.id === m.id);
      if (i >= 0) messages[i] = m;
      else { messages.push(m); changed = true; }
    }
    if (changed) messages.sort((a, b) => a.id - b.id);
    await tick();
    if (stick || list.some((m) => m.mine)) bottom();
    markRead();
  }

  function replace(m) {
    const i = messages.findIndex((x) => x.id === m.id);
    if (i >= 0) messages[i] = m;
  }

  // markRead marks what's on screen read, when someone's looking at it.
  let reading = 0;
  async function markRead() {
    const last = messages.findLast((m) => !m.parent_id)?.id;
    const c = chat.conversation(id);
    if (!last || !c || !atBottom || !chat.looking(id) || (c.last_read_id >= last && !c.unread) || reading >= last) return;
    reading = last;
    chat.read(id, last);
    try { await post(`${BASE}/conversations/${id}/read`, { message_id: last }, { quiet: true }); nav.refresh(); } catch { reading = 0; }
  }

  const stop = chat.listen(async (topic, data) => {
    if (topic === "resync") return since();
    const m = data.message;
    if (!m || m.conversation_id !== id) return;
    if (data.stale) return since();
    if (topic === "message") add([m]);
    else if (topic === "message_updated") replace(m);
  });
  onDestroy(stop);

  // Someone coming back to the tab reads what arrived.
  const back = () => markRead();
  addEventListener("focus", back);
  document.addEventListener("visibilitychange", back);
  onDestroy(() => { removeEventListener("focus", back); document.removeEventListener("visibilitychange", back); });

  // editLast is Up on an empty composer: your last message, to edit.
  function editLast() {
    const m = messages.findLast((x) => x.mine && !x.deleted);
    if (!m) return;
    editingId = m.id;
    tick().then(() => scroller.querySelector(`[data-message-id="${m.id}"]`)?.scrollIntoView({ block: "nearest" }));
  }

  async function join() {
    try {
      const r = await post(`${BASE}/conversations/${id}/members`, {});
      chat.upsert(r.conversation, true);
      toast(`You joined ${r.conversation.name}.`);
    } catch (e) { toastError(e); }
  }

  async function unarchive() {
    try {
      const r = await patch(`${BASE}/conversations/${id}`, { conversation: { archived: false } });
      chat.upsert(r.conversation);
      info = r.conversation;
    } catch (e) { toastError(e); }
  }

  function dropFiles(e) {
    e.preventDefault();
    dragging = 0;
    if (e.dataTransfer?.files?.length && composer) composer.addFiles([...e.dataTransfer.files]);
  }
  const hasFiles = (e) => [...(e.dataTransfer?.types || [])].includes("Files");
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="relative flex min-h-0 flex-1 flex-col"
  ondragenter={(e) => { if (hasFiles(e)) { e.preventDefault(); dragging++; } }}
  ondragover={(e) => { if (hasFiles(e)) e.preventDefault(); }}
  ondragleave={() => (dragging = Math.max(0, dragging - 1))}
  ondrop={dropFiles}>
  <header class="flex h-14 shrink-0 items-center gap-2 border-b px-3 md:px-4">
    <Button variant="ghost" size="icon" class="size-8 md:hidden" href="/chat" aria-label="All conversations"><ArrowLeft /></Button>
    {#if conv}
      {#if conv.kind === "channel"}
        {#if conv.private}<Lock class="text-muted-foreground size-4 shrink-0" aria-label="Private channel" />{:else}<Hash class="text-muted-foreground size-4 shrink-0" aria-hidden="true" />{/if}
      {:else if other}
        <ChatAvatar person={other} size="size-7" presence />
      {:else}
        <Users class="text-muted-foreground size-4 shrink-0" aria-hidden="true" />
      {/if}
      <div class="min-w-0 flex-1">
        <h2 class="flex items-center gap-1.5 truncate text-base leading-tight font-semibold">
          <span class="truncate">{conv.kind === "channel" ? conv.name.replace(/^#/, "") : conv.name}</span>
          {#if conv.muted}<BellOff class="text-muted-foreground size-3.5 shrink-0" aria-label="Muted" />{/if}
          {#if conv.archived}<span class="text-muted-foreground rounded border px-1.5 text-xs font-normal">Archived</span>{/if}
        </h2>
        {#if conv.topic}
          <p class="text-muted-foreground truncate text-xs">{conv.topic}</p>
        {:else if other}
          <p class="text-muted-foreground truncate text-xs">{other.status_emoji} {other.status_text || PRESENCE[other.presence]}</p>
        {/if}
      </div>
      {#if conv.kind !== "dm"}
        <Button variant="ghost" size="sm" class="h-8 gap-1.5 px-2" onclick={() => onpanel("details")} aria-label="{conv.member_count} members" title="Members">
          <span class="hidden -space-x-1.5 sm:flex">
            {#each conv.member_ids.slice(0, 3) as uid (uid)}<ChatAvatar person={chat.person(uid)} size="size-5" class="ring-background rounded-md ring-2" />{/each}
          </span>
          <span class="tabular-nums">{conv.member_count}</span>
        </Button>
      {/if}
      <Button variant={panel === "details" ? "secondary" : "ghost"} size="icon" class="size-8" onclick={() => onpanel(panel === "details" ? "" : "details")} aria-label="Details" title="Details" aria-pressed={panel === "details"}><Info /></Button>
    {/if}
  </header>

  <div bind:this={scroller} onscroll={scrolled} class="min-h-0 flex-1 overflow-y-auto overscroll-contain pb-2" role="log" aria-label="Messages" aria-live="polite" aria-relevant="additions">
    {#if loading}
      <div class="flex flex-col gap-4 p-4">
        {#each [1, 2, 3] as i (i)}
          <div class="flex gap-3"><Skeleton class="size-9 rounded-md" /><div class="flex flex-1 flex-col gap-2"><Skeleton class="h-4 w-32" /><Skeleton class="h-4 w-2/3" /></div></div>
        {/each}
      </div>
    {:else}
      {#if !more}
        <div class="px-4 pt-8 pb-4">
          {#if conv?.kind === "channel"}
            <p class="text-xl font-semibold">{conv.name}</p>
            <p class="text-muted-foreground mt-1 text-sm">{conv.description || `This is the start of ${conv.name}.`}</p>
          {:else if other}
            <ChatAvatar person={other} size="size-14" />
            <p class="mt-2 text-xl font-semibold">{other.name}</p>
            <p class="text-muted-foreground mt-1 text-sm">{other.id === chat.me?.id ? "Your own space: notes, drafts and files for yourself." : `This is the start of your messages with ${other.name}.`}</p>
          {:else if conv}
            <p class="text-xl font-semibold">{conv.name}</p>
            <p class="text-muted-foreground mt-1 text-sm">This is the start of your group's messages.</p>
          {/if}
        </div>
      {:else if loadingOlder}
        <p class="text-muted-foreground py-3 text-center text-xs">Loading older messages…</p>
      {/if}
      {#each messages as m, i (m.id)}
        {@const prev = messages[i - 1]}
        {#if !prev || !sameDay(prev.created_at, m.created_at)}
          <div class="relative my-2 flex items-center px-4" role="separator" aria-label={dayLabel(m.created_at)}>
            <div class="flex-1 border-t"></div>
            <span class="bg-background text-muted-foreground rounded-full border px-3 py-0.5 text-xs font-medium">{dayLabel(m.created_at)}</span>
            <div class="flex-1 border-t"></div>
          </div>
        {/if}
        {#if m.id === firstNew}
          <div data-new-line class="my-1 flex items-center gap-2 px-4" role="separator" aria-label="New messages">
            <div class="flex-1 border-t border-red-600/60"></div>
            <span class="text-xs font-semibold text-red-700 dark:text-red-400">New</span>
          </div>
        {/if}
        <MessageItem {m} grouped={grouped(prev, m) && m.id !== firstNew && sameDay(prev.created_at, m.created_at)} onthread={(x) => onthread(x.id)}
          bind:editing={() => editingId === m.id, (v) => (editingId = v ? m.id : null)} onchanged={replace} />
      {/each}
    {/if}
  </div>

  <div class="text-muted-foreground h-5 shrink-0 truncate px-4 text-xs" aria-live="polite">{typingWords(typers)}</div>

  <div class="shrink-0 px-3 pb-3 md:px-4">
    {#if conv?.archived}
      <div class="bg-muted/50 flex flex-wrap items-center justify-center gap-3 rounded-lg border px-4 py-3 text-sm">
        <span>This channel is archived: its history stays, and no one can send to it.</span>
        {#if chat.manage || conv.created_by_id === chat.me?.id}<Button size="sm" variant="outline" onclick={unarchive}>Bring it back</Button>{/if}
      </div>
    {:else if conv && !member}
      <div class="bg-muted/50 flex flex-wrap items-center justify-center gap-3 rounded-lg border px-4 py-3 text-sm">
        <span>You're looking at {conv.name}.</span>
        <Button size="sm" onclick={join}>Join channel</Button>
      </div>
    {:else if conv}
      <Composer bind:this={composer} conversationId={id} label="Message {conv.name}" placeholder="Message {conv.kind === 'channel' ? conv.name : conv.name.split(',')[0]}"
        onsent={(m) => add([m])} oneditlast={editLast} />
    {/if}
  </div>

  {#if dragging}
    <div class="bg-background/90 border-primary pointer-events-none absolute inset-2 z-20 flex flex-col items-center justify-center gap-2 rounded-xl border-2 border-dashed">
      <Upload class="text-primary size-8" />
      <p class="text-sm font-medium">Drop files to attach them</p>
    </div>
  {/if}
</div>
