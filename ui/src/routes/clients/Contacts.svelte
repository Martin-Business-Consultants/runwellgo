<script>
  // A client's contacts: who they are, whether they can sign in to the
  // portal and approve agreements. Adding or changing one opens a dialog.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import UserPlus from "@lucide/svelte/icons/user-plus";
  import ContactFields from "./ContactFields.svelte";
  import Globe from "@lucide/svelte/icons/globe";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import { post, patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { focusInvalid } from "$lib/forms.js";

  let { client, reload } = $props();
  let open = $state(false), editing = $state(null), form = $state({}), errors = $state({}), busy = $state(false);

  function edit(contact) {
    editing = contact;
    form = contact ? { name: contact.name, role: contact.role || "", email: contact.email || "", phone: contact.phone || "", portal_access: contact.portal_access, can_approve: contact.can_approve }
      : { name: "", role: "", email: "", phone: "", portal_access: false, can_approve: false };
    errors = {};
    open = true;
  }
  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    try {
      if (editing) await patch(`/contacts/${editing.id}`, { contact: form });
      else await post(`/clients/${client.id}/contacts`, { contact: form });
      toast(editing ? "Contact updated." : "Contact added.");
      open = false;
      reload();
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
  async function archive() {
    if (!(await confirm({ title: `Archive ${editing.name}?`, message: "They drop off the contacts and lose portal access and approval rights. Past approvals and requests stay.", confirmLabel: "Archive", destructive: true }))) return;
    try { await del(`/contacts/${editing.id}`); toast(`${editing.name} archived.`); open = false; reload(); } catch (e) { toastError(e); }
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>Contacts <span class="text-muted-foreground font-normal">{client.contacts.length}</span></Card.Title>
    <Card.Action><Button variant="outline" size="sm" onclick={() => edit(null)}><UserPlus />Add</Button></Card.Action>
  </Card.Header>
  <Card.Content>
    {#if !client.contacts.length}
      <p class="text-muted-foreground text-sm">No contacts yet.</p>
    {:else}
      <ul class="flex flex-col gap-1">
        {#each client.contacts as c (c.id)}
          <li>
            <button class="hover:bg-muted flex w-full items-center gap-2 rounded-md px-2 py-1.5 text-start" onclick={() => edit(c)}>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-sm font-medium">{c.name}</span>
                <span class="text-muted-foreground block truncate text-xs">{[c.role, c.email, c.phone].filter(Boolean).join(" · ")}</span>
              </span>
              {#if c.can_sign_in}<span title="Can sign in to the portal"><Globe class="text-muted-foreground size-4" aria-hidden="true" /><span class="sr-only">Can sign in to the portal</span></span>{/if}
              {#if c.can_approve}<span title="Can approve agreements"><CircleCheck class="text-primary size-4" aria-hidden="true" /><span class="sr-only">Can approve agreements</span></span>{/if}
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
        <Dialog.Title>{editing ? editing.name : "Add a contact"}</Dialog.Title>
        <Dialog.Description>Someone at {client.name}.</Dialog.Description>
      </Dialog.Header>
      <ContactFields bind:form {errors} />
      <Dialog.Footer class="sm:justify-between">
        {#if editing && session.can("delete_records")}<Button type="button" variant="ghost" class="text-destructive" onclick={archive}>Archive</Button>{:else}<span></span>{/if}
        <div class="flex gap-2">
          <Dialog.Close>{#snippet child({ props })}<Button {...props} variant="outline">Cancel</Button>{/snippet}</Dialog.Close>
          <Button type="submit" disabled={busy}>{editing ? "Save" : "Add contact"}</Button>
        </div>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>
