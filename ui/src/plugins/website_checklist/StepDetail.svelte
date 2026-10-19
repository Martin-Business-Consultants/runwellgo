<script>
  // The picked step, on the right of a website's page (the Rails
  // sites/_item): what it's for, how to do it, who confirmed it; its value
  // (or the Webhooks step's webhooks) and note to record, where saving a
  // value confirms it as yours; the work and files on it; and Doesn't apply,
  // History and Next.
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import History from "@lucide/svelte/icons/history";
  import Trash from "@lucide/svelte/icons/trash-2";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import WebhooksField from "./WebhooksField.svelte";
  import StepFiles from "./StepFiles.svelte";
  import { api, patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { href } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { date } from "$lib/format.js";
  import { wc, STATE_CLASSES } from "./wc.js";
  import { focusInvalid } from "$lib/forms.js";

  let { site, item, total, next = null, onchange, onpick } = $props();

  // The form starts from the step as it is (the page draws it afresh after
  // each save).
  // svelte-ignore state_referenced_locally
  let form = $state({ value: item.value || "", note: item.note || "", title: item.title, detail: item.detail || "" });
  // svelte-ignore state_referenced_locally
  let hooks = $state((item.webhooks || []).map((w) => ({ id: w.id, name: w.name || "", url: w.url || "", pairs: w.pairs.map((p) => ({ ...p })), _destroy: false, open: false })));
  let errors = $state({}), busy = $state(false);
  const settled = $derived(item.state === "confirmed" || item.state === "not_applicable");
  const base = $derived(`/websites/${site.id}/items/${item.id}`);

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    const body = { note: form.note };
    if (item.webhooks) body.webhooks_attributes = hooks.map((w) => ({ id: w.id, name: w.name, url: w.url, pairs: w.pairs, _destroy: w._destroy }));
    else body.value = form.value;
    if (item.custom) Object.assign(body, { title: form.title, detail: form.detail });
    try { const r = await patch(wc(base), { item: body }); toast(r.message); onchange(r); } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
  async function act(method, path, body) {
    try { const r = await api(method, wc(base + path), body); toast(r.message); onchange(r); } catch (e) { toastError(e); }
  }
  async function remove() {
    if (!(await confirm({ title: `Delete “${item.title}”?`, message: "It comes off this website’s checklist. Its history stays.", confirmLabel: "Delete", destructive: true }))) return;
    try { const r = await del(wc(base)); toast(r.message); onchange(r); } catch (e) { toastError(e); }
  }
</script>

<article class="flex flex-col gap-5" aria-label={item.title}>
  <header class="flex flex-col gap-1.5">
    <p class="text-muted-foreground text-xs">Step {item.step} of {total} · {item.group}</p>
    <div class="flex items-start justify-between gap-3">
      <h2 class="text-lg font-semibold">{item.title}</h2>
      <Badge variant="outline" class={STATE_CLASSES[item.state]}>{item.state_label}</Badge>
    </div>
    {#if item.detail}<p class="text-sm">{item.detail}</p>{/if}
  </header>

  {#if item.how}
    <div class="bg-muted flex flex-col gap-1 rounded-lg px-3 py-2 text-sm">
      <strong>How</strong>
      <p>{item.how}</p>
    </div>
  {/if}

  {#if (item.confirmed_value && item.value !== item.confirmed_value) || (item.confirmed_at && settled)}
    <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
      {#if item.confirmed_value && item.value !== item.confirmed_value}
        <dt class="text-muted-foreground">Was confirmed as</dt><dd class="break-words whitespace-pre-line">{item.confirmed_value}</dd>
      {/if}
      {#if item.confirmed_at && settled}
        <dt class="text-muted-foreground">{item.state === "not_applicable" ? "Marked by" : "Confirmed by"}</dt><dd>{item.confirmed_by_name}, {date(item.confirmed_at)}</dd>
      {/if}
    </dl>
  {/if}

  <form onsubmit={save}>
    <Field.Group class="gap-4">
      {#if item.custom}
        <Field.Field data-invalid={!!errors.title}>
          <Field.Label for="step-title">What to check</Field.Label>
          <Input id="step-title" bind:value={form.title} required maxlength={200} autocomplete="off" aria-invalid={!!errors.title} />
          {#if errors.title}<Field.Error>Title {errors.title[0]}</Field.Error>{/if}
        </Field.Field>
        <Field.Field>
          <Field.Label for="step-detail">Why it matters</Field.Label>
          <Input id="step-detail" bind:value={form.detail} autocomplete="off" />
        </Field.Field>
      {/if}
      {#if item.webhooks}
        <WebhooksField bind:rows={hooks} />
        <Field.Description>Saving confirms them as yours; with none, it’s to do again.</Field.Description>
      {:else}
        <Field.Field>
          <Field.Label for="step-value">Value</Field.Label>
          <Input id="step-value" bind:value={form.value} autocomplete="off" placeholder={item.example || ""} />
          <Field.Description>What is true now. Saving confirms it as yours; empty, it’s to do again. Never a password: say where it’s kept.</Field.Description>
        </Field.Field>
      {/if}
      <Field.Field>
        <Field.Label for="step-note">Note</Field.Label>
        <Textarea id="step-note" bind:value={form.note} rows={3} placeholder="Who gave access, what’s still waiting on the client…" />
      </Field.Field>
      <div><Button type="submit" disabled={busy}>Save</Button></div>
    </Field.Group>
  </form>

  <section class="flex flex-col gap-2" aria-label={session.term("work")}>
    <span class="text-sm font-medium">{session.term("work")}</span>
    {#if item.work.length}
      <ul class="flex flex-col gap-1.5 text-sm">
        {#each item.work as t (t.id)}
          <li class="flex min-w-0 items-center gap-2">
            <StatusBadge status={t.status} />
            <a href={t.path} class="truncate underline-offset-4 hover:underline">{t.title}</a>
            <span class="text-muted-foreground shrink-0 text-xs">{t.engagement_ref}{t.due_on ? `, due ${date(t.due_on)}` : ""}</span>
          </li>
        {/each}
      </ul>
    {:else}
      <p class="text-muted-foreground text-xs">None yet: {session.term("work")} at the top of the page adds some for {site.client.display_name}, about this step.</p>
    {/if}
  </section>

  <StepFiles {site} {item} {onchange} />

  <div class="flex flex-wrap items-center gap-2">
    {#if item.state === "not_applicable"}
      <Button variant="outline" size="sm" onclick={() => act("DELETE", "/confirmation")}>It applies after all</Button>
    {:else}
      <Button variant="outline" size="sm" onclick={() => act("POST", "/not_applicable", { item: {} })}>Doesn’t apply to this site</Button>
    {/if}
    {#if item.state === "confirmed"}
      <Button variant="outline" size="sm" onclick={() => act("DELETE", "/confirmation")}>Reopen</Button>
    {:else if item.state === "unconfirmed" && item.value}
      <Button variant="outline" size="sm" onclick={() => act("POST", "/confirmation", { item: {} })}>Confirm</Button>
    {/if}
    <Button variant="outline" size="sm" href={href("/websites/history", { site_id: site.id, item_id: item.id })}><History />History</Button>
    {#if item.custom}
      <Button variant="outline" size="sm" onclick={remove}><Trash />Delete</Button>
    {/if}
    {#if next}
      <Button variant="outline" size="sm" class="ms-auto max-w-full" onclick={() => onpick(next.id)}><span class="truncate">Next: {next.title}</span><ArrowRight /></Button>
    {/if}
  </div>
</article>
