<script>
  // An address that leads nowhere. A record page shows it too when its
  // record is gone: what wasn't found, and a link back up to its list.
  import EmptyState from "$lib/components/EmptyState.svelte";
  import { page } from "$lib/page.svelte.js";
  let { title = "", back = null } = $props();
  page.set("Not found");
</script>

<EmptyState icon="search" title={title || "There's nothing here"}>
  The address may be old, or what it pointed to was deleted.
  {#snippet actions()}
    <a class="text-primary underline-offset-4 hover:underline" href={back?.href || "/"}>{back?.label || "Go home"}</a>
  {/snippet}
</EmptyState>
