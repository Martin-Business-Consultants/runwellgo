<script>
  // A built-in plugin's own screen, drawn only while the plugin is on;
  // otherwise the address is as good as not there.
  import { isOn } from "../routes/plugins/extensions.svelte.js";
  import NotFound from "../routes/NotFound.svelte";
  let { plugin, screen, params } = $props();
  const Screen = $derived(screen);
</script>

{#if isOn(plugin)}
  <Screen {params} />
{:else}
  <NotFound />
{/if}
