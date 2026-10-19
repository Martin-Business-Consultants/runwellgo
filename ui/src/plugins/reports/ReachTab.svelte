<script>
  import EmptyState from "$lib/components/EmptyState.svelte";
  // Reach: where clients are, by country and by time zone (with the time
  // there now), how far the farthest is, how many are on the map for real,
  // and new clients by month.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Chart from "$lib/components/ui/chart/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { BarChart } from "layerchart";
  import { scaleBand } from "d3-scale";
  import MapPin from "@lucide/svelte/icons/map-pin";
  import { session } from "$lib/session.svelte.js";
  import { flag, timeIn, monthName } from "./reports.js";

  let { reach, clients, totals, onshow = () => {}, onplace = () => {} } = $props();
  const names = $derived(Object.fromEntries(clients.map((c) => [c.id, c.name])));
  const most = $derived(Math.max(1, ...reach.countries.map((c) => c.clients.length)));
  const config = { count: { label: `New ${session.term("client", 2).toLowerCase()}`, color: "var(--chart-1)" } };
  const anyNew = $derived(reach.new_by_month.some((m) => m.count));
  const offset = (s) => { const h = s / 3600; return `UTC${h >= 0 ? "+" : "−"}${Math.abs(h)}`; };
  const firstApprox = $derived(clients.find((c) => c.place?.approximate));
  let now = $state(new Date());
  $effect(() => { const t = setInterval(() => (now = new Date()), 30000); return () => clearInterval(t); });
</script>

{#snippet stat(label, value, hint)}
  <Card.Root class="card-framed:h-full">
    <Card.Header><Card.Title>{label}</Card.Title></Card.Header>
    <Card.Content class="flex-1 gap-0.5 px-4 py-3">
      <p class="text-2xl font-bold tracking-tight tabular-nums">{value}</p>
      {#if hint}<p class="text-muted-foreground text-xs">{hint}</p>{/if}
    </Card.Content>
  </Card.Root>
{/snippet}

<div class="flex flex-col gap-4">
  <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    {@render stat("Countries", totals.countries, reach.countries[0] ? `Most in ${reach.countries[0].name}` : "")}
    {@render stat("Time zones", totals.time_zones, "Counted by the hour they keep now")}
    {@render stat("Farthest", reach.farthest ? `${Math.round(reach.farthest.km).toLocaleString()} km` : "—", reach.farthest ? `${reach.farthest.name}, from ${reach.home.label}` : "")}
    {@render stat("On the map", `${totals.placed} of ${totals.clients}`, totals.approximate ? `${totals.approximate} placed by their time zone` : "Every one placed for real")}
  </div>

  {#if totals.approximate}
    <div class="bg-muted/50 flex flex-wrap items-center gap-3 rounded-lg border px-4 py-3 text-sm">
      <MapPin class="text-primary size-4" aria-hidden="true" />
      <p class="min-w-0 flex-1">Put your {session.term("client", 2).toLowerCase()} on the map: {totals.approximate === 1 ? "one is" : `${totals.approximate} are`} only placed by their time zone, a hollow pin on the globe.</p>
      {#if firstApprox}<Button size="sm" variant="outline" onclick={() => onplace(firstApprox.id)}>Place {firstApprox.name}</Button>{/if}
    </div>
  {/if}

  <div class="grid gap-4 lg:grid-cols-2">
    <Card.Root class="gap-3">
      <Card.Header><Card.Title>By country</Card.Title><Card.Description>Where they are, as far as you've told Runwell.</Card.Description></Card.Header>
      <Card.Content>
        <ul class="flex flex-col gap-3">
          {#each reach.countries as c (c.code)}
            <li>
              <div class="flex items-baseline gap-2 text-sm">
                <span aria-hidden="true">{flag(c.code)}</span>
                <span class="flex-1 font-medium">{c.name}</span>
                <span class="tabular-nums">{c.clients.length}</span>
              </div>
              <div class="bg-muted mt-1 h-1.5 overflow-hidden rounded-full" aria-hidden="true"><div class="bg-primary h-full rounded-full" style:width="{(c.clients.length / most) * 100}%"></div></div>
              <p class="text-muted-foreground mt-1 text-xs">
                {#each c.clients as id, i (id)}{#if i}{", "}{/if}<button type="button" class="hover:text-foreground hover:underline" onclick={() => onshow(id)}>{names[id]}</button>{/each}
                {#if c.approximate}<span>{" · "}{c.approximate} by time zone</span>{/if}
              </p>
            </li>
          {/each}
        </ul>
      </Card.Content>
    </Card.Root>

    <div class="flex flex-col gap-4">
      <Card.Root class="gap-3">
        <Card.Header><Card.Title>By time zone</Card.Title><Card.Description>West to east, with the time there now.</Card.Description></Card.Header>
        <Card.Content>
          <ul class="flex flex-col divide-y text-sm">
            {#each reach.zones as z (z.iana)}
              <li class="flex items-baseline gap-3 py-2 first:pt-0 last:pb-0">
                <span class="w-20 shrink-0 font-medium tabular-nums">{timeIn(z.iana, now)}</span>
                <span class="min-w-0 flex-1 truncate">{z.city || z.iana}{#if !z.zone}<span class="text-muted-foreground"> (ours)</span>{/if}</span>
                <span class="text-muted-foreground shrink-0 text-xs tabular-nums">{offset(z.offset)}</span>
                <span class="w-6 shrink-0 text-right tabular-nums" title={z.clients.map((id) => names[id]).join(", ")}>{z.clients.length}</span>
              </li>
            {/each}
          </ul>
        </Card.Content>
      </Card.Root>

      <Card.Root class="gap-2">
        <Card.Header><Card.Title>New {session.term("client", 2).toLowerCase()}</Card.Title><Card.Description>By the month they joined, the last 12.</Card.Description></Card.Header>
        <Card.Content class="px-2 sm:px-4">
          {#if anyNew}
            <Chart.Container {config} class="aspect-auto h-40 w-full">
              <BarChart data={reach.new_by_month} x="month" xScale={scaleBand().padding(0.3)} axis="x"
                series={[{ key: "count", label: config.count.label, color: config.count.color }]}
                props={{ bars: { stroke: "none", rounded: "top", radius: 4 }, xAxis: { format: (m) => monthName(m) }, highlight: { area: { fill: "none" } } }}>
                {#snippet tooltip()}<Chart.Tooltip labelFormatter={(m) => monthName(m, true)} />{/snippet}
              </BarChart>
            </Chart.Container>
          {:else}
            <EmptyState variant="inline" icon="everyone">No one new in the last 12 months.</EmptyState>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>
  </div>
</div>
