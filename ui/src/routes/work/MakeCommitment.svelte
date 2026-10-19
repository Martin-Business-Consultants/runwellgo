<script>
  // Work that was really a promise ("Send the invoice by Friday") becomes a
  // commitment on the client: who promised and by when. Its notes and
  // documents move to the engagement, and the work goes.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Label } from "$lib/components/ui/label/index.js";
  import Bookmark from "@lucide/svelte/icons/bookmark";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import { post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { inDays } from "./work.js";

  let { todo } = $props();
  let kind = $state("us"), busy = $state(false);
  // svelte-ignore state_referenced_locally
  let due = $state(todo.due_on || inDays(7));
  const word = session.term("commitment").toLowerCase();

  async function make() {
    busy = true;
    try {
      const r = await post(`/work/${todo.id}/commitment`, { owner_kind: kind, due_on: due });
      toast(r.summary);
      go(r.path);
    } catch (e) { toastError(e); }
    busy = false;
  }
</script>

<Card.Root>
  <Collapsible.Root>
    <Card.Header>
      <Collapsible.Trigger>
        {#snippet child({ props })}<Button {...props} variant="outline" size="sm" class="w-fit"><Bookmark />Make this a {word}</Button>{/snippet}
      </Collapsible.Trigger>
    </Card.Header>
    <Collapsible.Content>
      <Card.Content class="flex flex-col gap-4 pt-4 text-sm">
        <p class="text-muted-foreground">For a promise with a date rather than {session.term("work").toLowerCase()} to do. Its notes and documents move to {todo.engagement?.ref}.</p>
        <RadioGroup.Root bind:value={kind}>
          <div class="flex items-center gap-2"><RadioGroup.Item value="us" id="mc-us" /><Label for="mc-us">We promised it</Label></div>
          <div class="flex items-center gap-2"><RadioGroup.Item value="client" id="mc-client" /><Label for="mc-client">{todo.client?.display_name} promised it</Label></div>
        </RadioGroup.Root>
        <Field.Field>
          <Field.Label for="mc-due">By</Field.Label>
          <DatePicker id="mc-due" bind:value={due} clearable={false} />
        </Field.Field>
        <Button size="sm" class="w-fit" disabled={busy} onclick={make}>Make it a {word}</Button>
      </Card.Content>
    </Collapsible.Content>
  </Collapsible.Root>
</Card.Root>
