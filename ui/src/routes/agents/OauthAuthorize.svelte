<script>
  // A person approving an app to act as them (/oauth/authorize, where an
  // app's sign-in lands): signed in, they see which app and what it may do
  // (everything their role allows), and say yes or no. Signed out, they sign
  // in first, here. A request for a client's portal goes to the portal's own
  // consent; one the app got wrong goes back to it with the error.
  import PublicLayout from "../PublicLayout.svelte";
  import SignIn from "../SignIn.svelte";
  import OauthConsent from "./OauthConsent.svelte";
  import Spinner from "$lib/components/Spinner.svelte";
  import { get, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { router, go } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";

  let info = $state(null);

  async function load() {
    try {
      info = await get("/oauth/authorization?" + router.query.toString(), { quiet: true });
      if (info.status === "redirect") location.href = info.redirect;
      else if (info.status === "portal") go(info.redirect, undefined, { replace: true });
    } catch (e) { toastError(e); }
  }
  // Again once someone signs in here: the answer then names their agent.
  $effect(() => { if (session.ready) { session.me; load(); } });

  async function decide(approve, readOnly) {
    try {
      const res = await post("/oauth/authorization", { ...info.params, approve, read_only: readOnly });
      location.href = res.redirect;
    } catch (e) { toastError(e); }
  }
</script>

{#if !info || !session.ready || info.status === "redirect" || info.status === "portal"}
  <div class="flex min-h-dvh items-center justify-center"><Spinner /></div>
{:else if info.status === "invalid"}
  <PublicLayout title="This app can’t connect">
    <p class="text-sm">The sign-in link it opened isn’t one Runwell recognizes: an unknown app, or a return address it didn’t register. Try connecting again from the app.</p>
  </PublicLayout>
{:else if !session.me}
  <SignIn />
{:else}
  <OauthConsent {info} {decide} readOnlyLabel="Read only: it can look at everything you can, and change nothing">
    <p class="text-base"><strong>{info.client_name}</strong> will act as your agent, <strong>{info.agent_name}</strong>, in Runwell: everything your role ({info.role}) allows, from reading clients and engagements to adding work and notes.</p>
    <p class="text-muted-foreground">It asks you before anything reaches a client, and what it does is marked as your agent “via {info.client_name}”. You can disconnect it any time in Settings › Connected apps.</p>
  </OauthConsent>
{/if}
