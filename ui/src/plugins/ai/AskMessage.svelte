<script>
  // One message in Ask: the person's (with any pictures, Copy and, on the
  // last question, Edit to change it and ask again), or the AI's beside its
  // icon: what it was thinking (folded away), what it read or did (a
  // compact card each, opening to the result), each change it proposes to
  // approve or decline, its answer as markdown, a question handed to an
  // agent, and Copy, thumbs up or down and, on the last answer, Regenerate.
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import Brain from "@lucide/svelte/icons/brain";
  import Search from "@lucide/svelte/icons/search";
  import BookOpen from "@lucide/svelte/icons/book-open";
  import Wrench from "@lucide/svelte/icons/wrench";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Check from "@lucide/svelte/icons/check";
  import X from "@lucide/svelte/icons/x";
  import Copy from "@lucide/svelte/icons/copy";
  import ThumbsUp from "@lucide/svelte/icons/thumbs-up";
  import ThumbsDown from "@lucide/svelte/icons/thumbs-down";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import Bot from "@lucide/svelte/icons/bot";
  import Inbox from "@lucide/svelte/icons/inbox";
  import AskMarkdown from "./AskMarkdown.svelte";
  import { copyText } from "./ai.svelte.js";
  import { toast } from "$lib/toast.svelte.js";

  let { m, chatId = null, first = true, portal = false, busy = false, lastQuestion = false, lastAnswer = false, ondecide = null, onrate = null, onregenerate = null, onedit = null } = $props();

  let editing = $state(false), draft = $state("");
  const icons = { search: Search, read_record: BookOpen, find_tools: Search };

  async function copy() {
    if (await copyText(m.content || "")) toast("Copied.");
  }
  function startEdit() { draft = m.content; editing = true; }
  function saveEdit() {
    if (!draft.trim()) return;
    editing = false;
    onedit?.(draft.trim());
  }
  function editKey(e) {
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); saveEdit(); }
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); editing = false; }
  }
  const reads = $derived((m.tool_calls || []).filter((c) => !c.proposal));
  const proposals = $derived((m.tool_calls || []).filter((c) => c.proposal));
</script>

