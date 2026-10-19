<script>
  // Work across engagements: open work (or all), by owner and engagement,
  // as cards, a table (pick rows to change many at once) or the board.
  // Work is never added here: it lives on engagements.
  import NoMatches from "$lib/components/NoMatches.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import EllipsisVertical from "@lucide/svelte/icons/ellipsis-vertical";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import Trash from "@lucide/svelte/icons/trash-2";
  import X from "@lucide/svelte/icons/x";
  import IndexToolbar from "$lib/components/IndexToolbar.svelte";
  import QuickFilter from "$lib/components/QuickFilter.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import StatusPicker from "./StatusPicker.svelte";
  import OwnerPicker from "./OwnerPicker.svelte";
  import WorkBoard from "./WorkBoard.svelte";
  import { get, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, href, withQuery } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { customFields } from "$lib/custom-fields.svelte.js";
  import { humanize, money, date } from "$lib/format.js";
  import SortableHead from "$lib/components/SortableHead.svelte";
  import Pagination from "$lib/components/Pagination.svelte";
  import { pageQuery } from "$lib/paging.js";
  import { STATUSES, shortDate, ownersOf, copyBrief, bulk } from "./work.js";

  page.set(session.term("work"));
  const views = ["cards", "table", "board"];
  const view = $derived(views.includes(router.query.get("view")) ? router.query.get("view") : views.includes(session.me.index_view) ? session.me.index_view : "cards");
  const owner = $derived(router.query.get("owner") || "any");
  const engagement = $derived(router.query.get("engagement") || "all");
  let data = $state(null);
  let selected = $state([]);
  let listed = $state([]);
  let due = $state("");
  customFields("Todo").then((f) => (listed = f.filter((x) => x.listed)));

  async function load() {
    const q = router.query;
    try {
      data = await get(href("/work", { status: q.get("status"), owner: q.get("owner"), engagement: q.get("engagement"), sort: q.get("sort"), direction: q.get("direction"), ...pageQuery(view) }));
      selected = selected.filter((id) => data.work.some((t) => t.id === id));
    } catch (e) { toastError(e); }
  }
  $effect(() => { router.query; load(); });

  const work = $derived(data?.work || []);
  const words = $derived(session.term("work").toLowerCase());
  const replace = (t) => { const i = data.work.findIndex((x) => x.id === t.id); if (i >= 0) data.work[i] = t; };

  const statusOptions = [{ value: "", label: "Open work" }, { value: "all", label: "All work" }];
  const ownerOptions = $derived([{ value: "", label: "Anyone" }, ...(session.me.team || []).map((p) => ({ value: String(p.id), label: p.name }))]);
  const engagementOptions = $derived([{ value: "", label: "All " + session.term("engagement", 2).toLowerCase() }, ...(data?.engagements || []).map((e) => ({ value: e.ref, label: `${e.ref} ${e.title}` }))]);

  const allPicked = $derived(work.length > 0 && selected.length === work.length);
  const pick = (id, on) => (selected = on ? [...selected, id] : selected.filter((x) => x !== id));

  async function apply(method, path, body) {
    if (await bulk(method, path, { ids: selected, ...body })) { due = ""; load(); }
  }
  async function bulkDelete() {
    const n = selected.length;
    if (!(await confirm({ title: `Remove ${n} ${words}?`, message: `They come off their ${session.term("engagement", 2).toLowerCase()} and the client’s portal.`, confirmLabel: "Remove", destructive: true }))) return;
    if (await bulk("DELETE", "/bulk/work", { ids: selected })) { selected = []; load(); }
  }
  async function remove(t) {
    if (!(await confirm({ title: `Remove ${t.title}?`, message: "It comes off the engagement and the client’s portal. Its history stays.", confirmLabel: "Remove", destructive: true }))) return;
    try { await del(`/work/${t.id}`); toast("Removed."); load(); } catch (e) { toastError(e); }
  }
  const showValue = (f, v) => (v == null || v === "" ? "" : f.kind === "money" ? money(v) : f.kind === "date" ? date(v) : f.kind === "boolean" ? (v ? "Yes" : "No") : String(v));
