<script>
  // Resolving a commitment: how it ended (done, missed or dropped) with an
  // optional note. Final once resolved.
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { post } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { action } from "./commitments.js";

  let { commitment, open = $bindable(false), ondone = undefined } = $props();
  let note = $state(""), busy = $state(false);

  async function resolve(resolution) {
    busy = true;
    try {
      const r = await post(`/commitments/${commitment.id}/resolve`, { resolution, note });
      toast(r.summary);
      open = false;
      note = "";
      ondone?.(r.commitment);
    } catch (e) { toastError(e); }
    busy = false;
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-md">
    <Dialog.Header>
      <Dialog.Title>Resolve</Dialog.Title>
      {#if commitment}<Dialog.Description><strong>{commitment.promiser}</strong> will {action(commitment)}. Final once resolved.</Dialog.Description>{/if}
    </Dialog.Header>
    <form onsubmit={(e) => { e.preventDefault(); resolve("done"); }}>
      <Field.Field>
        <Field.Label for="resolve-note">Note (optional)</Field.Label>
        <Input id="resolve-note" bind:value={note} autocomplete="off" placeholder="How it went" />
      </Field.Field>
      <Dialog.Footer class="mt-4">
        <Button type="button" variant="outline" disabled={busy} onclick={() => resolve("dropped")}>Dropped</Button>
        <Button type="button" variant="outline" disabled={busy} onclick={() => resolve("missed")}>Missed</Button>
        <Button type="submit" disabled={busy}>Done</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
