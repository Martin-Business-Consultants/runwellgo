<script>
  // Accepting an invitation: choose a name and password, and you're in.
  import PublicLayout from "../PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { get, patch } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, errorMessage } from "$lib/toast.svelte.js";

  let { params } = $props();
  // svelte-ignore state_referenced_locally
  const token = params.token;
  let inv = $state(null), gone = $state(""), name = $state(""), password = $state(""), errors = $state({}), error = $state(""), busy = $state(false);

  get(`/invitations/${token}`, { quiet: true }).then((r) => (inv = r)).catch((err) => (gone = errorMessage(err)));
  const role = (r) => (r === "owner" ? "an owner" : "a " + r);

  async function submit(e) {
    e.preventDefault();
    busy = true; errors = {}; error = "";
    try {
      const res = await patch(`/invitations/${token}`, { user: { name, password } }, { quiet: true });
      session.apply(res);
      toast(`Welcome to Runwell, ${res.me?.name || name}.`);
      go("/");
    } catch (err) { errors = err.errors; error = Object.keys(errors).length ? "" : errorMessage(err); }
    busy = false;
  }
</script>

<PublicLayout title="Join Runwell">
  {#if gone}
    <Alert.Root variant="destructive"><Alert.Description>{gone}</Alert.Description></Alert.Root>
  {:else if inv}
    <form onsubmit={submit}>
      <Field.Group>
        <p class="text-center text-sm">{inv.invited_by?.name} invited <strong>{inv.email_address}</strong> as {role(inv.role)}.</p>
        {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
        {#if errors.email_address}<Alert.Root variant="destructive"><Alert.Description>Email address {errors.email_address[0]}.</Alert.Description></Alert.Root>{/if}
        <Field.Field data-invalid={!!errors.name}>
          <Field.Label for="name">Your full name</Field.Label>
          <Input id="name" bind:value={name} required maxlength={240} autocomplete="name" autofocus />
          {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
        </Field.Field>
        <Field.Field data-invalid={!!errors.password}>
          <Field.Label for="password">Choose a password</Field.Label>
          <Input id="password" type="password" bind:value={password} required maxlength={72} autocomplete="new-password" />
          {#if errors.password}<Field.Error>Password {errors.password[0]}</Field.Error>{/if}
        </Field.Field>
        <Button type="submit" disabled={busy}>Join</Button>
      </Field.Group>
    </form>
  {/if}
</PublicLayout>
