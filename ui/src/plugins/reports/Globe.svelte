<script>
  // The globe (d3-geo's orthographic projection on two canvases): the
  // world (sea, graticule, land, borders, the light on it) on one, drawn
  // again only when the view moves, and what's on it (arcs from home, pins,
  // their pulses) on the other, drawn each frame something moves. It turns
  // slowly until someone touches it; drag to spin, ⌘/Ctrl + scroll or pinch
  // (or plain scroll once you've grabbed it) and the buttons to zoom,
  // double-click to reset. A pin is a client: its colour is its health, its
  // size the money agreed, hollow when it's placed by its time zone, and it
  // pulses while something's happening there. Picking a pin flies to it.
  // With reduced motion it neither turns nor animates.
  //   pins: [{id, name, lat, lng, color, r, hollow, live}]
  //   home: {lat, lng, label}; selected: a pin's id
  //   placing: {lat, lng} while someone drops a client's pin (a click or a
  //   drag sets it: onplace({lat, lng}))
  //   view: "us" (turned to North America, zoomed to the contiguous US; it
  //   doesn't turn by itself) or "world" (home in view, turning slowly)
  import { onMount, untrack } from "svelte";
  import { geoOrthographic, geoPath, geoGraticule10, geoDistance, geoInterpolate } from "d3-geo";
  import { feature, mesh } from "topojson-client";
  import world110 from "world-atlas/countries-110m.json";
  import { Button } from "$lib/components/ui/button/index.js";
  import * as Tooltip from "$lib/components/ui/tooltip/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import Minus from "@lucide/svelte/icons/minus";
  import Locate from "@lucide/svelte/icons/locate-fixed";
  import Pause from "@lucide/svelte/icons/pause";
  import Play from "@lucide/svelte/icons/play";
  import { prefersReducedMotion, US } from "./reports.js";

  let { pins = [], home = null, selected = null, placing = null, label = "", offset = [0.07, 0.06], view = "world", onselect = () => {}, onplace = () => {} } = $props();

  let box = $state(null), baseCanvas = $state(null), topCanvas = $state(null), tip = $state(null);
  let spinning = $state(!prefersReducedMotion());
  let hovered = $state(null);

  // The world, simplified for while it moves and in more detail at rest.
  const shapes = (topo) => ({
    land: feature(topo, topo.objects.countries),
    borders: mesh(topo, topo.objects.countries, (a, b) => a !== b),
    coast: mesh(topo, topo.objects.countries, (a, b) => a === b),
  });
  const low = shapes(world110);
  let high = null;
  const graticule = geoGraticule10();
  const sphere = { type: "Sphere" };

  // The view: rotation (λ, φ), zoom, and the size of the box.
  const view_ = { lambda: 0, phi: -20, zoom: 1, w: 0, h: 0, dpr: 1 };
  const projection = geoOrthographic().clipAngle(90).precision(0.6);
  let colors = {};
  let dirty = true, lastMove = 0, hiDrawn = false;
  let touched = $state(false);
  let engaged = false;
  let start = 0; // when the arcs began to draw
  let fly = null; // {from, to, t0, dur}
  let raf = 0;
  const reduced = prefersReducedMotion();

  // The page's tokens as canvas colours: whatever the scheme (oklch, a
  // custom theme), read back from a pixel.
  const probe = typeof document !== "undefined" ? document.createElement("canvas").getContext("2d", { willReadFrequently: true }) : null;
  function rgb(css) {
    if (!probe) return [0, 0, 0];
    probe.clearRect(0, 0, 1, 1);
    probe.fillStyle = "#000";
    probe.fillStyle = css;
    probe.fillRect(0, 0, 1, 1);
    const d = probe.getImageData(0, 0, 1, 1).data;
    return [d[0], d[1], d[2]];
  }
  function token(name) {
    const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
    return rgb(v || "#888");
  }
  const mix = (a, b, t) => a.map((v, i) => Math.round(v * (1 - t) + b[i] * t));
  const css = (c, a = 1) => `rgba(${c[0]},${c[1]},${c[2]},${a})`;
  function readColors() {
    const dark = document.documentElement.classList.contains("dark");
    const bg = token("--background"), fg = token("--foreground"), primary = token("--primary");
    colors = {
      dark, bg, fg, primary,
      sea: mix(bg, primary, dark ? 0.1 : 0.06),
      land: mix(mix(bg, fg, dark ? 0.1 : 0.07), primary, dark ? 0.32 : 0.2),
      border: dark ? css(bg, 0.55) : css(bg, 0.85),
      coast: css(primary, dark ? 0.35 : 0.28),
      grid: css(primary, dark ? 0.12 : 0.09),
    };
    // A pin's colour is a token too ("var(--chart-2)"): resolve each once.
    pinColors.clear();
    dirty = true;
  }
  const pinColors = new Map();
  function pinColor(c) {
    if (!pinColors.has(c)) {
      const m = /^var\((--[\w-]+)\)$/.exec(c);
      pinColors.set(c, m ? token(m[1]) : rgb(c));
    }
    return pinColors.get(c);
  }

  // Wide, the globe sits right of centre and a little low, clear of the
  // headline (offset, as a share of the box).
  const wide = () => view_.w >= 640;
  const radius = () => (wide() ? Math.min(view_.w * 0.41, view_.h * 0.42) : Math.min(view_.w, view_.h) * 0.43) * view_.zoom;
  const centre = () => (wide() ? [view_.w * (0.5 + offset[0]), view_.h * (0.5 + offset[1])] : [view_.w / 2, view_.h * 0.6]);
  function setProjection() {
    projection.rotate([view_.lambda, view_.phi, 0]).scale(radius()).translate(centre());
  }

  // ---- drawing --------------------------------------------------------------

  function drawBase(detail) {
    const ctx = baseCanvas.getContext("2d");
    const { w, h, dpr } = view_;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    const r = radius(), [cx, cy] = centre();
    const c = colors;

    // The air around it.
    const glow = ctx.createRadialGradient(cx, cy, r * 0.92, cx, cy, r * 1.22);
    glow.addColorStop(0, css(c.primary, c.dark ? 0.32 : 0.22));
    glow.addColorStop(0.35, css(c.primary, c.dark ? 0.12 : 0.08));
    glow.addColorStop(1, css(c.primary, 0));
    ctx.fillStyle = glow;
    ctx.beginPath();
    ctx.arc(cx, cy, r * 1.22, 0, Math.PI * 2);
    ctx.fill();

    const path = geoPath(projection, ctx);
    ctx.beginPath(); path(sphere); ctx.fillStyle = css(c.sea); ctx.fill();
    ctx.beginPath(); path(graticule); ctx.strokeStyle = c.grid; ctx.lineWidth = 0.6; ctx.stroke();

    const world = detail && high ? high : low;
    ctx.beginPath(); path(world.land); ctx.fillStyle = css(c.land); ctx.fill();
    ctx.beginPath(); path(world.borders); ctx.strokeStyle = c.border; ctx.lineWidth = 0.6; ctx.stroke();
    ctx.beginPath(); path(world.coast); ctx.strokeStyle = c.coast; ctx.lineWidth = 0.7; ctx.stroke();

    // Light from the top left, the limb a little darker: a ball, not a disc.
    const shade = ctx.createRadialGradient(cx - r * 0.45, cy - r * 0.5, r * 0.1, cx, cy, r * 1.02);
    shade.addColorStop(0, c.dark ? css(c.primary, 0.1) : "rgba(255,255,255,0.35)");
    shade.addColorStop(0.55, "rgba(255,255,255,0)");
    shade.addColorStop(1, c.dark ? "rgba(0,0,0,0.35)" : css(c.fg, 0.1));
    ctx.beginPath(); path(sphere); ctx.fillStyle = shade; ctx.fill();
    ctx.beginPath(); path(sphere); ctx.strokeStyle = css(c.primary, c.dark ? 0.35 : 0.25); ctx.lineWidth = 1; ctx.stroke();
    hiDrawn = !!(detail && high);
  }

  const center = () => [-view_.lambda, -view_.phi];
  const visible = (lng, lat) => geoDistance([lng, lat], center()) < Math.PI / 2 - 0.02;
  // How much of a pin shows as it turns away (fades at the edge).
  const facing = (lng, lat) => Math.max(0, Math.min(1, (Math.PI / 2 - geoDistance([lng, lat], center())) / 0.25));

  function drawTop(now) {
    const ctx = topCanvas.getContext("2d");
    const { w, h, dpr } = view_;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, w, h);
    const r = radius(), [cx, cy] = centre();
    const c = colors;
    let animating = false;

    // Arcs from home, lifted off the surface by how far they go; drawn in
    // once, one after another.
    if (home) {
      const t0 = start || now;
      pins.forEach((p, i) => {
        const dist = geoDistance([home.lng, home.lat], [p.lng, p.lat]);
        if (dist < 0.004) return;
        const progress = reduced ? 1 : Math.max(0, Math.min(1, (now - t0 - i * 70) / 1400));
        if (progress < 1) animating = true;
        if (progress <= 0) return;
        const along = geoInterpolate([home.lng, home.lat], [p.lng, p.lat]);
        const lift = Math.min(0.28, 0.05 + dist * 0.09);
        const steps = Math.max(12, Math.round(48 * dist));
        const ease = 1 - Math.pow(1 - progress, 3);
        ctx.beginPath();
        let open = false;
        for (let s = 0; s <= steps * ease; s++) {
          const t = s / steps;
          const g = along(t);
          if (!visible(g[0], g[1])) { open = false; continue; }
          const q = projection(g);
          // Lifted less near the edge, so an arc sinks over the horizon.
          const k = 1 + Math.sin(Math.PI * t) * lift * Math.cos(geoDistance(g, center())) ** 0.6;
          const x = cx + (q[0] - cx) * k, y = cy + (q[1] - cy) * k;
          if (open) ctx.lineTo(x, y); else { ctx.moveTo(x, y); open = true; }
        }
        const on = selected === p.id;
        ctx.strokeStyle = css(on ? c.primary : c.primary, on ? 0.95 : c.dark ? 0.5 : 0.42);
        ctx.lineWidth = on ? 2 : 1.2;
        ctx.stroke();
      });
    }

    // The pins, smallest on top, the chosen one last.
    const order = [...pins].sort((a, b) => (a.id === selected) - (b.id === selected) || b.r - a.r);
    for (const p of order) {
      if (!visible(p.lng, p.lat)) continue;
      const [x, y] = projection([p.lng, p.lat]);
      const col = pinColor(p.color);
      const alpha = facing(p.lng, p.lat);
      const scale = Math.min(1.6, Math.max(0.8, Math.sqrt(view_.zoom)));
      const pr = p.r * scale;
      ctx.globalAlpha = alpha;
      if (p.live) {
        if (reduced) {
          ctx.beginPath(); ctx.arc(x, y, pr + 4, 0, Math.PI * 2); ctx.strokeStyle = css(col, 0.35); ctx.lineWidth = 1.5; ctx.stroke();
        } else {
          const phase = ((now + p.id * 677) % 2400) / 2400;
          ctx.beginPath(); ctx.arc(x, y, pr + 2 + phase * 16, 0, Math.PI * 2);
          ctx.strokeStyle = css(col, 0.55 * (1 - phase)); ctx.lineWidth = 2; ctx.stroke();
          animating = true;
        }
      }
      ctx.beginPath(); ctx.arc(x, y, pr, 0, Math.PI * 2);
      if (p.hollow) {
        ctx.fillStyle = css(c.bg, 0.75); ctx.fill();
        ctx.lineWidth = 2.25; ctx.strokeStyle = css(col); ctx.stroke();
      } else {
        ctx.shadowColor = css(col, 0.45); ctx.shadowBlur = 8;
        ctx.fillStyle = css(col); ctx.fill();
        ctx.shadowBlur = 0;
        ctx.lineWidth = 1.5; ctx.strokeStyle = css(c.bg, 0.95); ctx.stroke();
      }
      if (p.id === selected || p.id === hovered) {
        ctx.beginPath(); ctx.arc(x, y, pr + 4.5, 0, Math.PI * 2);
        ctx.lineWidth = 2; ctx.strokeStyle = css(c.fg, p.id === selected ? 0.9 : 0.5); ctx.stroke();
      }
      ctx.globalAlpha = 1;
    }

    // Home, over the pins near it.
    if (home && visible(home.lng, home.lat)) {
      const [x, y] = projection([home.lng, home.lat]);
      ctx.globalAlpha = facing(home.lng, home.lat);
      ctx.beginPath(); ctx.arc(x, y, 9, 0, Math.PI * 2); ctx.fillStyle = css(c.primary, 0.18); ctx.fill();
      ctx.beginPath(); ctx.arc(x, y, 5, 0, Math.PI * 2); ctx.fillStyle = css(c.primary); ctx.fill();
      ctx.lineWidth = 2; ctx.strokeStyle = css(c.bg); ctx.stroke();
      ctx.globalAlpha = 1;
    }

    // The pin being dropped.
    if (placing) {
      const [x, y] = projection([placing.lng, placing.lat]);
      if (visible(placing.lng, placing.lat)) {
        ctx.beginPath(); ctx.moveTo(x, y); ctx.lineTo(x, y - 22); ctx.lineWidth = 2; ctx.strokeStyle = css(c.fg); ctx.stroke();
        ctx.beginPath(); ctx.arc(x, y - 26, 7, 0, Math.PI * 2); ctx.fillStyle = css(c.primary); ctx.fill();
        ctx.lineWidth = 2; ctx.strokeStyle = css(c.bg); ctx.stroke();
        ctx.beginPath(); ctx.ellipse(x, y, 5, 2, 0, 0, Math.PI * 2); ctx.fillStyle = css(c.fg, 0.35); ctx.fill();
      }
    }
    placeTip();
    return animating;
  }

  // The label beside the pin under the pointer, or the chosen one.
  function placeTip() {
    if (!tip) return;
    const id = hovered ?? selected;
    const p = pins.find((x) => x.id === id);
    if (!p || !visible(p.lng, p.lat) || placing) { tip.style.opacity = "0"; return; }
    const [x, y] = projection([p.lng, p.lat]);
    tip.textContent = p.name;
    tip.style.opacity = "1";
    tip.style.transform = `translate(${Math.round(x)}px, ${Math.round(y - p.r - 10)}px) translate(-50%, -100%)`;
  }

  // ---- the loop -------------------------------------------------------------

  let last = 0;
  function frame(now) {
    raf = 0;
    const dt = last ? Math.min(64, now - last) : 16;
    last = now;
    let moving = false;
    if (fly) {
      const t = Math.min(1, (now - fly.t0) / fly.dur);
      const e = t < 0.5 ? 4 * t * t * t : 1 - Math.pow(-2 * t + 2, 3) / 2;
      view_.lambda = fly.from[0] + (fly.to[0] - fly.from[0]) * e;
      view_.phi = fly.from[1] + (fly.to[1] - fly.from[1]) * e;
      view_.zoom = fly.from[2] + (fly.to[2] - fly.from[2]) * e;
      if (t >= 1) fly = null;
      moving = true;
    } else if (spinning && !touched && !reduced && selected == null && !placing && view === "world") {
      view_.lambda += dt * 0.004;
      moving = true;
    } else if (inertia && (Math.abs(inertia.vx) > 0.002 || Math.abs(inertia.vy) > 0.002)) {
      view_.lambda += inertia.vx * dt;
      view_.phi = clampPhi(view_.phi - inertia.vy * dt);
      inertia.vx *= 0.92; inertia.vy *= 0.92;
      moving = true;
    }
    if (moving) { dirty = true; lastMove = now; }
    setProjection();
    if (dirty) { drawBase(false); dirty = false; hiDrawn = false; }
    else if (!hiDrawn && high && now - lastMove > 140) drawBase(true);
    const animating = drawTop(now);
    if (moving || animating || !hiDrawn || dragging) request();
  }
  const request = () => { if (!raf) raf = requestAnimationFrame(frame); };
  const clampPhi = (p) => Math.max(-80, Math.min(80, p));

  // ---- moving it ------------------------------------------------------------

  const ptrs = new Map();
  let dragging = false, dragPin = false, moved = 0, pinchFrom = null, inertia = null;
  function local(e) {
    const b = topCanvas.getBoundingClientRect();
    return [e.clientX - b.left, e.clientY - b.top];
  }
  function pinAt(x, y) {
    let best = null, bestD = Infinity;
    for (const p of pins) {
      if (!visible(p.lng, p.lat)) continue;
      const [px, py] = projection([p.lng, p.lat]);
      const d = Math.hypot(px - x, py - y);
      if (d < Math.max(10, p.r + 6) && d < bestD) { best = p; bestD = d; }
    }
    return best;
  }
  function onSphere(x, y) { const [cx, cy] = centre(); return Math.hypot(x - cx, y - cy) <= radius(); }
  function stopAuto() { touched = true; fly = null; }

  function down(e) {
    topCanvas.setPointerCapture?.(e.pointerId);
    ptrs.set(e.pointerId, local(e));
    engaged = true;
    moved = 0;
    inertia = null;
    if (ptrs.size === 2) {
      const [a, b] = [...ptrs.values()];
      pinchFrom = { d: Math.hypot(a[0] - b[0], a[1] - b[1]), zoom: view_.zoom };
      return;
    }
    const [x, y] = local(e);
    dragPin = false;
    if (placing) {
      const [px, py] = projection([placing.lng, placing.lat]);
      dragPin = Math.hypot(px - x, py - 26 - y) < 14 || Math.hypot(px - x, py - y) < 12;
    }
    dragging = true;
    request();
  }
  function move(e) {
    const [x, y] = local(e);
    if (!ptrs.has(e.pointerId)) {
      if (!placing) {
        const p = pinAt(x, y);
        if ((p?.id ?? null) !== hovered) { hovered = p?.id ?? null; request(); }
        topCanvas.style.cursor = p ? "pointer" : placing ? "crosshair" : "grab";
      }
      return;
    }
    const prev = ptrs.get(e.pointerId);
    ptrs.set(e.pointerId, [x, y]);
    if (ptrs.size === 2 && pinchFrom) {
      const [a, b] = [...ptrs.values()];
      zoomTo(pinchFrom.zoom * Math.hypot(a[0] - b[0], a[1] - b[1]) / pinchFrom.d);
      stopAuto();
      return;
    }
    const dx = x - prev[0], dy = y - prev[1];
    moved += Math.abs(dx) + Math.abs(dy);
    if (moved < 4) return;
    stopAuto();
    if (dragPin) {
      const g = projection.invert([x, y + 0]);
      if (g && onSphere(x, y)) onplace({ lng: g[0], lat: g[1] });
      request();
      return;
    }
    topCanvas.style.cursor = "grabbing";
    const k = 70 / radius();
    view_.lambda += dx * k;
    view_.phi = clampPhi(view_.phi - dy * k);
    inertia = { vx: (dx * k) / 16, vy: (dy * k) / 16 };
    dirty = true;
    request();
  }
  function up(e) {
    const wasClick = moved < 4 && ptrs.size === 1;
    ptrs.delete(e.pointerId);
    if (ptrs.size < 2) pinchFrom = null;
    if (ptrs.size) return;
    dragging = false;
    topCanvas.style.cursor = placing ? "crosshair" : "grab";
    if (!wasClick) { request(); return; }
    inertia = null;
    const [x, y] = local(e);
    if (placing) {
      const g = onSphere(x, y) && projection.invert([x, y]);
      if (g) onplace({ lng: g[0], lat: g[1] });
      return;
    }
    const p = pinAt(x, y);
    if (p) onselect(p.id);
    request();
  }
  function wheel(e) {
    if (!(e.ctrlKey || e.metaKey || engaged)) return;
    e.preventDefault();
    stopAuto();
    zoomTo(view_.zoom * Math.exp(-e.deltaY * (e.ctrlKey ? 0.01 : 0.0018)));
  }
  function zoomTo(z) {
    view_.zoom = Math.max(0.85, Math.min(8, z));
    dirty = true;
    request();
  }
  export function zoomBy(f) { stopAuto(); flyTo(-view_.lambda, -view_.phi, view_.zoom * f, 350); }
  export function reset() {
    stopAuto();
    const at = home || { lng: 0, lat: 20 };
    flyTo(at.lng, Math.max(-50, Math.min(50, at.lat)), 1, 900);
  }
  function key(e) {
    const step = 12 / view_.zoom;
    const keys = {
      ArrowLeft: () => (view_.lambda += step), ArrowRight: () => (view_.lambda -= step),
      ArrowUp: () => (view_.phi = clampPhi(view_.phi - step)), ArrowDown: () => (view_.phi = clampPhi(view_.phi + step)),
      "+": () => zoomTo(view_.zoom * 1.3), "=": () => zoomTo(view_.zoom * 1.3), "-": () => zoomTo(view_.zoom / 1.3), "0": () => reset(),
    };
    if (!keys[e.key]) return;
    e.preventDefault();
    stopAuto();
    keys[e.key]();
    dirty = true;
    request();
  }

  // The zoom that fits the contiguous US in the box (it's about 0.75 of
  // the radius across and 0.44 of it high, seen face on).
  function usZoom() {
    const base = wide() ? Math.min(view_.w * 0.41, view_.h * 0.42) : Math.min(view_.w, view_.h) * 0.43;
    const target = Math.min(((wide() ? 0.62 : 0.8) * view_.w) / 0.74, (0.62 * view_.h) / 0.44);
    return Math.max(1, Math.min(6, target / base));
  }
  // showView turns to the US or the world (instantly, or easing there).
  export function showView(v, instant = false) {
    const [lng, lat, zoom] = v === "us" ? [US.lng, US.lat - 1.5, usZoom()] : [home?.lng ?? 0, Math.max(-45, Math.min(45, (home?.lat ?? 20) * 0.6)), 1];
    if (v === "world") { touched = false; }
    if (instant) {
      view_.lambda = -lng; view_.phi = clampPhi(-lat); view_.zoom = zoom; fly = null; dirty = true; request();
    } else flyTo(lng, lat, zoom, 1200);
  }

  // flyTo turns the globe to a place (and a zoom), easing there.
  export function flyTo(lng, lat, zoom = view_.zoom, dur = 1100) {
    // The shortest way round.
    let to = -lng;
    while (to - view_.lambda > 180) to -= 360;
    while (to - view_.lambda < -180) to += 360;
    const target = [to, clampPhi(-lat), Math.max(0.85, Math.min(8, zoom))];
    if (reduced) {
      [view_.lambda, view_.phi, view_.zoom] = target;
      fly = null;
      dirty = true;
    } else {
      fly = { from: [view_.lambda, view_.phi, view_.zoom], to: target, t0: performance.now(), dur };
    }
    request();
  }

  // A pin chosen (here or in the list) flies there.
  let flewTo = null;
  let sized = $state(false);
  $effect(() => {
    const id = selected;
    if (id == null || id === flewTo || !sized) { if (id == null) flewTo = null; return; }
    const p = pins.find((x) => x.id === id);
    if (!p) return;
    flewTo = id;
    touched = true;
    flyTo(p.lng, p.lat, Math.max(view_.zoom, 1.3));
  });
  // The view chosen (the toggle beside it): turn there.
  let shownView = null;
  $effect(() => {
    const v = view;
    if (!sized || v === shownView) return;
    shownView = v;
    untrack(() => showView(v));
  });
  // Anything new to draw.
  $effect(() => { pins; home; placing; selected; request(); });

  onMount(() => {
    readColors();
    // Start facing home (then the view, once the box has a size).
    if (home) { view_.lambda = -home.lng + 25; view_.phi = Math.max(-45, Math.min(45, -home.lat * 0.6)); }
    const ro = new ResizeObserver(() => {
      const b = box.getBoundingClientRect();
      view_.w = Math.max(1, b.width); view_.h = Math.max(1, b.height);
      view_.dpr = Math.min(2.5, window.devicePixelRatio || 1);
      for (const c of [baseCanvas, topCanvas]) {
        c.width = Math.round(view_.w * view_.dpr); c.height = Math.round(view_.h * view_.dpr);
      }
      dirty = true; hiDrawn = false;
      if (!sized) showView(view, true);
      shownView = view;
      sized = true;
      request();
    });
    ro.observe(box);
    const mo = new MutationObserver(() => { readColors(); request(); });
    mo.observe(document.documentElement, { attributes: true, attributeFilter: ["class", "style"] });
    start = performance.now();
    // The detailed world, once the page has drawn.
    const idle = setTimeout(() => import("world-atlas/countries-50m.json").then((m) => { high = shapes(m.default); hiDrawn = false; request(); }), 600);
    request();
    return () => { ro.disconnect(); mo.disconnect(); clearTimeout(idle); cancelAnimationFrame(raf); };
  });
