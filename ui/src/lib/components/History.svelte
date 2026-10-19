<script>
  // A record's history (the Rails events/timeline): what happened, who did
  // it and from where, newest first.
  import { onMount } from "svelte";
  import { get } from "../api.js";
  import Time from "./Time.svelte";
  let { type, id, limit = 20 } = $props();
  let events = $state(null);
  onMount(async () => {
    try { events = (await get(`/events?subject_type=${type}&subject_id=${id}&limit=${limit}`)).events; } catch { events = []; }
  });
</script>

{#if events === null}
  <p class="text-muted-foreground text-sm">Loading…</p>
{:else if !events.length}
  <p class="text-muted-foreground text-sm">Nothing yet.</p>
{:else}
  <ol class="flex flex-col gap-3">
    {#each events as e (e.id)}
      <li class="text-sm">
        <p class="font-medium">{e.title}</p>
        <p class="text-muted-foreground text-xs">by {e.by} · {e.source_label} · <Time value={e.occurred_at} /></p>
        {#if e.details}<p class="text-muted-foreground text-xs">{e.details}</p>{/if}
      </li>
    {/each}
  </ol>
{/if}
