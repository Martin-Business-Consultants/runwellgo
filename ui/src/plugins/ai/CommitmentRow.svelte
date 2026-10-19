<script>
  // Each open commitment (commitment_row): a reminder to send. A button
  // until asked, so a list costs no lookups.
  import SuggestionCard from "./SuggestionCard.svelte";
  let { commitment } = $props();
</script>

<div class="mt-1.5"><SuggestionCard kind="reminder" record={`Commitment:${commitment.id}`} row /></div>
