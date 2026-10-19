<script>
  // The conversations down the left: Jump to (⌘K), then Channels and
  // Direct messages, each with + to start one. Unread ones are bold with
  // their count; muted ones are quiet. Your own picture, status and
  // settings at the foot.
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Kbd } from "$lib/components/ui/kbd/index.js";
  import Hash from "@lucide/svelte/icons/hash";
  import Lock from "@lucide/svelte/icons/lock";
  import Users from "@lucide/svelte/icons/users";
  import Plus from "@lucide/svelte/icons/plus";
  import Search from "@lucide/svelte/icons/search";
  import TextSearch from "@lucide/svelte/icons/text-search";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import BellOff from "@lucide/svelte/icons/bell-off";
  import SquarePen from "@lucide/svelte/icons/square-pen";
  import Settings from "@lucide/svelte/icons/settings";
  import ChatAvatar from "./ChatAvatar.svelte";
  import { chat, PRESENCE } from "./chat.svelte.js";
  import { go } from "$lib/router.svelte.js";

  let { current = null, onnew, onswitch, onsearch } = $props();

  const visible = $derived(chat.conversations.filter((c) => !c.archived || c.id === current));
  const channels = $derived(visible.filter((c) => c.kind === "channel").sort((a, b) => a.name.localeCompare(b.name)));
  const direct = $derived(visible.filter((c) => c.kind !== "channel"));
  let channelsOpen = $state(true), directOpen = $state(true);

  function open(e, c) {
    if (e.metaKey || e.ctrlKey || e.shiftKey || e.button) return;
    e.preventDefault();
    go("/chat", { c: c.id }, { transition: false });
  }
</script>

