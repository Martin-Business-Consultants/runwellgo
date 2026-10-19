<script>
  // A table header that sorts the list by its column (?sort=key&direction=):
  // first click ascending, then it flips; the arrow says which way it's
  // sorted now. Changing the sort goes back to the first page.
  import * as Table from "$lib/components/ui/table/index.js";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import ArrowDown from "@lucide/svelte/icons/arrow-down";
  import ArrowUpDown from "@lucide/svelte/icons/arrow-up-down";
  import { router, withQuery } from "../router.svelte.js";

  let { key, label, class: klass = "" } = $props();
  const on = $derived(router.query.get("sort") === key);
  const desc = $derived(on && router.query.get("direction") === "desc");
</script>

<Table.Head class={klass} aria-sort={on ? (desc ? "descending" : "ascending") : undefined}>
  <a href={withQuery({ sort: key, direction: on && !desc ? "desc" : "asc" })} class="hover:text-foreground inline-flex items-center gap-1 {on ? 'text-foreground' : ''}">
    {label}
    {#if !on}<ArrowUpDown class="size-3 opacity-40" />{:else if desc}<ArrowDown class="size-3.5" />{:else}<ArrowUp class="size-3.5" />{/if}
  </a>
</Table.Head>
