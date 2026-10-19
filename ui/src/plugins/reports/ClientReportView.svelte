<script>
  // A client's progress report, as they see it in their portal (and the
  // owner previews it): the figures (on time, requests answered, promises
  // kept, work done), their engagements and how far along, work done by
  // the month, what's next, and money agreed when the owner shows it. Only
  // the sections the report carries are drawn.
  //   report: /x/reports/portal/report's answer; term(key, n): the words
  //   (the portal's or the app's); preview: links don't lead anywhere
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Chart from "$lib/components/ui/chart/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { BarChart } from "layerchart";
  import { scaleBand } from "d3-scale";
  import { date } from "$lib/format.js";
  import { whole, monthName } from "./reports.js";

  let { report, term, preview = false } = $props();
  const has = (s) => report.sections?.includes(s) && report[s] !== undefined;
  const work = $derived(term("work"));
  const engagementWord = $derived(term("engagement", 2));
  const hours = (h) => (h == null ? "" : h < 1 ? "within the hour" : h < 48 ? `within ${Math.round(h)} hours` : `within ${Math.round(h / 24)} days`);
  const months = $derived((report.work_done?.months || []).map((m) => ({ ...m, label: m.month })));
  const chartConfig = $derived({ done: { label: `${work} done`, color: "var(--chart-2)" } });
  const stats = $derived.by(() => {
    const out = [];
    if (has("on_time")) {
      const o = report.on_time;
      out.push({ label: "Delivered on time", value: o.rate == null ? "—" : `${o.rate}%`, hint: o.with_due ? `${o.on_time} of ${o.with_due} with a due date, in the last year` : "Nothing with a due date finished yet" });
    }
    if (has("requests")) {
      const r = report.requests;
      out.push({ label: "Requests answered", value: r.received ? `${r.answered} of ${r.received}` : "—", hint: [r.typical_hours != null ? `Usually ${hours(r.typical_hours)}` : "", r.open ? `${r.open} waiting for an answer` : "", r.received ? "" : "No requests in the last year"].filter(Boolean).join("; ") });
    }
    if (has("commitments")) {
      const c = report.commitments;
      out.push({ label: "Promises kept", value: c.rate == null ? "—" : `${c.rate}%`, hint: c.kept + c.missed ? `${c.kept} kept${c.kept_on_time !== c.kept ? `, ${c.kept_on_time} on time` : " on time"}${c.missed ? `, ${c.missed} missed` : ""}` : c.open ? `${c.open} open now` : "None due yet" });
    }
    if (has("work_done")) out.push({ label: `${work} done`, value: report.work_done.total, hint: "In the last 12 months" });
    return out;
  });
  const href = (p) => (preview ? undefined : p);
</script>

