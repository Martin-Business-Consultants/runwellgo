<script>
  import EmptyState from "$lib/components/EmptyState.svelte";
  // Money: what's agreed and approved, by month, by client, by type,
  // recurring against fixed, and what's still in the pipeline. Each number
  // opens the list it counts. Nothing when the install hides prices.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Chart from "$lib/components/ui/chart/index.js";
  import { BarChart } from "layerchart";
  import { scaleBand } from "d3-scale";
  import { session } from "$lib/session.svelte.js";
  import { whole, compact, monthName, typeName } from "./reports.js";

  let { money } = $props();
  const config = { approved: { label: "Approved", color: "var(--chart-1)" } };
  const months = $derived(money.by_month.map((m) => ({ month: m.month, approved: m.approved_cents / 100, approvals: m.approvals })));
  const anyMonth = $derived(money.by_month.some((m) => m.approved_cents > 0));
  const topClient = $derived(Math.max(1, ...money.by_client.map((c) => c.agreed_cents + c.this_year_cents)));
  const labelTotal = $derived(Math.max(1, money.by_label.reduce((a, l) => a + l.cents, 0)));
  const split = $derived(money.recurring.cents + money.fixed.cents);
  const pipeline = $derived([
    { key: "draft", label: "Drafts", href: "/engagements?state=draft" },
    { key: "sent", label: "Sent, waiting", href: "/engagements?state=sent" },
    { key: "changes_requested", label: "Changes asked for", href: "/engagements?state=changes_requested" },
  ]);
  const labelColors = ["var(--chart-1)", "var(--chart-4)", "var(--chart-2)"];
</script>

