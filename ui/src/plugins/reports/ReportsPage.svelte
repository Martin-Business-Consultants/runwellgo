<script>
  // Reports: the business at a glance, on a map (Google Maps with a key,
  // else the built-in globe). The headline (how many clients, in how many
  // countries and time zones) over the map, which opens on the United
  // States or the whole world (the person's choice, kept in this browser),
  // a pin for each client, arcs from home; beside it the clients as a list
  // (the map's text alternative: j/k and Enter, like any list), which
  // becomes a client's card when one is picked (a bottom sheet on a phone).
  // Below, the snapshot in tabs: health, money, delivery, reach. Filters
  // (status, health, type) and the tab, the client and the sort are in the
  // address.
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import * as Sheet from "$lib/components/ui/sheet/index.js";
  import * as Popover from "$lib/components/ui/popover/index.js";
  import * as ToggleGroup from "$lib/components/ui/toggle-group/index.js";
  import Info from "@lucide/svelte/icons/info";
  import * as InputGroup from "$lib/components/ui/input-group/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Search from "@lucide/svelte/icons/search";
  import House from "@lucide/svelte/icons/house";
  import MapPin from "@lucide/svelte/icons/map-pin";
  import FilterMenu from "$lib/components/FilterMenu.svelte";
  import MapArea from "./MapArea.svelte";
  import ClientCard from "./ClientCard.svelte";
  import PlacePicker from "./PlacePicker.svelte";
  import HealthTab from "./HealthTab.svelte";
  import MoneyTab from "./MoneyTab.svelte";
  import DeliveryTab from "./DeliveryTab.svelte";
  import ReachTab from "./ReachTab.svelte";
  import { get, put } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, go, withQuery, href } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { rp, band, flag, placeName, timeIn, whole, typeName, savedView, saveView } from "./reports.js";

  page.set("Reports");

  let data = $state(last);
  let view = $state(savedView()), notice = $state(""), provider = $state("");
  const setView = (v) => { if (v) { view = v; saveView(v); } };
  let find = $state("");
  let placing = $state(null); // {id, lat, lng} while a pin is being dropped
  let savingPin = $state(false);
  let narrow = $state(typeof matchMedia !== "undefined" && matchMedia("(max-width: 1023px)").matches);
  let now = $state(new Date());

  const status = $derived(router.query.get("status") || "active");
  const health = $derived(router.query.get("health") || "all");
  const type = $derived(router.query.get("type") || "");
  const TABS = ["health", "money", "delivery", "reach"];
  const tab = $derived(TABS.includes(router.query.get("tab")) ? router.query.get("tab") : "health");
  const selectedId = $derived(Number(router.query.get("client")) || null);

  // The report as the filters have it: read again only when they change,
  // so picking a client or a tab keeps the page as it is.
  async function load() {
    try { data = last = await get(rp("/overview", { status, health, type })); } catch (e) { toastError(e); }
  }
  $effect(() => { status; health; type; load(); });
  $effect(() => {
    const m = matchMedia("(max-width: 1023px)");
    const on = () => (narrow = m.matches);
    m.addEventListener("change", on);
    const t = setInterval(() => (now = new Date()), 30000);
    return () => { m.removeEventListener("change", on); clearInterval(t); };
  });

  const clients = $derived(data?.clients || []);
  const selected = $derived(clients.find((c) => c.id === selectedId) || null);
  const t = $derived(data?.totals);
  const maxAgreed = $derived(Math.max(1, ...clients.map((c) => c.agreed_cents)));
  const pins = $derived(clients.filter((c) => c.place?.label).map((c) => ({
    id: c.id, name: c.name, lat: placing?.id === c.id ? placing.lat : c.place.lat, lng: placing?.id === c.id ? placing.lng : c.place.lng,
    color: band(c.health.band).color, band: c.health.band, hollow: c.place.approximate && placing?.id !== c.id, live: c.live,
    r: data.prices ? 4.5 + 8 * Math.sqrt(c.agreed_cents / maxAgreed) : 6,
  })));
  const listed = $derived(find.trim() ? clients.filter((c) => (c.name + " " + (c.place?.label || "") + " " + (c.place?.country || "")).toLowerCase().includes(find.trim().toLowerCase())) : clients);
  const globeLabel = $derived(data ? `A ${provider === "google" ? "map" : "globe"} with ${clients.length} ${session.term("client", clients.length).toLowerCase()} pinned where they are, and home in ${data.home.label}. The list beside it has the same ${session.term("client", 2).toLowerCase()}.` : "");

  const choose = (id) => go(withQuery({ client: id }), undefined, { replace: true, transition: false });
  const close = () => { placing = null; go(withQuery({ client: null }), undefined, { replace: true, transition: false }); };
  const filterTo = (param, v, fallback) => go(withQuery({ [param]: v === fallback ? null : v, client: null }), undefined, { replace: true });
  function showOnGlobe(id) {
    choose(id);
    document.getElementById("globe-hero")?.scrollIntoView({ behavior: matchMedia("(prefers-reduced-motion: reduce)").matches ? "auto" : "smooth", block: "start" });
  }

  // Dropping a client's pin: the globe shows it where it is now; a click or
  // a drag moves it; Save keeps it (the server names the place).
  function startPin(id = selectedId) {
    const c = clients.find((x) => x.id === id);
    if (!c) return;
    if (id !== selectedId) choose(id);
    placing = { id, lat: c.place?.lat ?? data.home.lat, lng: c.place?.lng ?? data.home.lng };
    document.getElementById("globe-hero")?.scrollIntoView({ block: "start" });
  }
  async function savePin() {
    savingPin = true;
    try {
      const r = await put(rp(`/clients/${placing.id}/place`), { place: { lat: placing.lat, lng: placing.lng, source: "pin" } });
      toast(r.summary);
      placing = null;
      await load();
    } catch (e) { toastError(e); }
    savingPin = false;
  }

  function keydown(e) {
    if (e.key !== "Escape" || document.querySelector("[role=dialog],[role=menu],[role=listbox]")) return;
    if (placing) { placing = null; e.preventDefault(); } else if (selectedId) { close(); e.preventDefault(); }
  }

  const statusOptions = $derived(data?.filters.status || []);
  const healthOptions = $derived(data?.filters.health || []);
  const typeOptions = $derived([{ value: "", label: "Every type" }, ...(data?.filters.type || []).map((o) => ({ ...o, label: typeName(o.value) }))]);
  const filtered = $derived(status !== "active" || health !== "all" || type !== "");
