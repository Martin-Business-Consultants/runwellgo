<script>
  // How a client's records connect, as a map (Svelte Flow): the requests
  // that came from them, the client, its engagements, what was agreed
  // (scope items) and promised (open commitments), and the work, left to
  // right. The record it's opened from is ringed; what it touches (what it
  // hangs from, what hangs from it, what a request became) stays bright and
  // the rest fades. A click opens a record.
  //   record: "Client:7", "Engagement:P-1" or "Todo:18"
  import { SvelteFlow, Background, Controls, MiniMap } from "@xyflow/svelte";
  import "@xyflow/svelte/dist/style.css";
  import { mode } from "mode-watcher";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import RecordNode from "./RecordNode.svelte";
  import { get } from "$lib/api.js";
  import { go } from "$lib/router.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { date } from "$lib/format.js";

  let { record, onopen = undefined } = $props();

  const COLUMN = { request: 0, client: 1, engagement: 2, scope_item: 3, commitment: 3, work: 4 };
  const COLOR = { client: "var(--primary)", request: "#0e7490", engagement: "#7c3aed", scope_item: "#b45309", commitment: "#be185d", work: "#15803d" };
  const kind = (t) => ({ client: session.term("client"), request: "Request", engagement: session.term("engagement"), scope_item: session.term("scope_item"), commitment: session.term("commitment"), work: session.term("work") })[t];
  const X = 300, Y = 76;

  let nodes = $state.raw([]), edges = $state.raw([]), loading = $state(true), focus = $state(null), fitNodes = $state([]);
  const nodeTypes = { record: RecordNode };

  async function load() {
    try {
      const d = await get("/connections?record=" + encodeURIComponent(record));
      draw(d);
    } catch (e) { toastError(e); }
    loading = false;
  }
  load();

  // Rows: the tree from the client, a leaf to a row, each parent beside
  // the middle of what hangs from it; requests stacked beside the client.
  function draw(d) {
    focus = d.focus;
    const byID = new Map(d.nodes.map((n) => [n.id, n]));
    const kids = new Map();
    for (const n of d.nodes) if (n.parent && n.type !== "request") (kids.get(n.parent) || kids.set(n.parent, []).get(n.parent)).push(n.id);
    const order = { scope_item: 0, commitment: 1, work: 2, engagement: 0 };
    for (const list of kids.values()) list.sort((a, b) => order[byID.get(a).type] - order[byID.get(b).type]);
    const y = new Map();
    let row = 0;
    const place = (id) => {
      const ch = kids.get(id) || [];
      if (!ch.length) { y.set(id, row++ * Y); return; }
      ch.forEach(place);
      y.set(id, (y.get(ch[0]) + y.get(ch[ch.length - 1])) / 2);
    };
    const client = d.nodes.find((n) => n.type === "client");
    place(client.id);
    const requests = d.nodes.filter((n) => n.type === "request");
    requests.forEach((n, i) => y.set(n.id, y.get(client.id) + (i - (requests.length - 1) / 2) * Y));

    // What the focus touches: up its parents, down what hangs from it, and
    // across what requests became.
    const lit = new Set();
    if (d.focus === client.id) d.nodes.forEach((n) => lit.add(n.id));
    else {
      for (let id = d.focus; id; id = byID.get(id)?.parent) lit.add(id);
      const down = (id) => { lit.add(id); (kids.get(id) || []).forEach(down); };
      down(d.focus);
      for (const e of d.edges) if (e.kind === "became" && (lit.has(e.target) || lit.has(e.source))) { lit.add(e.source); down(e.target); }
    }

    nodes = d.nodes.map((n) => ({
      id: n.id, type: "record", position: { x: COLUMN[n.type] * X, y: y.get(n.id) ?? 0 },
      data: { label: n.label, kind: kind(n.type), color: COLOR[n.type], path: n.path, focus: n.id === d.focus, faded: !lit.has(n.id),
        detail: n.type === "commitment" && n.due ? `Due ${date(n.due)}` : n.detail },
    }));
    edges = d.edges.map((e, i) => {
      const request = byID.get(e.target)?.type === "request";
      const source = request ? e.target : e.source, target = request ? e.source : e.target;
      const on = lit.has(source) && lit.has(target);
      return { id: `e${i}`, source, target, type: "smoothstep", animated: e.kind === "became",
        label: e.kind === "became" ? "became" : undefined,
        style: `stroke: ${on ? COLOR[byID.get(target)?.type] || "var(--primary)" : "var(--border)"}; stroke-width: ${on ? 1.75 : 1}; ${e.kind === "became" ? "stroke-dasharray: 5 4;" : ""}` };
    });
    fitNodes = [...lit].map((id) => ({ id }));
  }

  function opened({ node }) {
    onopen?.();
    go(node.data.path);
  }
</script>

<div class="flex h-full min-h-0 flex-col gap-2">
  <ul class="text-muted-foreground flex flex-wrap gap-x-4 gap-y-1 text-xs" aria-label="Kinds of record">
    {#each ["request", "client", "engagement", "scope_item", "commitment", "work"] as t (t)}
      <li class="flex items-center gap-1.5"><span class="size-2 rounded-full" style:background={COLOR[t]} aria-hidden="true"></span>{kind(t)}</li>
    {/each}
  </ul>
  <div class="bg-muted/30 min-h-0 flex-1 overflow-hidden rounded-lg border">
    {#if loading}
      <Skeleton class="size-full" />
    {:else if nodes.length}
      <SvelteFlow {nodes} {edges} {nodeTypes} fitView fitViewOptions={{ nodes: fitNodes, padding: 0.15, maxZoom: 1 }} minZoom={0.15}
        nodesDraggable={false} nodesConnectable={false} elementsSelectable={false} colorMode={mode.current === "dark" ? "dark" : "light"}
        proOptions={{ hideAttribution: true }} onnodeclick={opened}>
        <Background gap={24} />
        <Controls showLock={false} />
        <MiniMap pannable zoomable nodeColor={(n) => n.data.color} />
      </SvelteFlow>
    {/if}
  </div>
</div>
