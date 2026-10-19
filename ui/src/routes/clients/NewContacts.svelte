<script>
  // The contacts of a client not saved yet: added, changed and removed here,
  // kept with the form's draft, and sent with the client when it's saved
  // (all or none). errors are the server's for them, by position
  // ({"0.email": [...]}), from that save.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import UserPlus from "@lucide/svelte/icons/user-plus";
  import Globe from "@lucide/svelte/icons/globe";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import ContactFields from "./ContactFields.svelte";

  let { contacts = $bindable([]), errors = {} } = $props();
  let open = $state(false), index = $state(-1), form = $state({}), problems = $state({});

  const blank = () => ({ name: "", role: "", email: "", phone: "", portal_access: false, can_approve: false });
  // The server's problems for contact i, by field.
  const problemsOf = (i) => Object.fromEntries(Object.entries(errors).filter(([k]) => k.startsWith(i + ".")).map(([k, v]) => [k.slice(String(i).length + 1), v]));

  function edit(i) {
    index = i;
    form = i < 0 ? blank() : { ...contacts[i] };
    problems = i < 0 ? {} : problemsOf(i);
    open = true;
  }
  // What can be told here, before the server sees it.
  function check() {
    const p = {};
    const email = form.email.trim().toLowerCase();
    if (!form.name.trim()) p.name = ["can't be blank"];
    if (form.portal_access && !email) p.email = ["is needed for portal access"];
    else if (email && contacts.some((c, i) => i !== index && c.email.trim().toLowerCase() === email)) p.email = ["is already one of these contacts'"];
    return p;
  }
  function save(e) {
    e.preventDefault();
    problems = check();
    if (Object.keys(problems).length) return;
    const c = { ...form, name: form.name.trim(), email: form.email.trim().toLowerCase() };
    contacts = index < 0 ? [...contacts, c] : contacts.map((x, i) => (i === index ? c : x));
    open = false;
  }
  function remove() {
    contacts = contacts.filter((_, i) => i !== index);
    open = false;
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Contacts {#if contacts.length}<span class="text-muted-foreground font-normal">{contacts.length}</span>{/if}</Card.Title>
    <Card.Description>Added with the client when you save it.</Card.Description>
    <Card.Action><Button type="button" variant="outline" size="sm" onclick={() => edit(-1)}><UserPlus />Add</Button></Card.Action>
  </Card.Header>
  <Card.Content>
    {#if !contacts.length}
      <p class="text-muted-foreground text-sm">None yet. Add one who can approve agreements.</p>
    {:else}
      <ul class="flex flex-col gap-1">
        {#each contacts as c, i (i)}
          {@const bad = Object.keys(problemsOf(i)).length > 0}
          <li>
            <button type="button" class="hover:bg-muted flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-start" onclick={() => edit(i)}>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-sm font-medium">{c.name}</span>
                <span class="text-muted-foreground block truncate text-xs">{[c.role, c.email, c.phone].filter(Boolean).join(" · ")}</span>
              </span>
              {#if bad}<CircleAlert class="text-destructive size-4" aria-label="Needs a fix" />{/if}
              {#if c.portal_access}<Globe class="text-muted-foreground size-4" aria-label="Portal access" />{/if}
              {#if c.can_approve}<CircleCheck class="text-primary size-4" aria-label="Can approve" />{/if}
            </button>
          </li>
        {/each}
      </ul>
    {/if}
  </Card.Content>
</Card.Root>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-md">
    <form onsubmit={save} class="flex flex-col gap-4">
      <Dialog.Header>
        <Dialog.Title>{index < 0 ? "Add a contact" : form.name || "Contact"}</Dialog.Title>
        <Dialog.Description>Someone at the new client.</Dialog.Description>
      </Dialog.Header>
      <ContactFields bind:form errors={problems} />
      <Dialog.Footer class="sm:justify-between">
        {#if index >= 0}<Button type="button" variant="ghost" class="text-destructive" onclick={remove}>Remove</Button>{:else}<span></span>{/if}
        <div class="flex gap-2">
          <Dialog.Close>{#snippet child({ props })}<Button {...props} type="button" variant="outline">Cancel</Button>{/snippet}</Dialog.Close>
          <Button type="submit">{index < 0 ? "Add contact" : "Done"}</Button>
        </div>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
