<script>
  // Signing in on staff: email and password, then the code from an
  // authenticator app when the person has two-factor sign-in on. Clients
  // sign in to the portal (/portal) by an emailed link instead.
  import { errorMessage } from "$lib/toast.svelte.js";
  import PublicLayout from "./PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { get, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { router } from "$lib/router.svelte.js";

  let email = $state(router.query.get("email_address") || ""), password = $state(""), code = $state("");
  // A provider's answer comes back here with a two-factor challenge or why
  // it didn't work (Settings › Sign-in).
  let challenge = $state(router.query.get("two_factor")), error = $state(router.query.get("alert") || ""), busy = $state(false);
  let providers = $state({ providers: [], providers_only: false, names: "" });
  get("/sign_in_providers", { quiet: true }).then((p) => (providers = p)).catch(() => {});

  async function submit(e) {
    e.preventDefault();
    busy = true; error = "";
    try {
      const res = challenge
        ? await post("/session/two_factor", { challenge, code }, { quiet: true })
        : await post("/session", { email_address: email, password }, { quiet: true });
      if (res.two_factor_required) challenge = res.challenge;
      else session.apply(res);
    } catch (err) { error = errorMessage(err); }
    busy = false;
  }
</script>

<PublicLayout audience={challenge ? "" : "team"} title={challenge ? "Enter your code" : "Team sign in"} description={challenge ? "From your authenticator app, or a recovery code." : "For your team, with your email and password. Clients sign in under Client."}>
  <form onsubmit={submit}>
    <Field.Group>
      {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
      {#if challenge}
        <Field.Field>
          <Field.Label for="code">Code</Field.Label>
          <Input id="code" bind:value={code} inputmode="numeric" autocomplete="one-time-code" required autofocus />
          <Field.Description>Lost your phone? Enter one of your recovery codes instead. If you have none, an owner can reset two-factor sign-in for you in Settings › People.</Field.Description>
        </Field.Field>
      {:else}
        <Field.Field>
          <Field.Label for="email">Email</Field.Label>
          <Input id="email" type="email" bind:value={email} required autocomplete="username" autofocus />
        </Field.Field>
        <Field.Field>
          <div class="flex items-center">
            <Field.Label for="password">Password</Field.Label>
            <a href="/passwords/new" class="ms-auto text-sm underline-offset-4 hover:underline">Forgot your password?</a>
          </div>
          <Input id="password" type="password" bind:value={password} required autocomplete="current-password" />
        </Field.Field>
      {/if}
      <Field.Field>
        <Button type="submit" disabled={busy}>{challenge ? "Verify" : "Sign in"}</Button>
        {#if !challenge}
          {#each providers.providers as p (p.key)}
            <Button variant="outline" href={`/auth/${p.key}`} data-external>Continue with {p.name}</Button>
          {/each}
          {#if providers.providers_only}
            <Field.Description class="text-center">This install signs in through {providers.names}. The password is for owners.</Field.Description>
          {:else}
            <Field.Description class="text-center"><a href={email ? `/session/link?email_address=${encodeURIComponent(email)}` : "/session/link"}>Email me a sign-in link</a></Field.Description>
          {/if}
        {:else}
          <Field.Description class="text-center"><a href="/" data-external>Start over</a></Field.Description>
        {/if}
      </Field.Field>
    </Field.Group>
  </form>
</PublicLayout>
