<script>
  // Setting up two-factor sign-in: scan the QR code with an authenticator
  // app (or type the key), enter the code it shows, and save the recovery
  // codes, shown once. Required by the install, it's the one page open
  // until it's done.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import RecoveryCodes from "./RecoveryCodes.svelte";
  import { get, post, patch } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";

  let setup = $state(null), code = $state(""), error = $state(""), busy = $state(false), codes = $state(null);

  get("/settings/two_factor/new").then((s) => {
    if (s.on) { toast("Two-factor sign-in is already on."); go("/settings/account", undefined, { replace: true }); return; }
    setup = s;
  }).catch(toastError);

  async function turnOn(e) {
    e.preventDefault();
    busy = true; error = "";
    try {
      const res = await post("/settings/two_factor", { pending: setup.pending, code });
      toast(res.summary);
      codes = res.recovery_codes;
      session.apply(await get("/session", { quiet: true }));
    } catch (err) { error = errorMessage(err); }
    busy = false;
  }

  async function stopRequiring() {
    try {
      const res = await patch("/settings/two_factor_requirement", { setting: { require_two_factor: false } });
      toast(res.summary);
      session.apply(await get("/session", { quiet: true }));
      setup.required = false;
    } catch (err) { toastError(err); }
  }
</script>

<SettingsLayout title="Set up two-factor sign-in" description="After your password, Runwell will ask for a code from an app on your phone, so a stolen password isn’t enough to get in.">
  {#if session.me?.two_factor_required && !session.me?.two_factor}
    <Alert.Root><Alert.Description>Set up two-factor sign-in to carry on: this install requires it.</Alert.Description></Alert.Root>
  {/if}
  {#if !setup}
    <Skeleton class="h-80 w-full" />
  {:else}
    <Card.Root class="max-w-2xl">
      <Card.Content>
        <ol class="flex list-decimal flex-col gap-6 ps-5 text-sm">
          <li>Open an authenticator app: 1Password, Google Authenticator, Microsoft Authenticator, Authy or your phone’s built-in passwords app.</li>
          <li><div class="flex flex-col gap-3">
            <span>Scan this code with it.</span>
            <div class="w-48 rounded-md border bg-white p-2">{@html setup.qr_svg}</div>
            <Collapsible.Root>
              <Collapsible.Trigger class="text-muted-foreground text-sm underline-offset-4 hover:underline">Can’t scan? Enter this key instead</Collapsible.Trigger>
              <Collapsible.Content><code class="bg-muted mt-2 inline-block rounded px-2 py-1 font-mono text-sm select-all">{setup.secret_grouped}</code></Collapsible.Content>
            </Collapsible.Root>
          </div></li>
          <li><div class="flex flex-col gap-3">
            <span>Enter the 6-digit code it shows.</span>
            <form onsubmit={turnOn} class="flex items-start gap-2">
              <Field.Field data-invalid={!!error} class="w-40">
                <Input bind:value={code} required autofocus autocomplete="one-time-code" inputmode="numeric" placeholder="123456" maxlength={7} aria-label="Code" />
              </Field.Field>
              <Button type="submit" disabled={busy}>Turn on</Button>
            </form>
            {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
          </div></li>
        </ol>
      </Card.Content>
      {#if setup.required && setup.can_stop_requiring}
        <Card.Footer class="text-muted-foreground text-sm">
          This install requires two-factor sign-in. Changed your mind? <Button variant="link" size="sm" onclick={stopRequiring}>Stop requiring it</Button>
        </Card.Footer>
      {/if}
    </Card.Root>
  {/if}
</SettingsLayout>

<RecoveryCodes bind:codes onclose={() => go("/settings/account")} />