{#snippet empty(text)}<p class="text-muted-foreground text-sm">{text}</p>{/snippet}

<div class="flex flex-col gap-4">
  {#if stats.length}
    <section aria-label="At a glance" class="grid grid-cols-2 gap-3 lg:grid-cols-4 print:grid-cols-4">
      {#each stats as s (s.label)}
        <Card.Root class="gap-1 py-4 print:break-inside-avoid print:shadow-none">
          <Card.Content class="px-4">
            <p class="text-muted-foreground text-sm">{s.label}</p>
            <p class="text-2xl font-bold tracking-tight tabular-nums">{s.value}</p>
            {#if s.hint}<p class="text-muted-foreground mt-0.5 text-xs">{s.hint}</p>{/if}
          </Card.Content>
        </Card.Root>
      {/each}
    </section>
  {/if}

  {#if has("engagements")}
    <Card.Root class="print:break-inside-avoid print:shadow-none">
      <Card.Header>
        <Card.Title><h2>{engagementWord}</h2></Card.Title>
        <Card.Description>How much of the {work.toLowerCase()} shared with you is done.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if report.engagements.length}
          <ul class="flex flex-col gap-4">
            {#each report.engagements as e (e.ref)}
              <li class="flex flex-col gap-1.5">
                <div class="flex flex-wrap items-baseline gap-x-2 gap-y-1">
                  <a href={href(e.path)} class="min-w-0 flex-1 font-medium {preview ? '' : 'hover:underline'}"><span class="text-muted-foreground tabular-nums">{e.ref}</span> {e.title}</a>
                  <Badge variant={e.state === "sent" ? "default" : "secondary"}>{e.state_words}</Badge>
                </div>
                <div class="flex items-center gap-3">
                  <Progress value={e.progress} class="h-2 flex-1" aria-label="{e.title}: {e.progress}% done" />
                  <span class="text-muted-foreground w-28 shrink-0 text-right text-xs tabular-nums">{e.done + e.open ? `${e.done} of ${e.done + e.open} done` : "Nothing shared yet"}</span>
                </div>
                {#if e.late || e.agreed_cents}
                  <p class="text-muted-foreground text-xs">
                    {#if e.agreed_cents}{whole(e.agreed_cents)}{#if e.shape === "recurring"}{" "}{e.cadence || "monthly"}{/if}{/if}{#if e.agreed_cents && e.late}{" · "}{/if}{#if e.late}<span class="text-destructive">{e.late} past {e.late === 1 ? "its" : "their"} due date</span>{/if}
                  </p>
                {/if}
              </li>
            {/each}
          </ul>
        {:else}{@render empty(`No ${engagementWord.toLowerCase()} to show yet.`)}{/if}
      </Card.Content>
    </Card.Root>
  {/if}

  <div class="grid items-start gap-4 lg:grid-cols-5">
    {#if has("work_done")}
      <Card.Root class="gap-2 lg:col-span-3 print:break-inside-avoid print:shadow-none">
        <Card.Header>
          <Card.Title><h2>{work} done</h2></Card.Title>
          <Card.Description>Each month, the last 12.</Card.Description>
        </Card.Header>
        <Card.Content class="px-2 sm:px-4">
          {#if report.work_done.total}
            <Chart.Container config={chartConfig} class="aspect-auto h-56 w-full">
              <BarChart data={months} x="label" xScale={scaleBand().padding(0.3)} axis="x"
                series={[{ key: "done", label: chartConfig.done.label, color: chartConfig.done.color }]}
                props={{ bars: { stroke: "none", rounded: "top", radius: 3 }, xAxis: { format: (m) => monthName(m), ticks: 6 }, highlight: { area: { fill: "none" } } }}>
                {#snippet tooltip()}<Chart.Tooltip labelFormatter={(m) => monthName(m, true)} />{/snippet}
              </BarChart>
            </Chart.Container>
            <table class="sr-only">
              <caption>{work} done each month</caption>
              <tbody>{#each months as m (m.month)}<tr><th scope="row">{monthName(m.month, true)}</th><td>{m.done}</td></tr>{/each}</tbody>
            </table>
          {:else}<div class="px-2 sm:px-0">{@render empty("Nothing finished in the last year yet.")}</div>{/if}
        </Card.Content>
      </Card.Root>
    {/if}

    {#if has("next")}
      {@const n = report.next}
      <Card.Root class="gap-3 {has('work_done') ? 'lg:col-span-2' : 'lg:col-span-5'} print:break-inside-avoid print:shadow-none">
        <Card.Header>
          <Card.Title><h2>What’s next</h2></Card.Title>
        </Card.Header>
        <Card.Content class="flex flex-col gap-4 text-sm">
          {#if n.to_decide.length}
            <div>
              <h3 class="mb-1 font-medium">Waiting for your go-ahead</h3>
              <ul class="flex flex-col gap-1">{#each n.to_decide as d (d.ref)}<li><a href={href(d.path)} class="text-primary {preview ? '' : 'hover:underline'}">{d.ref} {d.title}</a></li>{/each}</ul>
            </div>
          {/if}
          <div>
            <h3 class="mb-1 font-medium">Coming up</h3>
            {#if n.work.length}
              <ul class="flex flex-col gap-1.5">
                {#each n.work as w, i (i)}
                  <li class="flex items-baseline gap-2">
                    <span class="min-w-0 flex-1">{w.title} <span class="text-muted-foreground text-xs whitespace-nowrap">{w.engagement}</span></span>
                    <span class="shrink-0 text-xs tabular-nums {w.late ? 'text-destructive' : 'text-muted-foreground'}">{w.due_on ? (w.late ? `Was due ${date(w.due_on)}` : `Due ${date(w.due_on)}`) : "No date yet"}</span>
                  </li>
                {/each}
              </ul>
              {#if n.more_work}<p class="text-muted-foreground mt-1 text-xs">And {n.more_work} more.</p>{/if}
            {:else}{@render empty("Nothing open right now.")}{/if}
          </div>
          {#if n.promised.length}
            <div>
              <h3 class="mb-1 font-medium">What we promised</h3>
              <ul class="flex flex-col gap-1.5">
                {#each n.promised as c, i (i)}<li class="flex items-baseline gap-2"><span class="min-w-0 flex-1">{c.description}</span>{#if c.due_on}<span class="shrink-0 text-xs tabular-nums {c.late ? 'text-destructive' : 'text-muted-foreground'}">{date(c.due_on)}</span>{/if}</li>{/each}
              </ul>
            </div>
          {/if}
          {#if n.waiting_on_you.length}
            <div>
              <h3 class="mb-1 font-medium">Waiting on you</h3>
              <ul class="flex flex-col gap-1.5">
                {#each n.waiting_on_you as c, i (i)}<li class="flex items-baseline gap-2"><span class="min-w-0 flex-1">{c.description}</span>{#if c.due_on}<span class="shrink-0 text-xs tabular-nums {c.late ? 'text-destructive' : 'text-muted-foreground'}">{date(c.due_on)}</span>{/if}</li>{/each}
              </ul>
            </div>
          {/if}
        </Card.Content>
      </Card.Root>
    {/if}
  </div>

  {#if has("money")}
    <Card.Root class="print:break-inside-avoid print:shadow-none">
      <Card.Header>
        <Card.Title><h2>Agreed</h2></Card.Title>
        <Card.Description>What’s agreed on {engagementWord.toLowerCase()} under way.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if report.money.by_engagement.length}
          <ul class="divide-y text-sm">
            {#each report.money.by_engagement as m (m.ref)}
              <li class="flex items-baseline gap-2 py-2"><span class="min-w-0 flex-1"><span class="text-muted-foreground tabular-nums">{m.ref}</span> {m.title}</span><span class="font-medium tabular-nums">{whole(m.agreed_cents)}{#if m.shape === "recurring"}<span class="text-muted-foreground font-normal"> {m.cadence || "monthly"}</span>{/if}</span></li>
            {/each}
          </ul>
          {#if report.money.monthly_recurring_cents}<p class="text-muted-foreground mt-2 text-xs">{whole(report.money.monthly_recurring_cents)} a month recurring.</p>{/if}
        {:else}{@render empty("Nothing agreed yet.")}{/if}
      </Card.Content>
    </Card.Root>
  {/if}

  <p class="text-muted-foreground text-xs">From {report.from}, as of {date(report.today)}.</p>
</div>
