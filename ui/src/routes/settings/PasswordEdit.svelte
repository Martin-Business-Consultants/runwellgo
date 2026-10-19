<script>
  // Choosing a new password from the emailed link (once, for 15 minutes).
  import PublicLayout from "../PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { get, put } from "$lib/api.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, errorMessage } from "$lib/toast.svelte.js";

  let { params } = $props();
  // svelte-ignore state_referenced_locally
  const token = params.token;
  let email = $state(""), gone = $state(""), password = $state(""), confirmation = $state(""), error = $state(""), busy = $state(false);

  get(`/passwords/${token}`, { quiet: true }).then((r) => (email = r.email_address)).catch((err) => (gone = errorMessage(err)));

  async function submit(e) {
    e.preventDefault();
    busy = true; error = "";
    try {
      const res = await put(`/passwords/${token}`, { password, password_confirmation: confirmation }, { quiet: true });
      toast(res.summary);
      go("/");
    } catch (err) { error = err.errors?.password ? `Password ${err.errors.password[0]}.` : errorMessage(err); }
    busy = false;
  }
</script>

<PublicLayout title="Choose a new password" description={email}>
  {#if gone}
    <Field.Group>
      <Alert.Root variant="destructive"><Alert.Description>{gone}</Alert.Description></Alert.Root>
      <Button href="/passwords/new" variant="outline">Ask for a new link</Button>
    </Field.Group>
  {:else}
    <form onsubmit={submit}>
      <Field.Group>
        {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
        <Field.Field>
          <Field.Label for="password">New password</Field.Label>
          <Input id="password" type="password" bind:value={password} required maxlength={72} autocomplete="new-password" autofocus />
        </Field.Field>
        <Field.Field>
          <Field.Label for="confirmation">The same again</Field.Label>
          <Input id="confirmation" type="password" bind:value={confirmation} required maxlength={72} autocomplete="new-password" />
        </Field.Field>
        <Button type="submit" disabled={busy}>Save</Button>
      </Field.Group>
    </form>
  {/if}
</PublicLayout>
