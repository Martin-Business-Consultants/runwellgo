<script>
  // Search everything (s, or ⌘K), shadcn's Command dialog: clients,
  // contacts, engagements, requests, work, commitments, notes and documents
  // as you type, arrows and Enter to open one. Each hit carries the path
  // it opens (GET /api/search).
  import * as Command from "$lib/components/ui/command/index.js";
  import { get } from "../api.js";
  import { go } from "../router.svelte.js";
  let { open = $bindable(false) } = $props();
  let q = $state(""), results = $state([]), loading = $state(false);
  let timer;
  $effect(() => { if (open) { q = ""; results = []; } });
  function search() {
    clearTimeout(timer);
    if (!q.trim()) { results = []; return; }
    timer = setTimeout(async () => {
      loading = true;
      try { results = (await get("/search?q=" + encodeURIComponent(q.trim()))).results; } catch { results = []; }
      loading = false;
    }, 180);
  }
  function openHit(r) { open = false; go(r.path); }
  const typing = (e) => ["INPUT", "TEXTAREA", "SELECT"].includes(e.target.tagName) || e.target.isContentEditable;
</script>

<svelte:window onopensearch={() => (open = true)} onkeydown={(e) => {
  if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") { e.preventDefault(); open = true; }
  else if (e.key === "s" && !e.metaKey && !e.ctrlKey && !e.altKey && !typing(e) && !document.querySelector("[role=dialog]")) { e.preventDefault(); open = true; }
}} />
<Command.Dialog bind:open shouldFilter={false} title="Search everything" description="Everything you keep in Runwell">
  <Command.Input bind:value={q} oninput={search} placeholder="Search everything…" />
  <Command.List class="max-h-[50vh]">
    {#if !q.trim()}
      <p class="text-muted-foreground py-6 text-center text-sm">Type to search everything.</p>
    {:else if !results.length}
      <p class="text-muted-foreground py-6 text-center text-sm">{loading ? "Searching…" : `Nothing matches “${q}”.`}</p>
    {:else}
      <Command.Group>
        {#each results as r, i (r.type + r.id + i)}
          <Command.Item value={r.type + r.id + i} onSelect={() => openHit(r)} class="items-start">
            <span class="min-w-0 flex-1">
              <span class="flex items-center gap-2"><span class="truncate font-medium">{@html r.title}</span><span class="text-muted-foreground text-xs">{r.label}</span></span>
              {#if r.snippet}<span class="text-muted-foreground block truncate text-xs">{@html r.snippet}</span>{/if}
            </span>
          </Command.Item>
        {/each}
      </Command.Group>
    {/if}
  </Command.List>
</Command.Dialog>
