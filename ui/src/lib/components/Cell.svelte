<script>
  // A table cell (shadcn's); `muted`, `mono`, `num`, `nowrap`.
  import * as Table from "$lib/components/ui/table/index.js";
  import { cn } from "$lib/utils.js";
  import { getContext } from "svelte";
  let { muted = false, mono = false, num = false, nowrap = false, class: klass = "", children } = $props();
  const density = getContext("table-density") || (() => "compact");
</script>

<Table.Cell class={cn("whitespace-normal", density() === "comfortable" && "px-3 py-3.5", muted && "text-muted-foreground", mono && "font-mono text-xs", num && "text-right tabular-nums", nowrap && "whitespace-nowrap", klass)}>{@render children()}</Table.Cell>
