<script>
  // A contact's fields, for the contact dialog on a client and the new
  // client form's contacts. form holds {name, role, email, phone,
  // portal_access, can_approve}; errors are the server's, by field.
  import * as Field from "$lib/components/ui/field/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { session } from "$lib/session.svelte.js";

  let { form = $bindable(), errors = {} } = $props();
</script>

<Field.Group>
  <Field.Field data-invalid={!!errors.name}>
    <Field.Label for="contact-name">Name</Field.Label>
    <Input id="contact-name" bind:value={form.name} required autofocus aria-invalid={!!errors.name} />
    {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
  </Field.Field>
  <div class="grid gap-4 sm:grid-cols-2">
    <Field.Field><Field.Label for="contact-role">Role</Field.Label><Input id="contact-role" bind:value={form.role} placeholder="Owner, Marketing…" /></Field.Field>
    <Field.Field><Field.Label for="contact-phone">Phone</Field.Label><Input id="contact-phone" type="tel" bind:value={form.phone} /></Field.Field>
  </div>
  <Field.Field data-invalid={!!errors.email}>
    <Field.Label for="contact-email">Email</Field.Label>
    <Input id="contact-email" type="email" bind:value={form.email} aria-invalid={!!errors.email} />
    {#if errors.email}<Field.Error>Email {errors.email[0]}</Field.Error>{/if}
  </Field.Field>
  <Field.Field orientation="horizontal">
    <Field.Content><Field.Label for="contact-portal">Portal access</Field.Label><Field.Description>Signs in by an emailed link and sees this {session.term("client").toLowerCase()}’s shared work and documents.</Field.Description></Field.Content>
    <Switch id="contact-portal" bind:checked={form.portal_access} />
  </Field.Field>
  <Field.Field orientation="horizontal">
    <Field.Content><Field.Label for="contact-approve">Can approve</Field.Label><Field.Description>Agreements go to them to approve or ask for changes.</Field.Description></Field.Content>
    <Switch id="contact-approve" bind:checked={form.can_approve} />
  </Field.Field>
</Field.Group>
