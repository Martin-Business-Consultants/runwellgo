<script>
  import EmptyState from "$lib/components/EmptyState.svelte";
  // Home: what needs a person now, and how the business stands. The day
  // and a greeting; the first-run checklist, folded to its progress; the
  // three things side by side (clients, engagements, work: PulseCards);
  // then two columns of tabs, so it fits in about a screen: wide, what
  // needs you (the briefing's sections, acted on in place), your
  // engagements and how far along each is, and twelve weeks at a glance
  // (PulseChart); down the side, what's coming up for you (ticked off in
  // place), who's doing what and the latest discussion.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import Check from "@lucide/svelte/icons/check";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import PulseCards from "./PulseCards.svelte";
  import PulseChart from "./PulseChart.svelte";
  import ComingUp from "./ComingUp.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import HomeQuestion from "./HomeQuestion.svelte";
  import PluginHome from "../plugins/PluginHome.svelte";
  import PluginSlot from "$lib/components/PluginSlot.svelte";
  import { get, post, patch, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { router, go, withQuery } from "$lib/router.svelte.js";
  import { timeAgo, date, money, humanize, plural } from "$lib/format.js";

  page.set("Home");
  let data = $state(null), pulse = $state(null), setupOpen = $state(false), side = $state("coming");
  // The section of what needs you on show (?need=), the first by default.
  const need = $derived(data?.sections.some((x) => x.key === router.query.get("need")) ? router.query.get("need") : data?.sections[0]?.key);
  const tab = $derived(["engagements", "trend"].includes(router.query.get("tab")) ? router.query.get("tab") : "needs");

  async function load() {
    try { [data, pulse] = await Promise.all([get("/briefing?home=1"), get("/home/pulse")]); } catch (e) { toastError(e); }
  }
  load();
  const reload = () => { load(); nav.refresh(); };

  const needs = $derived(data ? data.sections.reduce((n, s) => n + s.count, 0) : 0);
  const path = (url) => (url ? new URL(url).pathname : null);
  const work = $derived(session.term("work").toLowerCase());
  const commitment = (n) => (n === 1 ? session.term("commitment") : session.term("commitment", 2)).toLowerCase();

  // Where a section's whole list is.
  const more = {
    requests: "/requests",
    awaiting_client: "/engagements?state=sent",
    drafts: "/engagements?state=draft",
    waiting_on_them: "/commitments",
    you_promised: "/commitments",
    review: "/work?view=board",
    blocked: "/work?view=board",
    overdue_todos: "/work?view=table&sort=due",
  };

  const longDate = (ymd) => new Date(ymd + "T00:00:00").toLocaleDateString(undefined, { month: "long", day: "numeric", year: "numeric" });
  const clock = (iso) => new Date(iso).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });
  const priced = (v) => session.me.prices && (v.amount_cents || v.scope_items?.some((i) => i.price_cents));

  const nextStep = $derived(data?.setup?.steps.find((s) => !s.done));
  async function finish(item) {
    try { await patch(`/work/${item.id}`, { todo: { status: "done" } }); toast(`Done: ${item.title}.`); reload(); } catch (e) { toastError(e); }
  }
  async function resolve(c, resolution) {
    try { await post(`/commitments/${c.id}/resolve`, { resolution }); toast(`Marked ${resolution}.`); reload(); } catch (e) { toastError(e); }
  }
  async function hideSetup() {
    try { await del("/setup"); toast("Setup checklist hidden. How Runwell works is always in Settings › Help."); load(); } catch (e) { toastError(e); }
  }
</script>