{#snippet stat(label, value, hint, href)}
  <Card.Root class="card-framed:h-full">
    <Card.Header><Card.Title>{label}</Card.Title></Card.Header>
    <Card.Content class="flex-1 gap-0.5 px-4 py-3">
      <a {href} class="hover:underline"><p class="text-2xl font-bold tracking-tight tabular-nums">{value}</p></a>
      {#if hint}<p class="text-muted-foreground text-xs">{hint}</p>{/if}
    </Card.Content>
  </Card.Root>
{/snippet}

{#if money.hidden}
  <EmptyState variant="inline" icon="activity">Prices are hidden on this install, so there's no money to show. Settings › Names can show them.</EmptyState>
{:else}
  <div class="flex flex-col gap-4">
    <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
      {@render stat(`Agreed on open ${session.term("engagement", 2).toLowerCase()}`, whole(money.agreed_cents), "Fixed scope in full, recurring per period", "/engagements?state=approved")}
      {@render stat("Approved this year", whole(money.this_year_cents), `Since January 1`, "/engagements")}
      {@render stat("Recurring, a month", whole(money.monthly_recurring_cents), `${money.recurring.count} recurring ${session.term("engagement", money.recurring.count).toLowerCase()}`, "/engagements?label=service")}
      {@render stat("Waiting on clients", whole(money.pipeline.sent.cents), `${money.pipeline.sent.count} sent, not yet approved`, "/engagements?state=sent")}
    </div>

    <div class="grid gap-4 lg:grid-cols-5">
      <Card.Root class="gap-2 lg:col-span-3">
        <Card.Header>
          <Card.Title>Approved by month</Card.Title>
          <Card.Description>What clients said yes to, the last 12 months.</Card.Description>
        </Card.Header>
        <Card.Content class="px-2 sm:px-4">
          {#if anyMonth}
            <Chart.Container {config} class="aspect-auto h-60 w-full lg:h-[22rem]">
              <BarChart data={months} x="month" xScale={scaleBand().padding(0.3)} axis="x"
                series={[{ key: "approved", label: "Approved", color: config.approved.color }]}
                props={{ bars: { stroke: "none", rounded: "top", radius: 4 }, xAxis: { format: (m) => monthName(m) }, highlight: { area: { fill: "none" } } }}>
                {#snippet tooltip()}
                  <Chart.Tooltip labelFormatter={(m) => monthName(m, true)}>
                    {#snippet formatter({ value, item })}
                      <div class="flex w-full justify-between gap-3"><span class="text-muted-foreground">Approved</span><span class="font-medium tabular-nums">{whole(value * 100)}</span></div>
                    {/snippet}
                  </Chart.Tooltip>
                {/snippet}
              </BarChart>
            </Chart.Container>
          {:else}
            <EmptyState variant="inline" icon="activity">Nothing approved in the last 12 months yet: send an agreement and it shows here once they say yes.</EmptyState>
          {/if}
        </Card.Content>
      </Card.Root>

      <Card.Root class="gap-3 lg:col-span-2">
        <Card.Header>
          <Card.Title>Top {session.term("client", 2).toLowerCase()}</Card.Title>
          <Card.Description>Agreed now, and approved this year.</Card.Description>
        </Card.Header>
        <Card.Content>
          {#if money.by_client.length}
            <ul class="flex flex-col gap-2.5">
              {#each money.by_client as c (c.id)}
                <li>
                  <div class="flex items-baseline gap-2 text-sm">
                    <a href={c.url} class="min-w-0 flex-1 truncate hover:underline">{c.name}</a>
                    <span class="font-medium tabular-nums">{whole(c.agreed_cents)}</span>
                  </div>
                  <div class="bg-muted mt-1 flex h-1.5 overflow-hidden rounded-full" aria-hidden="true">
                    <div class="bg-primary h-full" style:width="{(c.agreed_cents / topClient) * 100}%"></div>
                    <div class="bg-primary/35 h-full" style:width="{(Math.max(0, c.this_year_cents - c.agreed_cents) / topClient) * 100}%"></div>
                  </div>
                </li>
              {/each}
            </ul>
          {:else}
            <p class="text-muted-foreground text-sm">Nothing agreed yet.</p>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>

    <div class="grid gap-4 md:grid-cols-3">
      <Card.Root class="gap-3">
        <Card.Header><Card.Title>By type</Card.Title><Card.Description>Agreed on open {session.term("engagement", 2).toLowerCase()}.</Card.Description></Card.Header>
        <Card.Content class="flex flex-col gap-3">
          {#if money.by_label.length}
            <div class="bg-muted flex h-2.5 overflow-hidden rounded-full" role="img" aria-label={money.by_label.map((l) => `${typeName(l.label)} ${whole(l.cents)}`).join(", ")}>
              {#each money.by_label as l, i (l.label)}<div class="h-full" style:width="{(l.cents / labelTotal) * 100}%" style:background={labelColors[i % 3]}></div>{/each}
            </div>
            <ul class="flex flex-col gap-1.5 text-sm">
              {#each money.by_label as l, i (l.label)}
                <li class="flex items-center gap-2">
                  <span class="size-2.5 rounded-sm" style:background={labelColors[i % 3]} aria-hidden="true"></span>
                  <a href="/engagements?label={l.label}" class="flex-1 hover:underline">{typeName(l.label)} <span class="text-muted-foreground tabular-nums">{l.count}</span></a>
                  <span class="font-medium tabular-nums">{whole(l.cents)}</span>
                </li>
              {/each}
            </ul>
          {:else}<p class="text-muted-foreground text-sm">Nothing agreed yet.</p>{/if}
        </Card.Content>
      </Card.Root>

      <Card.Root class="gap-3">
        <Card.Header><Card.Title>Recurring and fixed</Card.Title><Card.Description>Recurring is each one's amount per period.</Card.Description></Card.Header>
        <Card.Content class="flex flex-col gap-3">
          <div class="bg-muted flex h-2.5 overflow-hidden rounded-full" role="img" aria-label="Recurring {whole(money.recurring.cents)}, fixed {whole(money.fixed.cents)}">
            <div class="h-full" style:width="{split ? (money.recurring.cents / split) * 100 : 0}%" style:background="var(--chart-2)"></div>
            <div class="h-full" style:width="{split ? (money.fixed.cents / split) * 100 : 0}%" style:background="var(--chart-1)"></div>
          </div>
          <ul class="flex flex-col gap-1.5 text-sm">
            <li class="flex items-center gap-2"><span class="size-2.5 rounded-sm" style:background="var(--chart-2)" aria-hidden="true"></span><span class="flex-1">Recurring <span class="text-muted-foreground tabular-nums">{money.recurring.count}</span></span><span class="font-medium tabular-nums">{whole(money.recurring.cents)}</span></li>
            {#each Object.entries(money.cadences) as [cad, a] (cad)}
              <li class="text-muted-foreground flex gap-2 ps-4.5 text-xs"><span class="flex-1 capitalize">{cad}</span><span class="tabular-nums">{whole(a.cents)}</span></li>
            {/each}
            <li class="flex items-center gap-2"><span class="size-2.5 rounded-sm" style:background="var(--chart-1)" aria-hidden="true"></span><span class="flex-1">Fixed <span class="text-muted-foreground tabular-nums">{money.fixed.count}</span></span><span class="font-medium tabular-nums">{whole(money.fixed.cents)}</span></li>
          </ul>
          <p class="text-muted-foreground text-xs">About {whole(money.monthly_recurring_cents)} a month comes in from recurring work.</p>
        </Card.Content>
      </Card.Root>

      <Card.Root class="gap-3">
        <Card.Header><Card.Title>Pipeline</Card.Title><Card.Description>Not agreed yet.</Card.Description></Card.Header>
        <Card.Content>
          <ul class="flex flex-col divide-y">
            {#each pipeline as p (p.key)}
              {@const a = money.pipeline[p.key]}
              <li class="py-2 first:pt-0 last:pb-0">
                <a href={p.href} class="hover:bg-muted/60 -mx-1.5 flex items-baseline gap-2 rounded-md px-1.5 py-0.5 text-sm">
                  <span class="flex-1">{p.label} <span class="text-muted-foreground tabular-nums">{a.count}</span></span>
                  <span class="font-medium tabular-nums">{a.cents ? compact(a.cents) : "—"}</span>
                </a>
              </li>
            {/each}
          </ul>
        </Card.Content>
      </Card.Root>
    </div>
  </div>
{/if}