</script>

<!-- A custom widget: keys turn and zoom it (role application, focusable). -->
<!-- svelte-ignore a11y_no_noninteractive_tabindex, a11y_no_noninteractive_element_interactions -->
<div bind:this={box} class="focus-visible:ring-ring/50 relative size-full touch-none rounded-xl outline-none select-none focus-visible:ring-[3px] focus-visible:ring-inset"
  tabindex="0" role="application" aria-roledescription="globe" aria-label={label + " Arrow keys turn it, plus and minus zoom, 0 goes back home."} onkeydown={key}>
  <canvas bind:this={baseCanvas} class="absolute inset-0 size-full" aria-hidden="true"></canvas>
  <canvas bind:this={topCanvas} class="absolute inset-0 size-full {placing ? 'cursor-crosshair' : 'cursor-grab'}" aria-hidden="true"
    onpointerdown={down} onpointermove={move} onpointerup={up} onpointercancel={up}
    onpointerleave={(e) => { if (!ptrs.size) { engaged = false; if (hovered != null) { hovered = null; request(); } } }}
    ondblclick={reset} onwheel={wheel}></canvas>
  <div bind:this={tip} class="bg-popover text-popover-foreground pointer-events-none absolute top-0 left-0 rounded-md border px-2 py-1 text-xs font-medium whitespace-nowrap opacity-0 shadow-sm transition-opacity" aria-hidden="true"></div>

  <div class="absolute right-3 bottom-3 flex flex-col gap-1.5">
    {#snippet control(labelText, onclick, Icon)}
      <Tooltip.Root>
        <Tooltip.Trigger>
          {#snippet child({ props })}
            <Button {...props} variant="outline" size="icon-sm" class="bg-background/85 backdrop-blur" aria-label={labelText} {onclick}><Icon /></Button>
          {/snippet}
        </Tooltip.Trigger>
        <Tooltip.Content side="left">{labelText}</Tooltip.Content>
      </Tooltip.Root>
    {/snippet}
    {#if !reduced && view === "world"}
      {@render control(spinning && !touched ? "Stop turning" : "Turn slowly", () => { if (spinning && !touched) { spinning = false; touched = true; } else { spinning = true; touched = false; } request(); }, spinning && !touched ? Pause : Play)}
    {/if}
    {@render control("Zoom in", () => zoomBy(1.5), Plus)}
    {@render control("Zoom out", () => zoomBy(1 / 1.5), Minus)}
    {@render control("Back to home", reset, Locate)}
  </div>
</div>