{#if !data || !pulse}
  <Skeleton class="h-24" />
  <div class="grid gap-4 md:grid-cols-3"><Skeleton class="h-36" /><Skeleton class="h-36" /><Skeleton class="h-36" /></div>
  <div class="grid gap-4 lg:grid-cols-3"><Skeleton class="h-72 lg:col-span-2" /><Skeleton class="h-72" /></div>
{:else}
  {@const home = data.home}
  <!-- The one loud thing in the app: the day, and how much of it needs you. -->
  <header class="flex flex-col gap-1.5 pt-2 pb-1">
    <p class="text-muted-foreground text-sm">{longDate(home.date)}</p>
    <h1 class="text-[1.875rem] leading-[1.15] font-bold tracking-[-0.02em] sm:text-[2.25rem]">{home.greeting}</h1>
    <p class="text-muted-foreground text-base">
      {#if needs === 0}Nothing needs you right now.{:else}<a href="/" class="text-foreground font-semibold tabular-nums underline-offset-4 hover:underline">{plural(needs, "thing")}</a> {needs === 1 ? "needs" : "need"} you today.{/if}
    </p>
  </header>

  <PluginSlot name="home_top" />
  <PluginHome where="top" />

  {#if data.setup}
    <!-- Setup, folded to how far along it is and the next step. -->
    <Collapsible.Root bind:open={setupOpen}>
      <Card.Root class="gap-0 py-0">
        <div class="flex flex-wrap items-center gap-x-4 gap-y-2 px-6 py-4">
          <div class="flex min-w-0 flex-1 flex-col gap-1.5">
            <p class="text-sm"><span class="font-semibold">Get set up</span> <span class="text-muted-foreground tabular-nums">{data.setup.done_count} of {data.setup.steps.length}</span>
              {#if nextStep}<span class="text-muted-foreground"> · Next: </span><a href={nextStep.path} class="font-medium underline-offset-4 hover:underline">{nextStep.title}</a>{/if}</p>
            <Progress value={data.setup.done_count} max={data.setup.steps.length} class="h-1.5 max-w-md" aria-label={`${data.setup.done_count} of ${data.setup.steps.length} setup steps done`} />
          </div>
          <Collapsible.Trigger>
            {#snippet child({ props })}<Button {...props} variant="ghost" size="sm">{setupOpen ? "Fewer" : "All steps"}<ChevronDown class={setupOpen ? "rotate-180" : ""} /></Button>{/snippet}
          </Collapsible.Trigger>
          <Button variant="ghost" size="sm" onclick={hideSetup}>Hide this</Button>
        </div>
        <Collapsible.Content>
          <ol class="grid gap-3 border-t px-6 py-4 sm:grid-cols-2">
            {#each data.setup.steps as step (step.key)}
              <li class="flex items-start gap-3">
                {#if step.done}
                  <span class="bg-primary text-primary-foreground mt-0.5 flex size-5 shrink-0 items-center justify-center rounded-full" aria-hidden="true"><Check class="size-3.5" /></span>
                  <span class="text-muted-foreground text-sm line-through">{step.title}</span><span class="sr-only">(done)</span>
                {:else}
                  <span class="border-muted-foreground/40 mt-0.5 size-5 shrink-0 rounded-full border-2" aria-hidden="true"></span>
                  <div class="min-w-0 flex-1">
                    <a href={step.path} class="text-sm font-medium underline-offset-4 hover:underline">{step.title}</a>
                    {#if step.optional}<span class="text-muted-foreground ms-1 text-xs">optional</span>{/if}
                    <p class="text-muted-foreground text-xs">{step.text}</p>
                  </div>
                {/if}
              </li>
            {/each}
          </ol>
          <p class="border-t px-6 py-3 text-sm"><a href="/settings/help" class="underline-offset-4 hover:underline">How Runwell works</a></p>
        </Collapsible.Content>
      </Card.Root>
    </Collapsible.Root>
  {/if}

  <PulseCards {pulse} />

  <div class="grid items-start gap-4 lg:grid-cols-3">
    <!-- The wide column: what needs you, your engagements and the trend, as
         tabs (?tab=), so Home is about one screen tall. -->
    <Tabs.Root value={tab} onValueChange={(v) => go(withQuery({ tab: v === "needs" ? null : v }), undefined, { replace: true })} class="min-w-0 gap-4 lg:col-span-2">
      <Tabs.List aria-label="Home">
        <Tabs.Trigger value="needs">Needs you <span class="text-muted-foreground text-xs tabular-nums">{needs}</span></Tabs.Trigger>
        <Tabs.Trigger value="engagements">Your {session.term("engagement", 2).toLowerCase()} <span class="text-muted-foreground text-xs tabular-nums">{home.engagements.length}</span></Tabs.Trigger>
        <Tabs.Trigger value="trend">Last 12 weeks</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="needs">
        <PluginHome where="briefing" />
        <!-- One section at a time: the list of them down the side (along the
             top on a phone), its items beside it. -->
        <Card.Root class="gap-0 overflow-hidden py-0">
          <Tabs.Root orientation="vertical" value={need} onValueChange={(v) => go(withQuery({ need: v }), undefined, { replace: true })} class="flex-col gap-0 data-[orientation=vertical]:flex-col md:flex-row md:data-[orientation=vertical]:flex-row">
            <Tabs.List aria-label="What needs you" class="h-auto w-full shrink-0 flex-row justify-start gap-1 overflow-x-auto data-[orientation=vertical]:flex-row md:data-[orientation=vertical]:flex-col rounded-none border-b bg-transparent p-2 md:w-56 md:flex-col md:items-stretch md:border-e md:border-b-0">
              {#each data.sections as section (section.key)}
                <Tabs.Trigger value={section.key} class="data-[state=active]:bg-muted flex-none justify-between gap-3 px-2.5 py-1.5 data-[state=active]:shadow-none md:w-full">
                  <span class="truncate">{section.title}</span><span class="text-muted-foreground ms-auto text-xs tabular-nums">{section.count}</span>
                </Tabs.Trigger>
              {/each}
            </Tabs.List>
            {#each data.sections as section (section.key)}
              <Tabs.Content value={section.key} class="flex min-w-0 flex-col gap-3 p-4 md:p-6">
                <div class="flex items-center justify-between gap-2">
                  <h2 class="font-semibold">{section.title} <span class="text-muted-foreground font-normal tabular-nums">{section.count}</span></h2>
                  {#if more[section.key]}<a href={more[section.key]} class="text-muted-foreground text-sm hover:underline">See all</a>{/if}
                </div>
              <ul class="divide-y">
                {#each section.items as item, i (item.record || i)}
                  <li class="py-2.5 first:pt-0 last:pb-0">
                    {#if section.key === "questions"}
                      <HomeQuestion question={item} onchange={reload} />
                    {:else if section.key === "requests"}
                      <a href={`/requests/${item.id}`} class="block hover:underline">
                        <span class="text-sm font-medium">{item.subject}</span>
                        <span class="text-muted-foreground block text-xs">{[item.from || "Unknown sender", item.client?.display_name || "no client matched", item.source, timeAgo(item.received_at)].join(" · ")}</span>
                      </a>
                    {:else if section.key === "awaiting_client" || section.key === "drafts"}
                      <a href={path(item.url)} class="block hover:underline">
                        <span class="text-sm font-medium">{item.engagement.display_name}</span> <span class="text-muted-foreground text-sm">· {item.label}</span>
                        <span class="text-muted-foreground block text-xs">{item.client.display_name}{#if priced(item)} · {money(item.amount_cents)}{/if}{#if item.sent_at} · sent {timeAgo(item.sent_at)}{/if}</span>
                      </a>
                    {:else if section.key === "waiting_on_them" || section.key === "you_promised"}
                      <div class="flex flex-wrap items-center gap-2">
                        <div class="min-w-0 flex-1">
                          <span class="text-sm"><strong>{item.promiser}</strong> will {item.description.replace(/^\p{Lu}(?=\p{Ll})/u, (c) => c.toLowerCase())}</span>
                          <span class="text-muted-foreground block text-xs">{item.client.display_name}{#if item.engagement} · {item.engagement.ref}{/if} · {item.overdue ? "overdue since" : "by"} <span class={item.overdue ? "text-destructive" : ""}>{date(item.due_on)}</span></span>
                        </div>
                        {#each ["done", "missed", "dropped"] as r (r)}<Button variant="outline" size="xs" onclick={() => resolve(item, r)}>{humanize(r)}</Button>{/each}
                      </div>
                    {:else if section.key === "review" || section.key === "blocked" || section.key === "overdue_todos"}
                      <div class="flex items-center gap-2">
                        <a href={item.id ? `/work/${item.id}` : path(item.engagement.url)} class="block min-w-0 flex-1 hover:underline">
                          <span class="text-sm font-medium">{item.title}</span>
                          <span class="text-muted-foreground block text-xs">{item.engagement.display_name} · {item.owner?.name || "unowned"}{#if item.due_on} · due {date(item.due_on)}{/if} · {humanize(item.status)}</span>
                        </a>
                        {#if item.id && section.key !== "blocked"}<Button variant="outline" size="xs" onclick={() => finish(item)}><Check />Done</Button>{/if}
                      </div>
                    {:else}
                      <a href={path(item.url)} class="block text-sm hover:underline">{item.display_name || item.title || item.label}</a>
                    {/if}
                  </li>
                {/each}
              </ul>
              {#if section.count > section.items.length}
                <p class="text-muted-foreground mt-3 text-sm">
                  And {section.count - section.items.length} more.
                  {#if more[section.key]}<a href={more[section.key]} class="text-foreground underline-offset-4 hover:underline">See all {section.count}</a>{/if}
                </p>
              {/if}

              </Tabs.Content>
            {/each}
          </Tabs.Root>
        </Card.Root>
        {#if !data.sections.length}
          <EmptyState icon="check-circle" title="Nothing needs you right now." />
        {/if}
      </Tabs.Content>
      <Tabs.Content value="engagements">
        {#if home.engagements.length}
          <div class="grid gap-4 sm:grid-cols-2">
            {#each home.engagements as e (e.id)}
              <a href={e.path} class="group">
                <Card.Root class="group-hover:border-primary/40 h-full gap-3 transition-colors">
                  <Card.Header>
                    <Card.Description>{e.ref} · {e.client.name}</Card.Description>
                    <Card.Title class="truncate">{e.title}</Card.Title>
                    <Card.Action><StatusBadge status={e.health === "Behind" ? "overdue" : e.health === "Waiting" ? "sent" : e.health} label={e.health} /></Card.Action>
                  </Card.Header>
                  <Card.Content class="mt-auto flex flex-col gap-1.5">
                    <Progress value={e.done} max={Math.max(e.total, 1)} class="h-1.5" aria-label={`${e.done} of ${e.total} done`} />
                    <p class="text-muted-foreground flex justify-between text-xs tabular-nums"><span>{e.done} of {e.total} done</span><span>{e.open} open</span></p>
                  </Card.Content>
                </Card.Root>
              </a>
            {/each}
          </div>
          <p class="mt-3 text-sm"><a href="/engagements" class="text-muted-foreground hover:underline">All {session.term("engagement", 2).toLowerCase()}</a></p>
        {:else}
          <EmptyState icon="briefcase" title="None of yours are running."><a href="/engagements">All {session.term("engagement", 2).toLowerCase()}</a></EmptyState>
        {/if}
      </Tabs.Content>
      <Tabs.Content value="trend"><PulseChart weeks={pulse.weeks} /></Tabs.Content>
    </Tabs.Root>

    <!-- Down the side: your week, the team and the talk, as tabs of their own. -->
    <Tabs.Root bind:value={side} class="min-w-0 gap-4">
      <Tabs.List aria-label="Around you" class="w-full">
        <Tabs.Trigger value="coming">Coming up</Tabs.Trigger>
        {#if home.team}<Tabs.Trigger value="team">Team</Tabs.Trigger>{/if}
        <Tabs.Trigger value="talk">Discussion</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="coming"><ComingUp items={home.coming_up} today={home.date} onchange={reload} /></Tabs.Content>
      <Tabs.Content value="team">
        {#if home.team}
          <Card.Root class="gap-3">
            <Card.Header>
              <Card.Title>Who’s doing what</Card.Title>
              <Card.Action><span class="text-muted-foreground text-sm">It’s {clock(home.time)}</span></Card.Action>
            </Card.Header>
            <Card.Content>
              {#if home.team.length}
                <ul class="flex flex-col gap-3">
                  {#each home.team as person (person.user.id)}
                    <li class="flex items-start gap-2.5 text-sm">
                      <PersonAvatar person={person.user} size="size-7" />
                      <div class="min-w-0 flex-1">
                        <p class="font-medium">{person.user.name}</p>
                        {#each person.doing as t (t.id)}
                          <a href={t.path} class="text-muted-foreground hover:text-foreground block truncate">{t.title} <span class="text-xs">{t.ref}</span></a>
                        {/each}
                      </div>
                    </li>
                  {/each}
                </ul>
              {:else}
                <p class="text-muted-foreground text-sm">Nobody has {work} in progress.</p>
              {/if}
            </Card.Content>
          </Card.Root>
        {/if}
      </Tabs.Content>
      <Tabs.Content value="talk">
        <Card.Root class="gap-3">
          <Card.Header><Card.Title>Latest discussion</Card.Title></Card.Header>
          <Card.Content>
            {#if home.discussion.length}
              <ul class="flex flex-col gap-3">
                {#each home.discussion as note (note.id)}
                  <li class="flex items-start gap-2.5 text-sm">
                    <PersonAvatar person={note.author || { name: "Someone" }} size="size-7" />
                    <div class="min-w-0">
                      {#if note.path}<a href={note.path} class="line-clamp-2 hover:underline">{note.text}</a>{:else}<p class="line-clamp-2">{note.text}</p>{/if}
                      <span class="text-muted-foreground text-xs">{note.subject_label} · {timeAgo(note.occurred_at)}</span>
                    </div>
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="text-muted-foreground text-sm">Nothing new. Notes on {session.term("client", 2).toLowerCase()} and {session.term("engagement", 2).toLowerCase()} show up here.</p>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>
    </Tabs.Root>
  </div>
{/if}
