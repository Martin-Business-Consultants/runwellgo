<script>
  // A question for a person: what's asked and where it came from, Dismiss
  // in the corner, then one-tap choices and a field to type an answer.
  import { Button } from "$lib/components/ui/button/index.js";
  import * as InputGroup from "$lib/components/ui/input-group/index.js";
  import X from "@lucide/svelte/icons/x";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import { post } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { question, onchange } = $props();
  let answer = $state(""), busy = $state(false);
  const path = (url) => (url ? new URL(url).pathname : null);

  async function send(text) {
    if (!text.trim()) return;
    busy = true;
    try { await post(`/questions/${question.id}/answer`, { answer: text.trim() }); toast("Answered."); onchange?.(); } catch (e) { toastError(e); }
    busy = false;
  }
  async function dismiss() {
    busy = true;
    try { await post(`/questions/${question.id}/dismiss`); toast("Dismissed."); onchange?.(); } catch (e) { toastError(e); }
    busy = false;
  }
</script>

<div class="flex flex-col gap-2">
  <div class="flex items-start gap-2">
    <div class="min-w-0 flex-1">
      <p class="text-sm font-medium">{question.text}</p>
      <p class="text-muted-foreground text-xs">
        Asked by {question.asked_by} · {question.source}
        {#if question.subject} · <a href={path(question.subject.url)} class="underline-offset-4 hover:underline">{question.subject.display_name}</a>{/if}
      </p>
      {#if question.context}<p class="text-muted-foreground mt-1 text-xs whitespace-pre-line">{question.context}</p>{/if}
    </div>
    <Button variant="ghost" size="icon-sm" onclick={dismiss} disabled={busy} title="Dismiss"><X /><span class="sr-only">Dismiss this question</span></Button>
  </div>
  <div class="flex flex-wrap items-center gap-2">
    {#each question.choices as choice (choice)}
      <Button variant="outline" size="sm" disabled={busy} onclick={() => send(choice)}>{choice}</Button>
    {/each}
    <form class="min-w-48 flex-1" onsubmit={(e) => { e.preventDefault(); send(answer); }}>
      <InputGroup.Root class="h-8">
        <InputGroup.Input bind:value={answer} required placeholder="Or type an answer" autocomplete="off" aria-label="Answer" />
        <InputGroup.Addon align="inline-end">
          <InputGroup.Button type="submit" size="icon-xs" disabled={busy} title="Send answer"><ArrowRight /><span class="sr-only">Send answer</span></InputGroup.Button>
        </InputGroup.Addon>
      </InputGroup.Root>
    </form>
  </div>
</div>
