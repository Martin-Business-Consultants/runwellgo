<script>
  // /settings lands on the first settings page this person may see.
  import { settingsPages } from "$lib/settings-nav.js";
  import { session } from "$lib/session.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { onMount } from "svelte";

  // After mounting: moving while the screen is being made wouldn't redraw.
  onMount(() => {
    const first = settingsPages((p) => session.can(p))[0]?.[1]?.[0]?.href || "/settings/account";
    go(first, undefined, { replace: true });
  });
</script>
