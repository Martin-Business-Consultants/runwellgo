<script>
  // An engagement's page: its ref and title with Edit, Close or Reopen, and
  // Erase; a line of facts under it and what happens next; then the
  // working column (tabs: the Agreement, what other areas add — work,
  // commitments, notes, documents, plugins' panels — and History) beside
  // the side column (the scope, with how much of each item is done, and
  // Details).
  import * as Card from "$lib/components/ui/card/index.js";
  import ConnectionsButton from "$lib/components/ConnectionsButton.svelte";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Check from "@lucide/svelte/icons/check";
  import CircleX from "@lucide/svelte/icons/circle-x";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import Trash from "@lucide/svelte/icons/trash-2";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import CustomFieldValues from "$lib/components/CustomFieldValues.svelte";
  import History from "$lib/components/History.svelte";
  import AgreementTab from "./AgreementTab.svelte";
  import EngagementScope from "./EngagementScope.svelte";
  import { get, post, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, withQuery, go } from "$lib/router.svelte.js";
  import { tabsFor } from "$lib/record-tabs.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import NotFound from "../NotFound.svelte";
  import { money, date } from "$lib/format.js";

  let { params } = $props();
  let e = $state(null);
  let closing = $state(null), reason = $state(""), erasing = $state(false), confirmRef = $state(""), busy = $state(false);

  // The Agreement tab (none for an internal project: no agreement), what
  // other areas add, then History.
  const tabs = $derived(e ? [
    ...(e.internal ? [] : [{ key: "agreement", label: () => "Agreement", count: (r) => r.agreement_versions.length, component: AgreementTab }]),
    ...tabsFor("Engagement"),
    { key: "history", label: () => "History", component: null },
  ] : []);
  // Work first once something's agreed (or it's internal), else the agreement.
  const fallback = $derived(e && (e.agreed_items.length || e.internal) && tabs.some((t) => t.key === "work") ? "work" : tabs[0]?.key);
  const tab = $derived(tabs.some((t) => t.key === router.query.get("tab")) ? router.query.get("tab") : fallback);

  async function load() {
    try {
      e = (await get(`/engagements/${params.ref}`)).engagement;
      page.set(`${e.ref} ${e.title}`, [{ label: session.term("engagement", 2), href: "/engagements" }], `Engagement:${e.id}`);
    } catch (err) { if (err.status === 404 && !e) missing = true; else toastError(err); }
  }
  let missing = $state(false);
  load();

  async function close(evt) {
    evt.preventDefault();
    busy = true;
    try { const r = await post(`/engagements/${e.ref}/close`, { outcome: closing, reason }); toast(r.summary); e = r.engagement; closing = null; reason = ""; } catch (err) { toastError(err); }
    busy = false;
  }
  async function reopen() {
    try { const r = await post(`/engagements/${e.ref}/reopen`); toast(r.summary); e = r.engagement; } catch (err) { toastError(err); }
  }
  async function erase(evt) {
    evt.preventDefault();
    busy = true;
    try { const r = await post(`/engagements/${e.ref}/erasure`, { confirm_ref: confirmRef }); toast(r.summary); go(`/clients/${e.client.id}`); } catch (err) { toastError(err); }
    busy = false;
  }
  async function remove() {
    try { await del(`/engagements/${e.ref}`); toast(`${e.ref} deleted.`); go("/engagements"); } catch (err) { toastError(err); }
  }
  const amount = $derived(e && !e.internal && session.me.prices && e.agreed_amount_cents ? money(e.agreed_amount_cents) : "");
</script>

