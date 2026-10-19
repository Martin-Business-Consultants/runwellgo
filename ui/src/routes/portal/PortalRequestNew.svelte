<script>
  // Asking for something: a change, a question, something new, for them to
  // triage.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import PortalLayout from "./PortalLayout.svelte";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { ppost } from "./portal.svelte.js";
  import { focusInvalid } from "$lib/forms.js";

  let subject = $state(""), body = $state(""), errors = $state({}), busy = $state(false);

  async function send(e) {
    e.preventDefault();
    busy = true; errors = {};
    try {
      const res = await ppost("/portal/requests", { request: { subject, body } });
      toast(res.notice);
      go("/portal/requests");
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === "Escape" && !document.querySelector("[role=dialog]")) go("/portal"); }} />
<PortalLayout title="Ask for something" back={{ href: "/portal/requests", label: "Requests" }}>
  <Card.Root class="max-w-2xl">
    <Card.Header><Card.Description>A change, a question, something new. We’ll get back to you.</Card.Description></Card.Header>
    <Card.Content>
      <form onsubmit={send}>
        <Field.Group>
          <Field.Field data-invalid={!!errors.subject}>
            <Field.Label for="request-subject">Subject</Field.Label>
            <Input id="request-subject" bind:value={subject} required autofocus autocomplete="off" />
            {#if errors.subject}<Field.Error>Subject {errors.subject[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label>Details</Field.Label>
            <TiptapEditor bind:value={body} format="html" tools="email" />
          </Field.Field>
          <div><Button type="submit" disabled={busy}>Send request</Button></div>
        </Field.Group>
      </form>
    </Card.Content>
  </Card.Root>
</PortalLayout>
