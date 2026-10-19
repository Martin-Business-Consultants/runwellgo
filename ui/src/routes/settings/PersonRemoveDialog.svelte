<script>
  // Remove someone for good: who gets their work, what moves and what goes
  // (the server's preview), and their name typed to confirm. Deactivating
  // is the reversible option, offered beside it.
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Combobox from "$lib/components/Combobox.svelte";
  import { get, post } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { humanize } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";

  // person: { id, name }; ondeactivate: the reversible option, if offered.
  let { person = null, open = $bindable(false), onremoved = undefined, ondeactivate = undefined } = $props();

  let preview = $state(null), successor = $state(null), typed = $state(""), errors = $state({}), busy = $state(false);

  $effect(() => {
    if (open && person) {
      const id = person.id;
      preview = null; successor = null; typed = ""; errors = {};
      get(`/settings/people/${id}/removal`).then((p) => { if (person?.id === id) preview = p; }).catch((e) => { toastError(e); open = false; });
    }
  });

  const options = $derived((preview?.successors || []).map((p) => ({ value: p.id, label: p.name, hint: `${humanize(p.role)} · ${p.email_address}` })));
  const moving = $derived((preview?.moves || []).filter((g) => g.count > 0));
  const going = $derived((preview?.removes || []).filter((g) => g.count > 0));
  const successorName = $derived(options.find((o) => o.value === successor)?.label);
  const matches = $derived(!!preview && [preview.person.name, preview.person.email_address].some((v) => v && v.toLowerCase() === typed.trim().toLowerCase()));

  async function remove(e) {
    e.preventDefault();
    busy = true; errors = {};
    try {
      const res = await post(`/settings/people/${person.id}/removal`, { successor_id: successor, confirm: typed });
      toast(res.summary);
      open = false;
      onremoved?.(res);
    } catch (err) {
      errors = err.errors || {};
      if (!Object.keys(errors).length) toastError(err); else focusInvalid();
    }
    busy = false;
  }
</script>

<Dialog.Root bind:open>
  <Dialog.Content class="max-h-[90dvh] overflow-y-auto sm:max-w-lg">
    <Dialog.Header>
      <Dialog.Title>Remove {person?.name}?</Dialog.Title>
      <Dialog.Description>
        Their work goes to someone else, how they sign in goes, and they’re removed for good. History keeps their name.
      </Dialog.Description>
    </Dialog.Header>
    {#if !preview}
      <Skeleton class="h-48 w-full" />
    {:else if !preview.allowed}
      <Alert.Root variant="destructive"><Alert.Title>You can’t remove them</Alert.Title><Alert.Description>{preview.refusal}</Alert.Description></Alert.Root>
      <Dialog.Footer><Button variant="outline" onclick={() => (open = false)}>Close</Button></Dialog.Footer>
    {:else}
      <form onsubmit={remove} class="flex flex-col gap-5">
        <Field.Group>
          <Field.Field data-invalid={!!errors.successor_id}>
            <Field.Label for="removal-successor">Who gets their work</Field.Label>
            <Combobox id="removal-successor" bind:value={successor} {options} placeholder="Choose someone…" emptyText="No one active matches." clearable={false} invalid={!!errors.successor_id} />
            {#if errors.successor_id}<Field.Error>Successor {errors.successor_id[0]}</Field.Error>{/if}
          </Field.Field>
        </Field.Group>

        <div class="grid gap-4 sm:grid-cols-2">
          <section aria-labelledby="removal-moves" class="flex flex-col gap-2">
            <h3 id="removal-moves" class="text-sm font-semibold">{successorName ? `Goes to ${successorName}` : "Handed over"}</h3>
            {#if moving.length}
              <dl class="divide-y rounded-md border text-sm">
                {#each moving as g (g.key)}
                  <div class="flex items-baseline justify-between gap-3 px-3 py-1.5"><dt>{g.label}</dt><dd class="font-medium tabular-nums">{g.count}</dd></div>
                {/each}
              </dl>
            {:else}
              <p class="text-muted-foreground text-sm">Nothing to hand over.</p>
            {/if}
          </section>
          <section aria-labelledby="removal-removes" class="flex flex-col gap-2">
            <h3 id="removal-removes" class="text-sm font-semibold">Removed</h3>
            {#if going.length}
              <dl class="divide-y rounded-md border text-sm">
                {#each going as g (g.key)}
                  <div class="flex items-baseline justify-between gap-3 px-3 py-1.5"><dt>{g.label}</dt><dd class="font-medium tabular-nums">{g.count}</dd></div>
                {/each}
              </dl>
            {:else}
              <p class="text-muted-foreground text-sm">Nothing else.</p>
            {/if}
            <p class="text-muted-foreground text-xs">Plugins hand over their own parts too, such as priorities and chat channels.</p>
          </section>
        </div>

        <Field.Field data-invalid={!!errors.confirm}>
          <Field.Label for="removal-confirm">Type <strong>{preview.confirm_with}</strong> to confirm</Field.Label>
          <Input id="removal-confirm" bind:value={typed} autocomplete="off" spellcheck="false" aria-invalid={!!errors.confirm} />
          {#if errors.confirm}<Field.Error>That {errors.confirm[0]}</Field.Error>{/if}
        </Field.Field>

        <p class="text-muted-foreground text-sm">
          This can’t be undone.
          {#if ondeactivate}
            To keep them and their name on their work, <button type="button" class="text-primary underline underline-offset-4" onclick={() => { open = false; ondeactivate(); }}>deactivate them instead</button>; you can reactivate them later.
          {:else}
            Deactivating is the reversible option.
          {/if}
        </p>

        <Dialog.Footer>
          <Button type="button" variant="outline" onclick={() => (open = false)}>Cancel</Button>
          <Button type="submit" variant="destructive" disabled={busy || !successor || !matches}>Remove and hand over</Button>
        </Dialog.Footer>
      </form>
    {/if}
  </Dialog.Content>
</Dialog.Root>
