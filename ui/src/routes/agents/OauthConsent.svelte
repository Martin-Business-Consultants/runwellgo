<script>
  // The consent card an app sends a person to: which app, what it may do
  // (children), where it goes back to, the read-only choice, Cancel and
  // Connect. The answer is the app's redirect, which the browser follows.
  import PublicLayout from "../PublicLayout.svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";

  let { info, readOnlyLabel, decide, children } = $props();
  // svelte-ignore state_referenced_locally
  let readOnly = $state(!!info.read_only);
  let busy = $state(false);

  async function answer(approve) {
    busy = true;
    try { await decide(approve, readOnly); } finally { busy = false; }
  }
</script>

<PublicLayout title={`Connect ${info.client_name}?`}>
  <div class="flex flex-col gap-4 text-sm">
    {@render children()}
    <p class="text-muted-foreground text-xs">Returning to {info.returning_to}</p>
    <Field.Field orientation="horizontal">
      <Checkbox id="read-only" bind:checked={readOnly} />
      <Field.Label for="read-only" class="font-normal">{readOnlyLabel}</Field.Label>
    </Field.Field>
    <div class="flex justify-center gap-2">
      <Button variant="outline" disabled={busy} onclick={() => answer(false)}>Cancel</Button>
      <Button disabled={busy} onclick={() => answer(true)}>Connect</Button>
    </div>
  </div>
</PublicLayout>
