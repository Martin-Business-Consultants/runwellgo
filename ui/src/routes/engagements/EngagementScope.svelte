<script>
  // The engagement's scope down the side: all its work, each agreed scope
  // item, and work outside the scope, each with how much is done. Picking
  // one shows just its work in the Work tab (?tab=work&item=12, or
  // item=other). current: the one shown ("all", an id, "other"), null off
  // the Work tab.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import { session } from "$lib/session.svelte.js";
  import { withQuery } from "$lib/router.svelte.js";
  import { humanize } from "$lib/format.js";
  import { cn } from "$lib/utils.js";

  let { engagement, current = null } = $props();
  const scope = $derived(engagement.scope);
  const rows = $derived([
    { id: "all", name: `All ${session.term("work").toLowerCase()}`, done: scope.done, total: scope.total },
    ...scope.items.map((it) => ({ id: String(it.id), name: it.description, done: it.done, total: it.total, state: it.delivery_state })),
    ...(scope.other ? [{ id: "other", name: "Other work", done: scope.other.done, total: scope.other.total }] : []),
  ]);
  const dot = { done: "bg-primary", in_progress: "bg-amber-500", blocked: "bg-destructive", not_started: "bg-muted-foreground/40" };
</script>

<Card.Root>
  <Card.Header><Card.Title>{session.term("scope_item", 2)}</Card.Title></Card.Header>
  <Card.Content>
    <nav aria-label={session.term("scope_item", 2)} class="flex flex-col gap-1">
      {#each rows as r (r.id)}
        <a href={withQuery({ tab: "work", item: r.id === "all" ? null : r.id })} aria-current={current === r.id ? "page" : undefined}
          class={cn("hover:bg-muted flex flex-col gap-1.5 rounded-md px-2 py-1.5 text-sm", current === r.id && "bg-muted")}>
          <span class="flex items-center gap-2">
            {#if r.state}<span class={cn("size-2 shrink-0 rounded-full", dot[r.state])} title={humanize(r.state)}></span>{/if}
            <span class="min-w-0 flex-1 truncate" title={r.name}>{r.name}</span>
            <span class="text-muted-foreground text-xs tabular-nums">{r.done}/{r.total}</span>
          </span>
          <Progress value={r.done} max={Math.max(r.total, 1)} class="h-1" aria-label={`${r.done} of ${r.total} done`} />
        </a>
      {/each}
    </nav>
    {#if rows.length === 1 && !scope.total}
      <p class="text-muted-foreground mt-2 text-sm">
        {#if engagement.internal}
          No {session.term("work").toLowerCase()} yet: add it in the {session.term("work")} tab.
        {:else}
          Nothing agreed yet. Approving the agreement lists its {session.term("scope_item", 2).toLowerCase()} here.
        {/if}
      </p>
    {/if}
  </Card.Content>
</Card.Root>
