<script>
  // What the client asked us for, newest first, and whether each is open,
  // taken on, or closed.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import PortalLayout from "./PortalLayout.svelte";
  import { toastError } from "$lib/toast.svelte.js";
  import { portal, pget } from "./portal.svelte.js";

  let requests = $state(null);

  async function load() {
    if (!(await portal.require())) return;
    try { requests = (await pget("/portal/requests")).requests; } catch (e) { toastError(e); }
  }
  load();

  const label = (state) => (state === "open" ? "Open" : state === "promoted" ? "Taken on" : "Closed");
</script>

<PortalLayout title="Requests">
  <Card.Root>
    <Card.Header class="flex flex-row items-center justify-between gap-4">
      <Card.Description>What you’ve asked us for, newest first.</Card.Description>
      <Button href="/portal/requests/new" size="sm"><Plus />New request</Button>
    </Card.Header>
    <Card.Content>
      {#if !requests}
        <Skeleton class="h-24" />
      {:else if !requests.length}
        <p class="text-sm">Nothing yet.</p>
      {:else}
        <ul class="divide-y">
          {#each requests as r (r.id)}
            <li class="flex items-center gap-3 py-3">
              <div class="min-w-0 flex-1">
                <div class="truncate text-sm font-medium">{r.subject}</div>
                <div class="text-muted-foreground text-xs">{portal.dateTime(r.received_at)}</div>
              </div>
              <Badge variant={r.state === "promoted" ? "default" : r.state === "open" ? "outline" : "secondary"}>{label(r.state)}</Badge>
            </li>
          {/each}
        </ul>
      {/if}
    </Card.Content>
  </Card.Root>
</PortalLayout>
