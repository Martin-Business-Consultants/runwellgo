<script>
  // Home, above the briefing (home_top): the day ahead, suggested once a
  // day for each person.
  import * as Card from "$lib/components/ui/card/index.js";
  import SuggestionCard from "./SuggestionCard.svelte";
  import { status } from "./ai.svelte.js";
  status.load();
</script>

{#if status.available}
  <Card.Root>
    <Card.Header><Card.Title>Your day</Card.Title></Card.Header>
    <Card.Content><SuggestionCard kind="day" record="User:me" /></Card.Content>
  </Card.Root>
{/if}
