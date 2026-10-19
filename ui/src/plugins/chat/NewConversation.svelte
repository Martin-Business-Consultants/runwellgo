<script>
  // Starting a conversation, in a dialog of three views: message people
  // (one makes a direct message, a few a group; an existing one opens),
  // make a channel (public or private), or browse the public channels
  // you're not in and join one.
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import Hash from "@lucide/svelte/icons/hash";
  import Lock from "@lucide/svelte/icons/lock";
  import ChatAvatar from "./ChatAvatar.svelte";
  import { chat, BASE, PRESENCE } from "./chat.svelte.js";
  import { get, post, ApiError } from "$lib/api.js";
  import { go } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";

  let { open = $bindable(false), tab = $bindable("dm") } = $props();

  let find = $state("");
  let picked = $state([]);
  let channel = $state({ name: "", topic: "", private: false });
  let errors = $state({});
  let busy = $state(false);
  let browse = $state(null);

  const people = $derived(chat.people.filter((p) => !find.trim() || p.name.toLowerCase().includes(find.trim().toLowerCase())));

  $effect(() => {
    if (!open) return;
    find = ""; picked = []; errors = {}; channel = { name: "", topic: "", private: false };
  });
  $effect(() => { if (open && tab === "browse") loadBrowse(); });

  async function loadBrowse() {
    try { browse = (await get(`${BASE}/conversations?browse=1`)).channels; } catch (e) { toastError(e); }
  }

  function toggle(id, on) {
    picked = on ? [...picked, id] : picked.filter((x) => x !== id);
  }

  async function create(body) {
    busy = true;
    errors = {};
    try {
      const r = await post(`${BASE}/conversations`, { conversation: body });
      chat.upsert(r.conversation, true);
      open = false;
      go("/chat", { c: r.conversation.id }, { transition: false });
    } catch (e) {
      if (e instanceof ApiError && e.status === 422) errors = e.errors;
      else toastError(e);
    } finally { busy = false; }
  }

  function message(e) {
    e.preventDefault();
    const others = picked.filter((id) => id !== chat.me?.id);
    if (!picked.length) { errors = { member_ids: ["needs someone: pick a person (or yourself, for notes)"] }; return; }
    create({ kind: others.length > 1 ? "group" : "dm", member_ids: others });
  }

  function makeChannel(e) {
    e.preventDefault();
    create({ kind: "channel", ...channel });
  }

  async function join(c) {
    try {
      const r = await post(`${BASE}/conversations/${c.id}/members`, {});
      chat.upsert(r.conversation, true);
      open = false;
      go("/chat", { c: c.id }, { transition: false });
    } catch (e) { toastError(e); }
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-lg">
    <Dialog.Header>
      <Dialog.Title>Start a conversation</Dialog.Title>
      <Dialog.Description>Message one person or a few, or talk in a channel.</Dialog.Description>
    </Dialog.Header>
    <Tabs.Root bind:value={tab}>
      <Tabs.List class="w-full">
        <Tabs.Trigger value="dm">Message people</Tabs.Trigger>
        <Tabs.Trigger value="channel">New channel</Tabs.Trigger>
        <Tabs.Trigger value="browse">Browse</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="dm">
        <form onsubmit={message} class="flex flex-col gap-3 pt-2">
          <Input type="search" bind:value={find} placeholder="Find someone" aria-label="Find someone" />
          <ul class="max-h-72 overflow-y-auto rounded-md border" aria-label="People">
            {#each people as p (p.id)}
              <li>
                <label class="hover:bg-muted/50 flex cursor-pointer items-center gap-3 px-3 py-2">
                  <Checkbox checked={picked.includes(p.id)} onCheckedChange={(v) => toggle(p.id, v)} aria-label={p.name} />
                  <ChatAvatar person={p} size="size-7" presence />
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-sm">{p.name}{#if p.id === chat.me?.id} <span class="text-muted-foreground">(you)</span>{/if}</span>
                    <span class="text-muted-foreground block truncate text-xs">{p.status_emoji} {p.status_text || PRESENCE[p.presence]}</span>
                  </span>
                </label>
              </li>
            {:else}
              <li class="text-muted-foreground px-3 py-4 text-sm">No one on staff is called that.</li>
            {/each}
          </ul>
          {#if errors.member_ids}<p class="text-destructive text-sm">People {errors.member_ids[0]}</p>{/if}
          <Dialog.Footer>
            <Button type="submit" disabled={busy || !picked.length}>{picked.filter((id) => id !== chat.me?.id).length > 1 ? "Start group" : "Message"}</Button>
          </Dialog.Footer>
        </form>
      </Tabs.Content>
      <Tabs.Content value="channel">
        <form onsubmit={makeChannel} class="pt-2">
          <Field.Group class="gap-4">
            <Field.Field data-invalid={!!errors.name}>
              <Field.Label for="chat-new-name">Name</Field.Label>
              <Input id="chat-new-name" bind:value={channel.name} placeholder="design-team" required autocomplete="off" />
              <Field.Description>Lowercase words joined by -.</Field.Description>
              {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field>
              <Field.Label for="chat-new-topic">Topic <span class="text-muted-foreground font-normal">(optional)</span></Field.Label>
              <Input id="chat-new-topic" bind:value={channel.topic} placeholder="What it's about" />
            </Field.Field>
            <Field.Field orientation="horizontal">
              <Field.Content>
                <Field.Label for="chat-new-private">Private</Field.Label>
                <Field.Description>Only the people in it see it; they add others.</Field.Description>
              </Field.Content>
              <Switch id="chat-new-private" bind:checked={channel.private} />
            </Field.Field>
            <Dialog.Footer><Button type="submit" disabled={busy}>Create channel</Button></Dialog.Footer>
          </Field.Group>
        </form>
      </Tabs.Content>
      <Tabs.Content value="browse">
        <div class="pt-2">
          {#if browse === null}
            <p class="text-muted-foreground py-4 text-sm">Loading…</p>
          {:else if !browse.length}
            <p class="text-muted-foreground py-4 text-sm">You're in every public channel. Make a new one in New channel.</p>
          {:else}
            <ul class="max-h-80 divide-y overflow-y-auto rounded-md border">
              {#each browse as c (c.id)}
                <li class="flex items-center gap-3 px-3 py-2">
                  {#if c.private}<Lock class="text-muted-foreground size-4" />{:else}<Hash class="text-muted-foreground size-4" />{/if}
                  <span class="min-w-0 flex-1">
                    <span class="block truncate text-sm font-medium">{c.name.replace(/^#/, "")}{#if c.archived} <span class="text-muted-foreground font-normal">(archived)</span>{/if}</span>
                    <span class="text-muted-foreground block truncate text-xs">{c.member_count === 1 ? "1 member" : `${c.member_count} members`}{c.topic ? ` · ${c.topic}` : ""}</span>
                  </span>
                  <Button size="sm" variant="outline" href="/chat?c={c.id}" onclick={() => (open = false)}>View</Button>
                  {#if !c.archived}<Button size="sm" onclick={() => join(c)}>Join</Button>{/if}
                </li>
              {/each}
            </ul>
          {/if}
        </div>
      </Tabs.Content>
    </Tabs.Root>
  </Dialog.Content>
</Dialog.Root>