{#snippet row(c)}
  {@const here = c.id === current}
  {@const loud = c.unread > 0 && !c.muted}
  <li>
    <a href="/chat?c={c.id}" onclick={(e) => open(e, c)} aria-current={here ? "page" : undefined}
      class="group/row flex h-8 items-center gap-2 rounded-md px-2 text-sm outline-none focus-visible:ring-ring/50 focus-visible:ring-[3px] {here ? 'bg-primary/10 text-foreground font-medium' : 'hover:bg-muted text-muted-foreground'} {loud ? 'text-foreground font-semibold' : ''}">
      {#if c.kind === "channel"}
        {#if c.private}<Lock class="size-4 shrink-0 opacity-70" aria-label="Private channel" />{:else}<Hash class="size-4 shrink-0 opacity-70" aria-hidden="true" />{/if}
      {:else if c.kind === "dm"}
        <ChatAvatar person={chat.person(c.dm_user_id)} size="size-5" presence />
      {:else}
        <span class="bg-muted flex size-5 shrink-0 items-center justify-center rounded-md text-[10px] font-semibold" aria-hidden="true">{c.member_count}</span>
      {/if}
      <span class="min-w-0 flex-1 truncate">{c.kind === "channel" ? c.name.replace(/^#/, "") : c.name}</span>
      {#if c.muted}<BellOff class="size-3.5 shrink-0 opacity-60" aria-label="Muted" />{/if}
      {#if c.unread > 0 && !c.muted}
        <span class="bg-primary text-primary-foreground rounded-full px-1.5 text-[11px] leading-5 font-semibold tabular-nums">{c.unread > 99 ? "99+" : c.unread}<span class="sr-only"> unread{c.mentions ? `, ${c.mentions} mentioning you` : ""}</span></span>
      {/if}
    </a>
  </li>
{/snippet}

<div class="flex items-center gap-1 border-b p-2">
  <Button variant="outline" size="sm" class="text-muted-foreground h-8 min-w-0 flex-1 justify-start font-normal" onclick={onswitch}>
    <Search /><span class="truncate">Jump to…</span><Kbd class="ms-auto hidden md:inline-flex">⌘K</Kbd>
  </Button>
  <Button variant="ghost" size="icon" class="size-8" onclick={onsearch} aria-label="Search messages" title="Search messages"><TextSearch /></Button>
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>
      {#snippet child({ props })}
        <Button {...props} size="icon" class="size-8" aria-label="New conversation" title="New conversation"><SquarePen /></Button>
      {/snippet}
    </DropdownMenu.Trigger>
    <DropdownMenu.Content align="end">
      <DropdownMenu.Item onSelect={() => onnew("dm")}><Users />New message</DropdownMenu.Item>
      <DropdownMenu.Item onSelect={() => onnew("channel")}><Hash />New channel</DropdownMenu.Item>
      <DropdownMenu.Item onSelect={() => onnew("browse")}><Search />Browse channels</DropdownMenu.Item>
    </DropdownMenu.Content>
  </DropdownMenu.Root>
</div>

<nav class="flex-1 overflow-y-auto overscroll-contain p-2" aria-label="Conversations" data-nav-skip>
  {#if !chat.ready}
    <p class="text-muted-foreground px-2 py-4 text-sm">{chat.error || "Loading…"}</p>
  {:else}
    <Collapsible.Root bind:open={channelsOpen}>
      <div class="group/head flex items-center">
        <Collapsible.Trigger class="text-muted-foreground hover:text-foreground flex flex-1 items-center gap-1 rounded-md px-1 py-1 text-xs font-medium outline-none focus-visible:ring-ring/50 focus-visible:ring-[3px]">
          <ChevronDown class="size-3.5 transition-transform {channelsOpen ? '' : '-rotate-90'}" />Channels
        </Collapsible.Trigger>
        <Button variant="ghost" size="icon" class="size-6" onclick={() => onnew("channel")} aria-label="New channel" title="New channel"><Plus class="size-3.5" /></Button>
      </div>
      <Collapsible.Content>
        <ul class="mt-0.5 mb-3 flex flex-col gap-px">
          {#each channels as c (c.id)}{@render row(c)}{/each}
          {#if !channels.length}
            <li><button type="button" class="text-muted-foreground hover:text-foreground px-2 py-1 text-start text-sm" onclick={() => onnew("browse")}>Browse or start a channel</button></li>
          {/if}
        </ul>
      </Collapsible.Content>
    </Collapsible.Root>
    <Collapsible.Root bind:open={directOpen}>
      <div class="flex items-center">
        <Collapsible.Trigger class="text-muted-foreground hover:text-foreground flex flex-1 items-center gap-1 rounded-md px-1 py-1 text-xs font-medium outline-none focus-visible:ring-ring/50 focus-visible:ring-[3px]">
          <ChevronDown class="size-3.5 transition-transform {directOpen ? '' : '-rotate-90'}" />Direct messages
        </Collapsible.Trigger>
        <Button variant="ghost" size="icon" class="size-6" onclick={() => onnew("dm")} aria-label="New message" title="New message"><Plus class="size-3.5" /></Button>
      </div>
      <Collapsible.Content>
        <ul class="mt-0.5 flex flex-col gap-px">
          {#each direct as c (c.id)}{@render row(c)}{/each}
          {#if !direct.length}
            <li><button type="button" class="text-muted-foreground hover:text-foreground px-2 py-1 text-start text-sm" onclick={() => onnew("dm")}>Message someone</button></li>
          {/if}
        </ul>
      </Collapsible.Content>
    </Collapsible.Root>
  {/if}
</nav>

{#if chat.me}
  <a href="/settings/chat" class="hover:bg-muted flex items-center gap-2 border-t px-3 py-2.5 text-sm outline-none focus-visible:ring-ring/50 focus-visible:ring-[3px]">
    <ChatAvatar person={chat.me} size="size-7" presence />
    <span class="min-w-0 flex-1">
      <span class="block truncate font-medium">{chat.me.name}</span>
      <span class="text-muted-foreground block truncate text-xs">{chat.me.status_emoji} {chat.me.status_text || PRESENCE[chat.me.presence] || ""}</span>
    </span>
    <Settings class="text-muted-foreground size-4" aria-label="Chat settings" />
  </a>
{/if}
