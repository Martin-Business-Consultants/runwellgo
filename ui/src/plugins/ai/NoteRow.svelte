<script>
  // Each note (note_row): the promises and work in it. A button until asked.
  // Not on an agent's own reply.
  import SuggestionCard from "./SuggestionCard.svelte";
  let { note, subjectType, subjectId } = $props();
</script>

{#if note.author?.kind !== "agent"}
  <div class="mt-1"><SuggestionCard kind="note_actions" record={`Note:${note.id}`} on={`${subjectType}:${subjectId}`} row /></div>
{/if}
