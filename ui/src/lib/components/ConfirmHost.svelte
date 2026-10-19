<script>
  // Draws confirm() as shadcn's AlertDialog.
  import * as AlertDialog from "$lib/components/ui/alert-dialog/index.js";
  import { buttonVariants } from "$lib/components/ui/button/index.js";
  import { confirmState } from "../confirm.svelte.js";
  const c = $derived(confirmState.current);
  function finish(answer) {
    c?.resolve(answer);
    confirmState.current = null;
  }
</script>

<AlertDialog.Root open={!!c} onOpenChange={(v) => { if (!v && confirmState.current) finish(false); }}>
  <AlertDialog.Content>
    <AlertDialog.Header>
      <AlertDialog.Title>{c?.title || ""}</AlertDialog.Title>
      {#if c?.message}<AlertDialog.Description>{c.message}</AlertDialog.Description>{/if}
    </AlertDialog.Header>
    <AlertDialog.Footer>
      <AlertDialog.Cancel onclick={() => finish(false)}>Cancel</AlertDialog.Cancel>
      <AlertDialog.Action class={c?.destructive ? buttonVariants({ variant: "destructive" }) : ""} onclick={() => finish(true)}>{c?.confirmLabel}</AlertDialog.Action>
    </AlertDialog.Footer>
  </AlertDialog.Content>
</AlertDialog.Root>
