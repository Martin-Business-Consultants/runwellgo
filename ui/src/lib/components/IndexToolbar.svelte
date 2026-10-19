<script>
  // An index's toolbar, one row: on the left the list's tabs (a second way
  // to slice it: engagements' types, requests' sources), on the right the
  // Cards / Table toggle, the filters as dropdowns (a searchable combobox past five choices), then the search box and
  // the New button at the far right. Every choice is a query parameter, so a
  // filtered, searched list is bookmarkable and Back works.
  //   tabs:    {param, current, options: [{value, label, count}]}
  //   filters: [{param, current, title, options: [{value, label, count}]}]
  //   search:  the box's placeholder ("Search clients…"), or nothing for no box
  // Page-specific dropdowns go in as children, beside the filters.
  import { Button } from "$lib/components/ui/button/index.js";
  import * as ToggleGroup from "$lib/components/ui/toggle-group/index.js";
  import * as InputGroup from "$lib/components/ui/input-group/index.js";
  import LayoutGrid from "@lucide/svelte/icons/layout-grid";
  import Rows3 from "@lucide/svelte/icons/rows-3";
  import Kanban from "@lucide/svelte/icons/square-kanban";
  import Plus from "@lucide/svelte/icons/plus";
  import Search from "@lucide/svelte/icons/search";
  import X from "@lucide/svelte/icons/x";
  import { onMount, untrack } from "svelte";
  import FilterMenu from "./FilterMenu.svelte";
  import { router, withQuery, go } from "../router.svelte.js";
  import { page } from "../page.svelte.js";
  import { cn } from "$lib/utils.js";

  let { tabs = null, filters = [], search = "", view = undefined, views = ["cards", "table"], newText = "", newHref = "", children = undefined } = $props();
  const icons = { cards: LayoutGrid, table: Rows3, board: Kanban };

  // The search box: ?q, sent a moment after typing stops. The screen is
  // made afresh for the new address, so the box takes focus back after.
  let q = $state(router.query.get("q") || "");
  let box = $state(null), timer;
  function typed() {
    clearTimeout(timer);
    timer = setTimeout(() => {
      refocus = true;
      go(withQuery({ q: q.trim() || null }), undefined, { replace: true, transition: false });
    }, 300);
  }
  // Back or a link changed ?q: the box follows, unless it's being typed in.
  $effect(() => {
    const now = router.query.get("q") || "";
    if (document.activeElement !== box && untrack(() => q.trim()) !== now) q = now;
  });
  onMount(() => {
    if (refocus && box) { box.focus(); box.setSelectionRange(q.length, q.length); }
    refocus = false;
    return () => clearTimeout(timer);
  });
</script>

<script module>
  // Whether the search box had focus when its typing moved the address.
  let refocus = false;
</script>

<!-- An index has no visible heading (the sidebar marks where you are); this
     one is for screen readers. -->
<h1 class="sr-only">{page.title}</h1>
<div class="flex flex-wrap items-center gap-2">
  {#if tabs}
    <nav class="bg-muted text-muted-foreground inline-flex h-9 max-w-full items-center overflow-x-auto rounded-lg p-[3px]" aria-label="Show">
      {#each tabs.options as o (o.value)}
        <a href={withQuery({ [tabs.param]: o.value })} aria-current={tabs.current === o.value ? "page" : undefined}
          class={cn("inline-flex h-full items-center gap-1.5 rounded-md px-3 text-sm font-medium whitespace-nowrap transition-colors", tabs.current === o.value ? "bg-background text-foreground shadow-sm" : "hover:text-foreground")}>
          {o.label}{#if o.count != null}<span class="text-muted-foreground text-xs tabular-nums">{o.count}</span>{/if}
        </a>
      {/each}
    </nav>
  {/if}

  <div class="ms-auto flex flex-wrap items-center justify-end gap-2">
    {#if view}
      <ToggleGroup.Root type="single" variant="outline" size="sm" value={view} onValueChange={(v) => v && go(withQuery({ view: v }))}>
        {#each views as v (v)}
          {@const I = icons[v]}
          <ToggleGroup.Item value={v} aria-label={v}><I /></ToggleGroup.Item>
        {/each}
      </ToggleGroup.Root>
    {/if}
    {#each filters as f (f.param)}
      <FilterMenu title={f.title} options={f.options} current={f.current ?? f.options[0]?.value} onpick={(v) => go(withQuery({ [f.param]: v }))} />
    {/each}
    {@render children?.()}
    {#if search}
      <InputGroup.Root class="h-8 w-full sm:w-56">
        <InputGroup.Addon><Search /></InputGroup.Addon>
        <InputGroup.Input bind:ref={box} bind:value={q} oninput={typed} placeholder={search} aria-label={search} type="search"
          onkeydown={(e) => { if (e.key === "Escape" && q) { e.stopPropagation(); q = ""; typed(); } }} />
        {#if q}
          <InputGroup.Addon align="inline-end">
            <InputGroup.Button size="icon-xs" aria-label="Clear the search" onclick={() => { q = ""; typed(); }}><X /></InputGroup.Button>
          </InputGroup.Addon>
        {/if}
      </InputGroup.Root>
    {/if}
    {#if newHref}
      <Button href={newHref} size="sm" data-nav-new><Plus />{newText}</Button>
    {/if}
  </div>
</div>
