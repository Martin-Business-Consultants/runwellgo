<script>
  // A conversation's details, beside it: its name, topic and description
  // (members may change them), who's in it (add people, or leave), mute,
  // and archive for a channel's maker, owners and managers. For a direct
  // message, the other person: their status and whether they're here.
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Popover from "$lib/components/ui/popover/index.js";
  import * as Command from "$lib/components/ui/command/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import X from "@lucide/svelte/icons/x";
  import UserPlus from "@lucide/svelte/icons/user-plus";
  import LogOut from "@lucide/svelte/icons/log-out";
  import Archive from "@lucide/svelte/icons/archive";
  import ChatAvatar from "./ChatAvatar.svelte";
  import { chat, BASE, PRESENCE } from "./chat.svelte.js";
  import { get, patch, post, del } from "$lib/api.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { ApiError } from "$lib/api.js";

  let { id, onclose } = $props();

  let c = $state(null);
  let form = $state({ name: "", topic: "", description: "" });
  let errors = $state({});
  let saving = $state(false);
  let adding = $state(false);

  async function load() {
    try {
      c = (await get(`${BASE}/conversations/${id}`)).conversation;
      form = { name: c.channel_name || (c.kind === "group" ? "" : c.name), topic: c.topic, description: c.description };
    } catch (e) { toastError(e); }
  }
  load();

  const stop = chat.listen((topic, data) => { if (topic === "conversation" && data.id === id) load(); });
  $effect(() => stop);

  const other = $derived(c?.kind === "dm" ? chat.person(c.dm_user_id) : null);
  const mine = $derived(!!chat.conversation(id));
  const canManage = $derived(c && (chat.manage || c.created_by_id === chat.me?.id));
  const notIn = $derived(chat.people.filter((p) => c && !c.member_ids.includes(p.id)));

  async function save(e) {
    e.preventDefault();
    saving = true;
    errors = {};
    const body = { topic: form.topic, description: form.description };
    if (c.kind !== "dm") body.name = form.name;
    try {
      const r = await patch(`${BASE}/conversations/${id}`, { conversation: body });
      c = r.conversation;
      chat.upsert(r.conversation);
      toast("Saved.");
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) errors = err.errors;
      else toastError(err);
    } finally { saving = false; }
  }

  async function mute(muted) {
    try {
      const r = await patch(`${BASE}/conversations/${id}/membership`, { membership: { muted } });
      chat.upsert(r.conversation);
      c = r.conversation;
    } catch (e) { toastError(e); }
  }

  async function add(person) {
    adding = false;
    try {
      const r = await post(`${BASE}/conversations/${id}/members`, { user_ids: [person.id] });
      c = r.conversation;
      chat.upsert(r.conversation);
      toast(`Added ${person.name}.`);
    } catch (e) { toastError(e); }
  }

  async function remove(person) {
    const self = person.id === chat.me?.id;
    if (!(await confirm({ title: self ? `Leave ${c.name}?` : `Remove ${person.name}?`, message: self ? (c.private ? "You'll need someone to add you again to come back." : "You can join again whenever you like.") : `${person.name} won't see new messages here.`, confirmLabel: self ? "Leave" : "Remove", destructive: true }))) return;
    try {
      const r = await del(`${BASE}/conversations/${id}/members/${person.id}`);
      if (self) {
        chat.conversations = chat.conversations.filter((x) => x.id !== id);
        toast(r.summary || `You left ${c.name}.`);
        if (r.left) go("/chat");
        else onclose();
      } else {
        c = r.conversation;
        chat.upsert(r.conversation);
      }
    } catch (e) { toastError(e); }
  }

  async function archive() {
    if (!(await confirm({ title: `Archive ${c.name}?`, message: "No one can send to it any more; its history stays, and you can bring it back.", confirmLabel: "Archive" }))) return;
    try {
      const r = await patch(`${BASE}/conversations/${id}`, { conversation: { archived: true } });
      c = r.conversation;
      chat.upsert(r.conversation);
    } catch (e) { toastError(e); }
  }
</script>

<div class="flex h-14 shrink-0 items-center gap-2 border-b px-4">
  <h2 class="min-w-0 flex-1 truncate text-base font-semibold">Details</h2>
  <Button variant="ghost" size="icon" class="size-8" onclick={onclose} aria-label="Close details" title="Close (Esc)"><X /></Button>
