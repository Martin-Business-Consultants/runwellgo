<script>
  // Twelve weeks at a glance (shadcn's area chart): work added and work
  // done, with requests in, by the week. Hover for a week's numbers; the
  // legend shows which line is which.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Chart from "$lib/components/ui/chart/index.js";
  import { AreaChart } from "layerchart";
  import { scaleUtc } from "d3-scale";
  import { curveMonotoneX } from "d3-shape";
  import { session } from "$lib/session.svelte.js";

  let { weeks } = $props();
  const work = session.term("work").toLowerCase();
  const config = {
    done: { label: `${session.term("work")} done`, color: "var(--chart-2)" },
    added: { label: `${session.term("work")} added`, color: "var(--chart-1)" },
    requests: { label: "Requests in", color: "var(--chart-3)" },
  };
  const data = $derived(weeks.map((w) => ({ ...w, date: new Date(w.week + "T00:00:00Z") })));
  const totals = $derived(weeks.reduce((t, w) => ({ added: t.added + w.added, done: t.done + w.done, requests: t.requests + w.requests }), { added: 0, done: 0, requests: 0 }));
  const day = (d) => d.toLocaleDateString(undefined, { month: "short", day: "numeric", timeZone: "UTC" });
</script>

<Card.Root class="gap-2">
  <Card.Header>
    <Card.Title>The last 12 weeks</Card.Title>
    <Card.Description>{totals.done} {work} done, {totals.added} added, {totals.requests} {totals.requests === 1 ? "request" : "requests"} in.</Card.Description>
  </Card.Header>
  <Card.Content class="px-2 sm:px-4">
    <Chart.Container config={config} class="aspect-auto h-56 w-full">
      <AreaChart
        legend
        {data}
        x="date"
        xScale={scaleUtc()}
        series={Object.entries(config).map(([key, c]) => ({ key, label: c.label, color: c.color }))}
        props={{
          area: { curve: curveMonotoneX, "fill-opacity": 0.25, line: { class: "stroke-[1.5]" }, motion: "tween" },
          xAxis: { ticks: 6, format: day },
          yAxis: { format: (v) => (Number.isInteger(v) ? v : "") },
        }}
      >
        {#snippet tooltip()}
          <Chart.Tooltip labelFormatter={(d) => "Week of " + day(d)} indicator="line" />
        {/snippet}
      </AreaChart>
    </Chart.Container>
  </Card.Content>
</Card.Root>
