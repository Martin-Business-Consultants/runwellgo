<script>
  // An open request's side column (request_aside), for whoever triages:
  // what it should become.
  import * as Card from "$lib/components/ui/card/index.js";
  import SuggestionCard from "./SuggestionCard.svelte";
  import { session } from "$lib/session.svelte.js";
  let { record, reload } = $props();
</script>

{#if record.status === "open" && session.can("triage_requests")}
  <Card.Root>
    <Card.Header><Card.Title>AI</Card.Title></Card.Header>
    <Card.Content><SuggestionCard kind="triage" record={`Request:${record.id}`} onused={reload} /></Card.Content>
  </Card.Root>
{/if}
