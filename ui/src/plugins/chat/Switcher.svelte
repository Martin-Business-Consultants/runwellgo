<script>
  // Jump to (⌘K on the chat page): your conversations and everyone on
  // staff, filtered as you type; picking a person opens your direct
  // messages with them. Search Runwell is there too, for everything else.
  import * as Command from "$lib/components/ui/command/index.js";
  import Hash from "@lucide/svelte/icons/hash";
  import Lock from "@lucide/svelte/icons/lock";
  import Users from "@lucide/svelte/icons/users";
  import Search from "@lucide/svelte/icons/search";
  import ChatAvatar from "./ChatAvatar.svelte";
  import { chat, BASE } from "./chat.svelte.js";
  import { post } from "$lib/api.js";
  import { go } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";

  let { open = $bindable(false) } = $props();

  const dmWith = $derived(new Set(chat.conversations.filter((c) => c.kind === "dm").map((c) => c.dm_user_id)));
  const unread = $derived(chat.conversations.filter((c) => c.unread > 0 && !c.muted));

  function pick(c) {
    open = false;
    go("/chat", { c: c.id }, { transition: false });
  }

  async function person(p) {
    open = false;
    const existing = chat.conversations.find((c) => c.kind === "dm" && c.dm_user_id === p.id);
    if (existing) return pick(existing);
    try {
      const r = await post(`${BASE}/conversations`, { conversation: { kind: "dm", member_ids: [p.id] } });
      chat.upsert(r.conversation, true);
      go("/chat", { c: r.conversation.id }, { transition: false });
    } catch (e) { toastError(e); }
  }
</script>

<Command.Dialog bind:open title="Jump to a conversation" description="Your conversations and people on staff">
  <Command.Input placeholder="Jump to a conversation or person…" />
  <Command.List>
    <Command.Empty>Nothing by that name.</Command.Empty>
    {#if unread.length}
      <Command.Group heading="Unread">
        {#each unread as c (c.id)}
          <Command.Item value="unread {c.name} {c.id}" onSelect={() => pick(c)}>
            {#if c.kind === "channel"}<Hash />{:else}<Users />{/if}<span class="truncate font-semibold">{c.name}</span>
            <span class="text-muted-foreground ms-auto text-xs tabular-nums">{c.unread}</span>
          </Command.Item>
        {/each}
      </Command.Group>
    {/if}
    <Command.Group heading="Conversations">
      {#each chat.conversations.filter((c) => !c.archived) as c (c.id)}
        <Command.Item value="{c.name} {c.id}" onSelect={() => pick(c)}>
          {#if c.kind === "channel"}{#if c.private}<Lock />{:else}<Hash />{/if}{:else if c.kind === "dm"}<ChatAvatar person={chat.person(c.dm_user_id)} size="size-4" />{:else}<Users />{/if}
          <span class="truncate">{c.name}</span>
        </Command.Item>
      {/each}
    </Command.Group>
    <Command.Group heading="People">
      {#each chat.people.filter((p) => !dmWith.has(p.id)) as p (p.id)}
        <Command.Item value="person {p.name} {p.id}" onSelect={() => person(p)}>
          <ChatAvatar person={p} size="size-4" /><span class="truncate">{p.name}</span>
        </Command.Item>
      {/each}
    </Command.Group>
    <Command.Group heading="Elsewhere">
      <Command.Item value="search runwell everything" onSelect={() => { open = false; setTimeout(() => dispatchEvent(new Event("opensearch")), 50); }}>
        <Search />Search everything in Runwell
      </Command.Item>
    </Command.Group>
  </Command.List>
</Command.Dialog>
