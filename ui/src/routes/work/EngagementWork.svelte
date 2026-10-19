<script>
  // An engagement's Work tab: its work by scope item, open by default (the
  // filter shows done or all). Each scope item's "done of total" counts all
  // of its work. ?item=<id> (or "other") shows one scope item's work alone,
  // with an Add work form for it.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import QueryFilter from "$lib/components/QueryFilter.svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Separator } from "$lib/components/ui/separator/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import ArrowRight from "@lucide/svelte/icons/arrow-right";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import StatusPicker from "./StatusPicker.svelte";
  import OwnerPicker from "./OwnerPicker.svelte";
  import { get, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { router, href, withQuery, go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { shortDate, copyBrief } from "./work.js";

  let { record, reload = undefined } = $props();
  let data = $state(null);
  let adding = $state(false), busy = $state(false);
  let form = $state({ title: "", owner_id: "", due_on: "", scope_item_id: "" });

  const showing = $derived(["done", "all"].includes(router.query.get("work")) ? router.query.get("work") : "open");
  const itemParam = $derived(router.query.get("item"));
  const picked = $derived(data?.scope_items.find((i) => String(i.id) === itemParam) || null);
  const focus = $derived(!!picked || itemParam === "other");
  const word = session.term("work");

  async function load() {
    try { data = await get(href("/work", { engagement: record.ref, status: "all" })); } catch (e) { toastError(e); }
  }
  $effect(() => { record.ref; load(); });

  const all = $derived((data?.work || []).filter((t) => !focus || (picked ? t.scope_item?.id === picked.id : !t.scope_item)));
  const shown = $derived(all.filter((t) => showing === "all" || (showing === "done") === (t.status === "done")));
  // Groups: the agreed items in order, then any other item work points at,
  // then other work; only groups with something shown.
  const groups = $derived.by(() => {
    if (!data) return [];
    const items = [...data.scope_items];
    for (const t of all) if (t.scope_item && !items.some((i) => i.id === t.scope_item.id)) items.push({ ...t.scope_item, description: t.scope_item.display_name });
    const out = items.map((item) => ({ item, everything: all.filter((t) => t.scope_item?.id === item.id), list: shown.filter((t) => t.scope_item?.id === item.id) }));
    out.push({ item: null, everything: all.filter((t) => !t.scope_item), list: shown.filter((t) => !t.scope_item) });
    return out.filter((g) => g.list.length);
  });
  const done = (list) => list.filter((t) => t.status === "done").length;

  function replace(t) {
    const i = data.work.findIndex((x) => x.id === t.id);
    if (i >= 0) data.work[i] = t;
    load();
    reload?.();
  }
  async function add(e) {
    e.preventDefault();
    busy = true;
    const item = picked ? picked.id : focus ? null : form.scope_item_id || null;
    try {
      const r = await post(`/engagements/${encodeURIComponent(record.ref)}/work`, { todo: { title: form.title, owner_id: form.owner_id || null, due_on: form.due_on, scope_item_id: item } });
      toast(r.summary);
      form = { title: "", owner_id: "", due_on: "", scope_item_id: "" };
      load();
      reload?.();
    } catch (err) { toastError(err); }
    busy = false;
  }
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>{word}</Card.Title>
    <Card.Action>
      <QueryFilter param="work" title="Show" options={[{ value: "open", label: "Open" }, { value: "done", label: "Done" }, { value: "all", label: "All" }]} />
    </Card.Action>
  </Card.Header>
  <Card.Content class="flex flex-col gap-4">
    {#if !data}
      <Skeleton class="h-24" />
    {:else}
      {#if focus}
        <div class="flex items-center gap-2">
          <h3 class="truncate font-medium">{picked ? picked.description : "Other work"}</h3>
          <Separator class="flex-1" />
          <span class="text-muted-foreground text-sm whitespace-nowrap">{done(all)} of {all.length} done</span>
          {#if picked}<StatusBadge status={picked.delivery_state} />{/if}
          <Button variant="ghost" size="sm" href={withQuery({ item: null })}>Show all</Button>
          {#if picked}<Button variant="outline" size="sm" href={`/scope_items/${picked.id}`} title={`Open the ${session.term("scope_item").toLowerCase()}: its notes, documents and history`}><ArrowRight />Open</Button>{/if}
        </div>
      {/if}

      {#each groups as g (g.item?.id ?? "other")}
        {@const single = g.item && g.everything.length === 1 && g.list.length === 1 && g.list[0].title.trim().toLowerCase() === g.item.description.trim().toLowerCase()}
        <div class="flex flex-col gap-1">
          {#if !single && !focus}
            <div class="flex items-center gap-2 text-sm">
              {#if g.item}<a class="truncate font-medium hover:underline" href={withQuery({ tab: "work", item: g.item.id })}>{g.item.description}</a>
              {:else}<a class="font-medium hover:underline" href={withQuery({ tab: "work", item: "other" })}>Other work</a>{/if}
              <Separator class="flex-1" />
              <span class="text-muted-foreground whitespace-nowrap">{done(g.everything)} of {g.everything.length}</span>
              {#if g.item?.delivery_state}<StatusBadge status={g.item.delivery_state} />{/if}
            </div>
          {/if}
          <ul class="flex flex-col">
            {#each g.list as t (t.id)}
              <li class="hover:bg-muted/50 flex items-center gap-2 rounded-md px-2 py-1.5">
                <a href={`/work/${t.id}`} class="min-w-0 flex-1">
                  <p class="truncate text-sm font-medium">{t.title}</p>
                  <p class="text-muted-foreground truncate text-xs">{[t.due_on && shortDate(t.due_on), !t.client_visible && "Internal"].filter(Boolean).join(" · ")}</p>
                </a>
                {#if t.overdue}<Badge variant="destructive">Overdue</Badge>{/if}
                <OwnerPicker todo={t} onchange={replace} />
                <StatusPicker todo={t} onchange={replace} />
                <Button variant="ghost" size="icon-sm" title="Copy a brief for your AI" onclick={() => copyBrief(t)}><Sparkles /><span class="sr-only">Copy a brief of {t.title} for your AI</span></Button>
              </li>
            {/each}
          </ul>
        </div>
      {/each}

      {#if !all.length}
        <p class="text-muted-foreground text-sm">{focus ? `No ${word.toLowerCase()} here yet. Add the first below.` : `No ${word.toLowerCase()} yet. Approving the agreement adds ${word.toLowerCase()} for each ${session.term("scope_item").toLowerCase()}.`}</p>
      {:else if !shown.length}
        <p class="text-muted-foreground text-sm">{showing === "open" ? `Nothing open: all ${all.length} done.` : "Nothing done yet."} <a class="underline-offset-4 hover:underline" href={withQuery({ work: "all" })}>Show all</a></p>
      {/if}

      {#if !data.engagement_closed}
        {#if !adding}
          <Button variant="outline" size="sm" class="w-fit" onclick={() => (adding = true)}><Plus />Add {word.toLowerCase()}</Button>
        {:else}
          <form onsubmit={add} class="flex flex-col gap-2 rounded-lg border p-3">
            <Input bind:value={form.title} required autofocus autocomplete="off" placeholder="What needs doing…" aria-label="Title" />
            <div class="grid gap-2 sm:grid-cols-3">
              <Select.Root type="single" bind:value={form.owner_id}>
                <Select.Trigger class="w-full" aria-label="Owner">{(session.me.team || []).find((p) => String(p.id) === form.owner_id)?.name || "Unassigned"}</Select.Trigger>
                <Select.Content>
                  <Select.Item value="">Unassigned</Select.Item>
                  {#each session.me.team || [] as p (p.id)}<Select.Item value={String(p.id)}>{p.name}</Select.Item>{/each}
                </Select.Content>
              </Select.Root>
              <DatePicker bind:value={form.due_on} placeholder="Due" />
              {#if !focus}
                <Select.Root type="single" bind:value={form.scope_item_id}>
                  <Select.Trigger class="w-full" aria-label={session.term("scope_item")}><span class="truncate">{data.scope_items.find((i) => String(i.id) === form.scope_item_id)?.description || "Other work"}</span></Select.Trigger>
                  <Select.Content>
                    <Select.Item value="">Other work</Select.Item>
                    {#each data.scope_items as item (item.id)}<Select.Item value={String(item.id)}>{item.description}</Select.Item>{/each}
                  </Select.Content>
                </Select.Root>
              {/if}
            </div>
            <div class="flex gap-2">
              <Button type="submit" size="sm" disabled={busy}>Add {word.toLowerCase()}</Button>
              <Button type="button" variant="ghost" size="sm" onclick={() => (adding = false)}>Cancel</Button>
            </div>
          </form>
        {/if}
      {/if}
    {/if}
  </Card.Content>
</Card.Root>