</script>

<script module>
  // The report as it last was, so coming back draws it at once.
  let last = null;
</script>

<svelte:window onkeydown={keydown} />

{#snippet n(value, word, link)}
  <a href={link} class="text-foreground decoration-primary/40 underline-offset-[6px] hover:underline">{value} {word}</a>
{/snippet}

<div class="flex flex-wrap items-center gap-2">
  <FilterMenu title="Status" options={statusOptions} current={status} align="start" onpick={(v) => filterTo("status", v, "active")} />
  <FilterMenu title="Health" options={healthOptions} current={health} align="start" onpick={(v) => filterTo("health", v, "all")} />
  {#if typeOptions.length > 1}<FilterMenu title="Type" options={typeOptions} current={type} align="start" onpick={(v) => filterTo("type", v, "")} />{/if}
  {#if filtered}<Button variant="ghost" size="sm" href={href("/reports", { tab: router.query.get("tab") })}>Clear filters</Button>{/if}
</div>

{#if !data}
  <Skeleton class="h-[560px] rounded-xl" />
{:else}
  <section id="globe-hero" class="bg-card relative grid scroll-mt-4 overflow-hidden rounded-xl border lg:grid-cols-[minmax(0,1fr)_380px]" aria-label="Where your {session.term('client', 2).toLowerCase()} are">
    <div class="relative h-[min(68svh,620px)] min-h-[340px] overflow-hidden bg-[radial-gradient(ellipse_at_top_left,color-mix(in_oklab,var(--primary)_9%,transparent),transparent_60%)] lg:h-[640px]">
      <MapArea {pins} home={data.home} selected={selectedId} placing={placing} label={globeLabel} {view} bind:notice bind:provider
        onselect={(id) => (placing ? null : choose(id))} onplace={(p) => (placing = { ...placing, ...p })} />

      <div class="from-card via-card/75 pointer-events-none absolute inset-x-0 top-0 bg-gradient-to-b to-transparent p-4 pb-16 sm:p-6 sm:pb-20">
        <div class="pointer-events-auto max-w-xl">
          {#if !data.any_clients}
            <h1 class="text-2xl font-bold tracking-tight sm:text-3xl">Your work, around the world</h1>
            <p class="text-muted-foreground mt-2 text-sm">Each {session.term("client").toLowerCase()} becomes a pin here. <a class="text-primary underline-offset-4 hover:underline" href="/clients/new">Add your first {session.term("client").toLowerCase()}</a>.</p>
          {:else if !t.clients}
            <h1 class="text-2xl font-bold tracking-tight sm:text-3xl">No {session.term("client", 2).toLowerCase()} match these filters</h1>
            <p class="mt-2 text-sm"><a class="text-primary underline-offset-4 hover:underline" href={href("/reports")}>Clear filters</a></p>
          {:else}
            <h1 class="text-muted-foreground text-2xl leading-tight font-semibold tracking-tight text-balance sm:text-[2rem]">
              You serve {@render n(t.clients, session.term("client", t.clients).toLowerCase(), `/clients?status=${status === "all" ? "" : status}`)}{#if t.countries}{" "}in <span class="text-foreground">{t.countries} {t.countries === 1 ? "country" : "countries"}</span>{/if}{#if t.time_zones > 1}{" "}across <span class="text-foreground">{t.time_zones} time zones</span>{/if}
            </h1>
            <p class="text-muted-foreground mt-2 flex flex-wrap gap-x-3 gap-y-1 text-sm">
              {#if data.prices}<span><a href="?tab=money" onclick={(e) => { e.preventDefault(); go(withQuery({ tab: "money" }), undefined, { replace: true, transition: false }); document.getElementById("snapshot")?.scrollIntoView({ block: "start" }); }} class="text-foreground font-semibold tabular-nums hover:underline">{whole(t.agreed_cents)}</a> agreed</span>{/if}
              {#if data.prices && t.monthly_recurring_cents}<span><span class="text-foreground font-semibold tabular-nums">{whole(t.monthly_recurring_cents)}</span> a month recurring</span>{/if}
              <span><a href="/work" class="text-foreground font-semibold tabular-nums hover:underline">{t.open_work}</a> open {session.term("work").toLowerCase()}</span>
              {#if t.at_risk}<span><button type="button" class="text-destructive font-semibold tabular-nums hover:underline" onclick={() => filterTo("health", "at_risk", "all")}>{t.at_risk} at risk</button></span>{/if}
            </p>
          {/if}
          <ToggleGroup.Root type="single" variant="outline" size="sm" class="bg-background/85 mt-3 backdrop-blur" value={view} onValueChange={setView} aria-label="Show on the map">
            <ToggleGroup.Item value="us" class="px-3">United States</ToggleGroup.Item>
            <ToggleGroup.Item value="world" class="px-3">Whole world</ToggleGroup.Item>
          </ToggleGroup.Root>
        </div>
      </div>

      {#if placing}
        {@const pc = clients.find((c) => c.id === placing.id)}
        <div class="bg-popover absolute inset-x-3 bottom-3 z-10 flex flex-wrap items-center gap-2 rounded-lg border p-3 shadow-lg sm:inset-x-auto sm:left-1/2 sm:w-[min(34rem,calc(100%-6rem))] sm:-translate-x-1/2" role="status">
          <MapPin class="text-primary size-4" aria-hidden="true" />
          <p class="min-w-0 flex-1 text-sm">Click the {provider === "google" ? "map" : "globe"} where <strong>{pc?.name}</strong> is, or drag the pin.</p>
          <Button size="sm" onclick={savePin} disabled={savingPin}>Save the place</Button>
          <Button size="sm" variant="ghost" onclick={() => (placing = null)}>Cancel</Button>
        </div>
      {:else}
        <!-- The key -->
        <div class="text-muted-foreground pointer-events-none absolute bottom-3 left-3 flex max-w-[calc(100%-4.5rem)] flex-wrap items-center gap-x-3 gap-y-1 rounded-lg bg-background/70 px-2.5 py-1.5 text-xs backdrop-blur sm:bottom-4 sm:left-4">
          {#each ["on_track", "watch", "at_risk"] as b (b)}
            <span class="flex items-center gap-1.5"><span class="size-2.5 rounded-full" style:background={band(b).color} aria-hidden="true"></span>{band(b).label}</span>
          {/each}
          <span class="flex items-center gap-1.5"><span class="border-muted-foreground size-2.5 rounded-full border-2" aria-hidden="true"></span>By time zone</span>
          <span class="pointer-events-auto flex items-center gap-1.5">
            <span class="bg-primary ring-primary/25 size-2.5 rounded-full ring-4" aria-hidden="true"></span>
            {#if data.home.from === "internal"}
              <a href={`/clients/${data.home.client_id}`} class="hover:text-foreground underline-offset-2 hover:underline" title="Home is {data.home.client_name}, the {session.term('client').toLowerCase()} marked as us">Home: {data.home.label}</a>
            {:else if session.can("manage_settings")}
              <Popover.Root>
                <Popover.Trigger class="hover:text-foreground underline-offset-2 hover:underline">Home: {data.home.label}</Popover.Trigger>
                <Popover.Content class="w-80" align="start">
                  <p class="text-sm font-medium">Where you are</p>
                  <p class="text-muted-foreground mt-1 mb-2 text-xs">
                    {#if data.home.client_id}Home is where <a class="underline underline-offset-2" href={`/clients/${data.home.client_id}`}>{data.home.client_name}</a> is: give it an address and this follows. Until then, set it here.
                    {:else}Home is the {session.term("client").toLowerCase()} marked “This is us”, once it has an address. With none, set it here.{/if}
                  </p>
                  <PlacePicker path="/home" place={data.home.from === "override" ? data.home : null} onsaved={() => load()} />
                </Popover.Content>
              </Popover.Root>
            {:else}Home: {data.home.label}{/if}
          </span>
          {#if notice}
            <span class="pointer-events-auto flex basis-full items-start gap-1.5"><Info class="mt-px size-3.5 shrink-0" aria-hidden="true" />
              {#if session.can("manage_settings") && !notice.startsWith("Google")}<a href="/settings/reports" class="hover:text-foreground underline-offset-2 hover:underline">{notice}</a>{:else}{notice}{/if}
            </span>
          {/if}
        </div>
      {/if}
    </div>

    <!-- The clients: the list, or the one picked -->
    <div class="flex min-h-0 flex-col border-t lg:h-[640px] lg:border-t-0 lg:border-l">
      {#if selected && !narrow}
        <div class="min-h-0 flex-1 overflow-y-auto p-4 sm:p-5">
          <ClientCard row={selected} onclose={close} onpin={() => startPin(selected.id)} onchanged={load} />
        </div>
      {:else}
        <div class="flex items-center gap-2 border-b p-3">
          <h2 class="sr-only">{session.term("client", 2)}</h2>
          <InputGroup.Root class="h-8">
            <InputGroup.Addon><Search /></InputGroup.Addon>
            <InputGroup.Input bind:value={find} placeholder="Find a {session.term('client').toLowerCase()} or place…" aria-label="Find a {session.term('client').toLowerCase()} or place" data-nav-skip />
          </InputGroup.Root>
        </div>
        {#if t.approximate && !find}
          <button type="button" class="hover:bg-muted/60 flex items-center gap-2 border-b px-4 py-2.5 text-left text-sm" onclick={() => startPin(clients.find((c) => c.place?.approximate)?.id)}>
            <MapPin class="text-primary size-4 shrink-0" aria-hidden="true" />
            <span class="min-w-0 flex-1"><span class="font-medium">Put your {session.term("client", 2).toLowerCase()} on the map.</span> <span class="text-muted-foreground">{t.approximate} {t.approximate === 1 ? "is" : "are"} only placed by time zone.</span></span>
          </button>
        {/if}
        <ul class="min-h-0 flex-1 overflow-y-auto max-lg:max-h-[420px]" aria-label="{session.term('client', 2)}, at risk first">
          {#each listed as c (c.id)}
            {@const b = band(c.health.band)}
            <li class="border-b last:border-b-0">
              <button type="button" class="hover:bg-muted/60 focus-visible:bg-muted aria-[current=true]:bg-accent flex w-full items-center gap-3 px-4 py-2.5 text-left outline-none" aria-current={c.id === selectedId} onclick={() => choose(c.id)}>
                <span class="size-2.5 shrink-0 rounded-full {c.place?.approximate ? 'border-2 bg-transparent' : ''}" style:background={c.place?.approximate ? undefined : b.color} style:border-color={b.color} aria-hidden="true"></span>
                <span class="min-w-0 flex-1">
                  <span class="block truncate text-sm font-medium">{c.name}</span>
                  <span class="text-muted-foreground block truncate text-xs">{flag(c.place?.country_code)} {placeName(c.place)} · {timeIn(c.iana, now)}</span>
                </span>
                <span class="flex shrink-0 flex-col items-end">
                  <span class="text-sm font-semibold tabular-nums {b.text}">{c.health.score}</span>
                  <span class="text-muted-foreground text-[11px]">{b.label}</span>
                </span>
              </button>
            </li>
          {:else}
            <li class="text-muted-foreground p-6 text-center text-sm">{find ? "No one matches that." : `No ${session.term("client", 2).toLowerCase()} here.`}</li>
          {/each}
        </ul>
      {/if}
    </div>
  </section>

  {#if narrow}
    <Sheet.Root open={!!selected && !placing} onOpenChange={(o) => { if (!o) close(); }}>
      <Sheet.Content side="bottom" class="max-h-[62svh] overflow-y-auto rounded-t-xl p-4 pb-8">
        <Sheet.Header class="sr-only"><Sheet.Title>{selected?.name}</Sheet.Title><Sheet.Description>How they're doing, and what to do next.</Sheet.Description></Sheet.Header>
        {#if selected}<ClientCard row={selected} sheet onclose={close} onpin={() => startPin(selected.id)} onchanged={load} />{/if}
      </Sheet.Content>
    </Sheet.Root>
  {/if}

  <section id="snapshot" class="scroll-mt-4" aria-label="The snapshot">
    <Tabs.Root value={tab} onValueChange={(v) => go(withQuery({ tab: v === "health" ? null : v }), undefined, { replace: true, transition: false })}>
      <Tabs.List>
        <Tabs.Trigger value="health">Health</Tabs.Trigger>
        <Tabs.Trigger value="money">Money</Tabs.Trigger>
        <Tabs.Trigger value="delivery">Delivery</Tabs.Trigger>
        <Tabs.Trigger value="reach">Reach</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="health" class="mt-3"><HealthTab {clients} onshow={showOnGlobe} /></Tabs.Content>
      <Tabs.Content value="money" class="mt-3"><MoneyTab money={data.money} /></Tabs.Content>
      <Tabs.Content value="delivery" class="mt-3"><DeliveryTab delivery={data.delivery} /></Tabs.Content>
      <Tabs.Content value="reach" class="mt-3"><ReachTab reach={data.reach} {clients} totals={t} onshow={showOnGlobe} onplace={startPin} /></Tabs.Content>
    </Tabs.Root>
  </section>
{/if}
