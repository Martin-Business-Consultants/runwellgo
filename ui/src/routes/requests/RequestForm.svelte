<script>
  // Log a request, or change an open one: the subject, the details as rich
  // text, where it came from, and who sent it. A sender email that matches
  // a contact fills in the client and contact.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import Combobox from "$lib/components/Combobox.svelte";
  import { get, post, patch } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { focusInvalid } from "$lib/forms.js";

  let { params } = $props();
  // A screen is made afresh for each address, so its params never change.
  // svelte-ignore state_referenced_locally
  const id = params.id;
  const editing = !!id;
  const back = editing ? `/requests/${id}` : "/requests";
  let form = $state({ subject: "", body: "", source: "email", client_id: null, contact_id: null, sender_name: "", sender_email: "" });
  let clients = $state([]), contacts = $state([]), errors = $state({}), busy = $state(false), loaded = $state(!editing);

  page.set(editing ? "Edit" : "Log a request", [{ label: "Requests", href: "/requests" }]);
  get("/clients?status=all").then((r) => (clients = r.clients.map((c) => ({ value: c.id, label: c.name })))).catch(toastError);
  if (editing) {
    get(`/requests/${id}`).then(({ request: r }) => {
      if (r.status !== "open") { toastError(new Error("Only an open request can be edited.")); go(back); return; }
      form = { subject: r.subject, body: r.body_html || "", source: r.source, client_id: r.client_id, contact_id: r.contact_id, sender_name: r.sender_name || "", sender_email: r.sender_email || "" };
      page.set("Edit", [{ label: "Requests", href: "/requests" }, { label: r.subject, href: back }]);
      loaded = true;
    }).catch(toastError);
  }

  // The contacts to pick from are the chosen client's.
  let contactsFor = null;
  $effect(() => {
    const client = form.client_id;
    if (client === contactsFor) return;
    contactsFor = client;
    if (!client) { contacts = []; return; }
    get(`/clients/${client}`).then((r) => {
      contacts = r.client.contacts.map((c) => ({ value: c.id, label: c.name, hint: c.email || "" }));
      if (form.contact_id && !contacts.some((c) => c.value === form.contact_id)) form.contact_id = null;
    }).catch(() => (contacts = []));
  });

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    const body = { ...form, client_id: form.client_id || null, contact_id: form.contact_id || null };
    try {
      const res = editing ? await patch(`/requests/${id}`, { request: body }) : await post("/requests", { request: body });
      toast(editing ? "Changes saved." : res.summary);
      nav.refresh();
      go(`/requests/${res.request.id}`);
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === "Escape" && !document.querySelector("[role=dialog],[role=listbox]")) go(back); }} />
<form onsubmit={save} class="grid gap-4 lg:grid-cols-3">
  <Card.Root class="lg:col-span-2">
    <Card.Header><Card.Title class="text-xl"><h1>{editing ? form.subject || "Edit" : "Log a request"}</h1></Card.Title></Card.Header>
    <Card.Content>
      {#if loaded}
        <Field.Group>
          <Field.Field data-invalid={!!errors.subject}>
            <Field.Label for="request-subject">Subject</Field.Label>
            <Input id="request-subject" bind:value={form.subject} required autofocus autocomplete="off" placeholder="Can we add a newsletter signup?" aria-invalid={!!errors.subject} />
            {#if errors.subject}<Field.Error>Subject {errors.subject[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="request-body">Details</Field.Label>
            <TiptapEditor id="request-body" bind:value={form.body} format="html" minHeight="min-h-32" />
          </Field.Field>
          <Field.Field data-invalid={!!errors.source}>
            <Field.Label for="request-source">Source</Field.Label>
            <Input id="request-source" bind:value={form.source} required autocomplete="off" placeholder="email: Re: newsletter, or call 9/24" aria-invalid={!!errors.source} />
            {#if errors.source}<Field.Error>Source {errors.source[0]}</Field.Error>{/if}
          </Field.Field>
        </Field.Group>
      {/if}
    </Card.Content>
    <Card.Footer class="gap-2">
      <Button type="submit" disabled={busy || !loaded}>{editing ? "Save changes" : "Log request"}</Button>
      <Button href={back} variant="ghost">Cancel</Button>
    </Card.Footer>
  </Card.Root>

  <div class="flex flex-col gap-4">
    <Card.Root>
      <Card.Header>
        <Card.Title>Who sent it</Card.Title>
        <Card.Description>A sender email that matches a contact fills in the {session.term("client").toLowerCase()} and contact.</Card.Description>
      </Card.Header>
      <Card.Content>
        <Field.Group>
          <Field.Field data-invalid={!!errors.client}>
            <Field.Label for="request-client">{session.term("client")}</Field.Label>
            <Combobox id="request-client" bind:value={form.client_id} options={clients} placeholder="Unknown" invalid={!!errors.client} />
            {#if errors.client}<Field.Error>{session.term("client")} {errors.client[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="request-contact">Contact</Field.Label>
            <Combobox id="request-contact" bind:value={form.contact_id} options={contacts} placeholder="None" emptyText={form.client_id ? "No contacts." : `Choose a ${session.term("client").toLowerCase()} first.`} />
          </Field.Field>
          <Field.Field>
            <Field.Label for="request-sender-name">Sender name</Field.Label>
            <Input id="request-sender-name" bind:value={form.sender_name} autocomplete="off" />
          </Field.Field>
          <Field.Field>
            <Field.Label for="request-sender-email">Sender email</Field.Label>
            <Input id="request-sender-email" type="email" bind:value={form.sender_email} autocomplete="off" />
          </Field.Field>
        </Field.Group>
      </Card.Content>
    </Card.Root>
  </div>
</form>
