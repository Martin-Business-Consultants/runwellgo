<script>
  // The map: Google Maps when an owner added a key in Settings › Reports
  // (and Reports may load it), else the built-in globe, which needs no key
  // and works offline. If Google won't load, the globe takes its place and
  // notice says why; without a key, notice says how to get street-level
  // maps.
  //   view: "us" or "world"; pins, home, selected, placing, label,
  //   onselect, onplace: passed to the map; notice ($bindable): a line for
  //   the key under the map
  import { onMount } from "svelte";
  import { get } from "$lib/api.js";
  import Globe from "./Globe.svelte";
  import GoogleMap from "./GoogleMap.svelte";
  import { rp } from "./reports.js";

  let { pins = [], home = null, selected = null, placing = null, label = "", view = "us", onselect = () => {}, onplace = () => {}, notice = $bindable(""), provider = $bindable("") } = $props();

  let cfg = $state(cached), failed = $state("");
  onMount(() => {
    // Read once a visit: a key added in Settings shows on the next one.
    get(rp("/map"), { quiet: true }).then((c) => (cfg = cached = c)).catch(() => (cfg = cfg || { provider: "globe" }));
  });
  const google = $derived(cfg?.provider === "google" && !failed);
  $effect(() => {
    provider = cfg ? (google ? "google" : "globe") : "";
    notice = failed ? `${failed} Showing the built-in globe.` : cfg?.provider === "globe" ? cfg.message || "" : "";
  });
</script>

<script module>
  let cached = null;
</script>

{#if google}
  {#key cfg.key + cfg.map_id}
    <GoogleMap apiKey={cfg.key} mapId={cfg.map_id} {pins} {home} {selected} {placing} {view} {label} {onselect} {onplace} onfail={(m) => (failed = m)} />
  {/key}
{:else if cfg}
  <Globe {pins} {home} {selected} {placing} {view} {label} {onselect} {onplace} />
{/if}
