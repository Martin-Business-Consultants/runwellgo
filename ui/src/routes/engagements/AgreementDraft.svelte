<script>
  // A draft version: its summary, reason (a change), cadence and price per
  // period (recurring), its scope items edited in place, and Send (freezes
  // it; emails a contact an approval link) or Discard.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import Check from "@lucide/svelte/icons/check";
  import Minus from "@lucide/svelte/icons/minus";
  import Plus from "@lucide/svelte/icons/plus";
  import Mail from "@lucide/svelte/icons/mail";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import { post, patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { contactOptions, defaultApprover, dollars, toCents } from "./agreement.js";
  import PluginSlot from "$lib/components/PluginSlot.svelte";

  let { version, engagement, reload } = $props();
  // Each draft is drawn afresh after a reload, so its props are its start.
  // svelte-ignore state_referenced_locally
  const v = version;
  // svelte-ignore state_referenced_locally
  const e = engagement;
  const revision = v.kind === "revision";
  const prices = session.me.prices;
  const cadences = ["weekly", "monthly", "quarterly"];
  let details = $state({ summary: v.summary || "", reason: v.reason || "", cadence: v.cadence || "monthly", amount: dollars(v.amount_cents_set) });
  let items = $state(v.scope_items.map((it) => ({ id: it.id, description: it.description, price: dollars(it.price_cents), internal_estimate: it.internal_estimate || "" })));
  let fresh = $state({ description: "", price: "", internal_estimate: "" });
  let sending = $state(false), contact = $state(defaultApprover(e.client_contacts)), busy = $state(false);
  const options = contactOptions(e.client_contacts, { emailOnly: true });

  async function run(fn) {
    busy = true;
    try { const r = await fn(); toast(r.summary); await reload(); } catch (err) { toastError(err); }
    busy = false;
  }
  function saveDetails(evt) {
    evt.preventDefault();
    const body = { summary: details.summary };
    if (v.kind !== "initial") body.reason = details.reason;
    if (e.recurring) {
      body.cadence = details.cadence;
      if (prices && (revision || v.kind === "initial")) body.amount_cents = toCents(details.amount);
    }
    run(() => patch(`/agreement_versions/${v.id}`, { agreement_version: body }));
  }
  const itemBody = (it) => ({ description: it.description, internal_estimate: it.internal_estimate, ...(prices ? { price_cents: toCents(it.price) } : {}) });
  const saveItem = (evt, it) => { evt.preventDefault(); run(() => patch(`/scope_items/${it.id}`, { scope_item: itemBody(it) })); };
  const removeItem = (it) => run(() => del(`/scope_items/${it.id}`));
  const addItem = (evt) => { evt.preventDefault(); run(() => post(`/agreement_versions/${v.id}/scope_items`, { scope_item: itemBody(fresh) })); };
  function send(evt) {
    evt.preventDefault();
    sending = false;
    run(() => post(`/agreement_versions/${v.id}/send_out`, contact ? { contact_id: Number(contact) } : {}));
  }
  async function discard() {
    if (!(await confirm({ title: `Discard ${v.label}?`, message: "The draft and its items are deleted. Nothing has been sent, so the client never saw it.", confirmLabel: "Discard draft", destructive: true }))) return;
    run(() => del(`/agreement_versions/${v.id}`));
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>{v.label}</Card.Title>
    <Card.Action class="flex items-center gap-2">
      {#if v.priced}<span class="text-sm tabular-nums">{v.amount_text}</span>{/if}
      <Badge variant="outline">Draft</Badge>
    </Card.Action>
  </Card.Header>
  <Card.Content class="flex flex-col gap-4">
    <form onsubmit={saveDetails} class="flex flex-col gap-3">
      <Field.Field>
        <Field.Label for={`v${v.id}-summary`}>Summary</Field.Label>
        <Input id={`v${v.id}-summary`} bind:value={details.summary} autocomplete="off" placeholder="What this version covers" />
      </Field.Field>
      {#if v.kind !== "initial"}
        <Field.Field>
          <Field.Label for={`v${v.id}-reason`}>Reason</Field.Label>
          <TiptapEditor id={`v${v.id}-reason`} format="html" bind:value={details.reason} minHeight="min-h-16" />
        </Field.Field>
      {/if}
      {#if e.recurring}
        <div class="grid gap-3 sm:grid-cols-2">
          <Field.Field>
            <Field.Label for={`v${v.id}-cadence`}>Cadence</Field.Label>
            <Select.Root type="single" bind:value={details.cadence}>
              <Select.Trigger id={`v${v.id}-cadence`} class="w-full capitalize">{details.cadence}</Select.Trigger>
              <Select.Content>{#each cadences as c (c)}<Select.Item value={c} class="capitalize">{c}</Select.Item>{/each}</Select.Content>
            </Select.Root>
          </Field.Field>
          {#if prices && (revision || v.kind === "initial")}
            <Field.Field>
              <Field.Label for={`v${v.id}-amount`}>{revision ? "New price per period" : "Price per period"}</Field.Label>
              <Input id={`v${v.id}-amount`} bind:value={details.amount} inputmode="decimal" autocomplete="off" placeholder="1,500.00" />
            </Field.Field>
          {/if}
        </div>
      {/if}
      <div><Button type="submit" variant="outline" size="sm" disabled={busy}>Save details</Button></div>
    </form>

    {#if !revision}
      <div class="flex flex-col gap-2">
        <p class="text-sm font-medium">{session.term("scope_item", 2)}</p>
        {#each items as it (it.id)}
          <form onsubmit={(evt) => saveItem(evt, it)} class="flex flex-wrap items-center gap-2">
            <Input bind:value={it.description} required autocomplete="off" aria-label="Description" class="min-w-48 flex-1" />
            {#if prices}<Input bind:value={it.price} inputmode="decimal" autocomplete="off" placeholder="Price" aria-label="Price" class="w-28" />{/if}
            <Input bind:value={it.internal_estimate} autocomplete="off" placeholder="Internal est." aria-label="Internal estimate" class="w-32" />
            <Button type="submit" variant="ghost" size="icon-sm" title="Save item" disabled={busy}><Check /><span class="sr-only">Save item</span></Button>
            <Button type="button" variant="ghost" size="icon-sm" class="text-destructive" title="Remove item" disabled={busy} onclick={() => removeItem(it)}><Minus /><span class="sr-only">Remove item</span></Button>
          </form>
        {/each}
        <form onsubmit={addItem} class="flex flex-wrap items-center gap-2">
          <Input bind:value={fresh.description} required autocomplete="off" placeholder="New item" aria-label="New item" class="min-w-48 flex-1" />
          {#if prices}<Input bind:value={fresh.price} inputmode="decimal" autocomplete="off" placeholder="Price" aria-label="Price" class="w-28" />{/if}
          <Input bind:value={fresh.internal_estimate} autocomplete="off" placeholder="Internal est." aria-label="Internal estimate" class="w-32" />
          <Button type="submit" size="sm" disabled={busy}>Add<Plus /></Button>
        </form>
      </div>
    {/if}
    <PluginSlot name="draft_version" version={v} engagement={e} {reload} />
  </Card.Content>
  <Card.Footer class="flex flex-wrap items-center justify-between gap-2">
    {#if e.internal}
      <p class="text-muted-foreground text-sm">{e.client.display_name} is internal: this draft is a plan for the work, never sent.</p>
    {:else if session.can("send_agreements")}
      <Button size="sm" onclick={() => (sending = true)} disabled={busy}><Mail />Send to client</Button>
    {:else}
      <p class="text-muted-foreground text-sm">A manager or owner sends this to the client.</p>
    {/if}
    <Button variant="ghost" size="sm" class="text-destructive" onclick={discard} disabled={busy}>Discard this draft</Button>
  </Card.Footer>
</Card.Root>

<Dialog.Root bind:open={sending}>
  <Dialog.Content class="sm:max-w-md">
    <form onsubmit={send} class="flex flex-col gap-4">
      <Dialog.Header>
        <Dialog.Title>Send {v.label}?</Dialog.Title>
        <Dialog.Description>Sending freezes this version. Choose a contact to email them an approval link, or mark it sent and record their decision yourself.</Dialog.Description>
      </Dialog.Header>
      <Select.Root type="single" bind:value={contact}>
        <Select.Trigger class="w-full" aria-label="Contact">{options.find((o) => o.value === contact)?.label || "Don’t email anyone"}</Select.Trigger>
        <Select.Content>
          <Select.Item value="">Don’t email anyone</Select.Item>
          {#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}
        </Select.Content>
      </Select.Root>
      <Dialog.Footer>
        <Dialog.Close>{#snippet child({ props })}<Button {...props} variant="outline">Cancel</Button>{/snippet}</Dialog.Close>
        <Button type="submit">Send</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
