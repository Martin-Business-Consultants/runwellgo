<script>
  // The Work board: one column per status, planned first and done last.
  // Drag a card to another column or up and down within one; the drop is
  // saved (before the card it landed above, or at the end), and a move the
  // server refuses puts the cards back as they were.
  import Sortable from "sortablejs";
  import * as Card from "$lib/components/ui/card/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import { get, post } from "$lib/api.js";
  import { href } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { humanize } from "$lib/format.js";
  import { STATUSES, statusDot, shortDate, ownersOf } from "./work.js";

  let { owner = "any", engagement = "all" } = $props();
  let columns = $state(Object.fromEntries(STATUSES.map((s) => [s, null])));

  async function loadColumn(status) {
    try { columns[status] = (await get(href(`/columns/work/${status}`, { owner, engagement }))).work; } catch (e) { toastError(e); }
  }
  const loadAll = () => STATUSES.forEach(loadColumn);
  $effect(() => { owner; engagement; loadAll(); });

  // Where a drop goes: the planned stream, a doing column, or done.
  function dropPath(id, status) {
    if (status === "planned") return `/columns/work/${id}/drops/stream`;
    if (status === "done") return `/columns/work/${id}/drops/closure`;
    return `/columns/work/${id}/drops/column`;
  }

  function board() {
    return (el) => {
      const s = Sortable.create(el, {
        group: "work-board",
        draggable: "[data-id]",
        animation: 150,
        ghostClass: "opacity-40",
        async onEnd({ item, from, to, oldDraggableIndex: oldIndex, newDraggableIndex: newIndex }) {
          const fromStatus = from.dataset.status, toStatus = to.dataset.status;
          if (fromStatus === toStatus && oldIndex === newIndex) return;
          // Put the card back where Svelte drew it; the lists move it.
          const others = [...from.children].filter((c) => c !== item && c.matches("[data-id]"));
          from.insertBefore(item, others[oldIndex] ?? others[others.length - 1]?.nextSibling ?? null);
          const [card] = columns[fromStatus].splice(oldIndex, 1);
          columns[toStatus].splice(newIndex, 0, card);
          const before = columns[toStatus][newIndex + 1]?.id;
          try {
            await post(dropPath(card.id, toStatus), { column_id: toStatus, before, owner, engagement });
          } catch (e) { toastError(e); }
          loadColumn(fromStatus);
          if (toStatus !== fromStatus) loadColumn(toStatus);
        },
      });
      return () => s.destroy();
    };
  }
</script>

<div class="relative flex w-full gap-3 overflow-x-auto pb-2 [contain:inline-size]">
  {#each STATUSES as status (status)}
    {@const cards = columns[status]}
    <section class="bg-muted/40 flex w-72 shrink-0 flex-col rounded-xl border" aria-label={humanize(status)}>
      <header class="flex items-center gap-2 px-3 py-2.5">
        <span class="size-2.5 rounded-full {statusDot[status]}" aria-hidden="true"></span>
        <h2 class="text-sm font-medium">{humanize(status)}</h2>
        <span class="text-muted-foreground text-xs tabular-nums">{cards ? (cards.length > 99 ? "99+" : cards.length) : ""}</span>
      </header>
      {#if !cards}
        <div class="flex flex-col gap-2 p-2"><Skeleton class="h-20" /><Skeleton class="h-20" /></div>
      {:else}
        <div class="flex min-h-24 flex-1 flex-col gap-2 p-2 pt-0" data-status={status} {@attach board()}>
          {#each cards as t (t.id)}
            {@const owners = ownersOf(t)}
            <a href={`/work/${t.id}`} data-id={t.id} class="block cursor-grab active:cursor-grabbing">
              <Card.Root class="hover:border-primary/40 gap-2 py-3 shadow-xs transition-colors">
                <Card.Header class="gap-0.5 px-3">
                  <Card.Description class="truncate text-xs">{t.engagement?.ref} · {t.client?.display_name}</Card.Description>
                  <Card.Title class="line-clamp-3 text-sm leading-snug">{t.title}</Card.Title>
                </Card.Header>
                {#if t.due_on || owners.length || (t.status === "done" && t.completed_at)}
                  <Card.Footer class="text-muted-foreground flex items-center gap-2 px-3 text-xs">
                    {#if t.status === "done" && t.completed_at}<span>Done {shortDate(t.completed_at)}</span>
                    {:else if t.due_on}<span>Due {shortDate(t.due_on)}</span>{/if}
                    {#if t.overdue}<Badge variant="destructive">Overdue</Badge>{/if}
                    <span class="ms-auto flex -space-x-1.5">{#each owners.slice(0, 3) as p (p.id)}<span class="ring-card rounded-full ring-2"><PersonAvatar person={p} size="size-5" /></span>{/each}</span>
                  </Card.Footer>
                {/if}
              </Card.Root>
            </a>
          {/each}
        </div>
        {#if !cards.length}<p class="text-muted-foreground -mt-20 mb-8 px-3 text-center text-xs pointer-events-none">Drop cards here</p>{/if}
      {/if}
    </section>
  {/each}
</div>
