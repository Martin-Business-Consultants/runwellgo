<script>
  // A scope item's side column (scope_item_aside): the work it needs.
  import * as Card from "$lib/components/ui/card/index.js";
  import SuggestionCard from "./SuggestionCard.svelte";
  let { record, reload } = $props();
</script>

<Card.Root>
  <Card.Header><Card.Title>AI</Card.Title></Card.Header>
  <Card.Content><SuggestionCard kind="plan_work" record={`ScopeItem:${record.id}`} onused={reload} /></Card.Content>
</Card.Root>
