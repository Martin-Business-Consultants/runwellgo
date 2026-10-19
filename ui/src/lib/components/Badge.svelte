<script>
  // shadcn's Badge, the tone from the status the way the Rails admin chose
  // it, or given.
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { tone as toneOf, humanize } from "../format.js";
  let { status = "", tone = null, children = undefined, class: klass = "" } = $props();
  const t = $derived(tone || toneOf(status));
  const looks = {
    neutral: ["secondary", ""],
    info: ["outline", ""],
    success: ["outline", "border-emerald-500/30 bg-emerald-500/10 text-emerald-700 dark:text-emerald-400"],
    warning: ["outline", "border-amber-500/30 bg-amber-500/10 text-amber-700 dark:text-amber-400"],
    danger: ["destructive", ""],
  };
</script>

<Badge variant={looks[t][0]} class="{looks[t][1]} {klass}">
  {#if children}{@render children()}{:else}{humanize(status)}{/if}
</Badge>
