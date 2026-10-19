<script>
  // A new install's first visitor sets it up as its owner, and picks where
  // it starts: the words it uses and whether agreements carry prices.
  // Everyone after joins by invitation.
  import { errorMessage } from "$lib/toast.svelte.js";
  import PublicLayout from "./PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";

  const starts = [
    ["business", "A business", "Clients, projects and work orders, with agreements they approve and prices on them."],
    ["team", "A team", "Departments you deliver for, initiatives and their deliverables, tasks. No prices."],
    ["personal", "Personal life", "Areas of life (home, family, a wedding, a trip), plans and their steps. No prices."],
  ];
  let name = $state(""), email = $state(""), password = $state(""), start = $state("business");
  let errors = $state({}), error = $state(""), busy = $state(false);

  async function submit(e) {
    e.preventDefault();
    busy = true; error = ""; errors = {};
    try {
      session.apply(await post("/signup", { signup: { name, email_address: email, password, start } }, { quiet: true }));
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) error = errorMessage(err); }
    busy = false;
  }
</script>

<PublicLayout title="Set up Runwell" description="You'll be its owner. Everyone else joins by invitation.">
  <form onsubmit={submit}>
    <Field.Group>
      {#if error}<Alert.Root variant="destructive"><Alert.Description>{error}</Alert.Description></Alert.Root>{/if}
      <Field.Field data-invalid={!!errors.name}>
        <Field.Label for="name">Your name</Field.Label>
        <Input id="name" bind:value={name} required autocomplete="name" autofocus aria-invalid={!!errors.name} />
        {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
      </Field.Field>
      <Field.Field data-invalid={!!errors.email_address}>
        <Field.Label for="email">Email</Field.Label>
        <Input id="email" type="email" bind:value={email} required autocomplete="username" aria-invalid={!!errors.email_address} />
        {#if errors.email_address}<Field.Error>Email {errors.email_address[0]}</Field.Error>{/if}
      </Field.Field>
      <Field.Field data-invalid={!!errors.password}>
        <Field.Label for="password">Password</Field.Label>
        <Input id="password" type="password" bind:value={password} required minlength={8} autocomplete="new-password" aria-invalid={!!errors.password} />
        {#if errors.password}<Field.Error>Password {errors.password[0]}</Field.Error>{/if}
      </Field.Field>
      <Field.Set>
        <Field.Legend variant="label">What will you run?</Field.Legend>
        <RadioGroup.Root bind:value={start}>
          {#each starts as [key, label, text] (key)}
            <Field.Label for={"start-" + key}>
              <Field.Field orientation="horizontal">
                <Field.Content>
                  <Field.Title>{label}</Field.Title>
                  <Field.Description>{text}</Field.Description>
                </Field.Content>
                <RadioGroup.Item value={key} id={"start-" + key} />
              </Field.Field>
            </Field.Label>
          {/each}
        </RadioGroup.Root>
      </Field.Set>
      <Button type="submit" disabled={busy}>Create my account</Button>
    </Field.Group>
  </form>
</PublicLayout>
