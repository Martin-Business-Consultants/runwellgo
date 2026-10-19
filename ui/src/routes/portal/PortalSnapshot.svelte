<script>
  // A sent agreement version exactly as the client saw it: its heading
  // (unless compact), summary and reason, the items and their prices, the
  // total (or the change to the price), and its reference.
  import RichText from "$lib/components/RichText.svelte";
  import { money } from "$lib/format.js";
  import { portal } from "./portal.svelte.js";

  let { entry, compact = false } = $props();
  const snap = $derived(entry.agreement || {});
  const v = $derived(entry.version || {});
  const items = $derived(snap.items || []);
  const cadence = $derived(v.cadence ? v.cadence.toLowerCase() : "");
</script>

<div class="flex flex-col gap-3">
  {#if !compact}
    <header>
      <div class="text-muted-foreground text-sm">For {snap.client}</div>
      <h3 class="text-lg font-semibold">{snap.engagement?.ref} · {snap.engagement?.title}</h3>
      <div class="text-muted-foreground text-sm">{entry.label}{entry.sent_at ? ` · sent ${portal.dateTime(entry.sent_at)}` : ""}</div>
    </header>
  {/if}
  {#if v.summary}<p class="font-semibold">{v.summary}</p>{/if}
  {#if v.reason}<RichText html={v.reason} />{/if}
  {#if snap.description && !v.change}<RichText html={snap.description} class="text-muted-foreground" />{/if}

  {#if items.length}
    <ul class="divide-y rounded-md border text-sm">
      {#each items as item, i (i)}
        <li class="flex items-center gap-3 px-3 py-2">
          <span class="min-w-0 flex-1">{item.description}</span>
          {#if v.priced}<span class="whitespace-nowrap tabular-nums">{money(item.price_cents)}</span>{/if}
        </li>
      {/each}
      {#if v.priced}
        <li class="flex items-center gap-3 px-3 py-2 font-semibold">
          <span class="flex-1">{v.change && v.kind !== "revision" ? "Change to price" : "Total"}{cadence ? ` (${cadence})` : ""}</span>
          <span class="whitespace-nowrap tabular-nums">{money(v.total_cents)}</span>
        </li>
      {/if}
    </ul>
  {:else if cadence && v.priced}
    <p class="text-sm">New price: <strong>{money(v.total_cents)}</strong> {cadence}</p>
  {/if}

  {#if v.content_hash}<div class="text-muted-foreground font-mono text-xs">ref {v.content_hash.slice(0, 16)}</div>{/if}
</div>
