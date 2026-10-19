<script>
  // A sent version, frozen: what was sent, when and by whom, its hash, its
  // items, the client's decision and the approval links out. While it waits
  // on a decision: email the link again, or record the decision with
  // evidence.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import Mail from "@lucide/svelte/icons/mail";
  import Check from "@lucide/svelte/icons/check";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import { post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { dateTime, date, money, humanize } from "$lib/format.js";
  import { contactOptions, defaultApprover } from "./agreement.js";

  let { version, engagement, reload } = $props();
  const v = $derived(version);
  // svelte-ignore state_referenced_locally
  const contacts = engagement.client_contacts;
  let emailing = $state(false), deciding = $state(false), busy = $state(false);
  let linkTo = $state(defaultApprover(contacts));
  let decision = $state({ decision: "approved", contact_id: defaultApprover(contacts), approver_name: "", evidence: "", comment: "" });
  const emailOptions = contactOptions(contacts, { emailOnly: true });
  const allOptions = contactOptions(contacts);

  async function run(fn) {
    busy = true;
    try { const r = await fn(); toast(r.summary); emailing = deciding = false; await reload(); } catch (err) { toastError(err); }
    busy = false;
  }
  const emailLink = (evt) => { evt.preventDefault(); run(() => post(`/agreement_versions/${v.id}/issue_link`, { contact_id: Number(linkTo) })); };
  const record = (evt) => {
    evt.preventDefault();
    run(() => post(`/agreement_versions/${v.id}/record_decision`, { ...decision, contact_id: Number(decision.contact_id) || null }));
  };
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>{v.label}</Card.Title>
    <Card.Description>
      Sent {dateTime(v.sent_at)}{v.sent_by ? ` by ${v.sent_by.name}` : ""}
      · <span title={v.content_hash}>#{(v.content_hash || "").slice(0, 8)}</span>
    </Card.Description>
    <Card.Action class="flex items-center gap-2">
      {#if v.priced}<span class="text-sm tabular-nums">{v.amount_text}</span>{/if}
      <StatusBadge status={v.status} />
    </Card.Action>
  </Card.Header>
  <Card.Content class="flex flex-col gap-3 text-sm">
    {#if v.summary}<p>{v.summary}</p>{/if}
    {#if v.reason}<div class="prose-admin text-muted-foreground">{@html v.reason}</div>{/if}
    {#if v.scope_items.length}
      <ul class="flex flex-col gap-1.5">
        {#each v.scope_items as it (it.id)}
          <li class="flex items-center gap-2">
            <a href={`/scope_items/${it.id}`} class="min-w-0 flex-1 truncate hover:underline">{it.description}</a>
            {#if v.priced}<span class="tabular-nums">{money(it.price_cents)}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
    {#if v.approval}
      {@const ap = v.approval}
      <div class="bg-muted rounded-md p-3">
        <strong>{humanize(ap.decision)}</strong> by {ap.approver_name || "unknown"}{ap.approver_email ? ` (${ap.approver_email})` : ""}
        · {ap.method === "link" ? "by link" : ap.method === "agent" ? "by their agent" : `recorded by ${ap.recorded_by?.name || "someone"}`}
        · {dateTime(ap.decided_at)}
        {#if ap.evidence}<div class="text-muted-foreground">Evidence: {ap.evidence}</div>{/if}
        {#if ap.comment}<div class="text-muted-foreground">Comment: <div class="prose-admin inline-block">{@html ap.comment}</div></div>{/if}
      </div>
    {/if}
    {#if v.approval_links.length}
      <ul class="text-muted-foreground flex flex-col gap-1 text-xs">
        {#each v.approval_links as l (l.id)}
          <li>Link emailed to {l.contact.name} · expires {date(l.expires_at)} · {l.opened_at ? `opened ${dateTime(l.opened_at)}` : "not opened"}</li>
        {/each}
      </ul>
    {/if}
  </Card.Content>
  {#if v.awaiting_decision && session.can("send_agreements")}
    <Card.Footer class="flex flex-wrap justify-center gap-2">
      <Button variant="outline" size="sm" onclick={() => (emailing = true)}><Mail />Email approval link</Button>
      <Button size="sm" onclick={() => (deciding = true)}><Check />Record decision</Button>
    </Card.Footer>
  {/if}
</Card.Root>

<Dialog.Root bind:open={emailing}>
  <Dialog.Content class="sm:max-w-md">
    <form onsubmit={emailLink} class="flex flex-col gap-4">
      <Dialog.Header>
        <Dialog.Title>Email an approval link</Dialog.Title>
        <Dialog.Description>The contact gets a signed link to review this exact version and approve it or ask for changes.</Dialog.Description>
      </Dialog.Header>
      <Select.Root type="single" bind:value={linkTo}>
        <Select.Trigger class="w-full" aria-label="Contact">{emailOptions.find((o) => o.value === linkTo)?.label || "Choose a contact"}</Select.Trigger>
        <Select.Content>{#each emailOptions as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}</Select.Content>
      </Select.Root>
      <Dialog.Footer>
        <Dialog.Close>{#snippet child({ props })}<Button {...props} variant="outline">Cancel</Button>{/snippet}</Dialog.Close>
        <Button type="submit" disabled={busy || !linkTo}>Email link</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>

<Dialog.Root bind:open={deciding}>
  <Dialog.Content class="sm:max-w-lg">
    <form onsubmit={record} class="flex flex-col gap-4">
      <Dialog.Header>
        <Dialog.Title>Record the client’s decision</Dialog.Title>
        <Dialog.Description>For a decision that came by email, call or meeting. Say where it came from: the evidence is kept with the approval.</Dialog.Description>
      </Dialog.Header>
      <Field.Group>
        <div class="grid gap-3 sm:grid-cols-2">
          <Select.Root type="single" bind:value={decision.decision}>
            <Select.Trigger class="w-full" aria-label="Decision">{humanize(decision.decision)}</Select.Trigger>
            <Select.Content>
              <Select.Item value="approved">Approved</Select.Item>
              <Select.Item value="changes_requested">Changes requested</Select.Item>
            </Select.Content>
          </Select.Root>
          <Select.Root type="single" bind:value={decision.contact_id}>
            <Select.Trigger class="w-full" aria-label="Who decided">{allOptions.find((o) => o.value === decision.contact_id)?.label || "Someone else"}</Select.Trigger>
            <Select.Content>
              <Select.Item value="">Someone else</Select.Item>
              {#each allOptions as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}
            </Select.Content>
          </Select.Root>
        </div>
        <Input bind:value={decision.approver_name} autocomplete="off" placeholder="Their name, if not a contact" aria-label="Their name" />
        <Input bind:value={decision.evidence} required autocomplete="off" placeholder="Evidence: “Email from Ann, 3 March”" aria-label="Evidence" />
        <TiptapEditor format="html" bind:value={decision.comment} minHeight="min-h-16" placeholder="Their comment (optional)" />
      </Field.Group>
      <Dialog.Footer>
        <Dialog.Close>{#snippet child({ props })}<Button {...props} variant="outline">Cancel</Button>{/snippet}</Dialog.Close>
        <Button type="submit" disabled={busy}>Record decision</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
