<script>
  // A new engagement, or changing one: the form in the working column; the
  // side column says how it goes and lists the client's other engagements
  // (new), or its facts (edit). The client and shape are set once.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import CustomFieldInputs from "$lib/components/CustomFieldInputs.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import { get, post, patch } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { money } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";

  let { params } = $props();
  // A screen is made afresh for each address, so its params never change.
  // svelte-ignore state_referenced_locally
  const ref = params.ref;
  const editing = !!ref;
  const shapes = [{ value: "fixed", label: "Fixed scope" }, { value: "recurring", label: "Recurring" }];
  let form = $state({ client_id: router.query.get("client_id") || "", label: router.query.get("label") || "work_order", shape: "fixed", title: "", description: "", estimate_notes: "", custom_fields: {} });
  let clients = $state([]), engagement = $state(null), client = $state(null);
  let errors = $state({}), busy = $state(false), loaded = $state(!editing);
  const back = editing ? `/engagements/${ref}` : "/engagements";
  const engagements = session.term("engagement", 2);

  // The labels on offer: the enabled ones, and the one it has.
  const labels = $derived(Object.entries(session.me.terms?.labels || {}).filter(([k, l]) => l.enabled || k === form.label).map(([k, l]) => ({ value: k, label: l.one })));
  const clientName = $derived(clients.find((c) => String(c.id) === String(form.client_id))?.name);
  const internal = $derived(editing ? engagement?.internal : !!client?.internal);

  page.set(editing ? `Edit ${ref}` : "New " + session.term("engagement").toLowerCase(), [{ label: engagements, href: "/engagements" }]);
  get("/clients?status=all").then((r) => (clients = r.clients)).catch(toastError);
  if (editing) {
    get(`/engagements/${ref}`).then(({ engagement: e }) => {
      engagement = e;
      form = { client_id: String(e.client.id), label: e.label, shape: e.shape, title: e.title, description: e.description_html || "", estimate_notes: e.estimate_notes_html || "", custom_fields: { ...e.custom_fields } };
      page.set(`Edit ${ref}`, [{ label: engagements, href: "/engagements" }, { label: ref, href: back }]);
      loaded = true;
    }).catch(toastError);
  }
  // The client's other engagements and whether it's internal, beside a new one.
  $effect(() => {
    const id = form.client_id;
    if (editing || !id) { client = null; return; }
    get(`/clients/${id}`).then((r) => { if (String(form.client_id) === String(id)) client = r.client; }).catch(() => (client = null));
  });

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    try {
      const { client_id, shape, ...changes } = form;
      const res = editing ? await patch(`/engagements/${ref}`, { engagement: changes }) : await post("/engagements", { engagement: { ...form, client_id: Number(client_id) || null } });
      toast(editing ? "Changes saved." : res.summary);
      go(`/engagements/${res.engagement.ref}`);
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === "Escape" && !document.querySelector("[role=dialog],[role=listbox]")) go(back); }} />
<form onsubmit={save} class="grid gap-4 lg:grid-cols-3">
  <Card.Root class="lg:col-span-2">
    <Card.Header>
      <Card.Title class="text-xl"><h1>{editing ? `Edit ${ref}` : "New " + session.term("engagement").toLowerCase()}</h1></Card.Title>
      {#if !editing}
        <Card.Description>{internal ? `${client.name} is internal: no agreement, add ${session.term("work").toLowerCase()} straight away.` : "Scope and price go on the agreement draft next."}</Card.Description>
      {/if}
    </Card.Header>
    <Card.Content>
      {#if loaded}
        <Field.Group>
          {#if errors.base}<p class="text-destructive text-sm">{errors.base.join(" ")}</p>{/if}
          <Field.Field data-invalid={!!errors.client}>
            <Field.Label for="engagement-client">{session.term("client")}</Field.Label>
            <Select.Root type="single" bind:value={form.client_id} disabled={editing}>
              <Select.Trigger id="engagement-client" class="w-full" aria-invalid={!!errors.client}>{clientName || engagement?.client.display_name || `Choose a ${session.term("client").toLowerCase()}`}</Select.Trigger>
              <Select.Content class="max-h-72">
                {#each clients as c (c.id)}<Select.Item value={String(c.id)}>{c.name}</Select.Item>{/each}
              </Select.Content>
            </Select.Root>
            {#if errors.client}<Field.Error>Choose a {session.term("client").toLowerCase()}.</Field.Error>{/if}
          </Field.Field>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field data-invalid={!!errors.label}>
              <Field.Label for="engagement-label">Label</Field.Label>
              <Select.Root type="single" bind:value={form.label}>
                <Select.Trigger id="engagement-label" class="w-full">{labels.find((l) => l.value === form.label)?.label || form.label}</Select.Trigger>
                <Select.Content>{#each labels as l (l.value)}<Select.Item value={l.value}>{l.label}</Select.Item>{/each}</Select.Content>
              </Select.Root>
              {#if errors.label}<Field.Error>Label {errors.label[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field>
              <Field.Label for="engagement-shape">Shape</Field.Label>
              <Select.Root type="single" bind:value={form.shape} disabled={editing}>
                <Select.Trigger id="engagement-shape" class="w-full">{shapes.find((s) => s.value === form.shape)?.label}</Select.Trigger>
                <Select.Content>{#each shapes as s (s.value)}<Select.Item value={s.value}>{s.label}</Select.Item>{/each}</Select.Content>
              </Select.Root>
            </Field.Field>
          </div>
          <Field.Field data-invalid={!!errors.title}>
            <Field.Label for="engagement-title">Title</Field.Label>
            <Input id="engagement-title" bind:value={form.title} required autofocus autocomplete="off" placeholder="Spring landing page" aria-invalid={!!errors.title} />
            {#if errors.title}<Field.Error>Title {errors.title[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="engagement-description">Description</Field.Label>
            <TiptapEditor id="engagement-description" format="html" bind:value={form.description} minHeight="min-h-28" />
            {#if !internal}<Field.Description>Shown to the client on the agreement.</Field.Description>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="engagement-estimate">Estimate notes</Field.Label>
            <TiptapEditor id="engagement-estimate" format="html" bind:value={form.estimate_notes} minHeight="min-h-20" />
            <Field.Description>Internal. Never shown to clients.</Field.Description>
          </Field.Field>
          <CustomFieldInputs model="Engagement" bind:values={form.custom_fields} />
        </Field.Group>
      {/if}
    </Card.Content>
    <Card.Footer class="gap-2">
      <Button type="submit" disabled={busy || !loaded}>{editing ? "Save changes" : `Draft ${session.term("engagement").toLowerCase()}`}</Button>
      <Button href={back} variant="ghost">Cancel</Button>
    </Card.Footer>
  </Card.Root>

  <div class="flex flex-col gap-4">
    {#if editing}
      {#if engagement}
        <Card.Root>
          <Card.Header><Card.Title>Details</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3">
            <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <dt class="text-muted-foreground">Status</dt><dd><StatusBadge status={engagement.closed ? engagement.close_outcome || "closed" : engagement.state} label={engagement.state_label} /></dd>
              <dt class="text-muted-foreground">{session.term("client")}</dt><dd><a class="hover:underline" href={`/clients/${engagement.client.id}`}>{engagement.client.display_name}</a></dd>
              {#if !engagement.internal}<dt class="text-muted-foreground">Versions</dt><dd>{engagement.agreement_versions.length}</dd>{/if}
              <dt class="text-muted-foreground">{session.term("work")}</dt><dd>{engagement.scope.total} ({engagement.scope.total - engagement.scope.done} open)</dd>
              {#if !engagement.internal && session.me.prices && engagement.agreed_amount_cents}
                <dt class="text-muted-foreground">Agreed</dt><dd>{money(engagement.agreed_amount_cents)}{engagement.recurring ? " / period" : ""}</dd>
              {/if}
            </dl>
            {#if !engagement.internal}<p class="text-muted-foreground text-xs">Scope and prices change on the agreement, not here: a sent version never changes.</p>{/if}
          </Card.Content>
        </Card.Root>
      {/if}
    {:else}
      <Card.Root>
        <Card.Header><Card.Title>How it goes</Card.Title></Card.Header>
        <Card.Content class="text-muted-foreground text-sm">
          <ol class="list-decimal space-y-1 ps-4">
            {#if internal}
              <li>Add {session.term("work").toLowerCase()} as it comes up, with owners and due dates.</li>
              <li>Track it on the {session.term("work")} table or board like any other.</li>
              <li>Close the {session.term("engagement").toLowerCase()} when it’s done.</li>
            {:else}
              <li>Add scope items and prices to the draft agreement.</li>
              <li>Send it: the version is frozen and the client decides by link.</li>
              <li>Approval creates one piece of {session.term("work").toLowerCase()} per scope item.</li>
              <li>Changes later are change orders or revisions, each approved the same way.</li>
            {/if}
          </ol>
        </Card.Content>
      </Card.Root>
      {#if client}
        {@const existing = (client.engagements || []).slice(0, 8)}
        <Card.Root>
          <Card.Header><Card.Title>{client.name}’s {engagements.toLowerCase()} <span class="text-muted-foreground font-normal">{existing.length}</span></Card.Title></Card.Header>
          <Card.Content>
            {#if existing.length}
              <ul class="flex flex-col gap-2 text-sm">
                {#each existing as e (e.id)}
                  <li class="flex items-center gap-2">
                    <a href={`/engagements/${e.ref}`} class="min-w-0 flex-1 truncate hover:underline"><strong>{e.ref}</strong> {e.title}</a>
                    <StatusBadge status={e.closed ? e.close_outcome || "closed" : e.state} label={e.state_label} />
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="text-muted-foreground text-sm">This is their first.</p>
            {/if}
          </Card.Content>
        </Card.Root>
      {/if}
    {/if}
  </div>
</form>
