<script>
  // The piece of work picked on the left (days/_detail), in one card that
  // stays in view: where it's from, its title, then its status, due date and
  // people as one row of controls (each saving in place), its details (read,
  // click to edit), its place in your priorities, Copy for AI and Open in
  // Work, and its notes. What the day doesn't carry (its details, who's on
  // it, its notes) comes from the core's work page answer.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Check from "@lucide/svelte/icons/check";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import ArrowDown from "@lucide/svelte/icons/arrow-down";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import Pin from "@lucide/svelte/icons/pin";
  import PinOff from "@lucide/svelte/icons/pin-off";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import RichText from "$lib/components/RichText.svelte";
  import RichTextEditor from "$lib/components/RichTextEditor.svelte";
  import NotesSection from "$lib/components/NotesSection.svelte";
  import PeoplePicker from "./PeoplePicker.svelte";
  import { untrack } from "svelte";
  import { get } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { timeAgo } from "$lib/format.js";
  import { STATUSES, STATUS_LABELS, statusDot, call, copyBrief } from "./day.js";

  // todo: the day's (its place in the priorities, late); count: how many
  // priorities; limit: how many they hold; changed: reload the day.
  let { todo, count, limit, changed } = $props();
  // The work as last read, so a page drawn afresh (a filter, another row)
  // shows it at once while it's read again.
  let work = $state(untrack(() => seen.get(todo.id)) ?? null);
  let editing = $state(false);
  let details = $state("");
  let statusMenu = $state(false);

  async function load(id) {
    try {
      const w = (await get(`/work/${id}`)).todo;
      seen.set(w.id, w);
      if (w.id === todo.id) work = w;
    } catch (e) { toastError(e); }
  }
  $effect(() => { const id = todo.id; untrack(() => { editing = false; if (work?.id !== id) work = seen.get(id) ?? null; load(id); }); });

  // A change saved: the day again (its groups, priorities, summary), and
  // this card's work.
  async function after(ok) {
    if (!ok) return;
    await changed();
    load(todo.id);
  }
  const status = (s) => { statusMenu = false; return s !== todo.status && call("PATCH", `/todos/${todo.id}/status`, { status: s }).then(after); };
  const dueOn = (v) => call("PATCH", `/todos/${todo.id}`, { todo: { due_on: v } }).then(after);
  async function save(e) {
    e?.preventDefault();
    if (await call("PATCH", `/todos/${todo.id}`, { todo: { description: details } })) { editing = false; after(true); }
  }
  const priority = (method, body) => call(method, `/priorities/${todo.id}`, body).then((ok) => ok && changed());
</script>

<script module>
  const seen = new Map();
</script>

