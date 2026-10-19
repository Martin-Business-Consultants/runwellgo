<script>
  // Where a client is: look a place up (OpenStreetMap, as you type, after a
  // pause) and pick it, or drop the pin on the map (onpin), or go back to
  // their address (hasAddress) or their time zone's city. Saving answers
  // the place (onsaved).
  //   clientId; place: where they are now; path: where it saves (the
  //   client's place, or home's override)
  import * as InputGroup from "$lib/components/ui/input-group/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Spinner } from "$lib/components/ui/spinner/index.js";
  import Search from "@lucide/svelte/icons/search";
  import MapPin from "@lucide/svelte/icons/map-pin";
  import { get, put, del } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { rp, flag, placeName } from "./reports.js";

  let { clientId = null, place = null, path = null, hasAddress = false, onsaved = () => {}, onpin = null, autofocus = false } = $props();
  const target = $derived(path || `/clients/${clientId}/place`);

  let q = $state(""), results = $state(null), busy = $state(false), saving = $state(false), error = $state("");
  let timer, asked = 0;
  function typed() {
    clearTimeout(timer);
    error = "";
    if (q.trim().length < 2) { results = null; return; }
    // The geocoder takes a request a second: wait for a pause in typing.
    timer = setTimeout(look, 600);
  }
  async function look() {
    const n = ++asked;
    busy = true;
    try {
      const r = await get(rp("/places", { q: q.trim() }));
      if (n === asked) results = r.places;
    } catch (e) { if (n === asked) error = e.message || "Couldn't look places up just now."; }
    if (n === asked) busy = false;
  }
  async function choose(p) {
    saving = true;
    try {
      const r = await put(rp(target), { place: { ...p, source: "search" } });
      toast(r.summary);
      q = ""; results = null;
      onsaved(r.place || r.home);
    } catch (e) { toastError(e); }
    saving = false;
  }
  async function forget() {
    try {
      const r = await del(rp(target));
      toast(r.summary);
      onsaved(r.home || null);
    } catch (e) { toastError(e); }
  }
  const forgettable = $derived(place && !place.approximate && place.source !== "address");
</script>

<div class="flex flex-col gap-2">
  <InputGroup.Root>
    <InputGroup.Addon><Search /></InputGroup.Addon>
    <!-- svelte-ignore a11y_autofocus -->
    <InputGroup.Input bind:value={q} oninput={typed} placeholder="Find a city or address…" aria-label="Find a place" autocomplete="off" {autofocus}
      onkeydown={(e) => { if (e.key === "Enter") { e.preventDefault(); clearTimeout(timer); look(); } }} />
    {#if busy}<InputGroup.Addon align="inline-end"><Spinner /></InputGroup.Addon>{/if}
  </InputGroup.Root>
  {#if error}<p class="text-destructive text-sm" role="alert">{error}</p>{/if}
  {#if results}
    {#if results.length}
      <ul class="flex flex-col overflow-hidden rounded-md border" aria-label="Places found">
        {#each results as p, i (i)}
          <li class="border-b last:border-b-0">
            <button type="button" class="hover:bg-muted focus-visible:bg-muted flex w-full items-center gap-2 px-3 py-2 text-left text-sm outline-none disabled:opacity-60" disabled={saving} onclick={() => choose(p)}>
              <span aria-hidden="true">{flag(p.country_code) || "📍"}</span>
              <span class="min-w-0 flex-1 truncate"><span class="font-medium">{p.label}</span>{#if p.region}<span class="text-muted-foreground">, {p.region}</span>{/if}{#if p.country}<span class="text-muted-foreground">, {p.country}</span>{/if}</span>
            </button>
          </li>
        {/each}
      </ul>
    {:else if !busy}
      <p class="text-muted-foreground text-sm">No place matches that. Try a city and its country{onpin ? ", or drop the pin on the map" : ""}.</p>
    {/if}
  {/if}
  <div class="flex flex-wrap items-center gap-2">
    {#if onpin}<Button variant="outline" size="sm" onclick={onpin}><MapPin />Drop a pin on the map</Button>{/if}
    {#if forgettable}<Button variant="ghost" size="sm" onclick={forget}>{path === "/home" ? "Clear it" : hasAddress ? "Use their address again" : "Place by their time zone"}</Button>{/if}
  </div>
  <p class="text-muted-foreground text-xs">Places from <a href="https://www.openstreetmap.org/copyright" target="_blank" rel="noopener" data-external class="underline underline-offset-2">OpenStreetMap</a>. {place ? `Now: ${placeName(place, { country: true })}.` : ""}</p>
</div>
