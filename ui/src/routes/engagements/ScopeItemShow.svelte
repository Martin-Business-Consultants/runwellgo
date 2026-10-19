<script>
  // A scope item's page: its work (and what other areas add for a scope
  // item, as tabs: notes, documents) beside its Details: delivery, progress,
  // engagement, agreement, price and the internal estimate.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import { get } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, withQuery, go } from "$lib/router.svelte.js";
  import { tabsFor } from "$lib/record-tabs.js";
  import { toastError } from "$lib/toast.svelte.js";
  import NotFound from "../NotFound.svelte";
  import { date, humanize } from "$lib/format.js";
  import PluginSlot from "$lib/components/PluginSlot.svelte";

  let { params } = $props();
  let item = $state(null);
  const tabs = [{ key: "work", label: () => session.term("work"), count: (r) => r.work.length, component: null }, ...tabsFor("ScopeItem")];
  const tab = $derived(tabs.some((t) => t.key === router.query.get("tab")) ? router.query.get("tab") : "work");

  async function load() {
    try {
      item = (await get(`/scope_items/${params.id}`)).scope_item;
      page.set(item.description, [{ label: session.term("engagement", 2), href: "/engagements" }, { label: item.engagement.ref, href: `/engagements/${item.engagement.ref}` }]);
    } catch (e) { if (e.status === 404 && !item) missing = true; else toastError(e); }
  }
  let missing = $state(false);
  load();
</script>

{#if missing}
  <NotFound title={`Couldn’t find that ${session.term("scope_item").toLowerCase()}`} back={{ href: "/engagements", label: `All ${session.term("engagement", 2).toLowerCase()}` }} />
{:else if !item}
  <Skeleton class="h-64" />
{:else}
  <h1 class="text-2xl font-semibold tracking-tight">{item.description}</h1>
  <div class="grid gap-4 lg:grid-cols-3">
    <div class="min-w-0 lg:col-span-2">
      <Tabs.Root value={tab} onValueChange={(v) => go(withQuery({ tab: v }), undefined, { replace: true })}>
        <Tabs.List>
          {#each tabs as t (t.key)}
            {@const n = t.count?.(item)}
            <Tabs.Trigger value={t.key}>{t.label()}{#if n}<span class="text-muted-foreground ms-1 text-xs tabular-nums">{n}</span>{/if}</Tabs.Trigger>
          {/each}
        </Tabs.List>
        {#each tabs as t (t.key)}
          <Tabs.Content value={t.key} class="mt-2">
            {#if t.key === "work"}
              <Card.Root>
                <Card.Content>
                  {#if item.work.length}
                    <ul class="flex flex-col gap-2 text-sm">
                      {#each item.work as w (w.id)}
                        <li class="flex items-center gap-2">
                          <a href={`/work/${w.id}`} class="min-w-0 flex-1 truncate hover:underline">{w.title}</a>
                          {#if w.owner}<span class="text-muted-foreground text-xs">{w.owner.name}</span>{/if}
                          {#if w.due_on}<span class={w.overdue ? "text-destructive text-xs" : "text-muted-foreground text-xs"}>{date(w.due_on)}</span>{/if}
                          <StatusBadge status={w.status} />
                        </li>
                      {/each}
                    </ul>
                  {:else}
                    <p class="text-muted-foreground text-sm">No work under this yet.</p>
                  {/if}
                </Card.Content>
              </Card.Root>
            {:else}
              <t.component record={item} reload={load} />
            {/if}
          </Tabs.Content>
        {/each}
      </Tabs.Root>
    </div>

    <div class="flex flex-col gap-4">
      <Card.Root>
        <Card.Header><Card.Title>Details</Card.Title></Card.Header>
        <Card.Content>
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt class="text-muted-foreground">Delivery</dt><dd><StatusBadge status={item.delivery_state} /></dd>
            <dt class="text-muted-foreground">Progress</dt><dd><strong>{item.done} of {item.total}</strong> done</dd>
            <dt class="text-muted-foreground">{session.term("engagement")}</dt><dd><a class="hover:underline" href={`/engagements/${item.engagement.ref}`}>{item.engagement.display_name}</a></dd>
            <dt class="text-muted-foreground">{session.term("client")}</dt><dd><a class="hover:underline" href={`/clients/${item.client.id}`}>{item.client.display_name}</a></dd>
            <dt class="text-muted-foreground">Agreement</dt><dd>{item.agreement_version.label} ({humanize(item.agreement_version.status).toLowerCase()})</dd>
            {#if item.prices}<dt class="text-muted-foreground">Price</dt><dd>{item.price}</dd>{/if}
            {#if item.internal_estimate}<dt class="text-muted-foreground">Estimate</dt><dd>{item.internal_estimate} <span class="text-muted-foreground">(internal)</span></dd>{/if}
          </dl>
        </Card.Content>
      </Card.Root>
      <PluginSlot name="scope_item_aside" record={item} reload={load} />
    </div>
  </div>
{/if}
