<script>
  import EmptyState from "$lib/components/EmptyState.svelte";
  // What we're working on for the client across every agreement: the work
  // shared with them, by due date.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import PortalLayout from "./PortalLayout.svelte";
  import { toastError } from "$lib/toast.svelte.js";
  import { date, humanize } from "$lib/format.js";
  import { portal, pget } from "./portal.svelte.js";

  let work = $state(null);

  async function load() {
    if (!(await portal.require())) return;
    try { work = (await pget("/portal/work")).work; } catch (e) { toastError(e); }
  }
  load();
</script>

<PortalLayout title={portal.term("work")}>
  {#if !work}
    <Skeleton class="h-40" />
  {:else if !work.length}
    <EmptyState icon="check-square" title="No work to show yet." />
  {:else}
    <Card.Root>
      <Card.Header><Card.Description>What we are working on for you across every agreement.</Card.Description></Card.Header>
      <Card.Content>
        <ul class="divide-y">
          {#each work as w, i (i)}
            <li class="flex items-center gap-3 py-3">
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium">{w.title}</div>
                <div class="text-muted-foreground text-xs">
                  <a href={`/portal/engagements/${w.engagement.ref}`} class="hover:underline">{w.engagement.display_name}</a>{w.due_on ? ` · due ${date(w.due_on)}` : ""}
                </div>
              </div>
              <StatusBadge status={w.status} label={humanize(w.status)} />
            </li>
          {/each}
        </ul>
      </Card.Content>
    </Card.Root>
  {/if}
</PortalLayout>
