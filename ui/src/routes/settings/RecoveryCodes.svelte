<script>
  // Recovery codes, shown once: each signs in once in place of a code
  // from the phone. Closing it forgets them.
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import CopyButton from "$lib/components/CopyButton.svelte";

  let { codes = $bindable(null), onclose = undefined } = $props();
  function close() { codes = null; onclose?.(); }
</script>

<Dialog.Root open={!!codes} onOpenChange={(o) => { if (!o) close(); }}>
  <Dialog.Content class="sm:max-w-md" interactOutsideBehavior="ignore">
    <Dialog.Header>
      <Dialog.Title>Save your recovery codes</Dialog.Title>
      <Dialog.Description>If you lose your phone, each of these signs you in once in place of a code. Keep them somewhere safe, like your password manager. <strong>They won’t be shown again.</strong></Dialog.Description>
    </Dialog.Header>
    {#if codes}
      <pre class="bg-muted grid grid-cols-2 gap-x-6 gap-y-1 rounded-md border p-4 font-mono text-sm">{#each codes as c (c)}<span>{c}</span>{/each}</pre>
      <Dialog.Footer class="sm:justify-between">
        <CopyButton text={codes.join("\n")} label="Copy them" />
        <Button onclick={close}>I’ve saved them</Button>
      </Dialog.Footer>
    {/if}
  </Dialog.Content>
</Dialog.Root>
