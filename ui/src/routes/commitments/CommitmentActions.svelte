<script>
  // A commitment's row actions while it's open: Resolve (a dialog), and
  // Edit and Delete in its menu. A resolved one is final: no actions.
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import EllipsisVertical from "@lucide/svelte/icons/ellipsis-vertical";
  import ResolveDialog from "./ResolveDialog.svelte";
  import { del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { router } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";

  let { commitment, onchange = undefined } = $props();
  let resolving = $state(false);
  const word = session.term("commitment").toLowerCase();

  async function remove() {
    if (!(await confirm({ title: `Delete this ${word}?`, message: "It’s gone for good, with its history.", confirmLabel: "Delete", destructive: true }))) return;
    try { await del(`/commitments/${commitment.id}`); toast("Commitment deleted."); onchange?.(); } catch (e) { toastError(e); }
  }
</script>

{#if commitment.open}
  <div class="flex items-center gap-1">
    <Button variant="outline" size="xs" onclick={() => (resolving = true)}>Resolve</Button>
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><EllipsisVertical /><span class="sr-only">Actions for this {word}</span></Button>{/snippet}</DropdownMenu.Trigger>
      <DropdownMenu.Content align="end">
        <DropdownMenu.Item onSelect={() => router.navigate(`/commitments/${commitment.id}/edit`)}>Edit</DropdownMenu.Item>
        {#if session.can("delete_records")}<DropdownMenu.Item variant="destructive" onSelect={remove}>Delete</DropdownMenu.Item>{/if}
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  </div>
  <ResolveDialog {commitment} bind:open={resolving} ondone={() => onchange?.()} />
{/if}
