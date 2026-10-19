<script>
  // Search everything, as a page (/search?q=): the same results as the
  // search dialog, each opening where it lives.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import SearchIcon from "@lucide/svelte/icons/search";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import { get } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, go } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";

  const query = $derived(router.query.get("q")?.trim() || "");
  let q = $state(router.query.get("q") || "");
  let results = $state([]), loading = $state(false);

  $effect(() => {
    const current = query;
    page.set(current ? `Search results for “${current}”` : "Search", [{ label: "Home", href: "/" }]);
    if (!current) { results = []; return; }
    loading = true;
    get("/search?q=" + encodeURIComponent(current))
      .then((r) => { if (current === query) results = r.results; })
      .catch(toastError)
      .finally(() => { loading = false; });
  });

  function submit(e) {
    e.preventDefault();
    go("/search", { q: q.trim() }, { replace: !!query });
  }
</script>

<h1 class="sr-only">Search</h1>

<div class="mx-auto flex w-full max-w-3xl flex-col gap-4">
  <form class="flex gap-2" onsubmit={submit} role="search">
    <Input type="search" bind:value={q} placeholder="Find something…" aria-label="Search Runwell" autocomplete="off" autofocus class="flex-1" />
    <Button type="submit"><SearchIcon />Search</Button>
  </form>

  {#if !query}
    <EmptyState title="Search everything">
      Search {session.term("client", 2).toLowerCase()}, contacts, {session.term("engagement", 2).toLowerCase()}, requests, {session.term("work", 2).toLowerCase()}, {session.term("commitment", 2).toLowerCase()}, notes and documents
    </EmptyState>
  {:else if loading && !results.length}
    <Skeleton class="h-48" />
  {:else if !results.length}
    <EmptyState title="No matches">Nothing matches “{query}”.</EmptyState>
  {:else}
    <Card.Root>
      <Card.Content>
        <ul class="flex flex-col divide-y">
          {#each results as r, i (r.type + r.id + i)}
            <li>
              <a href={r.path} class="hover:bg-muted/50 -mx-2 flex flex-col gap-0.5 rounded-md px-2 py-3 [&_mark]:bg-primary/15 [&_mark]:text-foreground [&_mark]:rounded-sm">
                <span class="flex items-baseline gap-2">
                  <span class="min-w-0 flex-1 truncate font-medium">{@html r.title}</span>
                  <span class="text-muted-foreground shrink-0 text-xs">{r.context}</span>
                </span>
                {#if r.snippet}<span class="text-muted-foreground line-clamp-2 text-sm">{@html r.snippet}</span>{/if}
              </a>
            </li>
          {/each}
        </ul>
      </Card.Content>
    </Card.Root>
  {/if}
</div>
