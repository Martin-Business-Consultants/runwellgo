<script>
  // A client at a glance, beside the globe (a bottom sheet on a phone):
  // where they are and the time there, health and why, money, engagements
  // and how far along, what's open and late, the last touch, who has the
  // portal; and the ways to act on it (open them, a note, work, a request,
  // a commitment, a check-in to send, their connections, their place).
  //   row: the client as the report has it (drawn at once); onclose;
  //   onpin: start dropping their pin on the globe; onchanged: reload the
  //   report (a place or a note changed it)
  import { untrack } from "svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import X from "@lucide/svelte/icons/x";
  import ArrowUpRight from "@lucide/svelte/icons/arrow-up-right";
  import Plus from "@lucide/svelte/icons/plus";
  import Copy from "@lucide/svelte/icons/copy";
  import MapPin from "@lucide/svelte/icons/map-pin";
  import Moon from "@lucide/svelte/icons/moon";
  import Sun from "@lucide/svelte/icons/sun";
  import NotebookPen from "@lucide/svelte/icons/notebook-pen";
  import ConnectionsButton from "$lib/components/ConnectionsButton.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import PlacePicker from "./PlacePicker.svelte";
  import { get, post, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { timeAgo, date } from "$lib/format.js";
  import { rp, band, flag, placeName, placeSource, setByHand, timeIn, offsetFromHere, awake, whole, typeName } from "./reports.js";

  let { row, onclose = () => {}, onpin = null, onchanged = () => {}, sheet = false } = $props();
  let data = $state(null), noteOpen = $state(false), note = $state(""), saving = $state(false), placeOpen = $state(false);
  let now = $state(new Date());

  let asked = 0;
  async function load() {
    const n = ++asked;
    try {
      const d = await get(rp(`/clients/${id}`));
      if (n === asked) data = d;
    } catch (e) { toastError(e); }
  }
  // A new client starts afresh; the same one (the report read again) keeps its card.
  const id = $derived(row.id);
  $effect(() => { id; data = null; placeOpen = false; noteOpen = false; untrack(load); });
  // The report read again with a new place or score: read the card again too.
  const changed = $derived(`${row.place?.lat},${row.place?.lng},${row.health.score}`);
  let seen = "";
  $effect(() => { if (seen && changed !== seen) untrack(load); seen = changed; });
  $effect(() => { const t = setInterval(() => (now = new Date()), 30000); return () => clearInterval(t); });

  const c = $derived(data?.client || row);
  const h = $derived(c.health);
  const b = $derived(band(h.band));
  const openEngagements = $derived((data?.engagements || []).filter((e) => !e.closed));
  const firstRef = $derived(openEngagements[0]?.ref);
  const prices = $derived(session.me?.prices !== false && data?.prices !== false);

  async function saveNote() {
    if (!note.trim()) return;
    saving = true;
    try {
      await post(rp(`/clients/${row.id}/notes`), { note: { body: note, kind: "internal" } });
      toast("Note added.");
      note = ""; noteOpen = false;
      load(); onchanged();
    } catch (e) { toastError(e); }
    saving = false;
  }
  async function copyCheckin() {
    try { await navigator.clipboard.writeText(data.checkin); toast("Check-in copied: paste it into an email."); } catch { toast("Couldn't copy: select the text instead."); }
  }
  const touchWord = { note: "a note", work: "work done", approval: "a decision", request: "a request" };

  // Set by hand, with an address found: go back to it.
  async function useAddress() {
    try {
      const r = await del(rp(`/clients/${row.id}/place`));
      toast(r.summary);
      load(); onchanged();
    } catch (e) { toastError(e); }
  }
</script>

<section class="flex min-h-0 flex-col" aria-labelledby="card-{row.id}">
  <header class="flex items-start gap-3 {sheet ? 'pe-8' : ''}">
    <div class="min-w-0 flex-1">
      <h2 id="card-{row.id}" class="truncate text-lg font-semibold tracking-tight">{c.name}</h2>
      <p class="text-muted-foreground flex flex-wrap items-center gap-x-2 text-sm">
        <span>{flag(c.place?.country_code)} {placeName(c.place, { country: true })}{#if !c.place?.approximate}{" "}<button type="button" class="text-muted-foreground hover:text-foreground underline underline-offset-2" onclick={() => (placeOpen = !placeOpen)} aria-expanded={placeOpen}>Change</button>{/if}</span>
        {#if c.iana}
          <span class="inline-flex items-center gap-1 tabular-nums" title={c.iana}>
            {#if awake(c.iana, now)}<Sun class="size-3.5" aria-hidden="true" />{:else}<Moon class="size-3.5" aria-hidden="true" />{/if}
            {timeIn(c.iana, now)} there<span class="sr-only">, {offsetFromHere(c.iana, now)}</span>
          </span>
        {/if}
      </p>
      {#if c.place?.label && !c.place.approximate}
        <p class="text-muted-foreground mt-0.5 flex flex-wrap items-center gap-x-2 text-xs">
          <span title={c.place.source === "address" ? c.place.address : undefined}>{placeSource(c.place)}{#if setByHand(c.place) && c.place.set_by}{" "}· {c.place.set_by}{/if}</span>
          {#if setByHand(c.place) && c.address_status === "ok"}<button type="button" class="hover:text-foreground underline underline-offset-2" onclick={useAddress}>Use their address again</button>{/if}
        </p>
      {/if}
    </div>
    {#if !sheet}<Button variant="ghost" size="icon-sm" onclick={onclose} aria-label="Close {c.name}"><X /></Button>{/if}
  </header>

  {#if c.place?.approximate}
    <div class="bg-muted/60 mt-3 flex flex-wrap items-center gap-2 rounded-lg px-3 py-2 text-sm">
      <MapPin class="text-muted-foreground size-4" aria-hidden="true" />
      <span class="min-w-0 flex-1">{placeSource(c.place)}.{#if c.address_status === "not_found"}{" "}Their address couldn’t be found on the map.{:else}{" "}Where are they really?{/if}</span>
      <Button variant="outline" size="sm" onclick={() => (placeOpen = !placeOpen)} aria-expanded={placeOpen}>Set the place</Button>
    </div>
  {/if}
  {#if placeOpen}
    <div class="mt-3 rounded-lg border p-3">
      <PlacePicker clientId={row.id} place={c.place} hasAddress={c.address_status === "ok"} autofocus onpin={onpin ? () => { placeOpen = false; onpin(); } : null}
        onsaved={() => { placeOpen = false; load(); onchanged(); }} />
    </div>
  {/if}

  <!-- Health, and why -->
  <div class="mt-4 flex items-center gap-4">
    <div class="relative grid size-16 shrink-0 place-items-center" role="img" aria-label="Health {h.score} out of 100, {b.label.toLowerCase()}">
      <svg viewBox="0 0 36 36" class="absolute inset-0 -rotate-90" aria-hidden="true">
        <circle cx="18" cy="18" r="15.5" fill="none" stroke="var(--muted)" stroke-width="3" />
        <circle cx="18" cy="18" r="15.5" fill="none" stroke={b.color} stroke-width="3" stroke-linecap="round" stroke-dasharray="{(h.score / 100) * 97.4} 97.4" />
      </svg>
      <span class="text-xl font-bold tabular-nums">{h.score}</span>
    </div>
    <div class="min-w-0">
      <Badge variant="outline" class={b.badge}>{b.label}</Badge>
      <p class="text-muted-foreground mt-1 text-sm">
        {#if c.days_since_touch < 0}Nothing logged with them yet.
        {:else}Last touch: {touchWord[c.last_touch_kind] || "activity"}, {c.days_since_touch === 0 ? "today" : timeAgo(c.last_touch_at)}.{/if}
      </p>
    </div>
  </div>
  {#if h.reasons.length}
    <ul class="mt-3 flex flex-col gap-1 text-sm" aria-label="Why">
      {#each h.reasons as r (r.text)}
        <li class="flex items-baseline gap-2">
          <span class="w-8 shrink-0 text-right text-xs font-semibold tabular-nums {r.points < 0 ? 'text-destructive' : 'text-emerald-700 dark:text-emerald-400'}">{r.points > 0 ? "+" : "−"}{Math.abs(r.points)}</span>
          {#if r.href}<a href={r.href} class="hover:underline">{r.text}</a>{:else}<span>{r.text}</span>{/if}
        </li>
      {/each}
    </ul>
  {:else}
    <p class="text-muted-foreground mt-3 text-sm">Nothing late, nothing waiting, touched lately: all well.</p>
  {/if}

  <!-- The ways to act -->
  <div class="mt-4 flex flex-wrap gap-2">
    <Button href={c.url} size="sm"><ArrowUpRight />Open {session.term("client").toLowerCase()}</Button>
    <Button variant="outline" size="sm" onclick={() => (noteOpen = !noteOpen)} aria-expanded={noteOpen}><NotebookPen />Add a note</Button>
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm"><Plus />Add</Button>{/snippet}</DropdownMenu.Trigger>
      <DropdownMenu.Content align="start">
        <DropdownMenu.Item disabled={!firstRef}>{#snippet child({ props })}<a {...props} href={firstRef ? `/work/new?engagement=${firstRef}` : undefined}>{session.term("work")}</a>{/snippet}</DropdownMenu.Item>
        <DropdownMenu.Item>{#snippet child({ props })}<a {...props} href={`${c.url}?tab=commitments`}>{session.term("commitment")}</a>{/snippet}</DropdownMenu.Item>
        <DropdownMenu.Item>{#snippet child({ props })}<a {...props} href="/requests/new">Request</a>{/snippet}</DropdownMenu.Item>
        <DropdownMenu.Item>{#snippet child({ props })}<a {...props} href={`/engagements/new?client_id=${c.id}`}>{session.term("engagement")}</a>{/snippet}</DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
    <Button variant="outline" size="sm" onclick={copyCheckin} disabled={!data}><Copy />Copy a check-in</Button>
    <ConnectionsButton record={`Client:${c.id}`} name={c.name} />
  </div>
  {#if noteOpen}
    <div class="mt-3 flex flex-col gap-2">
      <Textarea bind:value={note} rows={3} placeholder="What happened, what's next…" aria-label="Note" />
      <div class="flex gap-2"><Button size="sm" onclick={saveNote} disabled={saving || !note.trim()}>Add note</Button><Button size="sm" variant="ghost" onclick={() => (noteOpen = false)}>Cancel</Button></div>
    </div>
  {/if}

  {#if !data}
    <div class="mt-5 flex flex-col gap-3"><Skeleton class="h-20" /><Skeleton class="h-24" /></div>
  {:else}
    {#if prices}
      <div class="mt-5 grid grid-cols-3 gap-2 rounded-lg border p-3 text-sm">
        <div><p class="text-muted-foreground text-xs">Agreed</p><p class="font-semibold tabular-nums">{whole(c.agreed_cents)}</p></div>
        <div><p class="text-muted-foreground text-xs">This year</p><p class="font-semibold tabular-nums">{whole(c.this_year_cents)}</p></div>
        <div><p class="text-muted-foreground text-xs">Waiting on them</p><p class="font-semibold tabular-nums">{whole(c.pending_cents)}</p></div>
      </div>
    {/if}

    <h3 class="mt-5 mb-2 text-sm font-semibold">{session.term("engagement", 2)}</h3>
    {#if data.engagements.length}
      <ul class="flex flex-col gap-3">
        {#each data.engagements.slice(0, 6) as e (e.id)}
          <li class="flex flex-col gap-1.5 {e.closed ? 'opacity-70' : ''}">
            <div class="flex items-baseline gap-2 text-sm">
              <a href={e.url} class="min-w-0 flex-1 truncate font-medium hover:underline"><span class="text-muted-foreground tabular-nums">{e.ref}</span> {e.title}</a>
              <StatusBadge status={e.closed ? e.close_outcome || "closed" : e.state} />
            </div>
            <div class="flex items-center gap-2">
              <Progress value={e.progress} class="h-1.5 flex-1" aria-label="{e.title}: {e.progress}% of the work done" />
              <span class="text-muted-foreground w-24 text-right text-xs tabular-nums">{e.work_done} of {e.work_done + e.work_open} done</span>
            </div>
            <p class="text-muted-foreground text-xs">
              {typeName(e.label, 1)}{#if prices && e.agreed_cents}{" · "}{whole(e.agreed_cents)}{#if e.shape === "recurring"}{" "}{e.cadence || "monthly"}{/if}{/if}{#if e.work_late}{" · "}<span class="text-destructive">{e.work_late} late</span>{/if}
            </p>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="text-muted-foreground text-sm">Nothing agreed yet. <a class="underline-offset-4 hover:underline" href={`/engagements/new?client_id=${c.id}`}>Start one</a>.</p>
    {/if}

    <div class="mt-5 grid gap-4 {sheet ? 'sm:grid-cols-2' : ''}">
      <div>
        <h3 class="mb-1.5 text-sm font-semibold">Open {session.term("work").toLowerCase()} <span class="text-muted-foreground font-normal tabular-nums">{c.open_work}</span>{#if c.late_work}<span class="text-destructive font-normal">{" · "}{c.late_work} late</span>{/if}</h3>
        {#if data.work.length}
          <ul class="flex flex-col gap-1 text-sm">
            {#each data.work.slice(0, 5) as w (w.id)}
              <li class="flex gap-2"><a href={w.path} class="min-w-0 flex-1 truncate hover:underline">{w.title}</a>{#if w.due_on}<span class="shrink-0 text-xs tabular-nums {w.overdue ? 'text-destructive' : 'text-muted-foreground'}">{date(w.due_on)}</span>{/if}</li>
            {/each}
          </ul>
        {:else}<p class="text-muted-foreground text-sm">None open.</p>{/if}
      </div>
      <div>
        <h3 class="mb-1.5 text-sm font-semibold">{session.term("commitment", 2)} <span class="text-muted-foreground font-normal tabular-nums">{c.open_commitments}</span>{#if c.due_soon_commitments}<span class="text-muted-foreground font-normal">{" · "}{c.due_soon_commitments} due this week</span>{/if}</h3>
        {#if data.commitments.length}
          <ul class="flex flex-col gap-1 text-sm">
            {#each data.commitments.slice(0, 5) as m (m.id)}
              <li class="flex gap-2"><a href={m.path} class="min-w-0 flex-1 truncate hover:underline">{m.description}</a><span class="shrink-0 text-xs tabular-nums {m.overdue ? 'text-destructive' : 'text-muted-foreground'}">{date(m.due_on)}</span></li>
            {/each}
          </ul>
        {:else}<p class="text-muted-foreground text-sm">None open.</p>{/if}
      </div>
      <div>
        <h3 class="mb-1.5 text-sm font-semibold">Requests <span class="text-muted-foreground font-normal tabular-nums">{c.open_requests}</span></h3>
        {#if data.requests.length}
          <ul class="flex flex-col gap-1 text-sm">
            {#each data.requests.slice(0, 4) as r (r.id)}
              <li class="flex gap-2"><a href={r.path} class="min-w-0 flex-1 truncate hover:underline">{r.subject}</a><span class="text-muted-foreground shrink-0 text-xs">{timeAgo(r.received_at)}</span></li>
            {/each}
          </ul>
        {:else}<p class="text-muted-foreground text-sm">Nothing to triage.</p>{/if}
      </div>
      <div>
        <h3 class="mb-1.5 text-sm font-semibold">People <span class="text-muted-foreground font-normal tabular-nums">{data.contacts.length}</span></h3>
        {#if data.contacts.length}
          <ul class="flex flex-col gap-1 text-sm">
            {#each data.contacts.slice(0, 5) as p (p.id)}
              <li class="flex items-center gap-2"><span class="min-w-0 flex-1 truncate">{p.name}{#if p.role}<span class="text-muted-foreground">, {p.role}</span>{/if}</span>{#if p.portal_access}<Badge variant="secondary" class="shrink-0">Portal</Badge>{/if}</li>
            {/each}
          </ul>
        {:else}<p class="text-muted-foreground text-sm">No one yet.</p>{/if}
      </div>
    </div>

    <Collapsible.Root class="mt-5">
      <Collapsible.Trigger class="text-muted-foreground hover:text-foreground text-sm underline-offset-4 hover:underline">The check-in it copies</Collapsible.Trigger>
      <Collapsible.Content><pre class="bg-muted/50 mt-2 rounded-md p-3 font-sans text-sm whitespace-pre-wrap">{data.checkin}</pre></Collapsible.Content>
    </Collapsible.Root>
  {/if}
</section>
