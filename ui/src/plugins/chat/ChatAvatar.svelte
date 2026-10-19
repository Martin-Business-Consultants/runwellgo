<script>
  // Someone's chat picture (their own, else their Runwell avatar, else
  // initials), with a dot for whether they're here: filled green online,
  // a ring away, nothing offline. The word is there for screen readers.
  import * as Avatar from "$lib/components/ui/avatar/index.js";
  import { initials } from "$lib/format.js";
  import { PRESENCE } from "./chat.svelte.js";

  let { person, size = "size-8", presence = false, class: klass = "" } = $props();
</script>

<span class="relative inline-flex shrink-0 {klass}">
  <Avatar.Root class="{size} rounded-md after:rounded-md">
    <Avatar.Image src={person?.avatar_url} alt="" class="rounded-md object-cover" />
    <Avatar.Fallback class="rounded-md text-[11px] font-semibold">{initials(person?.name)}</Avatar.Fallback>
  </Avatar.Root>
  {#if presence && person?.presence && person.presence !== "offline"}
    <span class="ring-background absolute -end-0.5 -bottom-0.5 size-2.5 rounded-full ring-2 {person.presence === 'online' ? 'bg-emerald-500' : 'bg-background border-2 border-amber-500'}" title={PRESENCE[person.presence]}></span>
  {/if}
  {#if presence && person?.presence}<span class="sr-only">({PRESENCE[person.presence]})</span>{/if}
</span>