</div>
<div class="min-h-0 flex-1 overflow-y-auto overscroll-contain p-4">
  {#if !c}
    <p class="text-muted-foreground text-sm">Loading…</p>
  {:else}
    {#if other}
      <div class="flex flex-col items-center gap-2 pb-4 text-center">
        <ChatAvatar person={other} size="size-20" />
        <p class="text-lg font-semibold">{other.name}</p>
        {#if other.full_name && other.full_name !== other.name}<p class="text-muted-foreground -mt-2 text-sm">{other.full_name}</p>{/if}
        <p class="text-sm"><span class="me-1.5 inline-block size-2 rounded-full {other.presence === 'online' ? 'bg-emerald-500' : other.presence === 'away' ? 'border-2 border-amber-500' : 'bg-muted-foreground/40'}"></span>{PRESENCE[other.presence]}{#if !other.available} · outside their hours{/if}</p>
        {#if other.status_text || other.status_emoji}<p class="text-muted-foreground text-sm">{other.status_emoji} {other.status_text}</p>{/if}
      </div>
    {/if}

    {#if c.kind !== "dm"}
      <form onsubmit={save} class="mb-6">
        <Field.Group class="gap-4">
          <Field.Field data-invalid={!!errors.name}>
            <Field.Label for="chat-details-name">Name</Field.Label>
            <Input id="chat-details-name" bind:value={form.name} placeholder={c.kind === "group" ? "Optional: the people's names show otherwise" : "design-team"} disabled={!mine} />
            {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field data-invalid={!!errors.topic}>
            <Field.Label for="chat-details-topic">Topic</Field.Label>
            <Input id="chat-details-topic" bind:value={form.topic} placeholder="What it's about now" disabled={!mine} />
            {#if errors.topic}<Field.Error>Topic {errors.topic[0]}</Field.Error>{/if}
          </Field.Field>
          {#if c.kind === "channel"}
            <Field.Field data-invalid={!!errors.description}>
              <Field.Label for="chat-details-description">Description</Field.Label>
              <Textarea id="chat-details-description" bind:value={form.description} rows={3} placeholder="What it's for" disabled={!mine} />
              {#if errors.description}<Field.Error>Description {errors.description[0]}</Field.Error>{/if}
            </Field.Field>
          {/if}
          {#if mine}<div><Button type="submit" size="sm" disabled={saving}>Save</Button></div>{/if}
        </Field.Group>
      </form>
    {/if}

    {#if mine}
      <Field.Field orientation="horizontal" class="mb-6">
        <Field.Content>
          <Field.Label for="chat-details-mute">Mute</Field.Label>
          <Field.Description>No notifications or unread count, unless someone mentions you.</Field.Description>
        </Field.Content>
        <Switch id="chat-details-mute" checked={c.muted} onCheckedChange={mute} />
      </Field.Field>
    {/if}

    {#if c.kind !== "dm"}
      <div class="mb-2 flex items-center gap-2">
        <h3 class="flex-1 text-sm font-semibold">Members <span class="text-muted-foreground font-normal tabular-nums">{c.member_count}</span></h3>
        {#if mine && c.kind === "channel" && !c.archived}
          <Popover.Root bind:open={adding}>
            <Popover.Trigger>
              {#snippet child({ props })}
                <Button {...props} variant="outline" size="sm"><UserPlus />Add people</Button>
              {/snippet}
            </Popover.Trigger>
            <Popover.Content align="end" class="w-64 p-0">
              <Command.Root>
                <Command.Input placeholder="Find someone" />
                <Command.List>
                  <Command.Empty>Everyone's in already.</Command.Empty>
                  {#each notIn as p (p.id)}
                    <Command.Item value={p.name} onSelect={() => add(p)}><ChatAvatar person={p} size="size-5" />{p.name}</Command.Item>
                  {/each}
                </Command.List>
              </Command.Root>
            </Popover.Content>
          </Popover.Root>
        {/if}
      </div>
      <ul class="mb-6 flex flex-col gap-1">
        {#each c.members as p (p.id)}
          <li class="group/member flex items-center gap-2 rounded-md py-1">
            <ChatAvatar person={chat.byId[p.id] || p} size="size-7" presence />
            <span class="min-w-0 flex-1">
              <span class="block truncate text-sm">{p.name}{#if p.id === chat.me?.id} <span class="text-muted-foreground">(you)</span>{/if}</span>
              {#if p.status_text}<span class="text-muted-foreground block truncate text-xs">{p.status_emoji} {p.status_text}</span>{/if}
            </span>
            {#if c.kind === "channel" && p.id !== chat.me?.id && canManage}
              <Button variant="ghost" size="sm" class="h-7" onclick={() => remove(p)} aria-label="Remove {p.name}">Remove</Button>
            {/if}
          </li>
        {/each}
      </ul>
      {#if mine}
        <div class="flex flex-wrap gap-2 border-t pt-4">
          <Button variant="outline" size="sm" onclick={() => remove(chat.me)}><LogOut />Leave</Button>
          {#if c.kind === "channel" && canManage && !c.archived}
            <Button variant="outline" size="sm" onclick={archive}><Archive />Archive channel</Button>
          {/if}
        </div>
      {/if}
    {/if}
  {/if}
</div>
