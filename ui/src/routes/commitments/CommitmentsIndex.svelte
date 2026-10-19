<script>
  // Commitments: promises with a date between you and the people you work
  // for. Open ones in two lanes, what they owe you (waiting on them) and
  // what you've promised them; resolved ones as one list. The table picks
  // rows to resolve or delete many at once.
  import NoMatches from "$lib/components/NoMatches.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import Trash from "@lucide/svelte/icons/trash-2";
  import X from "@lucide/svelte/icons/x";
  import IndexToolbar from "$lib/components/IndexToolbar.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import CommitmentActions from "./CommitmentActions.svelte";
  import { get } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, href, withQuery } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { humanize } from "$lib/format.js";
  import SortableHead from "$lib/components/SortableHead.svelte";
  import Pagination from "$lib/components/Pagination.svelte";
  import { pageQuery } from "$lib/paging.js";
  import { shortDate, bulk } from "../work/work.js";
  import { RESOLUTIONS, action, when, lanes, statusOf } from "./commitments.js";

  page.set(session.term("commitment", 2));
  const view = $derived(router.query.get("view") || session.me.index_view || "cards");
  let data = $state(null);
  let selected = $state([]);

  async function load() {
    const q = router.query;
    try {
      data = await get(href("/commitments", { state: q.get("state"), owner_kind: q.get("owner_kind"), sort: q.get("sort"), direction: q.get("direction"), ...pageQuery(view) }));
      selected = selected.filter((id) => data.commitments.some((c) => c.id === id));
    } catch (e) { toastError(e); }
  }
  $effect(() => { router.query; load(); });

  const list = $derived(data?.commitments || []);
  const words = $derived(session.term("commitment", 2).toLowerCase());
  const groups = $derived.by(() => {
    if (!data) return [];
    if (data.state !== "open") return [{ key: "resolved", title: null, items: list }];
    const [theirs, ours] = lanes(list);
    return [
      { key: "theirs", title: "Waiting on them", items: theirs, empty: "Nothing anyone has promised you is open." },
      { key: "ours", title: "You promised", items: ours, empty: "Nothing you’ve promised is open." },
    ];
  });
  const allPicked = $derived(list.length > 0 && selected.length === list.length);
  const pick = (id, on) => (selected = on ? [...selected, id] : selected.filter((x) => x !== id));
  const homeOf = (c) => (c.engagement ? `/engagements/${c.engagement.ref}?tab=commitments` : `/clients/${c.client.id}?tab=commitments`);

  async function resolveAll(resolution) {
    if (await bulk("POST", "/bulk/commitments/resolution", { ids: selected, resolution })) load();
  }
  async function deleteAll() {
    if (!(await confirm({ title: `Delete ${selected.length} ${words}?`, message: "Only open ones are deleted; resolved ones stay as history and are skipped.", confirmLabel: "Delete", destructive: true }))) return;
    if (await bulk("DELETE", "/bulk/commitments", { ids: selected })) { selected = []; load(); }
  }
</script>

<IndexToolbar filters={data ? [{ param: "state", current: data.state, title: "Show", options: data.filters }] : []} {view}
  search={`Search ${session.term("commitment", 2).toLowerCase()}…`} />