<Card.Root class={todo.status === "done" ? "border-emerald-500/40" : ""}>
  <Card.Header class="gap-2">
    <nav class="text-muted-foreground flex min-w-0 items-center gap-1.5 text-xs" aria-label="Where it's from">
      <a class="truncate hover:underline" href={`/clients/${todo.client_id}`}>{todo.client}</a>
      <span aria-hidden="true">›</span>
      <a class="truncate hover:underline" href={`/engagements/${todo.engagement}`}>{todo.engagement} {todo.engagement_title}</a>
    </nav>
    {#if todo.status === "done"}
      <p class="flex items-center gap-1.5 text-sm text-emerald-700 dark:text-emerald-400"><Check class="size-4" /> Done{#if work?.completed_at}&nbsp;{timeAgo(work.completed_at)}{/if}. Nice work.</p>
    {/if}
    <Card.Title class="text-xl leading-snug">{todo.title}</Card.Title>
    <div class="flex flex-wrap items-center gap-2 pt-1">
      <DropdownMenu.Root bind:open={statusMenu}>
        <DropdownMenu.Trigger>
          {#snippet child({ props })}
            <Button {...props} variant="outline" size="sm" class="rounded-full">
              <span class="size-2.5 rounded-full {statusDot[todo.status]}" aria-hidden="true"></span>{STATUS_LABELS[todo.status]}<ChevronDown />
              <span class="sr-only">: change the status</span>
            </Button>
          {/snippet}
        </DropdownMenu.Trigger>
        <DropdownMenu.Content align="start" class="w-44">
          <DropdownMenu.Label>Status</DropdownMenu.Label>
          <DropdownMenu.RadioGroup value={todo.status} onValueChange={status}>
            {#each STATUSES as s (s)}
              <DropdownMenu.RadioItem value={s} onSelect={() => (statusMenu = false)}><span class="size-2.5 rounded-full {statusDot[s]}" aria-hidden="true"></span>{STATUS_LABELS[s]}</DropdownMenu.RadioItem>
            {/each}
          </DropdownMenu.RadioGroup>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      <div class="w-44">
        <DatePicker value={todo.due_on || ""} placeholder="Due date" onchange={dueOn}
          class="h-8 rounded-full {todo.late ? 'border-destructive/50 text-destructive' : ''}" />
      </div>
      {#if work}<PeoplePicker todo={work} onchange={() => after(true)} />{:else}<Skeleton class="h-8 w-32 rounded-full" />{/if}
    </div>
  </Card.Header>

  <Card.Content class="flex flex-col gap-4">
    {#if editing}
      <form class="flex flex-col gap-2" onsubmit={save}>
        <RichTextEditor bind:value={details} placeholder="What needs doing, and anything worth knowing. @ to mention someone." onsubmit={save} />
        <div class="flex gap-2">
          <Button type="submit" size="sm">Save details</Button>
          <Button type="button" variant="ghost" size="sm" onclick={() => (editing = false)}>Cancel</Button>
        </div>
      </form>
    {:else}
      <button type="button" class="hover:bg-muted/50 group/edit relative -mx-2 rounded-md px-2 py-1.5 text-start"
        onclick={() => { details = work?.description || ""; editing = true; }}>
        {#if !work}
          <Skeleton class="h-5 w-2/3" />
        {:else if work.description_html}
          <RichText html={work.description_html} />
        {:else}
          <span class="text-muted-foreground text-sm">Add details…</span>
        {/if}
        <span class="text-muted-foreground absolute end-2 top-1.5 flex items-center gap-1 text-xs opacity-0 group-hover/edit:opacity-100"><Pencil class="size-3" /> Edit</span>
      </button>
    {/if}

    <div class="flex flex-wrap items-center gap-2 border-t pt-4">
      {#if todo.priority}
        <span class="flex items-center gap-1.5 text-sm"><Pin class="size-4" /> Priority {todo.priority} of {count}</span>
        {#if todo.priority > 1}
          <Button variant="outline" size="icon-sm" class="rounded-full" title="Move up" onclick={() => priority("PATCH", { direction: "up" })}><ArrowUp /><span class="sr-only">Move up</span></Button>
        {/if}
        {#if todo.priority < count}
          <Button variant="outline" size="icon-sm" class="rounded-full" title="Move down" onclick={() => priority("PATCH", { direction: "down" })}><ArrowDown /><span class="sr-only">Move down</span></Button>
        {/if}
        <Button variant="outline" size="sm" onclick={() => priority("DELETE")}><PinOff />Remove</Button>
      {:else if todo.status === "done"}
        <span class="text-muted-foreground text-sm">Finished work doesn't go in your priorities.</span>
      {:else if count >= limit}
        <span class="text-muted-foreground flex items-center gap-1.5 text-sm"><Pin class="size-4" /> Priorities are full ({limit})</span>
      {:else}
        <Button variant="link" size="sm" class="px-0" onclick={() => priority("POST")}><Pin />Add to priorities</Button>
      {/if}
      <div class="ms-auto flex flex-wrap gap-2">
        <Button variant="outline" size="sm" title="Copy a brief for your AI (c)" onclick={() => copyBrief(todo.id)}><Sparkles />Copy for AI</Button>
        <Button variant="outline" size="sm" href={`/work/${todo.id}`}>Open in {session.term("work")}<ArrowRight /></Button>
      </div>
    </div>

    {#if work}
      <NotesSection subjectType="Todo" subjectId={todo.id} notes={work.notes || []} reload={() => load(todo.id)} />
    {/if}
  </Card.Content>
</Card.Root>
