<script>
  // The clients list: a status filter, then cards or a table (the
  // install's default view, or ?view=). Each row's status changes in place.
  import NoMatches from "$lib/components/NoMatches.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import Trash from "@lucide/svelte/icons/trash-2";
  import X from "@lucide/svelte/icons/x";
  import EllipsisVertical from "@lucide/svelte/icons/ellipsis-vertical";
  import IndexToolbar from "$lib/components/IndexToolbar.svelte";
  import SortableHead from "$lib/components/SortableHead.svelte";
  import Pagination from "$lib/components/Pagination.svelte";
  import { pageQuery } from "$lib/paging.js";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import { api, get, patch, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, href, onQuery } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { plural } from "$lib/format.js";

  page.set(session.term("client", 2));
  const statuses = ["active", "dormant", "former"];
  let data = $state(null);
  const view = $derived(router.query.get("view") || session.me.index_view || "cards");

  async function load() {
    try { data = await get(href("/clients", { status: router.query.get("status"), ...pageQuery(view) })); } catch (e) { toastError(e); }
  }
  // A new filter, search or page reads the list again and drops what was picked.
  onQuery(() => { selected = []; load(); });

  // Rows picked in the table, for the bulk bar.
  let selected = $state([]);
  const allPicked = $derived(!!data && data.clients.length > 0 && selected.length === data.clients.length);
  const pick = (id, on) => (selected = on ? [...selected, id] : selected.filter((x) => x !== id));
  async function bulk(method, body) {
    try { toast((await api(method, "/bulk/clients", { ids: selected, ...body })).summary); selected = []; load(); } catch (e) { toastError(e); }
  }
  async function bulkDelete() {
    if (!(await confirm({ title: `Delete ${selected.length} ${session.term("client", selected.length).toLowerCase()}?`, message: "Any with engagements can’t be deleted, and are skipped and named.", confirmLabel: "Delete", destructive: true }))) return;
    bulk("DELETE", {});
  }

  async function setStatus(c, status) {
    try { await patch(`/clients/${c.id}`, { client: { status } }); toast(`${c.name} is ${status}.`); load(); } catch (e) { toastError(e); }
  }
  async function remove(c) {
    if (!(await confirm({ title: `Delete ${c.name}?`, message: `This removes its contacts, commitments and notes. A ${session.term("client").toLowerCase()} with ${session.term("engagement", 2).toLowerCase()} can’t be deleted.`, confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/clients/${c.id}`); toast(`Deleted ${c.name}.`); load(); } catch (e) { toastError(e); }
  }
</script>

<IndexToolbar filters={data ? [{ param: "status", current: data.status, title: "Show", options: data.filters }] : []} {view}
  search={`Search ${session.term("client", 2).toLowerCase()}…`}
  newText={"New " + session.term("client").toLowerCase()} newHref="/clients/new" />

{#if !data}
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{#each [1, 2, 3] as i (i)}<Skeleton class="h-28" />{/each}</div>
{:else if !data.clients.length && router.query.get("q")}
  <NoMatches />
{:else if !data.clients.length}
  {#if data.filters.at(-1).count}
    <EmptyState title={`No ${data.status} ${session.term("client", 2).toLowerCase()}.`}><a class="underline-offset-4 hover:underline" href={href("/clients", { status: "all", view: router.query.get("view") })}>Show all</a></EmptyState>
  {:else}
    <EmptyState title={`No ${session.term("client", 2).toLowerCase()} yet.`}>
      {#snippet actions()}<Button href="/clients/new" variant="outline">Add your first {session.term("client").toLowerCase()}</Button>{/snippet}
    </EmptyState>
  {/if}
{:else if view === "table"}
  {#if selected.length}
    <div class="bg-muted/50 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm">
      <span class="font-medium tabular-nums">{selected.length} selected</span>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Status</Button>{/snippet}</DropdownMenu.Trigger>
        <DropdownMenu.Content>{#each statuses as s (s)}<DropdownMenu.Item class="capitalize" onSelect={() => bulk("PATCH", { client: { status: s } })}>{s}</DropdownMenu.Item>{/each}</DropdownMenu.Content>
      </DropdownMenu.Root>
      {#if session.can("delete_records")}<Button variant="destructive" size="sm" onclick={bulkDelete}><Trash />Delete</Button>{/if}
      <Button variant="ghost" size="sm" class="ms-auto" onclick={() => (selected = [])}><X />Clear</Button>
    </div>
  {/if}
  <Card.Root class="py-0">
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head class="w-8"><Checkbox checked={allPicked} indeterminate={selected.length > 0 && !allPicked} onCheckedChange={(v) => (selected = v ? data.clients.map((c) => c.id) : [])} aria-label="Select all" /></Table.Head>
          <SortableHead key="name" label="Name" />
          <SortableHead key="status" label="Status" />
          <Table.Head class="text-end">Contacts</Table.Head>
          <Table.Head class="text-end">Open {session.term("engagement", 2).toLowerCase()}</Table.Head>
          <Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each data.clients as c (c.id)}
          <Table.Row data-state={selected.includes(c.id) ? "selected" : undefined}>
            <Table.Cell><Checkbox checked={selected.includes(c.id)} onCheckedChange={(v) => pick(c.id, v)} aria-label={`Select ${c.name}`} /></Table.Cell>
            <Table.Cell class="font-medium"><a href={`/clients/${c.id}`} class="hover:underline">{c.name}</a></Table.Cell>
            <Table.Cell>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger>{#snippet child({ props })}<button {...props}><StatusBadge status={c.status} /></button>{/snippet}</DropdownMenu.Trigger>
                <DropdownMenu.Content align="start">
                  <DropdownMenu.RadioGroup value={c.status} onValueChange={(v) => setStatus(c, v)}>
                    {#each statuses as s (s)}<DropdownMenu.RadioItem value={s} class="capitalize">{s}</DropdownMenu.RadioItem>{/each}
                  </DropdownMenu.RadioGroup>
                </DropdownMenu.Content>
              </DropdownMenu.Root>
            </Table.Cell>
            <Table.Cell class="text-end tabular-nums">{c.contacts_count}</Table.Cell>
            <Table.Cell class="text-end tabular-nums">{c.open_engagements_count}</Table.Cell>
            <Table.Cell>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><EllipsisVertical /><span class="sr-only">Actions for {c.name}</span></Button>{/snippet}</DropdownMenu.Trigger>
                <DropdownMenu.Content align="end">
                  <DropdownMenu.Item onSelect={() => router.navigate(`/clients/${c.id}/edit`)}>Edit</DropdownMenu.Item>
                  {#if session.can("delete_records")}<DropdownMenu.Item variant="destructive" onSelect={() => remove(c)}>Delete</DropdownMenu.Item>{/if}
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
    {#each data.clients as c (c.id)}
      <a href={`/clients/${c.id}`} class="group">
        <Card.Root class="group-hover:border-primary/40 h-full transition-colors">
          <Card.Header>
            {#if c.internal}<Card.Description>This is us</Card.Description>{/if}
            <Card.Title class="truncate">{c.name}</Card.Title>
            <Card.Action><StatusBadge status={c.status} /></Card.Action>
          </Card.Header>
          <Card.Footer class="text-muted-foreground mt-auto gap-3 text-sm">
            <span>{plural(c.contacts_count, "contact")}</span>
            <span>{c.open_engagements_count} open {(c.open_engagements_count === 1 ? session.term("engagement") : session.term("engagement", 2)).toLowerCase()}</span>
          </Card.Footer>
        </Card.Root>
      </a>
    {/each}
  </div>
{/if}
{#if data}<Pagination pagination={data.pagination} />{/if}