{#snippet HistoryTab({ record })}
  <Card.Root><Card.Content><History type="Engagement" id={record.id} /></Card.Content></Card.Root>
{/snippet}

{#if missing}
  <NotFound title={`Couldn’t find that ${session.term("engagement").toLowerCase()}`} back={{ href: "/engagements", label: `All ${session.term("engagement", 2).toLowerCase()}` }} />
{:else if !e}
  <Skeleton class="h-64" />
{:else}
  <div class="flex flex-col gap-2">
    <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
      <h1 class="min-w-0 text-2xl font-semibold tracking-tight">{e.ref} {e.title}</h1>
      <div class="flex shrink-0 items-center gap-2">
        <ConnectionsButton record={`Engagement:${e.id}`} name={`${e.ref} ${e.title}`} />
        <Button href={`/engagements/${e.ref}/edit`} variant="outline" size="sm"><Pencil />Edit</Button>
        {#if session.can("close_engagements") && !e.closed}
          <Button variant="outline" size="sm" onclick={() => (closing = "completed")}><Check />Mark complete</Button>
        {:else if session.can("close_engagements")}
          <Button variant="outline" size="sm" onclick={reopen}><RefreshCw />Reopen</Button>
        {/if}
        {#if (session.can("close_engagements") && !e.closed) || (e.closed && session.can("erase_engagements")) || (e.deletable && session.can("delete_records"))}
          <DropdownMenu.Root>
            <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><Ellipsis /><span class="sr-only">More</span></Button>{/snippet}</DropdownMenu.Trigger>
            <DropdownMenu.Content align="end">
              {#if session.can("close_engagements") && !e.closed}<DropdownMenu.Item onSelect={() => (closing = "cancelled")}><CircleX />Cancel {e.ref}</DropdownMenu.Item>{/if}
              {#if e.deletable && session.can("delete_records")}<DropdownMenu.Item variant="destructive" onSelect={remove}><Trash />Delete</DropdownMenu.Item>{/if}
              {#if e.closed && session.can("erase_engagements")}<DropdownMenu.Item variant="destructive" onSelect={() => { confirmRef = ""; erasing = true; }}><Trash />Delete permanently</DropdownMenu.Item>{/if}
            </DropdownMenu.Content>
          </DropdownMenu.Root>
        {/if}
      </div>
    </div>
    <p class="text-muted-foreground flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
      <StatusBadge status={e.closed ? e.close_outcome || "closed" : e.state} label={e.state_label} />
      <a class="text-foreground underline-offset-4 hover:underline" href={`/clients/${e.client.id}`}>{e.client.display_name}</a>
      <span>{e.type_name} · {e.recurring ? "Recurring" : "Fixed scope"}</span>
      {#if amount}<span>Agreed <strong class="text-foreground">{amount}</strong>{e.recurring ? " / period" : ""}</span>{/if}
      {#if e.closed}<span>{e.state_label} {date(e.closed_at)}{e.close_reason ? `: ${e.close_reason}` : ""}</span>{/if}
    </p>
    {#if e.next_step}
      <p class="text-sm">{e.next_step}{#if e.next_step_link} <a class="text-primary underline-offset-4 hover:underline" href={e.next_step_link.href}>{e.next_step_link.label}</a>{/if}</p>
    {/if}
    {#if e.description_html}<div class="prose-admin text-sm">{@html e.description_html}</div>{/if}
    {#if e.estimate_notes_html}
      <div class="text-muted-foreground text-sm"><strong>Estimate notes (internal):</strong> <div class="prose-admin">{@html e.estimate_notes_html}</div></div>
    {/if}
  </div>

  <div class="grid gap-4 lg:grid-cols-3">
    <div class="min-w-0 lg:col-span-2">
      <Tabs.Root value={tab} onValueChange={(v) => go(withQuery({ tab: v, item: v === "work" ? router.query.get("item") : null }), undefined, { replace: true })}>
        <Tabs.List>
          {#each tabs as t (t.key)}
            {@const n = t.count?.(e)}
            <Tabs.Trigger value={t.key}>{t.label()}{#if n}<span class="text-muted-foreground ms-1 text-xs tabular-nums">{n}</span>{/if}</Tabs.Trigger>
          {/each}
        </Tabs.List>
        {#each tabs as t (t.key)}
          <Tabs.Content value={t.key} class="mt-2">
            {#if t.key === "history"}
              {@render HistoryTab({ record: e })}
            {:else}
              <t.component record={e} reload={load} />
            {/if}
          </Tabs.Content>
        {/each}
      </Tabs.Root>
    </div>

    <div class="flex flex-col gap-4">
      <EngagementScope engagement={e} current={tab === "work" ? router.query.get("item") || "all" : null} />
      <Card.Root>
        <Card.Header><Card.Title>Details</Card.Title></Card.Header>
        <Card.Content>
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt class="text-muted-foreground">Status</dt><dd><StatusBadge status={e.closed ? e.close_outcome || "closed" : e.state} label={e.state_label} /></dd>
            <dt class="text-muted-foreground">{session.term("client")}</dt><dd><a class="hover:underline" href={`/clients/${e.client.id}`}>{e.client.display_name}</a></dd>
            <dt class="text-muted-foreground">Type</dt><dd>{e.type_name} · {e.recurring ? "Recurring" : "Fixed scope"}</dd>
            {#if amount}<dt class="text-muted-foreground">Agreed</dt><dd><strong>{amount}</strong>{e.recurring ? " / period" : ""}</dd>{/if}
            <dt class="text-muted-foreground">Started</dt><dd>{date(e.created_at)}</dd>
            {#if e.closed}<dt class="text-muted-foreground">{e.state_label}</dt><dd>{date(e.closed_at)}{e.close_reason ? `: ${e.close_reason}` : ""}</dd>{/if}
            <CustomFieldValues model="Engagement" values={e.custom_fields} />
          </dl>
        </Card.Content>
      </Card.Root>
    </div>
  </div>

  <Dialog.Root open={!!closing} onOpenChange={(v) => { if (!v) closing = null; }}>
    <Dialog.Content class="sm:max-w-md">
      <form onsubmit={close} class="flex flex-col gap-4">
        <Dialog.Header>
          <Dialog.Title>{closing === "cancelled" ? `Cancel ${e.ref}?` : `Mark ${e.ref} complete?`}</Dialog.Title>
          <Dialog.Description>{closing === "cancelled" ? "Stopped before it was delivered." : "Delivered and done."} It leaves the open list; the agreement and its history stay, and it can be reopened.</Dialog.Description>
        </Dialog.Header>
        <Field.Field>
          <Field.Label for="close-reason" class="sr-only">A note</Field.Label>
          <Input id="close-reason" bind:value={reason} autocomplete="off" placeholder="A note (optional)" autofocus />
        </Field.Field>
        <Dialog.Footer>
          <Dialog.Close>{#snippet child({ props })}<Button {...props} variant="outline">Not now</Button>{/snippet}</Dialog.Close>
          <Button type="submit" variant={closing === "cancelled" ? "destructive" : "default"} disabled={busy}>{closing === "cancelled" ? "Cancel it" : "Mark complete"}</Button>
        </Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>

  <Dialog.Root bind:open={erasing}>
    <Dialog.Content class="sm:max-w-md">
      <form onsubmit={erase} class="flex flex-col gap-4">
        <Dialog.Header>
          <Dialog.Title>Delete {e.ref} for good?</Dialog.Title>
          <Dialog.Description>
            It removes the {session.term("engagement").toLowerCase()} with its agreements and {e.client.display_name}’s decisions on them, its {session.term("work").toLowerCase()},
            commitments, notes, documents and history. There’s no undo; the {session.term("client").toLowerCase()} keeps one note that it was deleted.
          </Dialog.Description>
        </Dialog.Header>
        <Field.Field>
          <Field.Label for="erase-ref">Type <strong>{e.ref}</strong> to confirm</Field.Label>
          <Input id="erase-ref" bind:value={confirmRef} autocomplete="off" required />
        </Field.Field>
        <Dialog.Footer>
          <Dialog.Close>{#snippet child({ props })}<Button {...props} variant="outline">Cancel</Button>{/snippet}</Dialog.Close>
          <Button type="submit" variant="destructive" disabled={busy || confirmRef.trim().toUpperCase() !== e.ref}>Delete {e.ref} permanently</Button>
        </Dialog.Footer>
      </form>
    </Dialog.Content>
  </Dialog.Root>
{/if}
