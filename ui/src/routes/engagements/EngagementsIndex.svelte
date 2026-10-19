<script>
  // The engagements list: filters by derived state and by type, then cards
  // or a table (the install's default view, or ?view=). The table selects
  // rows for the bulk actions: close as completed or cancelled, delete.
  import NoMatches from "$lib/components/NoMatches.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import EllipsisVertical from "@lucide/svelte/icons/ellipsis-vertical";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import Trash from "@lucide/svelte/icons/trash-2";
  import IndexToolbar from "$lib/components/IndexToolbar.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import { get, post, api, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, href, onQuery } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { customFields } from "$lib/custom-fields.svelte.js";
  import { money, date } from "$lib/format.js";
  import SortableHead from "$lib/components/SortableHead.svelte";
  import Pagination from "$lib/components/Pagination.svelte";
  import { pageQuery } from "$lib/paging.js";

  page.set(session.term("engagement", 2));
  let data = $state(null);
  let listed = $state([]);
  let selected = $state([]);
  const view = $derived(router.query.get("view") || session.me.index_view || "cards");
  const noun = $derived(session.term("engagement", 2).toLowerCase());

  async function load() {
    try {
      data = await get(href("/engagements", { state: router.query.get("state"), label: router.query.get("label"), ...pageQuery(view) }));
      selected = selected.filter((id) => data.engagements.some((e) => e.id === id));
    } catch (e) { toastError(e); }
  }
  onQuery(load);
  customFields("Engagement").then((f) => (listed = f.filter((x) => x.listed)));

  // engagement_amount: only with prices on and something agreed.
  const amount = (e) => (session.me.prices && e.agreed_amount_cents ? money(e.agreed_amount_cents) + (e.shape === "recurring" ? " / period" : "") : "");
  const fieldText = (f, v) => (v == null || v === "" ? "" : f.kind === "money" ? money(v) : f.kind === "date" ? date(v) : f.kind === "boolean" ? (v ? "Yes" : "No") : String(v));

  function toggle(id, on) { selected = on ? [...selected, id] : selected.filter((x) => x !== id); }
  const allSelected = $derived(!!data?.engagements.length && selected.length === data.engagements.length);

  // A bulk answer names what it skipped; a 422 means nothing applied.
  async function bulk(run) {
    try { const r = await run(); toast(r.summary); } catch (e) { toastError(e); }
    selected = [];
    load();
  }
  async function bulkClose(outcome) {
    if (!(await confirm({ title: `${outcome === "cancelled" ? "Cancel" : "Mark complete"}: ${selected.length} ${noun}?`, message: "Each leaves the open list; agreements and history stay, and each can be reopened. Ones already closed are skipped.", confirmLabel: outcome === "cancelled" ? "Cancel them" : "Mark complete" }))) return;
    bulk(() => post("/bulk/engagements/closure", { ids: selected, outcome }));
  }
  async function bulkDelete() {
    if (!(await confirm({ title: `Delete ${selected.length} ${noun}?`, message: "Only those never sent to the client are deleted; sent ones are skipped (close them instead).", confirmLabel: "Delete", destructive: true }))) return;
    bulk(() => api("DELETE", "/bulk/engagements", { ids: selected }));
  }
  async function remove(e) {
    if (!(await confirm({ title: `Delete ${e.ref}?`, message: "It was never sent, so its draft agreement, work and notes go with it.", confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/engagements/${e.ref}`); toast(`${e.ref} deleted.`); load(); } catch (err) { toastError(err); }
  }
</script>

<IndexToolbar tabs={data ? { param: "label", current: data.label, options: data.labels } : null}
  filters={data ? [{ param: "state", current: data.state, title: "Show", options: data.filters }] : []} {view}
  search={`Search ${session.term("engagement", 2).toLowerCase()}…`}
  newText={"New " + session.term("engagement").toLowerCase()} newHref="/engagements/new" />

{#if view === "table" && selected.length}
  <div class="bg-muted/50 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm">
    <span class="text-muted-foreground">{selected.length} selected</span>
    {#if session.can("close_engagements")}
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Close as<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
        <DropdownMenu.Content align="start">
          <DropdownMenu.Item onSelect={() => bulkClose("completed")}>Completed</DropdownMenu.Item>
          <DropdownMenu.Item onSelect={() => bulkClose("cancelled")}>Cancelled</DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
    {/if}
    {#if session.can("delete_records")}
      <Button variant="outline" size="sm" class="text-destructive" onclick={bulkDelete}><Trash />Delete</Button>
    {/if}
    <Button variant="ghost" size="sm" class="ms-auto" onclick={() => (selected = [])}>Clear</Button>
  </div>
{/if}

{#if !data}
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{#each [1, 2, 3] as i (i)}<Skeleton class="h-28" />{/each}</div>
{:else if !data.engagements.length && router.query.get("q")}
  <NoMatches />
{:else if !data.engagements.length}
  {#if data.filters.at(-1).count}
    <EmptyState icon="briefcase" title={`No ${noun} match.`}><a class="underline-offset-4 hover:underline" href={href("/engagements", { state: "all", view: router.query.get("view") })}>Show all</a></EmptyState>
  {:else}
    <EmptyState icon="briefcase" title={`No ${noun} yet.`}>
      {#snippet actions()}<Button href="/engagements/new" variant="outline">Start your first {session.term("engagement").toLowerCase()}</Button>{/snippet}
    </EmptyState>
  {/if}
{:else if view === "table"}
  <Card.Root class="py-0">
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head class="w-8"><Checkbox checked={allSelected} onCheckedChange={(v) => (selected = v ? data.engagements.map((e) => e.id) : [])} aria-label="Select all" /></Table.Head>
          <SortableHead key="ref" label="Ref" />
          <SortableHead key="title" label="Title" />
          <SortableHead key="client" label={session.term("client")} />
          <SortableHead key="type" label="Type" />
          {#if session.me.prices}<Table.Head class="text-end">Agreed</Table.Head>{/if}
          <Table.Head>State</Table.Head>
          {#each listed as f (f.id)}<Table.Head>{f.label}</Table.Head>{/each}
          <Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each data.engagements as e (e.id)}
          <Table.Row data-state={selected.includes(e.id) ? "selected" : undefined}>
            <Table.Cell><Checkbox checked={selected.includes(e.id)} onCheckedChange={(v) => toggle(e.id, v)} aria-label={`Select ${e.ref}`} /></Table.Cell>
            <Table.Cell class="font-semibold whitespace-nowrap"><a href={`/engagements/${e.ref}`} class="hover:underline">{e.ref}</a></Table.Cell>
            <Table.Cell><a href={`/engagements/${e.ref}`} class="hover:underline">{e.title}</a></Table.Cell>
            <Table.Cell><a href={`/clients/${e.client.id}`} class="text-muted-foreground hover:underline">{e.client.display_name}</a></Table.Cell>
            <Table.Cell class="text-muted-foreground">{e.type_name}</Table.Cell>
            {#if session.me.prices}<Table.Cell class="text-end tabular-nums">{amount(e)}</Table.Cell>{/if}
            <Table.Cell><StatusBadge status={e.closed ? e.close_outcome || "closed" : e.state} label={e.state_label} /></Table.Cell>
            {#each listed as f (f.id)}<Table.Cell class="text-muted-foreground">{fieldText(f, e.custom_fields[f.key])}</Table.Cell>{/each}
            <Table.Cell>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><EllipsisVertical /><span class="sr-only">Actions for {e.ref}</span></Button>{/snippet}</DropdownMenu.Trigger>
                <DropdownMenu.Content align="end">
                  <DropdownMenu.Item onSelect={() => router.navigate(`/engagements/${e.ref}/edit`)}>Edit</DropdownMenu.Item>
                  {#if e.deletable && session.can("delete_records")}<DropdownMenu.Item variant="destructive" onSelect={() => remove(e)}>Delete</DropdownMenu.Item>{/if}
                </DropdownMenu.Content>
              </DropdownMenu.Root>
            </Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Root>
{:else}
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
    {#each data.engagements as e (e.id)}
      <a href={`/engagements/${e.ref}`} class="group">
        <Card.Root class="group-hover:border-primary/40 h-full transition-colors">
          <Card.Header>
            <Card.Description><span class="text-foreground font-semibold">{e.ref}</span> · {e.client.display_name}</Card.Description>
            <Card.Title class="truncate">{e.title}</Card.Title>
            <Card.Action><StatusBadge status={e.closed ? e.close_outcome || "closed" : e.state} label={e.state_label} /></Card.Action>
          </Card.Header>
          <Card.Footer class="text-muted-foreground mt-auto gap-3 text-sm">
            <span>{e.type_name}</span>
            {#if amount(e)}<span class="tabular-nums">{amount(e)}</span>{/if}
          </Card.Footer>
        </Card.Root>
      </a>
    {/each}
  </div>
{/if}
{#if data}<Pagination pagination={data.pagination} />{/if}
