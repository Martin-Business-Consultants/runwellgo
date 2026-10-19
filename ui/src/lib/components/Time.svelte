<script>
  // A time, relative ("3 hours ago"), with the exact one on hover.
  import { timeAgo, dateTime } from "../format.js";
  let { value, class: klass = "" } = $props();
</script>

{#if value}<time datetime={value} title={dateTime(value)} class="whitespace-nowrap {klass}">{timeAgo(value)}</time>{:else}<span class="text-muted-foreground">—</span>{/if}
