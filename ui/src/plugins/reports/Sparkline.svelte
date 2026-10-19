<script>
  // A small line of weekly counts, oldest first, with the last week marked.
  let { values = [], label = "", color = "var(--primary)", width = 88, height = 24 } = $props();
  const max = $derived(Math.max(1, ...values));
  const pts = $derived(values.map((v, i) => [values.length > 1 ? (i / (values.length - 1)) * (width - 4) + 2 : width / 2, height - 3 - (v / max) * (height - 6)]));
  const line = $derived(pts.map((p, i) => (i ? "L" : "M") + p[0].toFixed(1) + " " + p[1].toFixed(1)).join(" "));
  const area = $derived(pts.length ? `${line} L${pts.at(-1)[0].toFixed(1)} ${height} L${pts[0][0].toFixed(1)} ${height} Z` : "");
  const total = $derived(values.reduce((a, b) => a + b, 0));
</script>

<svg {width} {height} viewBox="0 0 {width} {height}" role="img" aria-label={label || `${total} in ${values.length} weeks`} class="overflow-visible">
  <path d={area} fill={color} fill-opacity="0.12" />
  <path d={line} fill="none" stroke={color} stroke-width="1.5" stroke-linejoin="round" stroke-linecap="round" />
  {#if pts.length}<circle cx={pts.at(-1)[0]} cy={pts.at(-1)[1]} r="2.25" fill={color} />{/if}
</svg>
