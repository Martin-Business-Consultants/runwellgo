<script>
  // A suggestion from the AI on a record (ai/suggestions/_card): the button
  // that asks for one, then, in its place, the suggestion as it's made and
  // used: one click to use it (as you), or dismiss it. row: in a list, where
  // it starts as just its button (no lookup per row).
  import { Button } from "$lib/components/ui/button/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Spinner } from "$lib/components/ui/spinner/index.js";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import Check from "@lucide/svelte/icons/check";
  import Copy from "@lucide/svelte/icons/copy";
  import AiText from "./AiText.svelte";
  import { staff, status, inline } from "./ai.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { date, timeAgo } from "$lib/format.js";

  let { kind, record, on = "", row = false, label = "", onused = undefined } = $props();
  // A card is made for one record and kind: its first props are its own.
  // svelte-ignore state_referenced_locally
  let sugg = $state(null), shown = $state(row), busy = $state(false), picked = $state([]), timer = null;
  // svelte-ignore state_referenced_locally
  let name = $state(label);

  const q = () => `?kind=${kind}&record=${encodeURIComponent(record)}${on ? "&on=" + encodeURIComponent(on) : ""}`;
  // svelte-ignore state_referenced_locally
  if (!row) {
    staff.get(`/suggestions/latest${q()}`).then((d) => {
      shown = d.ready && d.available;
      name ||= d.label;
      take(d.suggestion);
    }).catch(() => {});
  } else {
    status.load();
  }

  function take(s) {
    sugg = s;
    clearTimeout(timer);
    if (s?.state === "working") timer = setTimeout(() => staff.get(`/suggestions/${s.id}`).then((d) => take(d.suggestion)).catch(() => {}), 1500);
    if (s?.state === "ready") picked = ticks(s);
  }
  $effect(() => () => clearTimeout(timer));

  // Every item of a list starts ticked.
  function ticks(s) {
    const p = s.payload, out = [];
    (p.commitments || []).forEach((_, i) => out.push(`commitments-${i}`));
    (p.tasks || []).forEach((_, i) => out.push(`tasks-${i}`));
    (p.items || []).forEach((_, i) => out.push(`items-${i}`));
    return out;
  }
  let copied = $state(false);
  async function copy(text) {
    try { await navigator.clipboard.writeText(text || ""); copied = true; setTimeout(() => (copied = false), 1600); } catch {}
  }
  const toggle = (k, on) => (picked = on ? [...picked, k] : picked.filter((x) => x !== k));

  async function askFor() {
    busy = true;
    try { take((await staff.post("/suggestions", { kind, record, on })).suggestion); } catch (e) { toastError(e); }
    busy = false;
  }
  async function decide(decision) {
    busy = true;
    try {
      const r = await staff.patch(`/suggestions/${sugg.id}`, { decision, items: picked });
      if (decision === "accept") { toast(r.message); onused?.(); }
      take(r.suggestion);
    } catch (e) { toastError(e); }
    busy = false;
  }

  const idle = $derived(!sugg || ["accepted", "dismissed", "stale"].includes(sugg.state));
  const visible = $derived(row ? status.available : shown);
  const p = $derived(sugg?.payload || {});
  const person = (id) => (session.me?.team || []).find((t) => t.id === id)?.name;
  const word = (k, n = 1) => session.term(k, n);
  const become = $derived({
    new_engagement: `A new ${word("engagement").toLowerCase()} (${session.label(p.label || "work_order").toLowerCase()})`,
    scope: `Scope on ${p.engagement_ref}`,
    work: `${word("work")} on ${p.engagement_ref}`,
    commitment: `A ${word("commitment").toLowerCase()} (${p.owner === "client" ? "theirs" : "ours"})`,
    dismiss: "Dismiss it",
  }[p.become] || "");
</script>

