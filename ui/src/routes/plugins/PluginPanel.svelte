<script>
  // A plugin's panel as a tab on a record's page (a client's, an
  // engagement's, a piece of work's): its screen, called with
  // record=<Type>:<id>, in a card.
  import * as Card from "$lib/components/ui/card/index.js";
  import PluginScreen from "./PluginScreen.svelte";

  let { record, plugin, screen } = $props();
  const name = $derived(record.record || `${record.type}:${record.id}`);
</script>

<Card.Root>
  <Card.Content>
    {#key name}<PluginScreen {plugin} {screen} query={{ record: name }} />{/key}
  </Card.Content>
</Card.Root>
