<script>
  // A client's page: its name and Edit on one row, then the working column
  // (tabs: engagements, commitments, notes, documents and plugins' panels,
  // each added by its area) beside the side column (Details, contacts,
  // history).
  import * as Card from "$lib/components/ui/card/index.js";
  import ConnectionsButton from "$lib/components/ConnectionsButton.svelte";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Pencil from "@lucide/svelte/icons/pencil";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import CustomFieldValues from "$lib/components/CustomFieldValues.svelte";
  import History from "$lib/components/History.svelte";
  import Contacts from "./Contacts.svelte";
  import NotFound from "../NotFound.svelte";
  import PluginSlot from "$lib/components/PluginSlot.svelte";
  import { get } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, withQuery, go } from "$lib/router.svelte.js";
  import { tabsFor } from "$lib/record-tabs.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { date } from "$lib/format.js";

  let { params } = $props();
  let client = $state(null);
  let missing = $state(false);
  const tabs = tabsFor("Client");
  const tab = $derived(tabs.some((t) => t.key === router.query.get("tab")) ? router.query.get("tab") : tabs[0]?.key);

  async function load() {
    try {
      client = (await get(`/clients/${params.id}`)).client;
      page.set(client.name, [{ label: session.term("client", 2), href: "/clients" }], `Client:${client.id}`);
    } catch (e) { if (e.status === 404 && !client) missing = true; else toastError(e); }
  }
  load();
</script>

{#if missing}
  <NotFound title={`Couldn’t find that ${session.term("client").toLowerCase()}`} back={{ href: "/clients", label: `All ${session.term("client", 2).toLowerCase()}` }} />
{:else if !client}
  <Skeleton class="h-64" />
{:else}
  <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
    <div class="min-w-0">
      <h1 class="truncate text-2xl font-semibold tracking-tight">{client.name}</h1>
      <p class="text-muted-foreground text-sm">{client.internal ? "This is us: our own projects, no agreements" : session.term("client")}</p>
    </div>
    <div class="flex shrink-0 items-center gap-2">
      <ConnectionsButton record={`Client:${client.id}`} name={client.name} />
      <Button href={`/clients/${client.id}/edit`} variant="outline" size="sm"><Pencil />Edit</Button>
    </div>
  </div>

  <div class="grid gap-4 lg:grid-cols-3">
    <div class="min-w-0 lg:col-span-2">
      {#if tabs.length}
        <Tabs.Root value={tab} onValueChange={(v) => go(withQuery({ tab: v }), undefined, { replace: true })}>
          <Tabs.List>
            {#each tabs as t (t.key)}
              {@const n = t.count?.(client)}
              <Tabs.Trigger value={t.key}>{t.label()}{#if n}<span class="text-muted-foreground ms-1 text-xs tabular-nums">{n}</span>{/if}</Tabs.Trigger>
            {/each}
          </Tabs.List>
          {#each tabs as t (t.key)}
            <Tabs.Content value={t.key} class="mt-2">
              <t.component record={client} reload={load} />
            </Tabs.Content>
          {/each}
        </Tabs.Root>
      {/if}
    </div>

    <div class="flex flex-col gap-4">
      <Card.Root>
        <Card.Header><Card.Title>Details</Card.Title></Card.Header>
        <Card.Content>
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt class="text-muted-foreground">Status</dt><dd><StatusBadge status={client.status} /></dd>
            <dt class="text-muted-foreground">Since</dt><dd>{date(client.created_at)}</dd>
            <dt class="text-muted-foreground">Time zone</dt><dd>{client.time_zone || "Same as ours"}</dd>
            <CustomFieldValues model="Client" values={client.custom_fields} />
          </dl>
        </Card.Content>
      </Card.Root>
      <Contacts {client} reload={load} />
      <PluginSlot name="client_aside" record={client} reload={load} />
      <Card.Root>
        <Card.Header><Card.Title>History</Card.Title></Card.Header>
        <Card.Content><History type="Client" id={client.id} /></Card.Content>
      </Card.Root>
    </div>
  </div>
{/if}
