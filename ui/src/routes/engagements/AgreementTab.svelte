<script>
  // The Agreement tab: every version (a draft to edit and send, sent ones
  // read-only with their decision), starting the next one, and the scope in
  // force.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import AgreementDraft from "./AgreementDraft.svelte";
  import AgreementSent from "./AgreementSent.svelte";
  import { post } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { record, reload } = $props();
  const e = $derived(record);
  const hasDraft = $derived(e.agreement_versions.some((v) => v.draft));
  let starting = $state(false), busy = $state(false);
  let form = $state({ kind: "", summary: "", reason: "" });

  function start() {
    form = { kind: e.new_version_kinds[0].value, summary: "", reason: "" };
    starting = true;
  }
  async function create(evt) {
    evt.preventDefault();
    busy = true;
    try { const r = await post(`/engagements/${e.ref}/agreement_versions`, form); toast(r.summary); starting = false; await reload(); } catch (err) { toastError(err); }
    busy = false;
  }
  const startLabel = $derived(!e.agreement_versions.length ? "Start the agreement" : e.recurring ? "Start a revision or add-on" : "Start a change order");
</script>

<div class="flex flex-col gap-4">
  {#if !e.agreement_versions.length}
    <EmptyState icon="document" title="No agreement yet." description="Start one, then send it for approval.">
      {#snippet actions()}{#if !e.closed}<Button variant="outline" size="sm" onclick={start}><Plus />{startLabel}</Button>{/if}{/snippet}
    </EmptyState>
  {/if}
  {#each e.agreement_versions as v (v.id)}
    {#key JSON.stringify(v)}
      {#if v.draft}
        <AgreementDraft version={v} engagement={e} {reload} />
      {:else}
        <AgreementSent version={v} engagement={e} {reload} />
      {/if}
    {/key}
  {/each}

  {#if !e.closed && !hasDraft && e.agreement_versions.length}
    <div class="flex justify-center"><Button variant="outline" size="sm" onclick={start}><Plus />{startLabel}</Button></div>
  {/if}

  <Card.Root>
    <Card.Header><Card.Title>Scope in force</Card.Title></Card.Header>
    <Card.Content>
      {#if e.agreed_items.length}
        <ul class="flex flex-col gap-2 text-sm">
          {#each e.agreed_items as it (it.id)}
            <li class="flex items-center gap-2">
              <a href={`/scope_items/${it.id}`} class="min-w-0 flex-1 truncate hover:underline">{it.description}</a>
              <StatusBadge status={it.delivery_state} />
            </li>
          {/each}
        </ul>
      {:else}
        <p class="text-muted-foreground text-sm">Nothing approved yet.</p>
      {/if}
    </Card.Content>
  </Card.Root>
</div>

<Dialog.Root bind:open={starting}>
  <Dialog.Content class="sm:max-w-lg">
    <form onsubmit={create} class="flex flex-col gap-4">
      <Dialog.Header><Dialog.Title>Start a new version</Dialog.Title></Dialog.Header>
      <Field.Group>
        <Field.Field>
          <Field.Label for="version-kind">Kind</Field.Label>
          <Select.Root type="single" bind:value={form.kind}>
            <Select.Trigger id="version-kind" class="w-full">{e.new_version_kinds.find((k) => k.value === form.kind)?.label}</Select.Trigger>
            <Select.Content>{#each e.new_version_kinds as k (k.value)}<Select.Item value={k.value}>{k.label}</Select.Item>{/each}</Select.Content>
          </Select.Root>
        </Field.Field>
        <Field.Field>
          <Field.Label for="version-summary">Summary</Field.Label>
          <Input id="version-summary" bind:value={form.summary} autocomplete="off" placeholder="Add a blog section" />
        </Field.Field>
        <Field.Field>
          <Field.Label for="version-reason">Reason</Field.Label>
          <TiptapEditor id="version-reason" format="html" bind:value={form.reason} minHeight="min-h-20" placeholder="Why this is changing, in the client’s words if you have them." />
        </Field.Field>
      </Field.Group>
      <Dialog.Footer>
        <Dialog.Close>{#snippet child({ props })}<Button {...props} variant="outline">Cancel</Button>{/snippet}</Dialog.Close>
        <Button type="submit" disabled={busy}>Start draft</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
