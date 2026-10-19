<script>
  // The portal's home: everything agreed with the client, and where each one
  // stands.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import PortalLayout from "./PortalLayout.svelte";
  import { toastError } from "$lib/toast.svelte.js";
  import { portal, pget } from "./portal.svelte.js";

  let engagements = $state(null);

  async function load() {
    if (!(await portal.require())) return;
    try { engagements = (await pget("/portal/engagements")).engagements; } catch (e) { toastError(e); }
  }
  load();
</script>

<PortalLayout title={portal.term("engagement", 2)}>
  {#if !engagements}
    <Skeleton class="h-40" />
  {:else if !engagements.length}
    <Card.Root><Card.Content class="text-muted-foreground py-10 text-center text-sm">No agreements yet.</Card.Content></Card.Root>
  {:else}
    <Card.Root>
      <Card.Header><Card.Description>Everything we have agreed with you, and where each one stands.</Card.Description></Card.Header>
      <Card.Content>
        <ul class="divide-y">
          {#each engagements as e (e.ref)}
            <li>
              <a href={`/portal/engagements/${e.ref}`} class="hover:bg-muted/60 -mx-2 flex items-center gap-3 rounded-md px-2 py-3">
                <div class="min-w-0 flex-1">
                  <div class="truncate text-sm"><strong>{e.ref}</strong> {e.title}</div>
                  <div class="text-muted-foreground text-xs">{e.label}</div>
                </div>
                {#if e.awaiting_your_decision}<Badge variant="outline">Awaiting your decision</Badge>{/if}
                {#if e.priced}<span class="text-sm whitespace-nowrap tabular-nums">{e.agreed}{e.per_period ? " / period" : ""}</span>{/if}
                <Badge variant={e.active ? "default" : "secondary"}>{e.state_label}</Badge>
                <ChevronRight class="text-muted-foreground size-4" />
              </a>
            </li>
          {/each}
        </ul>
      </Card.Content>
    </Card.Root>
  {/if}
</PortalLayout>
