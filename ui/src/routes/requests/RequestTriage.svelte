<script>
  // Triage, in the request's side column: turn it into a new engagement
  // (a draft agreement whose first item it is), an item on a draft, work,
  // or a promise, or dismiss it. Every form is ready on the page; the
  // buttons only show the one picked.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as ToggleGroup from "$lib/components/ui/toggle-group/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import Combobox from "$lib/components/Combobox.svelte";
  import { get, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { request } = $props();
  let mode = $state("");
  let busy = $state(false);
  let clients = $state([]);

  const engagementWord = $derived(session.term("engagement").toLowerCase());
  const anEngagement = $derived(("aeiou".includes(engagementWord[0]) ? "an " : "a ") + engagementWord);
  const labels = $derived(Object.entries(session.me.terms?.labels || {}).filter(([, l]) => l.enabled !== false).map(([key, l]) => ({ value: key, label: l.one })));
  const options = $derived(request.engagement_options.map((o) => ({ value: o.ref, label: o.label })));
  const people = $derived(session.me.team.map((p) => ({ value: String(p.id), label: p.name })));
  const unmatched = $derived(!request.client_id);
  const labelOf = (list, v, fallback) => list.find((o) => o.value === v)?.label || fallback;

  // svelte-ignore state_referenced_locally
  let engagement = $state({ label: "work_order", title: "", client_id: null });
  // svelte-ignore state_referenced_locally
  let change = $state({ engagement_ref: request.engagement_options[0]?.ref || "", price: "" });
  // svelte-ignore state_referenced_locally
  let todo = $state({ engagement_ref: request.engagement_options[0]?.ref || "", owner_id: "none", due_on: "" });
  let commitment = $state({ due_on: "", owner_kind: "us", user_id: "none", engagement_ref: "none", client_id: null });
  let dismiss = $state({ reason: "" });

  $effect(() => {
    if (unmatched && (mode === "engagement" || mode === "commitment") && !clients.length) {
      get("/clients?status=all").then((r) => (clients = r.clients.map((c) => ({ value: c.id, label: c.name })))).catch(toastError);
    }
  });

  async function run(e, action, body) {
    e.preventDefault();
    busy = true;
    try {
      const res = await post(`/requests/${request.id}/${action}`, body);
      toast(res.summary);
      nav.refresh();
      go(res.path || "/requests");
    } catch (err) { toastError(err); }
    busy = false;
  }
  const cents = (s) => Math.round(parseFloat(String(s).replace(/[,$\s]/g, "")) * 100) || 0;
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Triage</Card.Title>
    <Card.Description>Turn this into scope, work or a promise, or dismiss it.</Card.Description>
  </Card.Header>
  <Card.Content class="flex flex-col gap-4">
    <ToggleGroup.Root type="single" variant="outline" size="sm" bind:value={mode} class="flex flex-wrap justify-start">
      <ToggleGroup.Item value="engagement">New {engagementWord}</ToggleGroup.Item>
      <ToggleGroup.Item value="change">Add as scope</ToggleGroup.Item>
      <ToggleGroup.Item value="todo">Add as work</ToggleGroup.Item>
      <ToggleGroup.Item value="commitment">Make a commitment</ToggleGroup.Item>
      <ToggleGroup.Item value="dismiss" class="text-destructive">Dismiss</ToggleGroup.Item>
    </ToggleGroup.Root>

    {#if mode === "engagement"}
      <form class="flex flex-col gap-4" onsubmit={(e) => run(e, "promote_engagement", engagement)}>
        <p class="text-muted-foreground text-sm">A new agreement whose first scope item is this request.</p>
        <Field.Field>
          <Field.Label for="triage-label">Kind</Field.Label>
          <Select.Root type="single" bind:value={engagement.label}>
            <Select.Trigger id="triage-label" class="w-full">{labelOf(labels, engagement.label, "Choose")}</Select.Trigger>
            <Select.Content>{#each labels as l (l.value)}<Select.Item value={l.value}>{l.label}</Select.Item>{/each}</Select.Content>
          </Select.Root>
        </Field.Field>
        <Field.Field>
          <Field.Label for="triage-title">Title (optional)</Field.Label>
          <Input id="triage-title" bind:value={engagement.title} placeholder={request.subject} autocomplete="off" />
        </Field.Field>
        {#if unmatched}
          <Field.Field>
            <Field.Label for="triage-client">{session.term("client")}</Field.Label>
            <Combobox id="triage-client" bind:value={engagement.client_id} options={clients} placeholder={`Choose a ${session.term("client").toLowerCase()}`} />
          </Field.Field>
        {/if}
        <div class="flex gap-2">
          <Button type="submit" size="sm" disabled={busy || (unmatched && !engagement.client_id)}>Draft {engagementWord}</Button>
          <Button type="button" size="sm" variant="ghost" onclick={() => (mode = "")}>Cancel</Button>
        </div>
      </form>
    {:else if mode === "change" || mode === "todo"}
      {#if !options.length}
        <p class="text-muted-foreground text-sm">This request’s {session.term("client").toLowerCase()} has no open {session.term("engagement", 2).toLowerCase()}. Draft one instead.</p>
      {:else if mode === "change"}
        <form class="flex flex-col gap-4" onsubmit={(e) => run(e, "promote_change", { engagement_ref: change.engagement_ref, price_cents: cents(change.price) })}>
          <p class="text-muted-foreground text-sm">Adds it to the {engagementWord}’s open draft, opening a change order if there isn’t one.</p>
          <Field.Field>
            <Field.Label for="triage-change-ref">{session.term("engagement")}</Field.Label>
            <Select.Root type="single" bind:value={change.engagement_ref}>
              <Select.Trigger id="triage-change-ref" class="w-full"><span class="truncate">{labelOf(options, change.engagement_ref, "Choose")}</span></Select.Trigger>
              <Select.Content>{#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}</Select.Content>
            </Select.Root>
          </Field.Field>
          {#if session.me.prices}
            <Field.Field>
              <Field.Label for="triage-price">Price</Field.Label>
              <Input id="triage-price" bind:value={change.price} inputmode="decimal" placeholder="0.00" autocomplete="off" />
            </Field.Field>
          {/if}
          <div class="flex gap-2">
            <Button type="submit" size="sm" disabled={busy}>Add as scope</Button>
            <Button type="button" size="sm" variant="ghost" onclick={() => (mode = "")}>Cancel</Button>
          </div>
        </form>
      {:else}
        <form class="flex flex-col gap-4" onsubmit={(e) => run(e, "promote_todo", { engagement_ref: todo.engagement_ref, owner_id: todo.owner_id !== "none" ? Number(todo.owner_id) : null, due_on: todo.due_on || null })}>
          <p class="text-muted-foreground text-sm">{session.term("work")} on {anEngagement} already agreed.</p>
          <Field.Field>
            <Field.Label for="triage-todo-ref">{session.term("engagement")}</Field.Label>
            <Select.Root type="single" bind:value={todo.engagement_ref}>
              <Select.Trigger id="triage-todo-ref" class="w-full"><span class="truncate">{labelOf(options, todo.engagement_ref, "Choose")}</span></Select.Trigger>
              <Select.Content>{#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}</Select.Content>
            </Select.Root>
          </Field.Field>
          <Field.Field>
            <Field.Label for="triage-owner">Owner</Field.Label>
            <Select.Root type="single" bind:value={todo.owner_id}>
              <Select.Trigger id="triage-owner" class="w-full">{labelOf(people, todo.owner_id, "Unowned")}</Select.Trigger>
              <Select.Content><Select.Item value="none">Unowned</Select.Item>{#each people as p (p.value)}<Select.Item value={p.value}>{p.label}</Select.Item>{/each}</Select.Content>
            </Select.Root>
          </Field.Field>
          <Field.Field>
            <Field.Label for="triage-todo-due">Due</Field.Label>
            <DatePicker id="triage-todo-due" bind:value={todo.due_on} />
          </Field.Field>
          <div class="flex gap-2">
            <Button type="submit" size="sm" disabled={busy}>Add as work</Button>
            <Button type="button" size="sm" variant="ghost" onclick={() => (mode = "")}>Cancel</Button>
          </div>
        </form>
      {/if}
    {:else if mode === "commitment"}
      <form class="flex flex-col gap-4" onsubmit={(e) => run(e, "promote_commitment", { ...commitment, user_id: commitment.user_id !== "none" ? Number(commitment.user_id) : null, engagement_ref: commitment.engagement_ref !== "none" ? commitment.engagement_ref : null })}>
        <p class="text-muted-foreground text-sm">A promise with an owner and a date.</p>
        <Field.Field>
          <Field.Label for="triage-commitment-due">Due</Field.Label>
          <DatePicker id="triage-commitment-due" bind:value={commitment.due_on} clearable={false} />
        </Field.Field>
        <Field.Field>
          <Field.Label for="triage-whose">Whose promise</Field.Label>
          <Select.Root type="single" bind:value={commitment.owner_kind}>
            <Select.Trigger id="triage-whose" class="w-full">{commitment.owner_kind === "client" ? "The client’s" : "Ours"}</Select.Trigger>
            <Select.Content><Select.Item value="us">Ours</Select.Item><Select.Item value="client">The client’s</Select.Item></Select.Content>
          </Select.Root>
        </Field.Field>
        <Field.Field>
          <Field.Label for="triage-person">Person (when it’s ours)</Field.Label>
          <Select.Root type="single" bind:value={commitment.user_id}>
            <Select.Trigger id="triage-person" class="w-full">{labelOf(people, commitment.user_id, "Anyone")}</Select.Trigger>
            <Select.Content><Select.Item value="none">Anyone</Select.Item>{#each people as p (p.value)}<Select.Item value={p.value}>{p.label}</Select.Item>{/each}</Select.Content>
          </Select.Root>
        </Field.Field>
        <Field.Field>
          <Field.Label for="triage-commitment-ref">{session.term("engagement")} (optional)</Field.Label>
          <Select.Root type="single" bind:value={commitment.engagement_ref}>
            <Select.Trigger id="triage-commitment-ref" class="w-full"><span class="truncate">{labelOf(options, commitment.engagement_ref, "None")}</span></Select.Trigger>
            <Select.Content><Select.Item value="none">None</Select.Item>{#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}</Select.Content>
          </Select.Root>
        </Field.Field>
        {#if unmatched}
          <Field.Field>
            <Field.Label for="triage-commitment-client">{session.term("client")}</Field.Label>
            <Combobox id="triage-commitment-client" bind:value={commitment.client_id} options={clients} placeholder={`Choose a ${session.term("client").toLowerCase()}`} />
          </Field.Field>
        {/if}
        <div class="flex gap-2">
          <Button type="submit" size="sm" disabled={busy || !commitment.due_on || (unmatched && !commitment.client_id)}>Make commitment</Button>
          <Button type="button" size="sm" variant="ghost" onclick={() => (mode = "")}>Cancel</Button>
        </div>
      </form>
    {:else if mode === "dismiss"}
      <form class="flex flex-col gap-4" onsubmit={(e) => run(e, "dismiss", dismiss)}>
        <Field.Field>
          <Field.Label for="triage-reason">Reason</Field.Label>
          <Input id="triage-reason" bind:value={dismiss.reason} placeholder="Not needed" autocomplete="off" />
        </Field.Field>
        <div class="flex gap-2">
          <Button type="submit" size="sm" variant="destructive" disabled={busy}>Dismiss request</Button>
          <Button type="button" size="sm" variant="ghost" onclick={() => (mode = "")}>Cancel</Button>
        </div>
      </form>
    {/if}
  </Card.Content>
</Card.Root>
