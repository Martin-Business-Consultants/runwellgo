<script>
  // A todo's status as a colored dot (the board's colors), or the dot and
  // its name when labelled. Clicking opens the statuses; choosing one saves
  // it, and onchange gets the todo as it now stands.
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { patch } from "$lib/api.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { humanize } from "$lib/format.js";
  import { STATUSES, statusDot } from "./work.js";

  let { todo, labelled = false, onchange = undefined } = $props();

  async function choose(status) {
    if (status === todo.status) return;
    try { onchange?.((await patch(`/work/${todo.id}`, { todo: { status } })).todo); } catch (e) { toastError(e); }
  }
</script>

<DropdownMenu.Root>
  <DropdownMenu.Trigger>
    {#snippet child({ props })}
      <button {...props} type="button" title="{humanize(todo.status)}: click to change"
        class="hover:bg-muted inline-flex items-center gap-1.5 rounded-md px-1.5 py-1 text-sm">
        <span class="size-2.5 shrink-0 rounded-full {statusDot[todo.status]}" aria-hidden="true"></span>
        {#if labelled}<span>{humanize(todo.status)}</span>{/if}
        <span class="sr-only">Status of {todo.title}: {humanize(todo.status)}. Change it</span>
      </button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="start" class="w-44">
    <DropdownMenu.Label>Status</DropdownMenu.Label>
    <DropdownMenu.RadioGroup value={todo.status} onValueChange={choose}>
      {#each STATUSES as s (s)}
        <DropdownMenu.RadioItem value={s}><span class="size-2.5 rounded-full {statusDot[s]}" aria-hidden="true"></span>{humanize(s)}</DropdownMenu.RadioItem>
      {/each}
    </DropdownMenu.RadioGroup>
  </DropdownMenu.Content>
</DropdownMenu.Root>
