<script>
  // The AI's words: paragraphs, bullets, **bold**, `code`, and record refs
  // ("Todo:12") as links to the records. Never HTML from the model.
  import { blocks } from "./ai.svelte.js";
  let { text = "", portal = false } = $props();
  const parts = $derived(blocks(text));
</script>

{#snippet line(pieces)}
  {#each pieces as p, i (i)}
    {#if p.t === "b"}<strong>{p.v}</strong>
    {:else if p.t === "code"}<code class="bg-muted rounded px-1 text-[0.85em]">{p.v}</code>
    {:else if p.t === "ref" && !portal}<a href={p.href} class="text-primary underline-offset-4 hover:underline">{p.v}</a>
    {:else}{p.v}{/if}
  {/each}
{/snippet}

<div class="flex flex-col gap-2 text-sm leading-relaxed break-words">
  {#each parts as b, i (i)}
    {#if b.list}
      <ul class="list-disc ps-5">{#each b.lines as l, j (j)}<li>{@render line(l)}</li>{/each}</ul>
    {:else}
      <p>{#each b.lines as l, j (j)}{#if j > 0}<br />{/if}{@render line(l)}{/each}</p>
    {/if}
  {/each}
</div>
