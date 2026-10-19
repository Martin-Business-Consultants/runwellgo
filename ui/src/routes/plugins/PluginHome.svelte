<script>
  // Plugins on home: where="top" draws their home_top panels above the
  // briefing, where="briefing" their briefing sections, each as a card.
  // Home renders <PluginHome where="top" /> and <PluginHome where="briefing" />.
  import * as Card from "$lib/components/ui/card/index.js";
  import PluginScreen from "./PluginScreen.svelte";
  import { extensions } from "./extensions.svelte.js";

  let { where = "briefing" } = $props();
  const cards = $derived(where === "top" ? extensions.top.map((p) => ({ ...p, title: p.label })) : extensions.home);
</script>

{#each cards as c (c.key + c.screen)}
  <Card.Root>
    <Card.Header><Card.Title>{c.title}</Card.Title><Card.Description>{c.plugin}</Card.Description></Card.Header>
    <Card.Content><PluginScreen plugin={c.key} screen={c.screen} /></Card.Content>
  </Card.Root>
{/each}