</script>

<IndexToolbar {view} {views} search={view === "board" ? "" : `Search ${session.term("work", 2).toLowerCase()}…`}>
  {#if view !== "board"}<QuickFilter title="Show…" param="status" options={statusOptions} />{/if}
  <QuickFilter title="Owner…" param="owner" options={ownerOptions} />
  <QuickFilter title={session.term("engagement") + "…"} param="engagement" options={engagementOptions} />
</IndexToolbar>

{#if view === "board"}
  <WorkBoard {owner} {engagement} />
{:else if !data}
  <div class="grid gap-4 sm:grid-cols-2 xl:grid-cols-3">{#each [1, 2, 3] as i (i)}<Skeleton class="h-28" />{/each}</div>
{:else if !work.length && router.query.get("q")}
  <NoMatches />
{:else if !work.length}
  {#if data.any_work}
    <EmptyState icon="check-square" title={`No ${words} matches these filters.`} />
  {:else}
    <EmptyState icon="check-square" title={`No ${words} yet.`}>
      It lives on {session.term("engagement", 2).toLowerCase()}: approving an agreement adds one per {session.term("scope_item").toLowerCase()}, and you can add more there.
      <a class="underline-offset-4 hover:underline" href="/engagements?state=open">Go to {session.term("engagement", 2).toLowerCase()}</a>
    </EmptyState>
  {/if}
{:else if view === "table"}
  {#if selected.length}
    <div class="bg-muted/50 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm">
      <span class="font-medium tabular-nums">{selected.length} selected</span>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Status<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
        <DropdownMenu.Content>{#each STATUSES as s (s)}<DropdownMenu.Item onSelect={() => apply("PATCH", "/bulk/work", { todo: { status: s } })}>{humanize(s)}</DropdownMenu.Item>{/each}</DropdownMenu.Content>
      </DropdownMenu.Root>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Owner<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
        <DropdownMenu.Content class="max-h-72">
          <DropdownMenu.Item onSelect={() => apply("PATCH", "/bulk/work", { todo: { owner_id: "none" } })}>Unassigned</DropdownMenu.Item>
          {#each session.me.team || [] as p (p.id)}<DropdownMenu.Item onSelect={() => apply("PATCH", "/bulk/work", { todo: { owner_id: p.id } })}><PersonAvatar person={p} size="size-5" />{p.name}</DropdownMenu.Item>{/each}
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Visibility<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
        <DropdownMenu.Content>
          <DropdownMenu.Item onSelect={() => apply("PATCH", "/bulk/work", { todo: { client_visible: true } })}>Shown to the {session.term("client").toLowerCase()}</DropdownMenu.Item>
          <DropdownMenu.Item onSelect={() => apply("PATCH", "/bulk/work", { todo: { client_visible: false } })}>Internal</DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      <div class="w-40"><DatePicker bind:value={due} placeholder="Due date" class="h-8" onchange={(v) => apply("PATCH", "/bulk/work", { todo: { due_on: v } })} /></div>
      <DropdownMenu.Root>
        <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Make {session.term("commitment", 2).toLowerCase()}<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
        <DropdownMenu.Content>
          <DropdownMenu.Item onSelect={() => apply("POST", "/bulk/work/commitments", { owner_kind: "us" })}>We promised</DropdownMenu.Item>
          <DropdownMenu.Item onSelect={() => apply("POST", "/bulk/work/commitments", { owner_kind: "client" })}>They promised</DropdownMenu.Item>
        </DropdownMenu.Content>
      </DropdownMenu.Root>
      {#if session.can("delete_records")}<Button variant="destructive" size="sm" onclick={bulkDelete}><Trash />Delete</Button>{/if}
      <Button variant="ghost" size="sm" class="ms-auto" onclick={() => (selected = [])}><X />Clear</Button>
    </div>
  {/if}
  <Card.Root class="py-0">
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head class="w-8"><Checkbox checked={allPicked} indeterminate={selected.length > 0 && !allPicked} onCheckedChange={(v) => (selected = v ? work.map((t) => t.id) : [])} aria-label="Select all" /></Table.Head>
          {#each [["title", session.term("work")], ["engagement", session.term("engagement")], ["client", session.term("client")], ["owner", "Owner"], ["due", "Due"], ["status", "Status"]] as [key, label] (key)}
            <SortableHead {key} {label} />
          {/each}
          {#each listed as f (f.id)}<Table.Head>{f.label}</Table.Head>{/each}
          <Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each work as t (t.id)}
          <Table.Row data-state={selected.includes(t.id) ? "selected" : undefined}>
            <Table.Cell><Checkbox checked={selected.includes(t.id)} onCheckedChange={(v) => pick(t.id, v)} aria-label={`Select ${t.title}`} /></Table.Cell>
            <Table.Cell class="max-w-72 truncate font-medium"><a href={`/work/${t.id}`} class="hover:underline">{t.title}</a></Table.Cell>
            <Table.Cell class="whitespace-nowrap"><a href={`/engagements/${t.engagement.ref}`} class="hover:underline">{t.engagement.ref}</a></Table.Cell>
            <Table.Cell class="text-muted-foreground max-w-48 truncate">{t.client?.display_name}</Table.Cell>
            <Table.Cell><OwnerPicker todo={t} onchange={replace} /></Table.Cell>
            <Table.Cell class="whitespace-nowrap">{shortDate(t.due_on)} {#if t.overdue}<Badge variant="destructive">Overdue</Badge>{/if}</Table.Cell>
            <Table.Cell><StatusPicker todo={t} onchange={replace} /></Table.Cell>
            {#each listed as f (f.id)}<Table.Cell class="text-muted-foreground">{showValue(f, t.custom_fields?.[f.key])}</Table.Cell>{/each}
            <Table.Cell>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><EllipsisVertical /><span class="sr-only">Actions for {t.title}</span></Button>{/snippet}</DropdownMenu.Trigger>
                <DropdownMenu.Content align="end">
                  <DropdownMenu.Item onSelect={() => copyBrief(t)}><Sparkles />Copy for AI</DropdownMenu.Item>
                  <DropdownMenu.Item onSelect={() => router.navigate(`/work/${t.id}/edit`)}>Edit</DropdownMenu.Item>
                  {#if session.can("delete_records")}<DropdownMenu.Item variant="destructive" onSelect={() => remove(t)}>Remove</DropdownMenu.Item>{/if}
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
    {#each work as t (t.id)}
      {@const owners = ownersOf(t)}
      <a href={`/work/${t.id}`} class="group">
        <Card.Root class="group-hover:border-primary/40 h-full transition-colors">
          <Card.Header>
            <Card.Description class="truncate">{t.engagement.ref} · {t.client?.display_name}</Card.Description>
            <Card.Title class="line-clamp-2">{t.title}</Card.Title>
            <Card.Action><StatusBadge status={t.overdue ? "overdue" : t.status} /></Card.Action>
          </Card.Header>
          <Card.Footer class="text-muted-foreground mt-auto gap-3 text-sm">
            {#if owners.length}
              <span class="flex -space-x-1.5">{#each owners.slice(0, 3) as p (p.id)}<span class="ring-card rounded-full ring-2"><PersonAvatar person={p} /></span>{/each}</span>
            {:else}<span>Unassigned</span>{/if}
            {#if t.due_on}<span>Due {shortDate(t.due_on)}</span>{/if}
          </Card.Footer>
        </Card.Root>
      </a>
    {/each}
  </div>
{/if}
{#if data && view !== "board"}<Pagination pagination={data.pagination} />{/if}
