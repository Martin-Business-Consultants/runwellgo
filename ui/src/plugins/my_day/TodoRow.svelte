<script>
  // One piece of work in the list or the priorities (days/_todo_link): its
  // number (in the priorities), its state (the dot, which a box takes over
  // to pick it for the bulk bar: x), its title with the client under it, its
  // engagement's ref, when it's due, and a copy button for its brief (c on
  // the row you're on). Dragged by the row onto Priorities (or back off).
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Check from "@lucide/svelte/icons/check";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import { STATUS_LABELS, WAITING, statusDot, due, copyBrief } from "./day.js";

  let { todo, today, number = null, current = false, cursor = false, picked = false, picking = false, link, onpick } = $props();
  const pill = $derived(due(todo, today));
</script>

<div data-nav-item data-id={todo.id} data-row
  class="group/row relative flex items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors
    {current || picked ? 'bg-accent text-accent-foreground' : todo.status === 'done' ? 'bg-emerald-50 text-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200' : 'hover:bg-muted/60'}
    {WAITING.includes(todo.status) && !current ? 'text-muted-foreground' : ''}
    {cursor ? 'ring-ring/60 ring-2' : ''}">
  {#if number}<span class="bg-primary text-primary-foreground flex size-5 shrink-0 items-center justify-center rounded-full text-[11px] font-semibold tabular-nums">{number}</span>{/if}
  <span class="relative flex size-4 shrink-0 items-center justify-center">
    <span class="flex items-center justify-center {picked || picking ? 'hidden' : 'group-hover/row:hidden'}">
      {#if todo.status === "done"}
        <Check class="size-4 text-emerald-600" aria-label="Done" />
      {:else}
        <span class="size-2.5 rounded-full {statusDot[todo.status]}" title={STATUS_LABELS[todo.status]}></span>
      {/if}
    </span>
    <span class="{picked || picking ? 'flex' : 'hidden group-hover/row:flex'} items-center">
      <Checkbox checked={picked} onCheckedChange={(v) => onpick(todo.id, v)} aria-label={`Pick ${todo.title}`} />
    </span>
  </span>
  <a href={link} draggable="false" class="flex min-w-0 flex-1 items-center gap-2" aria-current={current ? "page" : undefined}
    title={`${todo.title} · ${todo.engagement} ${todo.engagement_title}`}>
    <span class="flex min-w-0 flex-1 flex-col">
      <span class="truncate font-medium">{todo.title}</span>
      <span class="text-muted-foreground truncate text-xs">{todo.client}</span>
    </span>
    <span class="text-muted-foreground shrink-0 text-xs tabular-nums">{todo.engagement}</span>
    {#if pill}
      <span class="shrink-0 rounded-full px-2 py-0.5 text-xs whitespace-nowrap
        {pill.late ? 'bg-destructive/10 text-destructive' : pill.today ? 'bg-sky-500/15 text-sky-700 dark:text-sky-300' : 'bg-muted text-muted-foreground'}">{pill.text}</span>
    {/if}
  </a>
  <Button variant="ghost" size="icon-sm" class="text-muted-foreground shrink-0 opacity-0 group-hover/row:opacity-100 focus-visible:opacity-100 {cursor ? 'opacity-100' : ''}"
    title="Copy a brief for your AI (c)" onclick={() => copyBrief(todo.id)}>
    <Sparkles /><span class="sr-only">Copy for AI: {todo.title}</span>
  </Button>
</div>
