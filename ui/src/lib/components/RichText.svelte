<script>
  // Rich text as stored (HTML the server cleaned), or plain text as
  // paragraphs.
  let { html = "", class: klass = "" } = $props();
  const isHTML = $derived(/<[a-z]/i.test(html || ""));
</script>

{#if html}
  {#if isHTML}
    <div class="prose-admin text-sm {klass}">{@html html}</div>
  {:else}
    <div class="text-sm whitespace-pre-line {klass}">{html}</div>
  {/if}
{/if}
