<script>
  // Under a draft agreement (draft_version): scope to add, and a check
  // before it's sent. Never on an internal one.
  import SuggestionCard from "./SuggestionCard.svelte";
  let { version, engagement, reload } = $props();
</script>

{#if !engagement.internal}
  <div class="flex flex-col gap-3 border-t pt-4">
    <SuggestionCard kind="scope_draft" record={`Engagement:${engagement.ref}`} onused={reload} />
    {#if version.scope_items?.length}<SuggestionCard kind="agreement_check" record={`Engagement:${engagement.ref}`} onused={reload} />{/if}
  </div>
{/if}
