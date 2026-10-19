<script>
  // A new client, or changing one: the working column holds the form, the
  // side column its contacts and what comes next (new) or the Internal
  // switch and Delete (edit), like the Rails settings--split form pages. A
  // new client's draft (its fields and contacts) is kept in this browser
  // until it's saved or discarded, so a reload or a wander elsewhere loses
  // nothing.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import CustomFieldInputs from "$lib/components/CustomFieldInputs.svelte";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import NewContacts from "./NewContacts.svelte";
  import { prefs } from "$lib/prefs.svelte.js";
  import { get, post, patch, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { humanize } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";

  let { params } = $props();
  // A screen is made afresh for each address, so its params never change.
  // svelte-ignore state_referenced_locally
  const id = params.id;
  const editing = !!id;
  let form = $state({ name: "", status: "active", time_zone: "", internal: false, custom_fields: {} });
  let zones = $state([]), errors = $state({}), busy = $state(false), loaded = $state(!editing);
  const back = editing ? `/clients/${id}` : "/clients";

  // The new client's draft, in localStorage (prefs) under one key.
  const DRAFT = "client-draft";
  let contacts = $state([]), restored = $state(false);
  if (!editing) {
    const draft = prefs.get(DRAFT, null);
    if (draft?.form) {
      // svelte-ignore state_referenced_locally
      form = { ...form, ...draft.form };
      contacts = draft.contacts || [];
      restored = true;
    }
  }
  const blankDraft = () => !form.name.trim() && !contacts.length && !Object.values(form.custom_fields).some((v) => v !== "" && v != null);
  $effect(() => {
    if (editing) return;
    const draft = { form: $state.snapshot(form), contacts: $state.snapshot(contacts) };
    if (blankDraft()) prefs.remove(DRAFT); else prefs.set(DRAFT, draft);
  });
  function discard() {
    prefs.remove(DRAFT);
    form = { name: "", status: "active", time_zone: "", internal: false, custom_fields: {} };
    contacts = []; errors = {}; restored = false;
  }
  // The server's problems with the contacts, by position: {"0.email": [...]}.
  const contactErrors = $derived(Object.fromEntries(Object.entries(errors).filter(([k]) => k.startsWith("contacts.")).map(([k, v]) => [k.slice(9), v])));

  page.set(editing ? "Edit" : "New " + session.term("client").toLowerCase(), [{ label: session.term("client", 2), href: "/clients" }]);
  get("/time_zones").then((r) => (zones = r.zones)).catch(() => {});
  if (editing) {
    get(`/clients/${id}`).then(({ client }) => {
      form = { name: client.name, status: client.status, time_zone: client.time_zone || "", internal: client.internal, custom_fields: { ...client.custom_fields } };
      page.set("Edit", [{ label: session.term("client", 2), href: "/clients" }, { label: client.name, href: back }]);
      loaded = true;
    }).catch(toastError);
  }

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    try {
      const res = editing ? await patch(`/clients/${id}`, { client: form }) : await post("/clients", { client: { ...form, contacts } });
      if (!editing) prefs.remove(DRAFT);
      toast(editing ? "Changes saved." : `${session.term("client")} added${contacts.length ? `, with ${contacts.length === 1 ? "1 contact" : contacts.length + " contacts"}` : ""}.`);
      go(`/clients/${res.client.id}`);
    } catch (err) {
      errors = err.errors;
      if (!Object.keys(errors).length) toastError(err);
      else if (Object.keys(errors).some((k) => k.startsWith("contacts."))) toastError(new Error("A contact needs a fix: it’s marked in Contacts."));
      if (Object.keys(errors).length) focusInvalid();
    }
    busy = false;
  }
  async function remove() {
    if (!(await confirm({ title: `Delete ${form.name}?`, message: "This removes its contacts, commitments and notes.", confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/clients/${id}`); toast(`Deleted ${form.name}.`); go("/clients"); } catch (e) { toastError(e); }
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === "Escape" && !document.querySelector("[role=dialog],[role=listbox]")) go(back); }} />
<form onsubmit={save} class="grid gap-4 lg:grid-cols-3">
  <Card.Root class="lg:col-span-2">
    <Card.Header><Card.Title class="text-xl"><h1>{editing ? form.name || "Edit" : "New " + session.term("client").toLowerCase()}</h1></Card.Title></Card.Header>
    <Card.Content>
      {#if restored}
        <Alert.Root class="mb-4">
          <Alert.Description class="flex flex-wrap items-center justify-between gap-2">
            <span>Picked up where you left off: this draft was kept in this browser.</span>
            <Button type="button" variant="outline" size="sm" onclick={discard}>Discard draft</Button>
          </Alert.Description>
        </Alert.Root>
      {/if}
      {#if loaded}
        <Field.Group>
          {#if errors.base}<p class="text-destructive text-sm">{errors.base.join(" ")}</p>{/if}
          <Field.Field data-invalid={!!errors.name}>
            <Field.Label for="client-name">Name</Field.Label>
            <Input id="client-name" bind:value={form.name} required autofocus autocomplete="off" placeholder="Acme Inc." aria-invalid={!!errors.name} />
            {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
          </Field.Field>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field>
              <Field.Label for="client-status">Status</Field.Label>
              <Select.Root type="single" bind:value={form.status}>
                <Select.Trigger id="client-status" class="w-full">{humanize(form.status)}</Select.Trigger>
                <Select.Content>{#each ["active", "dormant", "former"] as s (s)}<Select.Item value={s}>{humanize(s)}</Select.Item>{/each}</Select.Content>
              </Select.Root>
            </Field.Field>
            <Field.Field data-invalid={!!errors.time_zone}>
              <Field.Label for="client-zone">Time zone</Field.Label>
              <Select.Root type="single" bind:value={form.time_zone}>
                <Select.Trigger id="client-zone" class="w-full">{form.time_zone || "Same as ours"}</Select.Trigger>
                <Select.Content class="max-h-72">
                  <Select.Item value="">Same as ours</Select.Item>
                  {#each zones as z (z.name)}<Select.Item value={z.name}>{z.label}</Select.Item>{/each}
                </Select.Content>
              </Select.Root>
              {#if errors.time_zone}<Field.Error>Time zone {errors.time_zone[0]}</Field.Error>{/if}
            </Field.Field>
          </div>
          <CustomFieldInputs model="Client" bind:values={form.custom_fields} />
        </Field.Group>
      {/if}
    </Card.Content>
    <Card.Footer class="gap-2">
      <Button type="submit" disabled={busy || !loaded}>{editing ? "Save changes" : "Add " + session.term("client").toLowerCase()}</Button>
      <Button href={back} variant="ghost">Cancel</Button>
    </Card.Footer>
  </Card.Root>

  <div class="flex flex-col gap-4">
    <Card.Root>
      <Card.Header><Card.Title>Internal</Card.Title></Card.Header>
      <Card.Content>
        <Field.Field orientation="horizontal">
          <Field.Content><Field.Label for="client-internal">This is us</Field.Label><Field.Description>The business or person running Runwell: its {session.term("engagement", 2).toLowerCase()} are internal projects, with no agreement to send.</Field.Description></Field.Content>
          <Switch id="client-internal" bind:checked={form.internal} />
        </Field.Field>
      </Card.Content>
    </Card.Root>
    {#if !editing}
      <NewContacts bind:contacts errors={contactErrors} />
      <Card.Root>
        <Card.Header><Card.Title>Next</Card.Title></Card.Header>
        <Card.Content class="text-muted-foreground text-sm">Draft an {session.term("engagement").toLowerCase()}: what you agreed to do.</Card.Content>
      </Card.Root>
    {:else if session.can("delete_records")}
      <Card.Root>
        <Card.Header><Card.Title>Delete</Card.Title><Card.Description>Only while it has no {session.term("engagement", 2).toLowerCase()}; otherwise set it to former.</Card.Description></Card.Header>
        <Card.Content><Button type="button" variant="destructive" size="sm" onclick={remove}>Delete {session.term("client").toLowerCase()}</Button></Card.Content>
      </Card.Root>
    {/if}
  </div>
</form>
