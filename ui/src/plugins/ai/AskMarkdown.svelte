<script>
  // The AI's words as markdown, drawn piece by piece (never HTML from the
  // model): headings, paragraphs, lists, quotes, tables, code blocks with a
  // Copy button, links (web addresses open in a new tab) and record refs
  // ("Todo:12") as links to the records.
  import AskCode from "./AskCode.svelte";
  import { md } from "./ai.svelte.js";
  let { text = "", portal = false } = $props();
  const blocks = $derived(md(text));
</script>

{#snippet line(pieces)}
  {#each pieces as p, i (i)}
    {#if p.t === "b"}<strong class="font-semibold">{p.v}</strong>
    {:else if p.t === "i"}<em>{p.v}</em>
    {:else if p.t === "code"}<code class="bg-muted rounded px-1 py-0.5 font-mono text-[0.85em]">{p.v}</code>
    {:else if p.t === "link"}<a href={p.href} class="text-primary underline underline-offset-4" target={p.external ? "_blank" : undefined} rel={p.external ? "noopener noreferrer" : undefined}>{p.v}</a>
    {:else if p.t === "ref" && !portal}<a href={p.href} class="text-primary font-medium underline-offset-4 hover:underline">{p.v}</a>
    {:else}{p.v}{/if}
  {/each}
{/snippet}

<div class="flex min-w-0 flex-col gap-3 text-sm leading-relaxed break-words">
  {#each blocks as b, i (i)}
    {#if b.t === "h"}
      <p class="font-semibold {b.level <= 2 ? 'text-base' : 'text-sm'}">{@render line(b.v)}</p>
    {:else if b.t === "ul" || b.t === "ol"}
      <svelte:element this={b.t} class="flex flex-col gap-1 ps-5 {b.t === 'ol' ? 'list-decimal' : 'list-disc'}">
        {#each b.items as item, j (j)}<li>{@render line(item)}</li>{/each}
      </svelte:element>
    {:else if b.t === "quote"}
      <blockquote class="text-muted-foreground border-s-2 ps-3">{#each b.lines as l, j (j)}{#if j > 0}<br />{/if}{@render line(l)}{/each}</blockquote>
    {:else if b.t === "code"}
      <AskCode code={b.code} lang={b.lang} />
    {:else if b.t === "table"}
      <div class="overflow-x-auto rounded-md border">
        <table class="w-full text-left text-[13px] tabular-nums">
          <thead class="bg-muted/50"><tr>{#each b.head as h, j (j)}<th class="px-2.5 py-1.5 font-semibold">{@render line(h)}</th>{/each}</tr></thead>
          <tbody>
            {#each b.rows as r, j (j)}<tr class="border-t">{#each r as c, k (k)}<td class="px-2.5 py-1.5 align-top">{@render line(c)}</td>{/each}</tr>{/each}
          </tbody>
        </table>
      </div>
    {:else}
      <p>{#each b.lines as l, j (j)}{#if j > 0}<br />{/if}{@render line(l)}{/each}</p>
    {/if}
  {/each}
</div>
