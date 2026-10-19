<script>
  // Requests: what came in. Status and source filters, then cards or a
  // table (the install's default view, or ?view=); the table sorts by its
  // headers and dismisses or deletes the ticked rows at once.
  import NoMatches from "$lib/components/NoMatches.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import EllipsisVertical from "@lucide/svelte/icons/ellipsis-vertical";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import ArrowDown from "@lucide/svelte/icons/arrow-down";
  import IndexToolbar from "$lib/components/IndexToolbar.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import { get, post, api, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { router, href, withQuery, onQuery } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { timeAgo, capitalize } from "$lib/format.js";
  import Pagination from "$lib/components/Pagination.svelte";
  import { pageQuery } from "$lib/paging.js";

  page.set("Requests");
  let data = $state(null);
  let selected = $state([]);
  let dismissing = $state(false), reason = $state(""), busy = $state(false);
  const view = $derived(router.query.get("view") || session.me.index_view || "cards");
  const sort = $derived(router.query.get("sort"));
  const direction = $derived(router.query.get("direction") || "asc");
  const noClient = $derived("No " + session.term("client").toLowerCase() + " matched");

  async function load() {
    try {
      data = await get(href("/requests", { status: router.query.get("status"), source: router.query.get("source"), sort, direction: router.query.get("direction"), ...pageQuery(view) }));
      selected = selected.filter((id) => data.requests.some((r) => r.id === id));
    } catch (e) { toastError(e); }
  }
  onQuery(load);

  const sources = $derived(data ? [{ value: "all", label: "All sources" }, ...data.sources.map((s) => ({ value: s, label: s.charAt(0).toUpperCase() + s.slice(1) }))] : []);
  const allTicked = $derived(!!data?.requests.length && selected.length === data.requests.length);
  const toggle = (id, on) => (selected = on ? [...selected, id] : selected.filter((x) => x !== id));
  const sortHref = (key) => withQuery({ sort: key, direction: sort === key && direction === "asc" ? "desc" : "asc" });

  async function bulkDismiss(e) {
    e.preventDefault();
    busy = true;
    try {
      const res = await post("/bulk/requests/dismissal", { ids: selected, reason });
      toast(res.summary); dismissing = false; reason = ""; selected = []; nav.refresh(); load();
    } catch (err) { toastError(err); }
    busy = false;
  }
  async function bulkDelete() {
    if (!(await confirm({ title: `Delete ${selected.length} ${selected.length === 1 ? "request" : "requests"}?`, message: "Only open ones are deleted; triaged ones stay as history and are skipped.", confirmLabel: "Delete", destructive: true }))) return;
    try {
      const res = await api("DELETE", "/bulk/requests", { ids: selected });
      toast(res.summary); selected = []; nav.refresh(); load();
    } catch (err) { toastError(err); }
  }
  async function remove(r) {
    if (!(await confirm({ title: `Delete “${r.subject}”?`, message: "The request goes for good.", confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/requests/${r.id}`); toast("Request deleted."); nav.refresh(); load(); } catch (e) { toastError(e); }
  }
</script>

{#snippet sortable(label, key)}
  <a href={sortHref(key)} class="hover:text-foreground inline-flex items-center gap-1">
    {label}
    {#if sort === key}{#if direction === "desc"}<ArrowDown class="size-3.5" />{:else}<ArrowUp class="size-3.5" />{/if}{/if}
  </a>
{/snippet}

<IndexToolbar tabs={data ? { param: "source", current: data.source, options: sources } : null}
  filters={data ? [{ param: "status", current: data.status, title: "Show", options: data.filters }] : []}
  {view} search="Search requests…" newText="Log a request" newHref="/requests/new" />

{#if view === "table" && selected.length}
  <div class="bg-muted/50 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm">
    <span class="text-muted-foreground me-auto">{selected.length} selected</span>
    {#if session.can("triage_requests")}<Button size="sm" variant="outline" onclick={() => (dismissing = true)}>Dismiss</Button>{/if}
    {#if session.can("delete_records")}<Button size="sm" variant="destructive" onclick={bulkDelete}>Delete</Button>{/if}
    <Button size="sm" variant="ghost" onclick={() => (selected = [])}>Clear</Button>
  </div>
{/if}

{#if !data}
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{#each [1, 2, 3] as i (i)}<Skeleton class="h-28" />{/each}</div>
{:else if !data.requests.length && router.query.get("q")}
  <NoMatches />
{:else if !data.requests.length}
  <EmptyState icon="inbox" title="No requests match.">
    {#if data.status !== "all"}<a class="underline-offset-4 hover:underline" href={href("/requests", { status: "all", view: router.query.get("view") })}>Show all</a>{/if}
  </EmptyState>
{:else if view === "table"}
  <Card.Root class="py-0">
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head class="w-10"><Checkbox checked={allTicked} onCheckedChange={(v) => (selected = v ? data.requests.map((r) => r.id) : [])} aria-label="Select all" /></Table.Head>
          <Table.Head>{@render sortable("Request", "subject")}</Table.Head>
          <Table.Head>{@render sortable("From", "from")}</Table.Head>
          <Table.Head>{@render sortable(session.term("client"), "client")}</Table.Head>
          <Table.Head>{@render sortable("Source", "source")}</Table.Head>
          <Table.Head>{@render sortable("Received", "received")}</Table.Head>
          <Table.Head>{@render sortable("Status", "status")}</Table.Head>
          <Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each data.requests as r (r.id)}
          <Table.Row data-state={selected.includes(r.id) ? "selected" : undefined}>
            <Table.Cell><Checkbox checked={selected.includes(r.id)} onCheckedChange={(v) => toggle(r.id, v)} aria-label={`Select ${r.subject}`} /></Table.Cell>
            <Table.Cell class="max-w-80 truncate font-medium"><a href={`/requests/${r.id}`} class="hover:underline">{r.subject}</a></Table.Cell>
            <Table.Cell class="text-muted-foreground">{r.from || "Unknown sender"}</Table.Cell>
            <Table.Cell class="text-muted-foreground">{r.client?.display_name || ""}</Table.Cell>
            <Table.Cell class="text-muted-foreground">{capitalize(r.source)}</Table.Cell>
            <Table.Cell class="text-muted-foreground whitespace-nowrap">{timeAgo(r.received_at)}</Table.Cell>
            <Table.Cell><StatusBadge status={r.status} /></Table.Cell>
            <Table.Cell>
              {#if r.status === "open"}
                <DropdownMenu.Root>
                  <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><EllipsisVertical /><span class="sr-only">Actions for {r.subject}</span></Button>{/snippet}</DropdownMenu.Trigger>
                  <DropdownMenu.Content align="end">
                    <DropdownMenu.Item onSelect={() => router.navigate(`/requests/${r.id}/edit`)}>Edit</DropdownMenu.Item>
                    {#if session.can("delete_records")}<DropdownMenu.Item variant="destructive" onSelect={() => remove(r)}>Delete</DropdownMenu.Item>{/if}
                  </DropdownMenu.Content>
                </DropdownMenu.Root>
              {/if}
            </Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Root>
{:else}
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
    {#each data.requests as r (r.id)}
      <a href={`/requests/${r.id}`} class="group">
        <Card.Root class={["group-hover:border-primary/40 h-full transition-colors", r.status === "open" && "border-primary/30"]}>
          <Card.Header>
            <Card.Description>{capitalize(r.source)}</Card.Description>
            <Card.Title class="line-clamp-2">{r.subject}</Card.Title>
            <Card.Action><StatusBadge status={r.status} /></Card.Action>
          </Card.Header>
          <Card.Content class="text-sm">{r.client?.display_name || noClient}</Card.Content>
          <Card.Footer class="text-muted-foreground mt-auto gap-3 text-sm">
            <span class="truncate">{r.from || "Unknown sender"}</span>
            <span class="ms-auto whitespace-nowrap">{timeAgo(r.received_at)}</span>
          </Card.Footer>
        </Card.Root>
      </a>
    {/each}
  </div>
{/if}

<Dialog.Root bind:open={dismissing}>
  <Dialog.Content class="sm:max-w-md">
    <form onsubmit={bulkDismiss} class="flex flex-col gap-4">
      <Dialog.Header>
        <Dialog.Title>Dismiss {selected.length} {selected.length === 1 ? "request" : "requests"}</Dialog.Title>
        <Dialog.Description>Requests already triaged are skipped.</Dialog.Description>
      </Dialog.Header>
      <Field.Field>
        <Field.Label for="bulk-dismiss-reason">Reason</Field.Label>
        <Input id="bulk-dismiss-reason" bind:value={reason} placeholder="Not needed" autocomplete="off" />
      </Field.Field>
      <Dialog.Footer>
        <Button type="button" variant="ghost" onclick={() => (dismissing = false)}>Cancel</Button>
        <Button type="submit" variant="destructive" disabled={busy}>Dismiss</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
{#if data}<Pagination pagination={data.pagination} />{/if}
