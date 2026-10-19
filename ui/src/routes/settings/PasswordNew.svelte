<script>
  // Forgot your password: an emailed link to choose a new one.
  import { errorMessage } from "$lib/toast.svelte.js";
  import PublicLayout from "../PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { post } from "$lib/api.js";
  import { router } from "$lib/router.svelte.js";

  let email = $state(router.query.get("email_address") || ""), sent = $state(""), error = $state(""), busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    busy = true; error = "";
    try { sent = (await post("/passwords", { email_address: email }, { quiet: true })).summary; }
    catch (err) { error = errorMessage(err); }
    busy = false;
  }
</script>

<PublicLayout title="Forgot your password?" description="We’ll email you a link to choose a new one.">
  {#if sent}
    <Field.Group>
      <Alert.Root><Alert.Description>{sent}</Alert.Description></Alert.Root>
      <Button href="/" variant="outline">Back to sign in</Button>
    </Field.Group>
  {:else}
    <form onsubmit={submit}>
      <Field.Group>
        {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
        <Field.Field>
          <Field.Label for="email">Email</Field.Label>
          <Input id="email" type="email" bind:value={email} required autocomplete="username" autofocus />
        </Field.Field>
        <Field.Field>
          <Button type="submit" disabled={busy}>Email reset link</Button>
          <Field.Description class="text-center"><a href="/">Back to sign in</a></Field.Description>
        </Field.Field>
      </Field.Group>
    </form>
  {/if}
</PublicLayout>
