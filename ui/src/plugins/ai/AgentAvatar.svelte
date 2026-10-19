<script>
  // An agent's picture, or its initials on a tinted square (an agent, not
  // a person, so never a circle).
  import * as Avatar from "$lib/components/ui/avatar/index.js";
  import { initials } from "$lib/format.js";
  let { agent, size = "size-8" } = $props();
</script>

<Avatar.Root class="{size} shrink-0 rounded-md after:rounded-md">
  {#if agent?.avatar_url}<Avatar.Image src={agent.avatar_url} alt="" class="rounded-md object-cover" />{/if}
  <Avatar.Fallback class="bg-primary/10 text-primary rounded-md text-[11px] font-semibold">{initials(agent?.name || "Agent")}</Avatar.Fallback>
</Avatar.Root>
