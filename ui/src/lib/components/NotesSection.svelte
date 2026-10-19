<script>
  // A record's notes (the Rails notes/_section): write one right here, above
  // the ones already written (the editor with @mentions, what kind of note
  // it is and where it came from), then the notes, latest first, each with
  // who wrote it and Delete for whoever may. ⌘/Ctrl+Enter saves. A
  // request's notes can be shared with the client, who sees them (and
  // writes their own) on the request in their portal.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Label } from "$lib/components/ui/label/index.js";
  import Trash from "@lucide/svelte/icons/trash-2";
  import RichTextEditor from "./RichTextEditor.svelte";
  import PersonAvatar from "./PersonAvatar.svelte";
  import { post, patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { dateTime, humanize } from "$lib/format.js";
  import PluginSlot from "$lib/components/PluginSlot.svelte";
  import EmptyState from "./EmptyState.svelte";

  let { subjectType, subjectId, notes = [], reload } = $props();

  const kinds = ["call", "meeting", "email", "internal"];
  let body = $state(""), kind = $state("internal"), source = $state(""), shared = $state(false), busy = $state(false), error = $state("");
  const shareable = $derived(subjectType === "Request");

  const empty = (html) => !html || !html.replace(/<[^>]*>/g, "").trim() && !/data-mention-id|data-mentionable|<img/.test(html);

  async function add(e) {
    e?.preventDefault();
    if (busy) return;
    if (empty(body)) { error = "Write the note first."; return; }
    busy = true; error = "";
    try {
      await post("/notes", { record: `${subjectType}:${subjectId}`, note: { body, kind, source, client_visible: shareable && shared } });
      toast("Note added.");
      body = ""; source = ""; kind = "internal"; shared = false;
      await reload?.();
    } catch (err) {
      const msgs = Object.entries(err.errors || {}).map(([k, v]) => `${humanize(k)} ${v.join(", ")}`);
      if (msgs.length) error = msgs.join(". ") + "."; else toastError(err);
    }
    busy = false;
  }

  // Its author, the author's person (a note their agent wrote), or anyone
  // who may delete records.
  const deletable = (n) => {
    const me = session.me;
    return !!me && (n.author?.id === me.id || n.author_agent_owner_id === me.id || session.can("delete_records"));
  };

  async function setShared(n, on) {
    try { const r = await patch(`/notes/${n.id}`, { note: { client_visible: on } }); toast(r.summary); await reload?.(); } catch (e) { toastError(e); }
  }

  // A note the client wrote in the portal says so; the team's say their
  // kind, unless it's shared (then "internal" would mislead).
  const meta = (n) => [n.author?.name || n.contact?.name, n.contact || n.client_visible ? "" : humanize(n.kind), dateTime(n.occurred_at), n.source && n.source !== "portal" ? n.source : ""].filter(Boolean).join(" · ");

  async function remove(n) {
    if (!(await confirm({ title: "Delete this note?", message: "It’s removed for everyone.", confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/notes/${n.id}`); toast("Note deleted."); await reload?.(); } catch (e) { toastError(e); }
  }
</script>

<Card.Root>
  <Card.Header><Card.Title>Notes{#if notes.length} <span class="text-muted-foreground font-normal tabular-nums">{notes.length}</span>{/if}</Card.Title></Card.Header>
  <Card.Content>
    <form onsubmit={add} class="flex flex-col gap-2 [&_[data-mention-id]]:text-primary [&_[data-mention-id]]:font-medium [&_[data-mentionable]]:text-primary [&_[data-mentionable]]:font-medium">
      <RichTextEditor bind:value={body} placeholder="Add a note: what was said or decided… @ to mention someone" invalid={!!error} onsubmit={add} />
      {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
      <div class="flex flex-wrap items-center gap-2">
        <Select.Root type="single" bind:value={kind}>
          <Select.Trigger class="w-32" aria-label="Kind">{humanize(kind)}</Select.Trigger>
          <Select.Content>{#each kinds as k (k)}<Select.Item value={k}>{humanize(k)}</Select.Item>{/each}</Select.Content>
        </Select.Root>
        <Input bind:value={source} class="min-w-40 flex-1" autocomplete="off" placeholder="Source, e.g. call with Ann" aria-label="Source" />
        <Button type="submit" disabled={busy}>Add note</Button>
      </div>
      {#if shareable}
        <div class="flex items-center gap-2">
          <Switch id="note-shared-{subjectId}" bind:checked={shared} />
          <Label for="note-shared-{subjectId}" class="font-normal">Share with the client: they see it on this request in their portal</Label>
        </div>
      {/if}
    </form>
  </Card.Content>

  {#if !notes.length}
    <EmptyState class="mt-1" icon="document" title="No notes yet." description="Calls, meetings and what was decided: add the first one above." />
  {:else}
    <Card.Content>
      <ul class="flex flex-col divide-y">
        {#each notes as n (n.id)}
          <li class="flex flex-col gap-1 py-3 first:pt-0 last:pb-0">
            <div class="text-muted-foreground flex items-center gap-2 text-xs">
              {#if n.author}<PersonAvatar person={n.author} size="size-5" />{/if}
              <span class="min-w-0 flex-1 truncate">{meta(n)}</span>
              {#if n.contact}<Badge variant="outline">From the client</Badge>
              {:else if shareable}
                <Switch checked={n.client_visible} onCheckedChange={(v) => setShared(n, v)} aria-label="Shared with the client" title={n.client_visible ? "The client can see it" : "Share with the client"} />
                {#if n.client_visible}<span>Shared</span>{/if}
              {/if}
              {#if deletable(n)}
                <Button variant="ghost" size="icon" class="size-7" aria-label="Delete this note" title="Delete this note" onclick={() => remove(n)}><Trash /></Button>
              {/if}
            </div>
            <div class="prose-admin text-sm [&_[data-mention-id]]:text-primary [&_[data-mention-id]]:font-medium [&_[data-mentionable]]:text-primary [&_[data-mentionable]]:font-medium [&_[data-type=mention]]:font-medium">{@html n.body_html || ""}</div>
            <PluginSlot name="note_row" note={n} {subjectType} {subjectId} />
          </li>
        {/each}
      </ul>
    </Card.Content>
  {/if}
</Card.Root>
