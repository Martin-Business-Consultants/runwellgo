<script>
  // An emailed sign-in link: a button, not a sign-in on opening (mail
  // scanners open links before people do), then two-factor if it's on.
  import { errorMessage } from "$lib/toast.svelte.js";
  import PublicLayout from "../PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { get, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { go } from "$lib/router.svelte.js";

  let { params } = $props();
  // svelte-ignore state_referenced_locally
  const token = params.token;
  let who = $state(null), gone = $state(""), challenge = $state(null), code = $state(""), error = $state(""), busy = $state(false);

  get(`/session/link/${token}`, { quiet: true }).then((r) => (who = r)).catch((err) => (gone = errorMessage(err)));

  async function submit(e) {
    e.preventDefault();
    busy = true; error = "";
    try {
      const res = challenge
        ? await post("/session/two_factor", { challenge, code }, { quiet: true })
        : await post(`/session/link/${token}`, undefined, { quiet: true });
      if (res.two_factor_required) challenge = res.challenge;
      else { session.apply(res); go("/"); }
    } catch (err) {
      error = errorMessage(err);
      if (err.body?.error === "expired") { challenge = null; gone = errorMessage(err); }
    }
    busy = false;
  }
</script>

<PublicLayout title={challenge ? "One more step" : who ? `Sign in as ${who.name}` : "Sign in"} description={challenge ? "Enter the 6-digit code from your authenticator app." : who?.email_address || ""}>
  {#if gone}
    <Field.Group>
      <Alert.Root variant="destructive"><Alert.Description>{gone}</Alert.Description></Alert.Root>
      <Button href="/session/link" variant="outline">Ask for a new link</Button>
    </Field.Group>
  {:else if who}
    <form onsubmit={submit}>
      <Field.Group>
        {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
        {#if challenge}
          <Field.Field>
            <Field.Label for="code">Code</Field.Label>
            <Input id="code" bind:value={code} inputmode="numeric" autocomplete="one-time-code" maxlength={11} required autofocus />
            <Field.Description>Lost your phone? Enter one of your recovery codes instead. If you have none, an owner can reset two-factor sign-in for you in Settings › People.</Field.Description>
          </Field.Field>
        {/if}
        <Button type="submit" disabled={busy}>Sign in</Button>
      </Field.Group>
    </form>
  {/if}
</PublicLayout>
