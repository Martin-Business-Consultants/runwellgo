<script>
  // A small emoji picker: a button that opens a grid; picking one calls
  // onpick and closes it. The trigger is the children snippet's, or a
  // smile-plus button.
  import * as Popover from "$lib/components/ui/popover/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import SmilePlus from "@lucide/svelte/icons/smile-plus";
  import { EMOJI } from "./chat.svelte.js";

  let { onpick, label = "Add a reaction", class: klass = "size-7", align = "end", current = "" } = $props();
  let open = $state(false);

  function pick(e) {
    onpick(e);
    open = false;
  }
</script>

<Popover.Root bind:open>
  <Popover.Trigger>
    {#snippet child({ props })}
      <Button {...props} variant="ghost" size="icon" class={klass} aria-label={label} title={label}>{#if current}<span class="text-lg leading-none">{current}</span>{:else}<SmilePlus />{/if}</Button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Content {align} class="w-72 p-2">
    <div class="grid grid-cols-8 gap-0.5" role="group" aria-label="Emoji">
      {#each EMOJI as e (e)}
        <button type="button" class="hover:bg-muted focus-visible:ring-ring/50 flex size-8 items-center justify-center rounded-md text-lg outline-none focus-visible:ring-[3px]" onclick={() => pick(e)} aria-label={e}>{e}</button>
      {/each}
    </div>
  </Popover.Content>
</Popover.Root>
