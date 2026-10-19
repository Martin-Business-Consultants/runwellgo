<script>
  // A place plugins draw into (lib/plugin-slots.js): each component added
  // for name, while its plugin is on, with the props given here.
  import { slotsFor } from "$lib/plugin-slots.js";
  import { isOn } from "../../routes/plugins/extensions.svelte.js";

  let { name, ...props } = $props();
  const entries = $derived(slotsFor(name).filter((e) => name.startsWith("portal_") || isOn(e.key)));
</script>

{#each entries as e (e.key + e.order)}
  <e.component {...props} />
{/each}
