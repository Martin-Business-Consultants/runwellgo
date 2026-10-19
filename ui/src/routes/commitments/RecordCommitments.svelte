<script>
  // A client's or an engagement's commitments, its Commitments tab: two
  // lanes, what the other side owes you (waiting on them) and what you've
  // promised them, each a sentence with its date; resolved ones folded away.
  // Adding one reads the same way: who will do what, by when.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import CommitmentActions from "./CommitmentActions.svelte";
  import { get, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { action, details, lanes, statusOf } from "./commitments.js";
  import { focusInvalid } from "$lib/forms.js";
  import PluginSlot from "$lib/components/PluginSlot.svelte";

  let { record, reload = undefined } = $props();
  const isEngagement = $derived(!!record.ref);
  const client = $derived(isEngagement ? record.client : record);
  const clientName = $derived(client?.display_name || client?.name || "them");
  let groups = $state([]), resolvedForClient = $state([]);
  let adding = $state(false), busy = $state(false), errors = $state({});
  let form = $state({ owner: "us", description: "", due_on: "", source: "" });

  $effect(() => {
    if (!client?.id) return;
    get(`/commitments/owners?client_id=${client.id}`).then((r) => (groups = r.owner_options)).catch(() => {});
    if (!isEngagement) loadResolved();
  });
  async function loadResolved() {
    try { resolvedForClient = (await get("/commitments?state=resolved")).commitments.filter((c) => c.client.id === record.id); } catch { resolvedForClient = []; }
  }

  const all = $derived(isEngagement ? record.commitments || [] : record.open_commitments || []);
  const open = $derived(all.filter((c) => c.open));
  const resolved = $derived(isEngagement ? all.filter((c) => !c.open).sort((a, b) => (b.resolved_at || "").localeCompare(a.resolved_at || "")) : resolvedForClient);
  const [theirs, ours] = $derived(lanes(open));
  const word = session.term("commitment").toLowerCase();
  const ownerLabel = $derived(groups.flatMap((g) => g.options).find((o) => o.value === form.owner)?.label || "We");

  function changed() {
    reload?.();
    if (!isEngagement) loadResolved();
  }
  async function add(e) {
    e.preventDefault();
    busy = true; errors = {};
    const path = isEngagement ? `/engagements/${encodeURIComponent(record.ref)}/commitments` : `/clients/${record.id}/commitments`;
    try {
      const r = await post(path, { commitment: form });
      toast(r.summary);
      form = { owner: "us", description: "", due_on: "", source: "" };
      adding = false;
      changed();
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
</script>

{#snippet row(c)}
  <li class="flex items-start gap-2 py-2">
    <div class="min-w-0 flex-1">
      <p class="text-sm"><strong>{c.promiser}</strong> will {action(c)}</p>
      <p class="text-muted-foreground text-xs">{details(c)}</p>
      {#if c.resolution_note}<p class="text-muted-foreground mt-1 text-xs">{c.resolution_note}</p>{/if}
      {#if c.open}<PluginSlot name="commitment_row" commitment={c} />{/if}
    </div>
    {#if c.overdue || !c.open}<StatusBadge status={statusOf(c)} />{/if}
    <CommitmentActions commitment={c} onchange={changed} />
  </li>
{/snippet}

<Card.Root>
  <Card.Header>
    <Card.Title>{session.term("commitment", 2)}</Card.Title>
    <Card.Action><Button variant="outline" size="sm" onclick={() => (adding = !adding)}><Plus />Add a {word}</Button></Card.Action>
  </Card.Header>
  <Card.Content class="flex flex-col gap-4">
    {#if adding}
      <form onsubmit={add} class="flex flex-col gap-2 rounded-lg border p-3">
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <Select.Root type="single" bind:value={form.owner}>
            <Select.Trigger class="w-44" aria-label="Who"><span class="truncate">{ownerLabel}</span></Select.Trigger>
            <Select.Content class="max-h-72">
              {#each groups as g (g.label)}
                <Select.Group>
                  <Select.Label>{g.label}</Select.Label>
                  {#each g.options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}
                </Select.Group>
              {/each}
            </Select.Content>
          </Select.Root>
          <span>will</span>
          <Input bind:value={form.description} required autocomplete="off" placeholder="send the logo" aria-label="What" class="min-w-48 flex-1" aria-invalid={!!errors.description} />
          <span>by</span>
          <div class="w-44"><DatePicker bind:value={form.due_on} placeholder="By when" invalid={!!errors.due_on} /></div>
        </div>
        <Input bind:value={form.source} autocomplete="off" placeholder="Where it was promised, e.g. call on Tuesday (optional)" />
        {#each Object.entries(errors) as [field, msgs] (field)}<p class="text-destructive text-sm">{field === "due_on" ? "Due" : field[0].toUpperCase() + field.slice(1)} {msgs[0]}</p>{/each}
        <div class="flex gap-2">
          <Button type="submit" size="sm" disabled={busy}>Add {word}</Button>
          <Button type="button" variant="ghost" size="sm" onclick={() => (adding = false)}>Cancel</Button>
        </div>
      </form>
    {/if}

    <div class="grid gap-4 xl:grid-cols-2">
      <section>
        <h3 class="text-sm font-medium">Waiting on {clientName} <span class="text-muted-foreground font-normal">{theirs.length}</span></h3>
        {#if theirs.length}<ul class="divide-y">{#each theirs as c (c.id)}{@render row(c)}{/each}</ul>
        {:else}<p class="text-muted-foreground mt-1 text-sm">Nothing they’ve promised is open.</p>{/if}
      </section>
      <section>
        <h3 class="text-sm font-medium">You promised <span class="text-muted-foreground font-normal">{ours.length}</span></h3>
        {#if ours.length}<ul class="divide-y">{#each ours as c (c.id)}{@render row(c)}{/each}</ul>
        {:else}<p class="text-muted-foreground mt-1 text-sm">Nothing you’ve promised is open.</p>{/if}
      </section>
    </div>

    {#if resolved.length}
      <Collapsible.Root>
        <Collapsible.Trigger class="text-muted-foreground hover:text-foreground group flex items-center gap-1 text-sm">
          <ChevronRight class="size-4 transition-transform group-data-[state=open]:rotate-90" />{resolved.length} resolved {resolved.length === 1 ? word : session.term("commitment", 2).toLowerCase()}
        </Collapsible.Trigger>
        <Collapsible.Content><ul class="divide-y">{#each resolved as c (c.id)}{@render row(c)}{/each}</ul></Collapsible.Content>
      </Collapsible.Root>
    {/if}
  </Card.Content>
</Card.Root>
