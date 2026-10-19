<script>
  // Client sign in: the email we have on file, and a link by email. It
  // always answers the same way, so it doesn't reveal which addresses
  // exist.
  import { errorMessage } from "$lib/toast.svelte.js";
  import PublicLayout from "../PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import MailCheck from "@lucide/svelte/icons/mail-check";
  import { session } from "$lib/session.svelte.js";
  import { router } from "$lib/router.svelte.js";
  import { ppost } from "./portal.svelte.js";

  let email = $state(""), notice = $state(""), error = $state(router.query.get("expired") ? "That link has expired. Request a new one." : ""), busy = $state(false);
  document.title = `Client sign in · ${session.brand}`;

  async function submit(e) {
    e.preventDefault();
    busy = true; error = "";
    try { notice = (await ppost("/portal/session", { email }, { quiet: true })).notice; } catch (err) { error = errorMessage(err); }
    busy = false;
  }
</script>

<PublicLayout title="Client sign in" description="Enter the email we have on file and we’ll send you a sign-in link. No password needed.">
  <form onsubmit={submit}>
    <Field.Group>
      {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
      {#if notice}
        <Alert.Root><MailCheck /><Alert.Description>{notice}</Alert.Description></Alert.Root>
      {/if}
      <Field.Field>
        <Field.Label for="portal-email">Email</Field.Label>
        <Input id="portal-email" type="email" bind:value={email} required autocomplete="email" autofocus placeholder="Enter your email address…" />
      </Field.Field>
      <Field.Field>
        <Button type="submit" disabled={busy}>Email me a link</Button>
      </Field.Field>
    </Field.Group>
  </form>
</PublicLayout>
