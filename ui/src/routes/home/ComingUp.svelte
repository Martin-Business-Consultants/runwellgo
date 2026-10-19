<script>
  // What's coming up for you, by the day: your work and the promises
  // waiting, late first. Tick work off or mark a promise done right here.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Handshake from "@lucide/svelte/icons/handshake";
  import { patch, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { items, today, onchange } = $props();
  let gone = $state([]);
  const work = session.term("work").toLowerCase();

  function when(ymd) {
    const days = Math.round((new Date(ymd + "T00:00:00") - new Date(today + "T00:00:00")) / 86400000);
    if (days < 0) return "Late";
    if (days === 0) return "Today";
    if (days === 1) return "Tomorrow";
    return new Date(ymd + "T00:00:00").toLocaleDateString(undefined, { weekday: "long", month: "short", day: "numeric" });
  }
  const groups = $derived.by(() => {
    const out = [];
    for (const it of items.filter((i) => !gone.includes(i.type + i.id))) {
      const label = when(it.due_on);
      (out.at(-1)?.label === label ? out.at(-1) : (out.push({ label, items: [] }), out.at(-1))).items.push(it);
    }
    return out;
  });

  async function done(it) {
    gone = [...gone, it.type + it.id];
    try {
      if (it.type === "Todo") { await patch(`/work/${it.id}`, { todo: { status: "done" } }); toast(`Done: ${it.title}.`); }
      else { await post(`/commitments/${it.id}/resolve`, { resolution: "done" }); toast("Marked done."); }
      onchange?.();
    } catch (e) { gone = gone.filter((g) => g !== it.type + it.id); toastError(e); }
  }
</script>

<Card.Root class="gap-3">
  <Card.Header>
    <Card.Title>Coming up for you</Card.Title>
    <Card.Action><a href="/work?owner=me" class="text-muted-foreground text-sm hover:underline">All your {work}</a></Card.Action>
  </Card.Header>
  <Card.Content>
    {#if !groups.length}
      <p class="text-muted-foreground text-sm">Nothing due this week. Enjoy the quiet.</p>
    {:else}
      <ol class="flex flex-col gap-4">
        {#each groups as g (g.label)}
          <li>
            <h3 class="mb-1.5 text-xs font-semibold {g.label === 'Late' ? 'text-destructive' : 'text-muted-foreground'}">{g.label} <span class="font-normal tabular-nums">{g.items.length}</span></h3>
            <ul class="flex flex-col gap-1">
              {#each g.items as it (it.type + it.id)}
                <li class="hover:bg-muted/50 -mx-2 flex items-center gap-2.5 rounded-md px-2 py-1.5">
                  {#if it.type === "Todo"}
                    <Checkbox aria-label={`Mark ${it.title} done`} onCheckedChange={(v) => v && done(it)} />
                  {:else}
                    <Handshake class="text-muted-foreground size-4 shrink-0" aria-label={session.term("commitment")} />
                  {/if}
                  <a href={it.path} class="min-w-0 flex-1 truncate text-sm hover:underline">
                    {#if it.type === "Commitment"}<strong class="font-medium">{it.promiser}</strong> will {it.action}{:else}{it.title}{/if}
                  </a>
                  {#if it.type === "Todo"}<span class="text-muted-foreground text-xs">{it.meta}</span>{:else}<Button variant="ghost" size="xs" onclick={() => done(it)}>Done</Button>{/if}
                </li>
              {/each}
            </ul>
          </li>
        {/each}
      </ol>
    {/if}
  </Card.Content>
</Card.Root>
