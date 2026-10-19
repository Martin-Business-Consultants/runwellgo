<script>
  // Changing an open commitment: what was promised, by when, and whose. A
  // resolved one is final, so it can't be opened here.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import { get, patch } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { date } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";

  let { params } = $props();
  // svelte-ignore state_referenced_locally
  const id = params.id;
  let commitment = $state(null), form = $state({ description: "", due_on: "", owner: "us" });
  let errors = $state({}), busy = $state(false);
  const home = $derived(!commitment ? "/commitments" : commitment.engagement ? `/engagements/${commitment.engagement.ref}?tab=commitments` : `/clients/${commitment.client.id}?tab=commitments`);
  const options = $derived((commitment?.owner_options || []).flatMap((g) => g.options));

  page.set("Edit commitment", [{ label: session.term("commitment", 2), href: "/commitments" }]);
  get(`/commitments/${id}`).then(({ commitment: c }) => {
    commitment = c;
    form = { description: c.description, due_on: c.due_on, owner: c.owner };
    const up = c.engagement ? { label: c.engagement.ref, href: `/engagements/${c.engagement.ref}` } : { label: c.client.display_name, href: `/clients/${c.client.id}` };
    page.set("Edit commitment", [up, { label: "Edit commitment" }]);
  }).catch(toastError);

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    try {
      await patch(`/commitments/${id}`, { commitment: form });
      toast("Changes saved.");
      go(home);
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === "Escape" && !document.querySelector("[role=dialog],[role=listbox]")) go(home); }} />
<form onsubmit={save} class="grid gap-4 lg:grid-cols-3">
  <Card.Root class="lg:col-span-2">
    <Card.Header><Card.Title class="text-xl">Edit commitment</Card.Title></Card.Header>
    <Card.Content>
      {#if commitment && !commitment.open}
        <p class="text-muted-foreground text-sm">It’s resolved ({commitment.resolution}), so it’s final.</p>
      {:else if commitment}
        <Field.Group>
          {#if errors.contact}<p class="text-destructive text-sm">Contact {errors.contact[0]}</p>{/if}
          <Field.Field data-invalid={!!errors.description}>
            <Field.Label for="cm-description">What was promised</Field.Label>
            <Input id="cm-description" bind:value={form.description} required autofocus autocomplete="off" aria-invalid={!!errors.description} />
            {#if errors.description}<Field.Error>Description {errors.description[0]}</Field.Error>{/if}
          </Field.Field>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field data-invalid={!!errors.due_on}>
              <Field.Label for="cm-due">Due</Field.Label>
              <DatePicker id="cm-due" bind:value={form.due_on} clearable={false} invalid={!!errors.due_on} />
              {#if errors.due_on}<Field.Error>Due {errors.due_on[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field>
              <Field.Label for="cm-owner">Whose</Field.Label>
              <Select.Root type="single" bind:value={form.owner}>
                <Select.Trigger id="cm-owner" class="w-full"><span class="truncate">{options.find((o) => o.value === form.owner)?.label || "We"}</span></Select.Trigger>
                <Select.Content class="max-h-72">
                  {#each commitment.owner_options as g (g.label)}
                    <Select.Group>
                      <Select.Label>{g.label}</Select.Label>
                      {#each g.options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}
                    </Select.Group>
                  {/each}
                </Select.Content>
              </Select.Root>
            </Field.Field>
          </div>
        </Field.Group>
      {/if}
    </Card.Content>
    <Card.Footer class="gap-2">
      <Button type="submit" disabled={busy || !commitment?.open}>Save changes</Button>
      <Button href={home} variant="ghost">Cancel</Button>
    </Card.Footer>
  </Card.Root>

  {#if commitment}
    <Card.Root>
      <Card.Header><Card.Title>Details</Card.Title></Card.Header>
      <Card.Content class="flex flex-col gap-3">
        <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <dt class="text-muted-foreground">{session.term("client")}</dt><dd><a class="hover:underline" href={`/clients/${commitment.client.id}`}>{commitment.client.display_name}</a></dd>
          {#if commitment.engagement}<dt class="text-muted-foreground">{session.term("engagement")}</dt><dd><a class="hover:underline" href={`/engagements/${commitment.engagement.ref}`}>{commitment.engagement.display_name}</a></dd>{/if}
          <dt class="text-muted-foreground">Whose</dt><dd>{commitment.owner_kind === "us" ? "Ours" : "Theirs"}</dd>
          <dt class="text-muted-foreground">Added</dt><dd>{date(commitment.created_at)}</dd>
        </dl>
        <p class="text-muted-foreground text-xs">A commitment is final once resolved: done, missed or dropped.</p>
      </Card.Content>
    </Card.Root>
  {/if}
</form>
