<script>
  // The map as Google Maps (the Maps JavaScript API, loaded the first time
  // it's needed, with the key from Settings › Reports): the same pins as
  // the globe, as advanced markers drawn in our own HTML (colour the
  // health, size the money, hollow when placed only by time zone), grouped
  // into a count where they crowd, home marked, and thin geodesic arcs from
  // home. A click flies to the client and opens their card (onselect).
  // Light or dark follows the app's theme (the map is made again when it
  // changes). If Google won't load (no network, the key refused, the
  // content blocked), onfail(message) and the page shows the globe.
  //   apiKey, mapId: from /x/reports/map; view: "us" or "world";
  //   pins, home, selected, placing, onselect, onplace: as the globe's
  import { onMount, untrack } from "svelte";
  import { US, cssColor, prefersReducedMotion } from "./reports.js";

  let { apiKey, mapId = "", pins = [], home = null, selected = null, placing = null, view = "us", label = "", onselect = () => {}, onplace = () => {}, onfail = () => {}, onready = () => {} } = $props();

  let box = $state(null);
  let map = null, lib = null, ready = $state(false);
  const markers = new Map(); // pin id → {marker, el, pin}
  let clusters = [], arcs = [], homeMarker = null, dropMarker = null, listeners = [];
  const reduced = prefersReducedMotion();

  // ---- loading Google's script, once a page --------------------------------

  function load(key) {
    const w = window;
    if (w.google?.maps?.importLibrary) return Promise.resolve();
    if (w.__runwellMaps) return w.__runwellMaps;
    w.__runwellMaps = new Promise((resolve, reject) => {
      w.__runwellMapsReady = () => resolve();
      const s = document.createElement("script");
      s.src = `https://maps.googleapis.com/maps/api/js?key=${encodeURIComponent(key)}&v=weekly&loading=async&libraries=marker&callback=__runwellMapsReady`;
      s.async = true;
      s.onerror = () => reject(new Error("blocked"));
      document.head.append(s);
      setTimeout(() => reject(new Error("timeout")), 15000);
    });
    w.__runwellMaps.catch(() => { w.__runwellMaps = null; });
    return w.__runwellMaps;
  }

  // ---- markers --------------------------------------------------------------

  const dark = () => document.documentElement.classList.contains("dark");

  function pinStyle(el, p) {
    const col = cssColor(p.color);
    const on = p.id === selected;
    const size = Math.round(p.r * 2.2);
    Object.assign(el.style, {
      width: `${size}px`, height: `${size}px`, borderRadius: "9999px", boxSizing: "border-box", cursor: "pointer",
      background: p.hollow ? (dark() ? "rgba(18,20,28,.8)" : "rgba(255,255,255,.85)") : col,
      border: p.hollow ? `2.5px solid ${col}` : `2px solid ${dark() ? "#12141C" : "#fff"}`,
      boxShadow: on ? `0 0 0 3px ${dark() ? "#fff" : "#1B1E2E"}, 0 2px 6px rgba(0,0,0,.35)` : "0 1px 4px rgba(0,0,0,.3)",
      transform: on ? "scale(1.15)" : "", transition: reduced ? "" : "transform .15s",
    });
  }

  function makePin(p) {
    const el = document.createElement("div");
    pinStyle(el, p);
    const marker = new lib.AdvancedMarkerElement({ position: { lat: p.lat, lng: p.lng }, content: el, title: p.name, gmpClickable: true, zIndex: Math.round(100 - p.r) });
    marker.addEventListener("gmp-click", () => { if (!placing) onselect(p.id); });
    return { marker, el, pin: p };
  }

  function homeEl() {
    const el = document.createElement("div");
    const primary = cssColor("var(--primary)");
    el.innerHTML = `<svg xmlns="http://www.w3.org/2000/svg" width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="#fff" stroke-width="2.4" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M15 21v-8a1 1 0 0 0-1-1h-4a1 1 0 0 0-1 1v8"/><path d="M3 10a2 2 0 0 1 .709-1.528l7-5.999a2 2 0 0 1 2.582 0l7 5.999A2 2 0 0 1 21 10v9a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2z"/></svg>`;
    Object.assign(el.style, { width: "28px", height: "28px", borderRadius: "9999px", display: "grid", placeItems: "center", background: primary,
      border: `2px solid ${dark() ? "#12141C" : "#fff"}`, boxShadow: `0 0 0 5px color-mix(in oklab, ${primary} 25%, transparent), 0 2px 6px rgba(0,0,0,.3)` });
    return el;
  }

  function clusterEl(group) {
    const el = document.createElement("div");
    // Coloured by the worst health in it: a crowd hiding a client at risk says so.
    const worst = group.find((p) => p.band === "at_risk") || group.find((p) => p.band === "watch") || group[0];
    const col = cssColor(worst.color);
    const size = Math.min(52, 28 + group.length * 2);
    el.textContent = String(group.length);
    Object.assign(el.style, { width: `${size}px`, height: `${size}px`, borderRadius: "9999px", display: "grid", placeItems: "center", cursor: "pointer",
      font: "600 13px Manrope, system-ui, sans-serif", color: dark() ? "#F5F6FA" : "#1B1E2E", background: dark() ? "rgba(18,20,28,.92)" : "rgba(255,255,255,.95)",
      border: `3px solid ${col}`, boxShadow: "0 2px 8px rgba(0,0,0,.3)" });
    return el;
  }

  // Pins closer than this many pixels at the current zoom are drawn as one
  // count (the chosen client always stands alone).
  const CROWD = 34;
  function recluster() {
    if (!map) return;
    const proj = map.getProjection();
    const zoom = map.getZoom();
    for (const c of clusters) c.map = null;
    clusters = [];
    if (!proj) return;
    const scale = 2 ** zoom;
    const shown = pins.filter((p) => !(placing && p.id === placing.id));
    const pts = shown.map((p) => { const w = proj.fromLatLngToPoint({ lat: p.lat, lng: p.lng }); return { p, x: w.x * scale, y: w.y * scale }; });
    const used = new Set();
    const groups = [];
    for (const a of pts) {
      if (used.has(a.p.id)) continue;
      used.add(a.p.id);
      const g = [a];
      if (zoom < 13 && a.p.id !== selected) {
        for (const b of pts) {
          if (!used.has(b.p.id) && b.p.id !== selected && Math.hypot(a.x - b.x, a.y - b.y) < CROWD) { used.add(b.p.id); g.push(b); }
        }
      }
      groups.push(g);
    }
    const alone = new Set();
    for (const g of groups) {
      if (g.length === 1) { alone.add(g[0].p.id); continue; }
      const list = g.map((x) => x.p);
      const lat = list.reduce((s, p) => s + p.lat, 0) / list.length, lng = list.reduce((s, p) => s + p.lng, 0) / list.length;
      const m = new lib.AdvancedMarkerElement({ map, position: { lat, lng }, content: clusterEl(list), title: `${list.length} clients here: ${list.map((p) => p.name).join(", ")}`, gmpClickable: true, zIndex: 200 });
      m.addEventListener("gmp-click", () => {
        const b = new google.maps.LatLngBounds();
        list.forEach((p) => b.extend({ lat: p.lat, lng: p.lng }));
        if (b.getNorthEast().equals(b.getSouthWest())) map.setZoom(Math.min(18, zoom + 3)); else map.fitBounds(b, 80);
      });
      clusters.push(m);
    }
    for (const [id, m] of markers) m.marker.map = alone.has(id) ? map : null;
  }

  // Arcs from home to each client, faint; the chosen one stronger. Left out
  // past a hundred clients, where they'd be a scribble.
  function drawArcs() {
    for (const a of arcs) a.setMap(null);
    arcs = [];
    if (!home || pins.length > 100) return;
    const primary = cssColor("var(--primary)");
    for (const p of pins) {
      if (Math.abs(p.lat - home.lat) < 0.01 && Math.abs(p.lng - home.lng) < 0.01) continue;
      const on = p.id === selected;
      arcs.push(new google.maps.Polyline({ map, path: [{ lat: home.lat, lng: home.lng }, { lat: p.lat, lng: p.lng }], geodesic: true, clickable: false,
        strokeColor: primary, strokeOpacity: on ? 0.9 : dark() ? 0.35 : 0.28, strokeWeight: on ? 2.2 : 1.1, zIndex: on ? 2 : 1 }));
    }
  }

  function sync() {
    if (!map) return;
    const seen = new Set();
    for (const p of pins) {
      seen.add(p.id);
      const have = markers.get(p.id);
      if (!have) { markers.set(p.id, makePin(p)); continue; }
      have.pin = p;
      have.marker.position = { lat: p.lat, lng: p.lng };
      pinStyle(have.el, p);
    }
    for (const [id, m] of markers) if (!seen.has(id)) { m.marker.map = null; markers.delete(id); }
    if (home) {
      if (!homeMarker) homeMarker = new lib.AdvancedMarkerElement({ map, content: homeEl(), title: `Home: ${home.label}`, zIndex: 300 });
      homeMarker.position = { lat: home.lat, lng: home.lng };
    }
    drawArcs();
    recluster();
  }

  // ---- the view ---------------------------------------------------------------

  function fit(v) {
    if (!map) return;
    if (v === "us") map.fitBounds({ south: US.south, west: US.west, north: US.north, east: US.east }, { top: 120, bottom: 24, left: 16, right: 16 });
    else { map.setCenter({ lat: 22, lng: home?.lng ?? -30 }); map.setZoom(2); }
  }

  function flyTo(p) {
    if (!map) return;
    const target = Math.max(map.getZoom(), 7);
    map.panTo({ lat: p.lat, lng: p.lng });
    if (reduced) { map.setZoom(target); return; }
    // Zoom in a step at a time once it's panned: a short flight, not a jump.
    const step = () => { const z = map.getZoom(); if (z < target) { map.setZoom(z + 1); setTimeout(step, 180); } };
    setTimeout(step, 350);
  }

  // ---- making the map -------------------------------------------------------

  async function create() {
    const at = map ? { center: map.getCenter(), zoom: map.getZoom() } : null;
    destroy();
    const { Map: GMap } = await google.maps.importLibrary("maps");
    const core = await google.maps.importLibrary("core");
    lib = await google.maps.importLibrary("marker");
    const scheme = core.ColorScheme ? (dark() ? core.ColorScheme.DARK : core.ColorScheme.LIGHT) : undefined;
    map = new GMap(box, {
      mapId: mapId || "DEMO_MAP_ID", colorScheme: scheme, disableDefaultUI: true, zoomControl: true, clickableIcons: false,
      gestureHandling: "cooperative", minZoom: 2, center: at?.center || { lat: US.lat, lng: US.lng }, zoom: at?.zoom || 4,
    });
    listeners.push(map.addListener("idle", recluster));
    listeners.push(map.addListener("click", (e) => { if (placing && e.latLng) onplace({ lat: e.latLng.lat(), lng: e.latLng.lng() }); }));
    if (!at) fit(view);
    for (const m of markers.values()) m.marker.map = null;
    markers.clear();
    homeMarker = null;
    sync();
    syncPlacing();
    ready = true;
    onready();
  }

  function destroy() {
    for (const l of listeners) l.remove();
    listeners = [];
    for (const m of markers.values()) m.marker.map = null;
    for (const c of clusters) c.map = null;
    for (const a of arcs) a.setMap(null);
    if (homeMarker) homeMarker.map = null;
    if (dropMarker) dropMarker.map = null;
    markers.clear(); clusters = []; arcs = []; homeMarker = null; dropMarker = null;
  }

  // The pin being dropped: dragged, or the map clicked.
  function syncPlacing() {
    if (!map) return;
    if (!placing) { if (dropMarker) { dropMarker.map = null; dropMarker = null; } recluster(); return; }
    if (!dropMarker) {
      const pin = new lib.PinElement({ background: cssColor("var(--primary)"), borderColor: "#fff", glyphColor: "#fff" });
      dropMarker = new lib.AdvancedMarkerElement({ map, content: pin.element, gmpDraggable: true, title: "Drag to where they are", zIndex: 400 });
      dropMarker.addListener("dragend", () => { const p = dropMarker.position; onplace({ lat: typeof p.lat === "function" ? p.lat() : p.lat, lng: typeof p.lng === "function" ? p.lng() : p.lng }); });
    }
    dropMarker.position = { lat: placing.lat, lng: placing.lng };
  }

  onMount(() => {
    let gone = false;
    // Google calls this when it refuses the key (wrong, restricted to
    // another site, or the API not enabled for it).
    window.gm_authFailure = () => {
      if (!gone) onfail("Google refused the Maps key: check it in Google Cloud (the Maps JavaScript API enabled, this site’s address allowed).");
    };
    load(apiKey).then(() => !gone && create()).catch((e) => {
      if (!gone) onfail(e.message === "timeout" ? "Google Maps didn’t answer: are you offline?" : "Google Maps couldn’t load here (the network or the page’s content rules blocked it).");
    });
    const mo = new MutationObserver(() => { if (map && ready) create(); });
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ["class"] });
    return () => { gone = true; mo.disconnect(); destroy(); map = null; };
  });

  $effect(() => { pins; home; selected; if (ready) untrack(sync); });
  $effect(() => { placing; if (ready) untrack(syncPlacing); });
  let shownView = untrack(() => view);
  $effect(() => { const v = view; if (ready && v !== shownView) { shownView = v; untrack(() => fit(v)); } });
  let flewTo = null;
  $effect(() => {
    const id = selected;
    if (!ready || id == null || id === flewTo) { if (id == null) flewTo = null; return; }
    const p = pins.find((x) => x.id === id);
    if (p) { flewTo = id; untrack(() => flyTo(p)); }
  });
</script>

<div class="relative size-full">
  <div bind:this={box} class="size-full" role="region" aria-label={label}></div>
</div>
