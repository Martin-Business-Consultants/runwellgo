<script>
  // What the client asked us for, newest first, and whether each is open,
  // taken on, or closed.
  import * as Table from "$lib/components/ui/table/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Frame from "$lib/components/Frame.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
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
  <Frame description="What you’ve asked us for, newest first." bare={!!requests?.length}>
    {#snippet actions()}<Button href="/portal/requests/new" size="sm" data-nav-new><Plus />New request</Button>{/snippet}
    {#if !requests}
      <Skeleton class="h-24" />
    {:else if !requests.length}
      <EmptyState icon="inbox" title="No requests yet." description="Anything you ask us for shows here, with where it stands." />
    {:else}
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <Table.Head>Subject</Table.Head>
            <Table.Head class="w-40">Received</Table.Head>
            <Table.Head class="w-28">Status</Table.Head>
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each requests as r (r.id)}
            <Table.Row>
              <Table.Cell class="max-w-0 truncate font-medium" title={r.subject}><a href={`/portal/requests/${r.id}`} class="hover:underline">{r.subject}</a></Table.Cell>
              <Table.Cell class="text-muted-foreground tabular-nums">{portal.dateTime(r.received_at)}</Table.Cell>
              <Table.Cell><Badge variant={r.state === "promoted" ? "default" : r.state === "open" ? "outline" : "secondary"}>{label(r.state)}</Badge></Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    {/if}
  </Frame>
</PortalLayout>
