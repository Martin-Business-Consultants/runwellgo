<script>
  // The emailed sign-in link: signs the contact in, then on to where they
  // were headed (or the portal's home). An expired link goes back to ask
  // for a new one.
  import Spinner from "$lib/components/Spinner.svelte";
  import { go } from "$lib/router.svelte.js";
  import { portal, ppost } from "./portal.svelte.js";

  let { params } = $props();

  async function signIn() {
    try {
      // svelte-ignore state_referenced_locally
      await ppost(`/portal/session/${encodeURIComponent(params.token)}`, undefined, { quiet: true });
      await portal.load();
      go(portal.returnTo(), undefined, { replace: true });
    } catch {
      go("/portal/session/new", { expired: 1 }, { replace: true });
    }
  }
  signIn();
</script>

<div class="flex min-h-dvh items-center justify-center"><Spinner /></div>