{#if !data}
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{#each [1, 2, 3] as i (i)}<Skeleton class="h-28" />{/each}</div>
{:else if !list.length && router.query.get("q")}
  <NoMatches />
{:else if !list.length}
  <EmptyState title={data.state === "open" ? `No open ${words}.` : "Nothing resolved yet."}>
    {#if !data.any_commitments}
      A {session.term("commitment").toLowerCase()} is a promise with a date between you and the people you work for: something they’ll send you, or something you’ve told them they’ll have. Add one from a {session.term("client").toLowerCase()} or {session.term("engagement").toLowerCase()} page, or triage a request into one.
    {/if}
  </EmptyState>
{:else if view === "table"}
  {#if selected.length}
    <div class="bg-muted/50 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm">
      <span class="font-medium tabular-nums">{selected.length} selected</span>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Resolve as<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
        <DropdownMenu.Content>{#each RESOLUTIONS as r (r)}<DropdownMenu.Item onSelect={() => resolveAll(r)}>{humanize(r)}</DropdownMenu.Item>{/each}</DropdownMenu.Content>
      </DropdownMenu.Root>
      {#if session.can("delete_records")}<Button variant="destructive" size="sm" onclick={deleteAll}><Trash />Delete</Button>{/if}
      <Button variant="ghost" size="sm" class="ms-auto" onclick={() => (selected = [])}><X />Clear</Button>
    </div>
  {/if}
  <Card.Root class="py-0">
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head class="w-8"><Checkbox checked={allPicked} indeterminate={selected.length > 0 && !allPicked} onCheckedChange={(v) => (selected = v ? list.map((c) => c.id) : [])} aria-label="Select all" /></Table.Head>
          {#each [["description", session.term("commitment")], ["due", "Due"], ["client", session.term("client")], ["status", "Status"]] as [key, label] (key)}
            <SortableHead {key} {label} />
          {/each}
          <Table.Head class="w-32"><span class="sr-only">Actions</span></Table.Head>
        </Table.Row>
      </Table.Header>
      {#each groups as g (g.key)}
        <Table.Body>
          {#if g.title}
            <Table.Row class="bg-muted/30 hover:bg-muted/30"><Table.Cell colspan={6} class="font-medium">{g.title} <span class="text-muted-foreground font-normal">{g.items.length}</span></Table.Cell></Table.Row>
          {/if}
          {#each g.items as c (c.id)}
            <Table.Row data-state={selected.includes(c.id) ? "selected" : undefined}>
              <Table.Cell><Checkbox checked={selected.includes(c.id)} onCheckedChange={(v) => pick(c.id, v)} aria-label={`Select ${c.description}`} /></Table.Cell>
              <Table.Cell class="max-w-96 truncate"><strong>{c.promiser}</strong> will {action(c)}</Table.Cell>
              <Table.Cell class="whitespace-nowrap">{shortDate(c.due_on)}</Table.Cell>
              <Table.Cell class="text-muted-foreground">
                <a class="hover:underline" href={`/clients/${c.client.id}`}>{c.client.display_name}</a>{#if c.engagement}{" · "}<a class="hover:underline" href={`/engagements/${c.engagement.ref}`}>{c.engagement.ref}</a>{/if}
              </Table.Cell>
              <Table.Cell><StatusBadge status={statusOf(c)} /></Table.Cell>
              <Table.Cell><CommitmentActions commitment={c} onchange={load} /></Table.Cell>
            </Table.Row>
          {:else}
            <Table.Row><Table.Cell colspan={6} class="text-muted-foreground">{g.empty}</Table.Cell></Table.Row>
          {/each}
        </Table.Body>
      {/each}
    </Table.Root>
  </Card.Root>
{:else}
  {#each groups as g (g.key)}
    <section class="flex flex-col gap-3">
      {#if g.title}<h2 class="text-sm font-medium">{g.title} <span class="text-muted-foreground font-normal">{g.items.length}</span></h2>{/if}
      {#if g.items.length}
        <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">
          {#each g.items as c (c.id)}
            <a href={homeOf(c)} class="group">
              <Card.Root class="group-hover:border-primary/40 h-full transition-colors">
                <Card.Header>
                  <Card.Description class="truncate">{[c.engagement?.ref, c.client.display_name].filter(Boolean).join(" · ")}</Card.Description>
                  <Card.Title class="line-clamp-2">{c.promiser} will {action(c)}</Card.Title>
                  <!-- An open one is what this list is; the badge says only what's different. -->
                  {#if statusOf(c) !== "open"}<Card.Action><StatusBadge status={statusOf(c)} /></Card.Action>{/if}
                </Card.Header>
                <Card.Footer class="text-muted-foreground mt-auto gap-3 text-sm">
                  <span>{when(c).replace(/^./, (ch) => ch.toUpperCase())}</span>
                  {#if c.source && !["app", "seed"].includes(c.source)}<span class="truncate">From {c.source === "agent" ? "an agent" : c.source}</span>{/if}
                </Card.Footer>
              </Card.Root>
            </a>
          {/each}
        </div>
      {:else}
        <p class="text-muted-foreground text-sm">{g.empty}</p>
      {/if}
    </section>
  {/each}
{/if}
{#if data}<Pagination pagination={data.pagination} />{/if}
