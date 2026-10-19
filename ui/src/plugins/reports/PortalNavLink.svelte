<script>
  // "Your progress" in the portal's nav (the portal_nav slot), while the
  // owner shows clients their report and hasn't switched it off for this
  // one. Its own portal route says whether it shows.
  import { router } from "$lib/router.svelte.js";
  import { cn } from "$lib/utils.js";
  import { pget } from "../../routes/portal/portal.svelte.js";

  let status = $state(cached);
  pget("/x/reports/portal/status", { quiet: true }).then((s) => (status = cached = s)).catch(() => (status = null));
  const active = $derived(router.path === "/portal/progress");
</script>

<script module>
  let cached = null;
</script>

{#if status?.available}
  <a href="/portal/progress" aria-current={active ? "page" : undefined}
    class={cn("hover:bg-muted rounded-md px-2 py-1.5", active ? "bg-muted font-medium" : "text-muted-foreground")}>{status.label}</a>
{/if}