{#snippet action(label, Icon, fn, pressed = undefined)}
  <Button variant="ghost" size="icon-sm" class="text-muted-foreground hover:text-foreground size-7 {pressed ? 'text-foreground bg-accent' : ''}" onclick={fn} aria-label={label} title={label} aria-pressed={pressed} disabled={busy}><Icon class="size-3.5" /></Button>
{/snippet}

{#if m.role === "user"}
  <div class="group flex flex-col items-end gap-1">
    {#if m.images > 0 && chatId && !portal}
      <div class="flex flex-wrap justify-end gap-1.5">
        {#each Array(m.images) as _, i (i)}
          <img src={`/api/x/ai/chats/${chatId}/messages/${m.id}/images/${i}`} alt="Sent with the question" class="size-20 rounded-md border object-cover" loading="lazy" />
        {/each}
      </div>
    {/if}
    {#if editing}
      <div class="flex w-full max-w-[90%] flex-col gap-2">
        <Textarea bind:value={draft} rows={3} onkeydown={editKey} aria-label="Edit your question" class="max-h-48 min-h-16 resize-none" autofocus />
        <div class="flex justify-end gap-2">
          <Button size="sm" variant="ghost" onclick={() => (editing = false)}>Cancel</Button>
          <Button size="sm" onclick={saveEdit} disabled={!draft.trim()}>Send</Button>
        </div>
      </div>
    {:else}
      <div class="bg-primary text-primary-foreground max-w-[85%] rounded-2xl rounded-br-md px-3.5 py-2 text-sm break-words whitespace-pre-wrap">{m.content}</div>
      {#if !portal}
        <div class="flex gap-0.5 opacity-100 transition-opacity focus-within:opacity-100 sm:opacity-0 sm:group-hover:opacity-100">
          {@render action("Copy", Copy, copy)}
          {#if lastQuestion && onedit}{@render action("Edit and ask again", Pencil, startEdit)}{/if}
        </div>
      {/if}
    {/if}
  </div>
{:else}
  <div class="group flex gap-2.5">
    {#if first}
      <span class="bg-background text-primary mt-0.5 flex size-7 shrink-0 items-center justify-center rounded-full border" aria-hidden="true">
        {#if m.agent_run}<Bot class="size-3.5" />{:else}<Sparkles class="size-3.5" />{/if}
      </span>
    {:else}
      <span class="w-7 shrink-0" aria-hidden="true"></span>
    {/if}
    <div class="flex min-w-0 flex-1 flex-col gap-2">
      {#if m.thinking}
        <Collapsible.Root>
          <Collapsible.Trigger>
            {#snippet child({ props })}
              <button {...props} class="text-muted-foreground hover:text-foreground group/t flex items-center gap-1.5 text-xs">
                <Brain class="size-3.5" />Thinking<ChevronRight class="size-3.5 transition-transform group-data-[state=open]/t:rotate-90 motion-reduce:transition-none" />
              </button>
            {/snippet}
          </Collapsible.Trigger>
          <Collapsible.Content>
            <p class="text-muted-foreground mt-1.5 border-s-2 ps-3 text-xs leading-relaxed whitespace-pre-wrap">{m.thinking}</p>
          </Collapsible.Content>
        </Collapsible.Root>
      {/if}

      {#if reads.length}
        <div class="flex flex-col gap-1">
          {#each reads as c (c.id)}
            {@const Icon = icons[c.name] || Wrench}
            <Collapsible.Root class="bg-muted/30 rounded-md border">
              <Collapsible.Trigger>
                {#snippet child({ props })}
                  <button {...props} class="group/c flex w-full items-center gap-2 px-2.5 py-1.5 text-left text-xs">
                    <Icon class="text-muted-foreground size-3.5 shrink-0" />
                    <span class="min-w-0 flex-1 truncate">{c.activity}</span>
                    {#if c.state === "failed"}<span class="text-destructive">Didn’t work</span>{/if}
                    <ChevronRight class="text-muted-foreground size-3.5 shrink-0 transition-transform group-data-[state=open]/c:rotate-90 motion-reduce:transition-none" />
                  </button>
                {/snippet}
              </Collapsible.Trigger>
              <Collapsible.Content>
                <p class="text-muted-foreground border-t px-2.5 py-1.5 text-xs break-words">{c.summary || (c.state === "done" ? "Done, nothing to say." : "No answer yet.")}</p>
              </Collapsible.Content>
            </Collapsible.Root>
          {/each}
        </div>
      {/if}

      {#if m.agent_run}
        <a href={m.agent_run.url} class="hover:bg-accent flex items-center gap-2 rounded-md border px-3 py-2 text-sm">
          <Bot class="text-primary size-4 shrink-0" />
          <span class="min-w-0 flex-1"><span class="font-medium">Asked @{m.agent_run.handle}</span><span class="text-muted-foreground block truncate text-xs">{m.content}</span></span>
          <span class="text-primary text-xs">See its run</span>
        </a>
      {:else if m.content}
        {#if m.failed}
          <p class="text-destructive text-sm break-words">{m.content}</p>
        {:else}
          <AskMarkdown text={m.content} {portal} />
        {/if}
      {/if}

      {#each proposals as c (c.id)}
        <div class="flex flex-col gap-2 rounded-lg border p-3 text-sm {c.state === 'pending' ? 'border-primary/40 bg-primary/5' : ''}">
          <p class="flex items-center gap-2 font-medium"><Pencil class="text-primary size-4 shrink-0" />{c.activity}</p>
          {#if c.details?.length}
            <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1 text-[13px]">
              {#each c.details as [label, value], i (i)}<dt class="text-muted-foreground">{label}</dt><dd class="min-w-0 break-words">{value}</dd>{/each}
            </dl>
          {/if}
          {#if c.reaches_client}<p class="text-muted-foreground text-xs">Reaches the client: {c.reaches_client}</p>{/if}
          {#if c.state === "pending"}
            <div class="flex flex-wrap items-center gap-2">
              <Button size="sm" onclick={() => ondecide?.(c, "approve")} disabled={busy}><Check />Approve</Button>
              <Button size="sm" variant="outline" onclick={() => ondecide?.(c, "decline")} disabled={busy}><X />Decline</Button>
              {#if !portal}<a href="/ai/approvals" class="text-muted-foreground hover:text-foreground ms-auto inline-flex items-center gap-1 text-xs"><Inbox class="size-3.5" />Also in Approvals</a>{/if}
            </div>
          {:else if c.state === "declined"}
            <p class="text-muted-foreground text-xs">Declined</p>
          {:else if c.state === "failed"}
            <p class="text-destructive text-xs">Approved, but it didn’t work: {c.summary || "it failed"}</p>
          {:else if c.state === "done"}
            <p class="flex items-center gap-1 text-xs"><Check class="size-3.5" />Approved. {c.summary || "Done."}</p>
          {:else}
            <p class="text-muted-foreground text-xs">Approved, running…</p>
          {/if}
        </div>
      {/each}

      {#if m.stopped}<p class="text-muted-foreground text-xs">You stopped this answer.</p>{/if}

      {#if !portal && !m.failed && !m.agent_run && m.content}
        <div class="-ms-1.5 flex gap-0.5 opacity-100 transition-opacity focus-within:opacity-100 sm:opacity-0 sm:group-hover:opacity-100 {m.feedback || lastAnswer ? 'sm:opacity-100' : ''}">
          {@render action("Copy", Copy, copy)}
          {@render action("Helpful", ThumbsUp, () => onrate?.(m, m.feedback === "up" ? "none" : "up"), m.feedback === "up")}
          {@render action("Not helpful", ThumbsDown, () => onrate?.(m, m.feedback === "down" ? "none" : "down"), m.feedback === "down")}
          {#if lastAnswer && onregenerate}{@render action("Regenerate", RefreshCw, onregenerate)}{/if}
        </div>
      {:else if !portal && lastAnswer && onregenerate && m.failed}
        <div class="-ms-1.5 flex">{@render action("Try again", RefreshCw, onregenerate)}</div>
      {/if}
    </div>
  </div>
{/if}
