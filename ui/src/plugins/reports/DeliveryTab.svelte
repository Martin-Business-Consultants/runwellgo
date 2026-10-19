<script>
  // Delivery: work done against work added over 26 weeks, how much is done
  // on time, how long agreements wait for a yes, requests in against
  // triaged. Each number opens the list it counts.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Chart from "$lib/components/ui/chart/index.js";
  import { AreaChart, BarChart } from "layerchart";
  import { scaleUtc, scaleBand } from "d3-scale";
  import { curveMonotoneX } from "d3-shape";
  import { session } from "$lib/session.svelte.js";

  let { delivery } = $props();
  const work = session.term("work");
  const workConfig = { done: { label: `${work} done`, color: "var(--chart-2)" }, added: { label: `${work} added`, color: "var(--chart-1)" } };
  const reqConfig = { requests: { label: "Requests in", color: "var(--chart-3)" }, triaged: { label: "Triaged", color: "var(--chart-4)" } };
  const data = $derived(delivery.weeks.map((w) => ({ ...w, date: new Date(w.week + "T00:00:00Z") })));
  const day = (d) => d.toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" });
  const recent = $derived(delivery.weeks.slice(-13).map((w) => ({ ...w, label: w.week })));
  const hours = (h) => (h == null ? "—" : h < 48 ? `${h} h` : `${Math.round(h / 24)} days`);
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

<div class="flex flex-col gap-4">
  <div class="grid grid-cols-2 gap-3 lg:grid-cols-4">
    {@render stat(`${work} done`, delivery.done, `${delivery.added} added in 26 weeks`, "/work?view=board")}
    {@render stat("Done on time", delivery.on_time_rate == null ? "—" : `${delivery.on_time_rate}%`, delivery.with_due ? `${delivery.on_time} of ${delivery.with_due} with a due date` : "None done with a due date yet", "/work?view=table&sort=due")}
    {@render stat("Time to a yes", delivery.approval_days == null ? "—" : `${delivery.approval_days} days`, delivery.approvals ? `From sent to approved, ${delivery.approvals} in a year` : "No approvals in a year", "/engagements?state=sent")}
    {@render stat("Time to triage", hours(delivery.triage_hours), `${delivery.triaged} of ${delivery.requests} requests triaged`, "/requests")}
  </div>

  <div class="grid gap-4 lg:grid-cols-5">
    <Card.Root class="gap-2 lg:col-span-3">
      <Card.Header>
        <Card.Title>{work} done and added</Card.Title>
        <Card.Description>Each week, the last 26.</Card.Description>
      </Card.Header>
      <Card.Content class="px-2 sm:px-4">
        <Chart.Container config={workConfig} class="aspect-auto h-60 w-full">
          <AreaChart legend {data} x="date" xScale={scaleUtc()}
            series={Object.entries(workConfig).map(([key, c]) => ({ key, label: c.label, color: c.color }))}
            props={{ area: { curve: curveMonotoneX, "fill-opacity": 0.22, line: { class: "stroke-[1.5]" }, motion: "tween" }, xAxis: { ticks: 6, format: day }, yAxis: { format: (v) => (Number.isInteger(v) ? v : "") } }}>
            {#snippet tooltip()}<Chart.Tooltip labelFormatter={(d) => "Week of " + day(d)} indicator="line" />{/snippet}
          </AreaChart>
        </Chart.Container>
      </Card.Content>
    </Card.Root>

    <Card.Root class="gap-2 lg:col-span-2">
      <Card.Header>
        <Card.Title>Requests in and triaged</Card.Title>
        <Card.Description>Each week, the last 13.</Card.Description>
      </Card.Header>
      <Card.Content class="px-2 sm:px-4">
        <Chart.Container config={reqConfig} class="aspect-auto h-60 w-full">
          <BarChart data={recent} x="label" xScale={scaleBand().padding(0.25)} axis="x" seriesLayout="group" legend
            series={Object.entries(reqConfig).map(([key, c]) => ({ key, label: c.label, color: c.color }))}
            props={{ bars: { stroke: "none", rounded: "top", radius: 3 }, xAxis: { format: (w) => day(new Date(w + "T00:00:00Z")), ticks: 4 }, highlight: { area: { fill: "none" } } }}>
            {#snippet tooltip()}<Chart.Tooltip labelFormatter={(w) => "Week of " + day(new Date(w + "T00:00:00Z"))} />{/snippet}
          </BarChart>
        </Chart.Container>
      </Card.Content>
    </Card.Root>
  </div>
</div>
