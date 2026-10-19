<script>
  // Adding a website (/websites/new, ?client_id= to start with one) or
  // changing one (/websites/:id/edit): its client (when new), name and
  // address; deleting it beside, for those who may.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { get, post, patch, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { wc } from "./wc.js";
  import { focusInvalid } from "$lib/forms.js";

  let { params } = $props();
  // svelte-ignore state_referenced_locally
  const id = params.id;
  const editing = !!id;
  let form = $state({ client_id: router.query.get("client_id") || "", name: "", url: "" });
  let clients = $state([]), errors = $state({}), busy = $state(false), loaded = $state(!editing), canManage = $state(true);
  const back = editing ? `/websites/${id}` : "/websites";

  page.set(editing ? "Edit" : "New website", [{ label: "Websites", href: "/websites" }]);
  if (editing) {
    get(wc(`/websites/${id}`)).then((s) => {
      form = { client_id: String(s.client.id), name: s.name, url: s.address || "" };
      canManage = s.can_manage;
      page.set(`Edit ${s.name}`, [{ label: "Websites", href: "/websites" }, { label: s.name, href: back }]);
      loaded = true;
    }).catch(toastError);
  } else {
    get(wc("/websites/clients")).then((r) => (clients = r.clients)).catch(toastError);
  }
  const clientName = $derived(clients.find((c) => String(c.id) === form.client_id)?.name);

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    try {
      const r = editing
        ? await patch(wc(`/websites/${id}`), { site: { name: form.name, url: form.url } })
        : await post(wc("/websites"), { site: { ...form, client_id: Number(form.client_id) || null } });
      toast(r.message);
      go(`/websites/${r.id}`);
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
  async function remove() {
    if (!(await confirm({ title: `Delete ${form.name}?`, message: "This forgets the website and its checklist.", confirmLabel: "Delete", destructive: true }))) return;
    try { toast((await del(wc(`/websites/${id}`))).message); go("/websites"); } catch (e) { toastError(e); }
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === "Escape" && !document.querySelector("[role=dialog],[role=listbox]")) go(back); }} />
<form onsubmit={save} class="grid gap-4 lg:grid-cols-3">
  <Card.Root class="lg:col-span-2">
    <Card.Header><Card.Title class="text-xl">{editing ? `Edit ${form.name}` : "New website"}</Card.Title></Card.Header>
    <Card.Content>
      {#if loaded}
        <Field.Group>
          {#if !editing}
            <Field.Field data-invalid={!!errors.client}>
              <Field.Label for="site-client">{session.term("client")}</Field.Label>
              <Select.Root type="single" bind:value={form.client_id}>
                <Select.Trigger id="site-client" class="w-full" aria-invalid={!!errors.client}>{clientName || `Choose a ${session.term("client").toLowerCase()}`}</Select.Trigger>
                <Select.Content class="max-h-72">{#each clients as c (c.id)}<Select.Item value={String(c.id)}>{c.name}</Select.Item>{/each}</Select.Content>
              </Select.Root>
              {#if errors.client}<Field.Error>{session.term("client")} {errors.client[0]}</Field.Error>{/if}
            </Field.Field>
          {/if}
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field data-invalid={!!errors.name}>
              <Field.Label for="site-name">Name</Field.Label>
              <Input id="site-name" bind:value={form.name} required autofocus={!editing} autocomplete="off" placeholder="Main site" aria-invalid={!!errors.name} />
              {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field data-invalid={!!errors.url}>
              <Field.Label for="site-url">Address</Field.Label>
              <Input id="site-url" type="url" bind:value={form.url} autocomplete="off" placeholder="https://example.com" aria-invalid={!!errors.url} />
              {#if errors.url}<Field.Error>Address {errors.url[0]}</Field.Error>{/if}
            </Field.Field>
          </div>
        </Field.Group>
      {/if}
    </Card.Content>
    <Card.Footer class="gap-2">
      <Button type="submit" disabled={busy || !loaded}>{editing ? "Save changes" : "Add website"}</Button>
      <Button href={back} variant="ghost">Cancel</Button>
    </Card.Footer>
  </Card.Root>

  <div class="flex flex-col gap-4">
    {#if !editing}
      <Card.Root>
        <Card.Header><Card.Title>The go-live checklist</Card.Title></Card.Header>
        <Card.Content class="text-muted-foreground text-sm">It comes with the standard steps, in order: access (registrar, DNS, email, Search Console, Analytics…), forms and email, launch, and after launch.</Card.Content>
      </Card.Root>
    {:else if canManage}
      <Card.Root>
        <Card.Header><Card.Title>Delete</Card.Title><Card.Description>Forgets the website and its checklist. Its history stays.</Card.Description></Card.Header>
        <Card.Content><Button type="button" variant="destructive" size="sm" onclick={remove}>Delete website</Button></Card.Content>
      </Card.Root>
    {/if}
  </div>
</form>
