<script>
  // A request's page: what was asked in the working column (its details,
  // then the tabs other areas add: notes, documents), and in the side column
  // the triage panel (or what it became), its details and history.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Trash from "@lucide/svelte/icons/trash-2";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import RichText from "$lib/components/RichText.svelte";
  import History from "$lib/components/History.svelte";
  import RequestTriage from "./RequestTriage.svelte";
  import { get, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { router, withQuery, go } from "$lib/router.svelte.js";
  import { tabsFor } from "$lib/record-tabs.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import NotFound from "../NotFound.svelte";
  import { confirm } from "$lib/confirm.svelte.js";
  import { dateTime, capitalize } from "$lib/format.js";
  import PluginSlot from "$lib/components/PluginSlot.svelte";

  let { params } = $props();
  let request = $state(null);
  const tabs = tabsFor("Request");
  const tab = $derived(tabs.some((t) => t.key === router.query.get("tab")) ? router.query.get("tab") : tabs[0]?.key);
  const path = (url) => (url ? new URL(url).pathname : null);

  async function load() {
    try {
      request = (await get(`/requests/${params.id}`)).request;
      page.set(request.subject, [{ label: "Requests", href: "/requests" }], `Request:${request.id}`);
    } catch (e) { if (e.status === 404 && !request) missing = true; else toastError(e); }
  }
  let missing = $state(false);
  load();

  async function remove() {
    if (!(await confirm({ title: `Delete “${request.subject}”?`, message: "The request goes for good, with its notes and documents.", confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/requests/${request.id}`); toast("Request deleted."); nav.refresh(); go("/requests"); } catch (e) { toastError(e); }
  }
</script>

{#if missing}
  <NotFound title="Couldn’t find that request" back={{ href: "/requests", label: "All requests" }} />
{:else if !request}
  <Skeleton class="h-64" />
{:else}
  <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
    <div class="min-w-0">
      <h1 class="text-2xl font-semibold tracking-tight">{request.subject}</h1>
      <p class="text-muted-foreground text-sm">{request.from || "Unknown sender"} · {request.source}</p>
    </div>
    {#if request.status === "open"}
      <Button href={`/requests/${request.id}/edit`} variant="outline" size="sm"><Pencil />Edit</Button>
    {/if}
  </div>

  <div class="grid gap-4 lg:grid-cols-3">
    <div class="flex min-w-0 flex-col gap-4 lg:col-span-2">
      <Card.Root>
        <Card.Content>
          {#if request.body_html}
            <RichText html={request.body_html} />
          {:else}
            <p class="text-muted-foreground text-sm">No details.</p>
          {/if}
        </Card.Content>
      </Card.Root>
      {#if tabs.length}
        <Tabs.Root value={tab} onValueChange={(v) => go(withQuery({ tab: v }), undefined, { replace: true })}>
          <Tabs.List>
            {#each tabs as t (t.key)}
              {@const n = t.count?.(request)}
              <Tabs.Trigger value={t.key}>{t.label()}{#if n}<span class="text-muted-foreground ms-1 text-xs tabular-nums">{n}</span>{/if}</Tabs.Trigger>
            {/each}
          </Tabs.List>
          {#each tabs as t (t.key)}
            <Tabs.Content value={t.key} class="mt-2"><t.component record={request} reload={load} /></Tabs.Content>
          {/each}
        </Tabs.Root>
      {/if}
    </div>

    <div class="flex flex-col gap-4">
      {#if request.status === "open"}
        {#if session.can("triage_requests")}
          <RequestTriage {request} />
        {:else}
          <Card.Root>
            <Card.Header><Card.Title>Triage</Card.Title></Card.Header>
            <Card.Content class="text-muted-foreground text-sm">A manager or owner triages requests: promoting this to work or dismissing it.</Card.Content>
          </Card.Root>
        {/if}
      {:else}
        <Card.Root>
          <Card.Header><Card.Title>{request.status === "promoted" ? "Became" : "Dismissed"}</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-1 text-sm">
            {#if request.status === "promoted" && request.promoted_label}
              <a href={request.promoted_path} class="font-medium underline-offset-4 hover:underline">{request.promoted_label}</a>
            {:else}
              <p>{request.dismissed_reason || ""}</p>
            {/if}
            <p class="text-muted-foreground text-xs">{request.triaged_by?.name || "—"}{#if request.triaged_at} · {dateTime(request.triaged_at)}{/if}</p>
          </Card.Content>
        </Card.Root>
      {/if}

      <PluginSlot name="request_aside" record={request} reload={load} />

      <Card.Root>
        <Card.Header><Card.Title>Details</Card.Title></Card.Header>
        <Card.Content>
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt class="text-muted-foreground">Status</dt><dd><StatusBadge status={request.status} /></dd>
            <dt class="text-muted-foreground">{session.term("client")}</dt>
            <dd>
              {#if request.client}<a href={path(request.client.url)} class="underline-offset-4 hover:underline">{request.client.display_name}</a>
              {:else}<span class="text-muted-foreground">No {session.term("client").toLowerCase()} matched</span>{/if}
            </dd>
            {#if request.contact}
              <dt class="text-muted-foreground">Contact</dt><dd>{request.contact.display_name}</dd>
            {/if}
            <dt class="text-muted-foreground">From</dt><dd>{request.from || "Unknown sender"}</dd>
            {#if request.sender_email}
              <dt class="text-muted-foreground">Email</dt><dd class="break-all"><a href={`mailto:${request.sender_email}`} data-external class="underline-offset-4 hover:underline">{request.sender_email}</a></dd>
            {/if}
            <dt class="text-muted-foreground">Source</dt><dd>{capitalize(request.source)}</dd>
            <dt class="text-muted-foreground">Received</dt><dd>{dateTime(request.received_at)}</dd>
          </dl>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>History</Card.Title></Card.Header>
        <Card.Content><History type="Request" id={request.id} /></Card.Content>
      </Card.Root>

      {#if request.status === "open" && session.can("delete_records")}
        <Card.Root>
          <Card.Header><Card.Title>Delete</Card.Title><Card.Description>Only while it’s open; a triaged request stays as history.</Card.Description></Card.Header>
          <Card.Content><Button variant="destructive" size="sm" onclick={remove}><Trash />Delete request</Button></Card.Content>
        </Card.Root>
      {/if}
    </div>
  </div>
{/if}
