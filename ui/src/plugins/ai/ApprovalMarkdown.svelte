<script>
  // An approval's details, written in markdown by whoever asked: headings,
  // paragraphs, bullets, **bold**, `code`, links (web addresses and app
  // paths only) and fenced code, a diff's lines tinted as added or taken
  // out. Text only: never HTML from the source.
  let { text = "" } = $props();

  function blocks(src) {
    const out = [];
    const lines = (src || "").replace(/\r\n/g, "\n").split("\n");
    for (let i = 0; i < lines.length; i++) {
      const l = lines[i];
      const fence = l.match(/^```(\w*)/);
      if (fence) {
        const body = [];
        for (i++; i < lines.length && !lines[i].startsWith("```"); i++) body.push(lines[i]);
        out.push({ t: "code", lang: fence[1], lines: body });
        continue;
      }
      if (!l.trim()) continue;
      const h = l.match(/^(#{1,4})\s+(.*)/);
      if (h) { out.push({ t: "h", v: h[2] }); continue; }
      if (/^\s*([-*•]|\d+\.)\s+/.test(l)) {
        const items = [];
        for (; i < lines.length && /^\s*([-*•]|\d+\.)\s+/.test(lines[i]); i++) items.push(lines[i].replace(/^\s*([-*•]|\d+\.)\s+/, ""));
        i--;
        out.push({ t: "ul", items });
        continue;
      }
      const para = [];
      for (; i < lines.length && lines[i].trim() && !/^(```|#{1,4}\s|\s*([-*•]|\d+\.)\s)/.test(lines[i]); i++) para.push(lines[i]);
      i--;
      out.push({ t: "p", lines: para });
    }
    return out;
  }

  function inline(s) {
    const out = [];
    const re = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\(((?:https?:\/\/|\/)[^)\s]+)\)|(https?:\/\/[^\s)]+)/g;
    let last = 0, m;
    while ((m = re.exec(s))) {
      if (m.index > last) out.push({ t: "text", v: s.slice(last, m.index) });
      if (m[1] !== undefined) out.push({ t: "b", v: m[1] });
      else if (m[2] !== undefined) out.push({ t: "code", v: m[2] });
      else if (m[3] !== undefined) out.push({ t: "a", v: m[3], href: m[4] });
      else out.push({ t: "a", v: m[5], href: m[5] });
      last = re.lastIndex;
    }
    if (last < s.length) out.push({ t: "text", v: s.slice(last) });
    return out;
  }
  const tint = (lang, l) => (lang !== "diff" ? "" : l.startsWith("+") ? "bg-emerald-50 text-emerald-900 dark:bg-emerald-950/40 dark:text-emerald-200" : l.startsWith("-") ? "bg-red-50 text-red-900 dark:bg-red-950/40 dark:text-red-200" : l.startsWith("@@") ? "text-muted-foreground" : "");
  const parts = $derived(blocks(text));
</script>

{#snippet line(s)}
  {#each inline(s) as p, i (i)}
    {#if p.t === "b"}<strong>{p.v}</strong>
    {:else if p.t === "code"}<code class="bg-muted rounded px-1 text-[0.85em]">{p.v}</code>
    {:else if p.t === "a"}<a href={p.href} class="text-primary underline underline-offset-4" target={p.href.startsWith("/") ? undefined : "_blank"} rel="noopener noreferrer">{p.v}</a>
    {:else}{p.v}{/if}
  {/each}
{/snippet}

<div class="flex flex-col gap-2 text-sm leading-relaxed break-words">
  {#each parts as b, i (i)}
    {#if b.t === "h"}<p class="font-semibold">{@render line(b.v)}</p>
    {:else if b.t === "ul"}<ul class="list-disc ps-5">{#each b.items as it, j (j)}<li>{@render line(it)}</li>{/each}</ul>
    {:else if b.t === "code"}
      <pre class="bg-muted/60 max-h-80 overflow-auto rounded-md border text-xs leading-5"><code class="block min-w-max py-1">{#each b.lines as l, j (j)}<span class="block px-3 {tint(b.lang, l)}">{l || " "}</span>{/each}</code></pre>
    {:else}<p>{#each b.lines as l, j (j)}{#if j > 0}<br />{/if}{@render line(l)}{/each}</p>{/if}
  {/each}
</div>
