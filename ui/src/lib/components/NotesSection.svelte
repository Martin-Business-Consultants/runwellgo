<script>
  // A record's notes (the Rails notes/_section): write one right here, above
  // the ones already written (the editor with @mentions, what kind of note
  // it is and where it came from), then the notes, latest first, each with
  // who wrote it and Delete for whoever may. ⌘/Ctrl+Enter saves.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import Trash from "@lucide/svelte/icons/trash-2";
  import RichTextEditor from "./RichTextEditor.svelte";
  import PersonAvatar from "./PersonAvatar.svelte";
  import { post, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { dateTime, humanize } from "$lib/format.js";
  import PluginSlot from "$lib/components/PluginSlot.svelte";

  let { subjectType, subjectId, notes = [], reload } = $props();

  const kinds = ["call", "meeting", "email", "internal"];
  let body = $state(""), kind = $state("internal"), source = $state(""), busy = $state(false), error = $state("");

  const empty = (html) => !html || !html.replace(/<[^>]*>/g, "").trim() && !/data-mention-id|data-mentionable|<img/.test(html);

  async function add(e) {
    e?.preventDefault();
    if (busy) return;
    if (empty(body)) { error = "Write the note first."; return; }
    busy = true; error = "";
    try {
      await post("/notes", { record: `${subjectType}:${subjectId}`, note: { body, kind, source } });
      toast("Note added.");
      body = ""; source = ""; kind = "internal";
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

  async function remove(n) {
    if (!(await confirm({ title: "Delete this note?", message: "It’s removed for everyone.", confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/notes/${n.id}`); toast("Note deleted."); await reload?.(); } catch (e) { toastError(e); }
  }
</script>

<Card.Root>
  <Card.Content class="flex flex-col gap-4">
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
    </form>

    {#if !notes.length}
      <p class="text-muted-foreground text-sm">No notes yet.</p>
    {:else}
      <ul class="flex flex-col divide-y">
        {#each notes as n (n.id)}
          <li class="flex flex-col gap-1 py-3 first:pt-0 last:pb-0">
            <div class="text-muted-foreground flex items-center gap-2 text-xs">
              {#if n.author}<PersonAvatar person={n.author} size="size-5" />{/if}
              <span class="min-w-0 flex-1 truncate">{[n.author?.name, humanize(n.kind), dateTime(n.occurred_at), n.source].filter(Boolean).join(" · ")}</span>
              {#if deletable(n)}
                <Button variant="ghost" size="icon" class="size-7" aria-label="Delete this note" title="Delete this note" onclick={() => remove(n)}><Trash /></Button>
              {/if}
            </div>
            <div class="prose-admin text-sm [&_[data-mention-id]]:text-primary [&_[data-mention-id]]:font-medium [&_[data-mentionable]]:text-primary [&_[data-mentionable]]:font-medium [&_[data-type=mention]]:font-medium">{@html n.body_html || ""}</div>
            <PluginSlot name="note_row" note={n} {subjectType} {subjectId} />
          </li>
        {/each}
      </ul>
    {/if}
  </Card.Content>
</Card.Root>