{#snippet dismiss()}
  <Button variant="ghost" size="sm" onclick={() => decide("dismiss")} disabled={busy}>Dismiss</Button>
{/snippet}

{#snippet pick(k, children)}
  <label class="flex items-start gap-2 text-sm">
    <Checkbox checked={picked.includes(k)} onCheckedChange={(v) => toggle(k, v)} class="mt-0.5" />
    <span class="min-w-0 flex-1">{@render children()}</span>
  </label>
{/snippet}

{#if visible}
  <div class="flex flex-col gap-2">
    {#if idle}
      <div><Button variant="outline" size="sm" onclick={askFor} disabled={busy}><Sparkles />{name || "Ask AI"}</Button></div>
    {:else if sugg.state === "working"}
      <p class="text-muted-foreground flex items-center gap-2 text-sm"><Spinner class="size-4" />Thinking…</p>
    {:else if sugg.state === "failed"}
      <p class="text-destructive text-sm break-words">It couldn’t: {sugg.error}</p>
      <div><Button variant="outline" size="sm" onclick={askFor} disabled={busy}>Try again</Button></div>
    {:else if kind === "agreement_check"}
      {#if p.issues?.length}
        <ul class="list-disc ps-5 text-sm">{#each p.issues as issue, i (i)}<li><strong>{issue.item}</strong>: {issue.problem}</li>{/each}</ul>
      {:else}
        <p class="flex items-center gap-1 text-sm"><Check class="size-4" />Nothing to fix.</p>
      {/if}
      <p class="text-sm"><span class="text-muted-foreground">Suggested summary:</span> {p.summary}</p>
      <div class="flex flex-wrap gap-2"><Button size="sm" onclick={() => decide("accept")} disabled={busy}>Use this summary</Button>{@render dismiss()}</div>
    {:else if kind === "client_week"}
      <AiText text={p.summary} />
      <div class="flex flex-wrap gap-2"><Button size="sm" onclick={() => decide("accept")} disabled={busy}>Save as a note</Button><Button variant="outline" size="sm" onclick={() => copy(p.summary)}>{#if copied}<Check />Copied{:else}<Copy />Copy{/if}</Button>{@render dismiss()}</div>
    {:else if kind === "day"}
      <AiText text={p.summary} />
      {#if p.priorities?.length}
        <ol class="flex list-decimal flex-col gap-1 ps-5 text-sm">
          {#each p.priorities as item, i (i)}
            <li>
              <strong>{item.what}</strong>
              <span class="text-muted-foreground">
                {#each inline(`${item.record} · ${item.why}`) as piece, j (j)}{#if piece.t === "ref"}<a href={piece.href} class="text-primary underline-offset-4 hover:underline">{piece.v}</a>{:else}{piece.v}{/if}{/each}
              </span>
            </li>
          {/each}
        </ol>
      {/if}
      <div class="flex flex-wrap items-center gap-2"><span class="text-muted-foreground text-xs">Written {timeAgo(sugg.updated_at)}.</span>{@render dismiss()}</div>
    {:else if kind === "note_actions"}
      {#if !p.commitments?.length && !p.tasks?.length}
        <p class="text-muted-foreground text-sm">No promises or work in this note.</p>
        <div>{@render dismiss()}</div>
      {:else}
        {#each p.commitments || [] as item, i (i)}
          {#snippet text()}<strong>{item.owner === "client" ? "They" : "We"}</strong> will {item.description}{#if item.due_on}{" · "}by {date(item.due_on)}{/if}{/snippet}
          {@render pick(`commitments-${i}`, text)}
        {/each}
        {#each p.tasks || [] as item, i (i)}
          {#snippet text()}{word("work")}: {item.title}{#if item.engagement_ref}{" · "}{item.engagement_ref}{/if}{#if item.due_on}{" · "}{date(item.due_on)}{/if}{/snippet}
          {@render pick(`tasks-${i}`, text)}
        {/each}
        <div class="flex flex-wrap gap-2"><Button size="sm" onclick={() => decide("accept")} disabled={busy || !picked.length}>Add the ticked ones</Button>{@render dismiss()}</div>
      {/if}
    {:else if kind === "plan_work"}
      {#each p.tasks || [] as task, i (i)}
        {#snippet text()}{task.title}{#if person(task.owner_id)}{" · "}{person(task.owner_id)}{/if}{#if task.due_on}{" · "}{date(task.due_on)}{/if}{#if task.why}{" "}<span class="text-muted-foreground">({task.why})</span>{/if}{/snippet}
        {@render pick(`tasks-${i}`, text)}
      {/each}
      <div class="flex flex-wrap gap-2"><Button size="sm" onclick={() => decide("accept")} disabled={busy || !picked.length}>Add the ticked ones</Button>{@render dismiss()}</div>
    {:else if kind === "scope_draft"}
      {#each p.items || [] as item, i (i)}
        {#snippet text()}{item.description}{#if item.internal_estimate}{" "}<span class="text-muted-foreground">· {item.internal_estimate} (internal)</span>{/if}{/snippet}
        {@render pick(`items-${i}`, text)}
      {/each}
      <div class="flex flex-wrap gap-2"><Button size="sm" onclick={() => decide("accept")} disabled={busy || !picked.length}>Add the ticked ones to the draft</Button>{@render dismiss()}</div>
    {:else if kind === "reminder"}
      <p class="bg-muted rounded-md px-3 py-2 text-sm whitespace-pre-wrap">{p.message}</p>
      <div class="flex flex-wrap gap-2"><Button variant="outline" size="sm" onclick={() => copy(p.message)}>{#if copied}<Check />Copied{:else}<Copy />Copy{/if}</Button><Button variant="outline" size="sm" onclick={() => decide("accept")} disabled={busy}>Used it</Button>{@render dismiss()}</div>
    {:else if kind === "triage"}
      <p class="text-sm"><strong>{become}</strong>{#if p.due_on}{" · "}by {date(p.due_on)}{/if}</p>
      {#if p.reason}<p class="text-muted-foreground text-sm">{p.reason}</p>{/if}
      {#if p.out_of_scope}<div><Badge variant="outline">Beyond the agreed scope</Badge></div>{/if}
      <div class="flex flex-wrap gap-2">
        {#if session.can("triage_requests")}<Button size="sm" onclick={() => decide("accept")} disabled={busy}>Do this</Button>{/if}
        {@render dismiss()}
      </div>
    {/if}
  </div>
{/if}
