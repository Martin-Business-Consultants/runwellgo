<script>
  import EmptyState from "$lib/components/EmptyState.svelte";
  // Health: every client scored, at risk first (or as the header sorts
  // them), with why, twelve weeks of activity, the last touch and the
  // counts behind the score, each a link to its list. How the score works
  // is one click away.
  import * as Table from "$lib/components/ui/table/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import SortableHead from "$lib/components/SortableHead.svelte";
  import Info from "@lucide/svelte/icons/info";
  import Globe from "@lucide/svelte/icons/globe";
  import Sparkline from "./Sparkline.svelte";
  import { router } from "$lib/router.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { band } from "./reports.js";

  let { clients, onshow = () => {} } = $props();
  const keys = {
    name: (c) => c.name.toLowerCase(), score: (c) => c.health.score, touch: (c) => (c.days_since_touch < 0 ? 1e6 : c.days_since_touch),
    late: (c) => c.late_work, overdue: (c) => c.overdue_commitments, waiting: (c) => c.waiting_approvals, requests: (c) => c.open_requests,
    activity: (c) => c.activity.reduce((a, b) => a + b, 0),
  };
  const sorted = $derived.by(() => {
    const k = keys[router.query.get("sort")];
    if (!k) return clients;
    const dir = router.query.get("direction") === "desc" ? -1 : 1;
    return [...clients].sort((a, b) => (k(a) > k(b) ? dir : k(a) < k(b) ? -dir : 0));
  });
  const touch = (c) => (c.days_since_touch < 0 ? "Never" : c.days_since_touch === 0 ? "Today" : c.days_since_touch === 1 ? "Yesterday" : `${c.days_since_touch} days ago`);
</script>

<div class="flex flex-col gap-3">
  <Collapsible.Root>
    <div class="flex flex-wrap items-center justify-between gap-2">
      <p class="text-muted-foreground text-sm">Each {session.term("client").toLowerCase()} starts at 100; what's late, overdue, waiting or quiet takes points off.</p>
      <Collapsible.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="sm"><Info />How it's scored</Button>{/snippet}</Collapsible.Trigger>
    </div>
    <Collapsible.Content>
      <div class="bg-muted/40 mt-2 grid gap-x-8 gap-y-1 rounded-lg border p-4 text-sm sm:grid-cols-2">
        <p><span class="font-medium tabular-nums">−10</span> each piece of late {session.term("work").toLowerCase()} (at most −30)</p>
        <p><span class="font-medium tabular-nums">−12</span> each overdue {session.term("commitment").toLowerCase()} we owe (at most −30)</p>
        <p><span class="font-medium tabular-nums">−4</span> each overdue one of theirs (at most −12)</p>
        <p><span class="font-medium tabular-nums">−8</span> each agreement waiting over a week (−3 if newer; at most −16)</p>
        <p><span class="font-medium tabular-nums">−5</span> each request to triage, −5 more once one's 3 days old</p>
        <p><span class="font-medium tabular-nums">−4</span> each piece of blocked {session.term("work").toLowerCase()} (at most −12)</p>
        <p><span class="font-medium tabular-nums">−8 / −15 / −25</span> no touch in 14 / 30 / 60 days</p>
        <p><span class="font-medium tabular-nums">−10</span> nothing done in 4 weeks; <span class="font-medium tabular-nums">+5</span> more done than the 4 before</p>
        <p class="sm:col-span-2 text-muted-foreground">80 and up is on track, 55 to 79 needs attention, under 55 is at risk.</p>
      </div>
    </Collapsible.Content>
  </Collapsible.Root>

  {#if !clients.length}
    <EmptyState variant="inline" icon="everyone">No {session.term("client", 2).toLowerCase()} match these filters.</EmptyState>
  {:else}
    <div class="overflow-x-auto rounded-lg border">
      <Table.Root>
        <Table.Header>
          <Table.Row>
            <SortableHead key="name" label={session.term("client")} />
            <SortableHead key="score" label="Health" />
            <Table.Head class="min-w-56">Why</Table.Head>
            <SortableHead key="activity" label="12 weeks" />
            <SortableHead key="touch" label="Last touch" />
            <SortableHead key="late" label="Late" class="text-right" />
            <SortableHead key="overdue" label="Overdue" class="text-right" />
            <SortableHead key="waiting" label="Waiting" class="text-right" />
            <SortableHead key="requests" label="Requests" class="text-right" />
          </Table.Row>
        </Table.Header>
        <Table.Body>
          {#each sorted as c (c.id)}
            {@const b = band(c.health.band)}
            <Table.Row>
              <Table.Cell class="max-w-52">
                <div class="flex items-center gap-1">
                  <a href={c.url} class="truncate font-medium hover:underline">{c.name}</a>
                  <Button variant="ghost" size="icon-sm" class="text-muted-foreground size-7 shrink-0" onclick={() => onshow(c.id)} aria-label="Show {c.name} on the globe" title="Show on the globe"><Globe class="size-3.5" /></Button>
                </div>
              </Table.Cell>
              <Table.Cell>
                <div class="flex items-center gap-2">
                  <span class="w-7 text-right font-semibold tabular-nums">{c.health.score}</span>
                  <Badge variant="outline" class={b.badge}>{b.label}</Badge>
                </div>
              </Table.Cell>
              <Table.Cell class="text-muted-foreground max-w-80 text-sm whitespace-normal">
                {#if c.health.reasons.length}
                  {#each c.health.reasons.slice(0, 2) as r, i (r.text)}{#if i}{"; "}{/if}{#if r.href}<a href={r.href} class="hover:text-foreground hover:underline">{r.text}</a>{:else}{r.text}{/if}{/each}{#if c.health.reasons.length > 2}<span title={c.health.reasons.slice(2).map((r) => r.text).join("; ")}>{`; ${c.health.reasons.length - 2} more`}</span>{/if}
                {:else}All well{/if}
              </Table.Cell>
              <Table.Cell><Sparkline values={c.activity} color={b.color} label="{c.activity.reduce((a, x) => a + x, 0)} things in 12 weeks" /></Table.Cell>
              <Table.Cell class="text-sm whitespace-nowrap {c.days_since_touch >= 30 || c.days_since_touch < 0 ? 'text-destructive' : ''}"><a href="{c.url}?tab=notes" class="hover:underline">{touch(c)}</a></Table.Cell>
              <Table.Cell class="text-right tabular-nums"><a href="{c.url}?tab=engagements" class="hover:underline {c.late_work ? 'text-destructive font-semibold' : 'text-muted-foreground'}">{c.late_work}</a></Table.Cell>
              <Table.Cell class="text-right tabular-nums"><a href="{c.url}?tab=commitments" class="hover:underline {c.overdue_commitments ? 'text-destructive font-semibold' : 'text-muted-foreground'}">{c.overdue_commitments}</a></Table.Cell>
              <Table.Cell class="text-right tabular-nums"><a href="/engagements?state=sent" class="hover:underline {c.waiting_approvals ? 'font-semibold' : 'text-muted-foreground'}">{c.waiting_approvals}</a></Table.Cell>
              <Table.Cell class="text-right tabular-nums"><a href="/requests" class="hover:underline {c.open_requests ? 'font-semibold' : 'text-muted-foreground'}">{c.open_requests}</a></Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    </div>
  {/if}
</div>
