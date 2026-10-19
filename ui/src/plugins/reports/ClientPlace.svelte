<script>
  // "Where they are", in a client's side column: their place, where it came
  // from (set by hand, their address, or their time zone) and the time
  // there, set or changed by a search, or on the map in Reports.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import MapIcon from "@lucide/svelte/icons/map";
  import PlacePicker from "./PlacePicker.svelte";
  import { get, del } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { rp, flag, placeName, placeSource, setByHand, timeIn, offsetFromHere } from "./reports.js";

  let { record } = $props();
  let client = $state(null), loaded = $state(false), editing = $state(false), failed = $state(false);
  let now = $state(new Date());
  const place = $derived(client?.place), iana = $derived(client?.iana || "");

  async function load() {
    try {
      const d = await get(rp(`/clients/${record.id}`), { quiet: true });
      client = d.client;
    } catch { failed = true; }
    loaded = true;
  }
  async function useAddress() {
    try { toast((await del(rp(`/clients/${record.id}/place`))).summary); load(); } catch (e) { toastError(e); }
  }
  $effect(() => { record.id; load(); });
  $effect(() => { const t = setInterval(() => (now = new Date()), 30000); return () => clearInterval(t); });
</script>

{#if !failed}
  <Card.Root>
    <Card.Header>
      <Card.Title>Where they are</Card.Title>
      {#if loaded && place && !editing}
        <Card.Action><Button variant="ghost" size="sm" onclick={() => (editing = true)}>{place.approximate ? "Set" : "Change"}</Button></Card.Action>
      {/if}
    </Card.Header>
    <Card.Content class="flex flex-col gap-3">
      {#if !loaded}
        <Skeleton class="h-10" />
      {:else}
        <div class="text-sm">
          <p class="font-medium">{flag(place?.country_code)} {placeName(place, { country: true })}</p>
          {#if iana}<p class="text-muted-foreground tabular-nums">{timeIn(iana, now)} there, {offsetFromHere(iana, now)}</p>{/if}
          {#if place?.label}
            <p class="text-muted-foreground mt-1">{placeSource(place)}{#if place.source === "address" && place.address}: {place.address}{/if}.</p>
          {/if}
          {#if place?.approximate && !editing}
            <p class="text-muted-foreground mt-1">{client.address_status === "not_found" ? "Their address couldn’t be found on the map: check it, or set the place." : "Add their address, or set the place, to put them on the map."}</p>
          {/if}
          {#if setByHand(place) && client.address_status === "ok" && !editing}
            <Button variant="link" size="sm" class="mt-1 h-auto px-0" onclick={useAddress}>Use their address again</Button>
          {/if}
        </div>
        {#if editing}
          <PlacePicker clientId={record.id} {place} hasAddress={client.address_status === "ok"} autofocus onsaved={() => { editing = false; load(); }} />
          <div class="flex gap-2">
            <Button variant="outline" size="sm" href={`/reports?client=${record.id}`}><MapIcon />Drop a pin in Reports</Button>
            <Button variant="ghost" size="sm" onclick={() => (editing = false)}>Done</Button>
          </div>
        {:else}
          <a href={`/reports?client=${record.id}`} class="text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-sm underline-offset-4 hover:underline"><MapIcon class="size-4" />See them on the map</a>
        {/if}
      {/if}
    </Card.Content>
  </Card.Root>
{/if}
