<script>
  // Home's three things, side by side: who we work for (clients, and the
  // requests waiting from them), what we agreed (engagements running, what
  // they're worth, what waits on a client or is still a draft) and what's
  // happening (work open, with a bar of how it stands). Every number opens
  // the list it counts.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Tooltip from "$lib/components/ui/tooltip/index.js";
  import Users from "@lucide/svelte/icons/users";
  import Briefcase from "@lucide/svelte/icons/briefcase";
  import ListChecks from "@lucide/svelte/icons/list-checks";
  import { session } from "$lib/session.svelte.js";
  import { money } from "$lib/format.js";

  let { pulse } = $props();
  const c = $derived(pulse.clients), e = $derived(pulse.engagements), w = $derived(pulse.work);
  // The work bar: late, due this week, in review, blocked, the rest open.
  const rest = $derived(Math.max(0, w.open - w.late - w.due_week - w.in_review - w.blocked));
  const bar = $derived([
    { key: "late", label: "Late", n: w.late, cls: "bg-destructive", href: "/work?view=table&sort=due" },
    { key: "due", label: "Due this week", n: w.due_week, cls: "bg-amber-500", href: "/work?view=table&sort=due" },
    { key: "review", label: "In review", n: w.in_review, cls: "bg-violet-500", href: "/work?view=board" },
    { key: "blocked", label: "Blocked", n: w.blocked, cls: "bg-orange-700", href: "/work?view=board" },
    { key: "rest", label: "Open, later", n: rest, cls: "bg-primary/35", href: "/work" },
  ].filter((s) => s.n > 0));
</script>

{#snippet stat(href, n, label, tone = "")}
  <a {href} class="hover:bg-muted/60 -mx-1.5 flex items-baseline gap-1.5 rounded-md px-1.5 py-0.5 text-sm">
    <span class="font-semibold tabular-nums {tone}">{n}</span><span class="text-muted-foreground">{label}</span>
  </a>
{/snippet}

<div class="grid gap-4 md:grid-cols-3">
  <Card.Root class="gap-3">
    <Card.Header>
      <Card.Description class="flex items-center gap-2"><Users class="size-4" />Who we work for</Card.Description>
      <Card.Title><a href="/clients?status=active" class="hover:underline"><span class="text-3xl font-bold tabular-nums">{c.active}</span> <span class="text-base font-medium">active {session.term("client", c.active).toLowerCase()}</span></a></Card.Title>
    </Card.Header>
    <Card.Content class="flex flex-col">
      {@render stat("/requests", c.requests_open, c.requests_open === 1 ? "request to triage" : "requests to triage", c.requests_open ? "text-primary" : "")}
    </Card.Content>
  </Card.Root>

  <Card.Root class="gap-3">
    <Card.Header>
      <Card.Description class="flex items-center gap-2"><Briefcase class="size-4" />What we agreed</Card.Description>
      <Card.Title><a href="/engagements" class="hover:underline"><span class="text-3xl font-bold tabular-nums">{e.active}</span> <span class="text-base font-medium">{session.term("engagement", e.active).toLowerCase()} running</span></a></Card.Title>
    </Card.Header>
    <Card.Content class="flex flex-col">
      {#if e.agreed_cents != null}<p class="text-muted-foreground px-0 py-0.5 text-sm"><span class="text-foreground font-semibold tabular-nums">{money(e.agreed_cents)}</span> agreed</p>{/if}
      {@render stat("/engagements?state=sent", e.waiting, "waiting on a client")}
      {@render stat("/engagements?state=draft", e.drafts, e.drafts === 1 ? "draft" : "drafts")}
    </Card.Content>
  </Card.Root>

  <Card.Root class="gap-3">
    <Card.Header>
      <Card.Description class="flex items-center gap-2"><ListChecks class="size-4" />What's happening</Card.Description>
      <Card.Title><a href="/work" class="hover:underline"><span class="text-3xl font-bold tabular-nums">{w.open}</span> <span class="text-base font-medium">open {session.term("work", w.open).toLowerCase()}</span></a></Card.Title>
    </Card.Header>
    <Card.Content class="flex flex-col gap-2">
      {#if bar.length}
        <div class="bg-muted flex h-2 overflow-hidden rounded-full" role="img" aria-label={bar.map((s) => `${s.n} ${s.label.toLowerCase()}`).join(", ")}>
          {#each bar as s (s.key)}
            <Tooltip.Root>
              <Tooltip.Trigger>
                {#snippet child({ props })}<a {...props} href={s.href} class="{s.cls} h-full transition-opacity hover:opacity-80" style:width="{(s.n / Math.max(w.open, 1)) * 100}%" aria-label="{s.n} {s.label.toLowerCase()}"></a>{/snippet}
              </Tooltip.Trigger>
              <Tooltip.Content>{s.n} {s.label.toLowerCase()}</Tooltip.Content>
            </Tooltip.Root>
          {/each}
        </div>
      {/if}
      <div class="flex flex-wrap gap-x-3">
        {@render stat("/work?view=table&sort=due", w.late, "late", w.late ? "text-destructive" : "")}
        {@render stat("/work?view=table&sort=due", w.due_week, "due this week")}
        {@render stat("/work?view=board", w.in_review, "in review")}
        {@render stat("/work?view=board", w.blocked, "blocked")}
      </div>
    </Card.Content>
  </Card.Root>
</div>
