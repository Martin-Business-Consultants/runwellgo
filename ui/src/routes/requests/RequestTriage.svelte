<script>
  // Triage, in the request's side column: what should happen with it, as a
  // choice with a line on each (work, a promise, scope on an agreement, a
  // new engagement, or dismiss), and the picked one's form under it. Work
  // is the usual answer, so it's picked first; when the client has no
  // engagement open for it, it starts one in the same step.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
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
  let mode = $state("todo");
  let busy = $state(false);
  let clients = $state([]);

  const NEW = "__new";
  const engagementWord = $derived(session.term("engagement").toLowerCase());
  const engagementsWord = $derived(session.term("engagement", 2).toLowerCase());
  const workWord = $derived(session.term("work").toLowerCase());
  const clientWord = $derived(session.term("client").toLowerCase());
  const anEngagement = $derived(("aeiou".includes(engagementWord[0]) ? "an " : "a ") + engagementWord);
  const clientName = $derived(request.client?.display_name || `This ${clientWord}`);
  const labels = $derived(Object.entries(session.me.terms?.labels || {}).filter(([, l]) => l.enabled !== false).map(([key, l]) => ({ value: key, label: l.one })));
  const options = $derived(request.engagement_options.map((o) => ({ value: o.ref, label: o.label })));
  const people = $derived(session.me.team.map((p) => ({ value: String(p.id), label: p.name })));
  const unmatched = $derived(!request.client_id);
  const labelOf = (list, v, fallback) => list.find((o) => o.value === v)?.label || fallback;

  const choices = $derived([
    { value: "todo", title: `Do it as ${workWord}`, text: `Your team does it, as ${workWord} on ${anEngagement}.` },
    { value: "commitment", title: "Promise it by a date", text: "Track a promise, yours or the client’s, without making work." },
    { value: "change", title: "Add it to an agreement", text: "A scope item the client approves, with its price." },
    { value: "engagement", title: `Start a new ${engagementWord}`, text: "A draft agreement, with this as its first item." },
    { value: "dismiss", title: "Dismiss", text: "Close it with a reason. Nothing is made." },
  ]);

  // svelte-ignore state_referenced_locally
  let engagement = $state({ label: "work_order", title: "", client_id: null });
  // svelte-ignore state_referenced_locally
  let change = $state({ engagement_ref: request.engagement_options[0]?.ref || "", price: "" });
  // svelte-ignore state_referenced_locally
  let todo = $state({ engagement_ref: request.engagement_options[0]?.ref || NEW, title: request.subject, label: "work_order", client_id: null, owner_id: "none", due_on: "" });
  let commitment = $state({ due_on: "", owner_kind: "us", user_id: "none", engagement_ref: "none", client_id: null });
  let dismiss = $state({ reason: "" });

  $effect(() => {
    if (unmatched && mode !== "change" && mode !== "dismiss" && !clients.length) {
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

  function addWork(e) {
    const owner_id = todo.owner_id !== "none" ? Number(todo.owner_id) : null;
    const due_on = todo.due_on || null;
    if (todo.engagement_ref === NEW) {
      run(e, "promote_todo", { new_engagement: { title: todo.title, label: todo.label }, client_id: todo.client_id, owner_id, due_on });
    } else {
      run(e, "promote_todo", { engagement_ref: todo.engagement_ref, owner_id, due_on });
    }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Triage</Card.Title>
    <Card.Description>What should happen with this request?</Card.Description>
  </Card.Header>
  <Card.Content class="flex flex-col gap-5">
    <RadioGroup.Root bind:value={mode} aria-label="What should happen with this request" class="gap-2">
      {#each choices as c (c.value)}
        <Field.Label for="triage-{c.value}">
          <Field.Field orientation="horizontal" class="py-2.5">
            <Field.Content>
              <Field.Title class={c.value === "dismiss" ? "text-destructive" : ""}>{c.title}</Field.Title>
              <Field.Description class="text-xs">{c.text}</Field.Description>
            </Field.Content>
            <RadioGroup.Item value={c.value} id="triage-{c.value}" />
          </Field.Field>
        </Field.Label>
      {/each}
    </RadioGroup.Root>

    {#if mode === "todo"}
      <form class="flex flex-col gap-4 border-t pt-5" onsubmit={addWork}>
        {#if unmatched}
          <Field.Field>
            <Field.Label for="triage-todo-client">{session.term("client")}</Field.Label>
            <Combobox id="triage-todo-client" bind:value={todo.client_id} options={clients} placeholder={`Choose a ${clientWord}`} />
            <Field.Description>No {clientWord} matched this request, so say whose it is.</Field.Description>
          </Field.Field>
        {:else if options.length}
          <Field.Field>
            <Field.Label for="triage-todo-ref">{session.term("engagement")}</Field.Label>
            <Select.Root type="single" bind:value={todo.engagement_ref}>
              <Select.Trigger id="triage-todo-ref" class="w-full"><span class="truncate">{todo.engagement_ref === NEW ? `Start a new ${engagementWord}` : labelOf(options, todo.engagement_ref, "Choose")}</span></Select.Trigger>
              <Select.Content>
                {#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}
                <Select.Separator />
                <Select.Item value={NEW}>Start a new {engagementWord}</Select.Item>
              </Select.Content>
            </Select.Root>
          </Field.Field>
        {:else}
          <p class="bg-muted/60 rounded-lg px-3 py-2 text-sm">{clientName} has no open {engagementsWord} yet, so this starts one for the {workWord} to go on.</p>
        {/if}

        {#if todo.engagement_ref === NEW}
          <Field.Field>
            <Field.Label for="triage-todo-title">New {engagementWord}’s title</Field.Label>
            <Input id="triage-todo-title" bind:value={todo.title} placeholder={request.subject} autocomplete="off" />
          </Field.Field>
          <Field.Field>
            <Field.Label for="triage-todo-label">Kind</Field.Label>
            <Select.Root type="single" bind:value={todo.label}>
              <Select.Trigger id="triage-todo-label" class="w-full">{labelOf(labels, todo.label, "Choose")}</Select.Trigger>
              <Select.Content>{#each labels as l (l.value)}<Select.Item value={l.value}>{l.label}</Select.Item>{/each}</Select.Content>
            </Select.Root>
          </Field.Field>
        {/if}

        <Field.Field>
          <Field.Label for="triage-owner">Owner</Field.Label>
          <Select.Root type="single" bind:value={todo.owner_id}>
            <Select.Trigger id="triage-owner" class="w-full">{labelOf(people, todo.owner_id, "Unowned")}</Select.Trigger>
            <Select.Content><Select.Item value="none">Unowned</Select.Item>{#each people as p (p.value)}<Select.Item value={p.value}>{p.label}</Select.Item>{/each}</Select.Content>
          </Select.Root>
        </Field.Field>
        <Field.Field>
          <Field.Label for="triage-todo-due">Due (optional)</Field.Label>
          <DatePicker id="triage-todo-due" bind:value={todo.due_on} />
        </Field.Field>
        <Button type="submit" disabled={busy || (unmatched && !todo.client_id)}>
          {todo.engagement_ref === NEW ? `Start ${engagementWord} and add ${workWord}` : `Add as ${workWord}`}
        </Button>
      </form>
    {:else if mode === "commitment"}
      <form class="flex flex-col gap-4 border-t pt-5" onsubmit={(e) => run(e, "promote_commitment", { ...commitment, user_id: commitment.user_id !== "none" ? Number(commitment.user_id) : null, engagement_ref: commitment.engagement_ref !== "none" ? commitment.engagement_ref : null })}>
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
        {#if commitment.owner_kind === "us"}
          <Field.Field>
            <Field.Label for="triage-person">Who keeps it</Field.Label>
            <Select.Root type="single" bind:value={commitment.user_id}>
              <Select.Trigger id="triage-person" class="w-full">{labelOf(people, commitment.user_id, "Anyone")}</Select.Trigger>
              <Select.Content><Select.Item value="none">Anyone</Select.Item>{#each people as p (p.value)}<Select.Item value={p.value}>{p.label}</Select.Item>{/each}</Select.Content>
            </Select.Root>
          </Field.Field>
        {/if}
        {#if options.length}
          <Field.Field>
            <Field.Label for="triage-commitment-ref">{session.term("engagement")} (optional)</Field.Label>
            <Select.Root type="single" bind:value={commitment.engagement_ref}>
              <Select.Trigger id="triage-commitment-ref" class="w-full"><span class="truncate">{labelOf(options, commitment.engagement_ref, "None")}</span></Select.Trigger>
              <Select.Content><Select.Item value="none">None</Select.Item>{#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}</Select.Content>
            </Select.Root>
          </Field.Field>
        {/if}
        {#if unmatched}
          <Field.Field>
            <Field.Label for="triage-commitment-client">{session.term("client")}</Field.Label>
            <Combobox id="triage-commitment-client" bind:value={commitment.client_id} options={clients} placeholder={`Choose a ${clientWord}`} />
          </Field.Field>
        {/if}
        <Button type="submit" disabled={busy || !commitment.due_on || (unmatched && !commitment.client_id)}>Make the promise</Button>
      </form>
    {:else if mode === "change"}
      <div class="flex flex-col gap-4 border-t pt-5">
        {#if !options.length}
          <p class="text-muted-foreground text-sm">{clientName} has no open {engagementsWord} to add it to.</p>
          <Button variant="outline" onclick={() => (mode = "engagement")}>Start a new {engagementWord} instead</Button>
        {:else}
          <form class="flex flex-col gap-4" onsubmit={(e) => run(e, "promote_change", { engagement_ref: change.engagement_ref, price_cents: cents(change.price) })}>
            <Field.Field>
              <Field.Label for="triage-change-ref">{session.term("engagement")}</Field.Label>
              <Select.Root type="single" bind:value={change.engagement_ref}>
                <Select.Trigger id="triage-change-ref" class="w-full"><span class="truncate">{labelOf(options, change.engagement_ref, "Choose")}</span></Select.Trigger>
                <Select.Content>{#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}</Select.Content>
              </Select.Root>
              <Field.Description>It goes on the {engagementWord}’s open draft; a change order opens if there isn’t one.</Field.Description>
            </Field.Field>
            {#if session.me.prices}
              <Field.Field>
                <Field.Label for="triage-price">Price</Field.Label>
                <Input id="triage-price" bind:value={change.price} inputmode="decimal" placeholder="0.00" autocomplete="off" />
              </Field.Field>
            {/if}
            <Button type="submit" disabled={busy}>Add to the agreement</Button>
          </form>
        {/if}
      </div>
    {:else if mode === "engagement"}
      <form class="flex flex-col gap-4 border-t pt-5" onsubmit={(e) => run(e, "promote_engagement", engagement)}>
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
            <Combobox id="triage-client" bind:value={engagement.client_id} options={clients} placeholder={`Choose a ${clientWord}`} />
          </Field.Field>
        {/if}
        <Button type="submit" disabled={busy || (unmatched && !engagement.client_id)}>Draft the {engagementWord}</Button>
      </form>
    {:else if mode === "dismiss"}
      <form class="flex flex-col gap-4 border-t pt-5" onsubmit={(e) => run(e, "dismiss", dismiss)}>
        <Field.Field>
          <Field.Label for="triage-reason">Reason</Field.Label>
          <Input id="triage-reason" bind:value={dismiss.reason} placeholder="Not needed" autocomplete="off" />
          {#if request.client_id}<Field.Description>The client sees this reason in their portal.</Field.Description>{/if}
        </Field.Field>
        <Button type="submit" variant="destructive" disabled={busy}>Dismiss request</Button>
      </form>
    {/if}
  </Card.Content>
</Card.Root>
