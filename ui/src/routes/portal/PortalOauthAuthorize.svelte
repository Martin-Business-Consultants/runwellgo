<script>
  // A client's contact approving their own AI app: signed in to the portal
  // (by emailed link, which brings them back here), they see the app and
  // what it may do as them, and say yes or no. The token it gets reaches
  // their portal only.
  import PublicLayout from "../PublicLayout.svelte";
  import OauthConsent from "../agents/OauthConsent.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import { router } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { portal, pget, ppost } from "./portal.svelte.js";

  let info = $state(null);

  async function load() {
    try {
      info = await pget("/portal/oauth/authorization?" + router.query.toString(), { quiet: true });
      if (info.status === "redirect") location.href = info.redirect;
      else if (info.status === "ask" && !info.contact_name) await portal.require();
    } catch (e) { toastError(e); }
  }
  load();

  async function decide(approve, readOnly) {
    try {
      await portal.load();
      const res = await ppost("/portal/oauth/authorization", { ...info.params, approve, read_only: readOnly });
      location.href = res.redirect;
    } catch (e) { toastError(e); }
  }
</script>

{#if !info || info.status === "redirect" || (info.status === "ask" && !info.contact_name)}
  <div class="flex min-h-dvh items-center justify-center"><Spinner /></div>
{:else if info.status === "invalid"}
  <PublicLayout title="This app can’t connect">
    <p class="text-sm">The sign-in link it opened isn’t one Runwell recognizes: an unknown app, or a return address it didn’t register. Try connecting again from the app.</p>
  </PublicLayout>
{:else}
  <OauthConsent {info} {decide} readOnlyLabel="Read only: it can look, and change nothing">
    <p class="text-base"><strong>{info.client_name}</strong> will act as you, <strong>{info.contact_name}</strong>, in {info.brand}’s portal for {info.client}: seeing your engagements, agreements, work and documents, and sending requests{info.may_decide ? ", and deciding agreements once you’ve seen them and given your name" : ""}.</p>
    <p class="text-muted-foreground">It sees only what you see here. What it does is marked as done through {info.client_name}. You can disconnect it any time under Connected apps.</p>
  </OauthConsent>
{/if}
