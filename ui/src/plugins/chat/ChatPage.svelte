<script>
  // Chat: your conversations down the left, the one you picked in the
  // middle, and its thread, details or a search on the right. The address
  // says which (?c=12&thread=40, &panel=details or search), so moving
  // between them keeps the page and a link opens the same place. On a
  // phone, the list and the conversation are a screen each, and the right
  // panel covers the conversation.
  //
  // Keys: ⌘K (Ctrl+K) jumps to a conversation; Alt+↑ / Alt+↓ moves to the
  // one above or below; Escape closes the right panel. In the composer,
  // Enter sends and Up edits your last message.
  import { onDestroy } from "svelte";
  import { MediaQuery } from "svelte/reactivity";
  import { Button } from "$lib/components/ui/button/index.js";
  import MessagesSquare from "@lucide/svelte/icons/messages-square";
  import ConversationList from "./ConversationList.svelte";
  import Conversation from "./Conversation.svelte";
  import ThreadPanel from "./ThreadPanel.svelte";
  import DetailsPanel from "./DetailsPanel.svelte";
  import SearchPanel from "./SearchPanel.svelte";
  import NewConversation from "./NewConversation.svelte";
  import Switcher from "./Switcher.svelte";
  import { chat } from "./chat.svelte.js";
  import { page } from "$lib/page.svelte.js";
  import { router, go } from "$lib/router.svelte.js";
  import { nav } from "$lib/nav.svelte.js";

  page.set("Chat");
  chat.start();

  const wide = new MediaQuery("min-width: 768px");
  const cid = $derived(Number(router.query.get("c")) || null);
  const thread = $derived(Number(router.query.get("thread")) || null);
  const panel = $derived(thread ? "thread" : ["details", "search"].includes(router.query.get("panel")) ? router.query.get("panel") : "");

  let newOpen = $state(false), newTab = $state("dm"), switcherOpen = $state(false);

  $effect(() => { chat.current = cid; });
  onDestroy(() => { chat.current = null; });

  // The title says what's open.
  $effect(() => {
    const c = cid && chat.conversation(cid);
    page.set(c ? c.name : "Chat", c ? [{ label: "Chat", href: "/chat" }] : []);
  });

  // On a wide screen there's always a conversation open: the latest.
  $effect(() => {
    if (chat.ready && !cid && wide.current && chat.conversations.length && router.path === "/chat") {
      go("/chat", { c: chat.conversations.find((c) => !c.archived)?.id ?? chat.conversations[0].id }, { replace: true, transition: false });
    }
  });

  const to = (query) => go("/chat", query, { transition: false });
  const openThread = (id) => to({ c: cid, thread: id });
  const setPanel = (p) => to({ c: cid, panel: p || undefined });
  const close = () => to({ c: cid });

  function step(dir) {
    const list = [...chat.conversations.filter((c) => !c.archived && c.kind === "channel").sort((a, b) => a.name.localeCompare(b.name)), ...chat.conversations.filter((c) => !c.archived && c.kind !== "channel")];
    if (!list.length) return;
    const i = list.findIndex((c) => c.id === cid);
    const next = list[(i + dir + list.length) % list.length];
    to({ c: next.id });
  }

  // Before the app's own keys (⌘K is search everywhere else).
  function keys(e) {
    if ((e.metaKey || e.ctrlKey) && !e.altKey && e.key.toLowerCase() === "k") {
      e.preventDefault();
      e.stopPropagation();
      switcherOpen = true;
    } else if (e.key === "Escape") {
      escape(e);
    } else if (e.altKey && !e.metaKey && !e.ctrlKey && (e.key === "ArrowUp" || e.key === "ArrowDown")) {
      e.preventDefault();
      step(e.key === "ArrowUp" ? -1 : 1);
    }
  }
  // Escape closes the right panel, unless it's closing something nearer
  // first (a menu, a dialog, the @ list, a message being edited). Heard
  // before the editor, which takes Escape for itself.
  function escape(e) {
    if (e.key !== "Escape" || !panel || e.target?.closest?.("[data-editing]") || document.querySelector("[role=dialog], [role=menu], [role=listbox]")) return;
    close();
  }
  addEventListener("keydown", keys, { capture: true });
  onDestroy(() => removeEventListener("keydown", keys, { capture: true }));

  nav.refresh();
</script>

<h1 class="sr-only">Chat</h1>
<div class="bg-background relative -mx-4 -my-6 flex h-[calc(100dvh-3rem)] min-h-0 overflow-hidden md:h-[calc(100dvh-4rem)] lg:-mx-6">
  <aside class="{cid ? 'hidden md:flex' : 'flex'} bg-muted/30 w-full shrink-0 flex-col border-e md:w-60 lg:w-64" aria-label="Your conversations">
    <ConversationList current={cid} onnew={(t) => { newTab = t; newOpen = true; }} onswitch={() => (switcherOpen = true)} onsearch={() => setPanel(panel === "search" ? "" : "search")} />
  </aside>

  <section class="{cid ? 'flex' : 'hidden md:flex'} min-w-0 flex-1 flex-col" aria-label="Conversation">
    {#if cid}
      {#key cid}
        <Conversation id={cid} {panel} onthread={openThread} onpanel={setPanel} />
      {/key}
    {:else}
      <div class="flex flex-1 flex-col items-center justify-center gap-3 p-6 text-center">
        <MessagesSquare class="text-muted-foreground size-10" />
        {#if chat.ready && !chat.conversations.length}
          <p class="text-lg font-semibold">Talk with your team here</p>
          <p class="text-muted-foreground max-w-sm text-sm">Message someone, or start a channel for a team or a topic. Everyone on staff can find public channels.</p>
          <div class="flex gap-2">
            <Button onclick={() => { newTab = "dm"; newOpen = true; }}>Message someone</Button>
            <Button variant="outline" onclick={() => { newTab = "channel"; newOpen = true; }}>New channel</Button>
          </div>
        {:else}
          <p class="text-muted-foreground text-sm">Pick a conversation.</p>
        {/if}
      </div>
    {/if}
  </section>

  {#if panel && cid}
    <aside class="bg-background absolute inset-0 z-20 flex flex-col md:inset-y-0 md:start-auto md:w-96 md:border-s md:shadow-lg xl:static xl:z-auto xl:w-96 xl:shadow-none"
      aria-label={panel === "thread" ? "Thread" : panel === "details" ? "Details" : "Search"}>
      {#if panel === "thread"}
        {#key thread}<ThreadPanel parentId={thread} onclose={close} />{/key}
      {:else if panel === "details"}
        {#key cid}<DetailsPanel id={cid} onclose={close} />{/key}
      {:else}
        <SearchPanel onclose={close} />
      {/if}
    </aside>
  {:else if panel === "search"}
    <aside class="bg-background absolute inset-0 z-20 flex flex-col md:inset-y-0 md:start-auto md:w-96 md:border-s md:shadow-lg xl:static xl:z-auto xl:w-96 xl:shadow-none" aria-label="Search">
      <SearchPanel onclose={close} />
    </aside>
  {/if}
</div>

<NewConversation bind:open={newOpen} bind:tab={newTab} />
<Switcher bind:open={switcherOpen} />
