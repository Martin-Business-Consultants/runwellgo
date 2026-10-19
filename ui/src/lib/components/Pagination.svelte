<script>
  // A list's pages, under it: where you are, and links to the others with
  // gaps, in shadcn's pagination look (links, since the page is in the
  // address). pagination is the list's answer: {page, per, total, pages}.
  import { withQuery } from "../router.svelte.js";
  import { buttonVariants } from "$lib/components/ui/button/index.js";
  import ChevronLeft from "@lucide/svelte/icons/chevron-left";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import { cn } from "$lib/utils.js";

  let { pagination } = $props();
  const page = $derived(pagination?.page || 1);
  const per = $derived(pagination?.per || 1);
  const total = $derived(pagination?.total || 0);
  const pages = $derived(pagination?.pages || 1);
  const list = $derived.by(() => {
    const out = [];
    for (let p = 1; p <= pages; p++) {
      if (p === 1 || p === pages || Math.abs(p - page) <= 1) out.push(p);
      else if (out[out.length - 1] !== "…") out.push("…");
    }
    return out;
  });
  const to = (p) => withQuery({ page: p > 1 ? p : null });
  const link = (on = false, off = false) => cn(buttonVariants({ variant: on ? "outline" : "ghost", size: "icon-sm" }), off && "pointer-events-none opacity-50");
</script>

{#if pages > 1}
  <nav class="flex flex-wrap items-center justify-between gap-3" aria-label="Pages">
    <p class="text-muted-foreground text-xs tabular-nums">{(page - 1) * per + 1}–{Math.min(page * per, total)} of {total}</p>
    <div class="flex items-center gap-1">
      <a href={to(page - 1)} class={link(false, page <= 1)} aria-label="Previous page" aria-disabled={page <= 1}><ChevronLeft /></a>
      {#each list as p, i (i)}
        {#if p === "…"}<span class="text-muted-foreground px-1.5">…</span>
        {:else}<a href={to(p)} class={link(p === page)} aria-current={p === page ? "page" : undefined}>{p}</a>{/if}
      {/each}
      <a href={to(page + 1)} class={link(false, page >= pages)} aria-label="Next page" aria-disabled={page >= pages}><ChevronRight /></a>
    </div>
  </nav>
{/if}
