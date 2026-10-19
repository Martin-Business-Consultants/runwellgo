<script>
  // A person as their avatar (the Rails person_tag): their name on hover and
  // for screen readers. person is {id, name}; none shows fallback. A
  // plugin's mentionable (an AI agent) brings its own avatar_url.
  import * as Avatar from "$lib/components/ui/avatar/index.js";
  import * as Tooltip from "$lib/components/ui/tooltip/index.js";
  import { initials } from "../format.js";
  let { person, size = "size-6", fallback = "Unassigned" } = $props();
</script>

<Tooltip.Provider>
  <Tooltip.Root>
    <Tooltip.Trigger>
      {#snippet child({ props })}
        <span {...props} class="inline-flex">
          <Avatar.Root class="{size} text-[10px]">
            {#if person?.avatar_url}<Avatar.Image src={person.avatar_url} alt={person.name} />{:else if person && /^\d+$/.test(String(person.id))}<Avatar.Image src={`/api/users/${person.id}/avatar`} alt={person.name} />{/if}
            <Avatar.Fallback>{person ? initials(person.name) : "?"}</Avatar.Fallback>
          </Avatar.Root>
          <span class="sr-only">{person ? person.name : fallback}</span>
        </span>
      {/snippet}
    </Tooltip.Trigger>
    <Tooltip.Content>{person ? person.name : fallback}</Tooltip.Content>
  </Tooltip.Root>
</Tooltip.Provider>
