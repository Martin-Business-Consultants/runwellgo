<script>
  // A website's page (the Rails sites/show): its name with Work, Audit,
  // History and Edit; then the numbered steps on the left, by group, each
  // with a dot for its state, and the picked one on the right (StepDetail).
  // The page opens on the first step still to do (or ?item=); picking a step
  // keeps the address, so it can be shared.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import SquareKanban from "@lucide/svelte/icons/square-kanban";
  import Search from "@lucide/svelte/icons/search";
  import History from "@lucide/svelte/icons/history";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Plus from "@lucide/svelte/icons/plus";
  import StateDot from "./StateDot.svelte";
  import StepDetail from "./StepDetail.svelte";
  import WorkSheet from "./WorkSheet.svelte";
  import AuditSheet from "./AuditSheet.svelte";
  import { get, post } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { untrack } from "svelte";
  import { router, href } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { cn } from "$lib/utils.js";
  import { wc, summaryClass, groupsOf, groupSummary } from "./wc.js";

  let { params } = $props();
  let site = $state(null), picked = $state(Number(router.query.get("item")) || null);
  let workOpen = $state(false), auditOpen = $state(false);
  let added = $state({ title: "", value: "" }), addOpen = $state(false);

  async function load() {
    try { show(await get(wc(`/websites/${params.id}`))); } catch (e) { toastError(e); }
  }
  // show takes the page as the server answers it (a read, or a write's
  // answer), keeping the step on screen unless the write names another.
  function show(r) {
    site = r;
    page.set(r.name, [{ label: "Websites", href: "/websites" }]);
    const want = r.item_id || picked;
    pick(r.items.some((i) => i.id === want) ? want : r.next_step?.id || r.items[0]?.id, true);
  }
  function pick(id, quiet = false) {
    picked = id;
    // The address follows without drawing the screen afresh.
    const to = href(`/websites/${params.id}`, { item: id });
    if (location.pathname + location.search !== to) history.replaceState(history.state, "", to);
    if (!quiet) document.getElementById("website-step")?.scrollIntoView({ block: "nearest" });
  }
  load();
  // A link or Back to another step of this website (?item=) picks it.
  $effect(() => {
    const want = Number(router.query.get("item")) || null;
    if (want && site && want !== untrack(() => picked)) pick(want, true);
  });

  const index = $derived(site ? site.items.findIndex((i) => i.id === picked) : -1);
  const item = $derived(index >= 0 ? site.items[index] : null);
  const groups = $derived(site ? groupsOf(site.items) : []);
  // A step's form starts afresh when it, or what it holds, changes.
  const itemKey = $derived(item ? JSON.stringify([item.id, item.value, item.note, item.state, item.title, item.detail, item.webhooks]) : "");

  async function addItem(e) {
    e.preventDefault();
    try {
      const r = await post(wc(`/websites/${site.id}/items`), { item: added });
      toast(r.message);
      added = { title: "", value: "" }; addOpen = false;
      show(r);
    } catch (err) { toastError(err); }
  }
</script>

{#if !site}
  <Skeleton class="h-96" />
{:else}
  <div class="flex flex-wrap items-start justify-between gap-4">
    <div class="min-w-0">
      <h1 class="truncate text-2xl font-semibold tracking-tight">{site.name}</h1>
      <div class="text-muted-foreground mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-sm">
        <Badge variant="outline" class={summaryClass(site.checklist)}>{site.checklist_text}</Badge>
        {#if site.needs_attention}<span>{site.needs_attention}</span>{/if}
        <a href={`/clients/${site.client.id}?tab=website_checklist`} class="text-foreground underline-offset-4 hover:underline">{site.client.display_name}</a>
        {#if site.address}<a href={site.address} target="_blank" rel="noopener" class="text-foreground underline-offset-4 hover:underline">{site.host}</a>{/if}
      </div>
    </div>
    <div class="flex flex-wrap gap-2">
      <Button variant="outline" size="sm" title={`Add ${session.term("work").toLowerCase()} for ${site.client.display_name}`} onclick={() => (workOpen = true)}><SquareKanban />{session.term("work")}</Button>
      <Button variant="outline" size="sm" onclick={() => (auditOpen = true)}><Search />Audit</Button>
      <Button variant="outline" size="sm" href={href("/websites/history", { site_id: site.id })}><History />History</Button>
      {#if site.can_manage}<Button variant="outline" size="sm" href={`/websites/${site.id}/edit`}><Pencil />Edit</Button>{/if}
    </div>
  </div>

  <div class="grid items-start gap-4 lg:grid-cols-2">
    <Card.Root>
      <Card.Content class="flex flex-col gap-5">
        {#each groups as g (g.group)}
          {@const sum = groupSummary(g.items)}
          <section class="flex flex-col gap-1.5">
            <header class="flex items-center gap-3">
              <h2 class="text-muted-foreground text-xs font-medium tracking-wide uppercase">{g.group}</h2>
              <span class="bg-border h-px flex-1"></span>
              <span class={cn("flex items-center gap-1.5 text-xs", sum.done ? "font-semibold text-emerald-700 dark:text-emerald-400" : "text-muted-foreground")}>
                {#if sum.done}<StateDot state="confirmed" />{/if}{sum.text}
              </span>
            </header>
            <nav class="flex flex-col gap-0.5" aria-label={g.group}>
              {#each g.items as i (i.id)}
                <a href={href(`/websites/${site.id}`, { item: i.id })} onclick={(e) => { e.preventDefault(); pick(i.id); }}
                  aria-current={i.id === picked ? "page" : undefined}
                  class={cn("flex min-w-0 items-center gap-2 rounded-md px-2 py-1.5 text-sm transition-colors",
                    i.state === "confirmed" ? "bg-emerald-500/10 text-emerald-900 hover:bg-emerald-500/15 aria-[current=page]:bg-emerald-500/20 dark:text-emerald-200" : "hover:bg-muted aria-[current=page]:bg-muted",
                    "aria-[current=page]:font-medium")}>
                  <span class="text-muted-foreground w-6 shrink-0 text-end text-xs tabular-nums">{i.step}</span>
                  <StateDot state={i.state} title={i.state_label} />
                  <span class={cn("truncate", i.state === "not_applicable" && "text-muted-foreground")} title={i.title}>{i.title}</span>
                  {#if i.files.length || i.work.length}<span class="text-muted-foreground ms-auto shrink-0 text-xs tabular-nums">{i.files.length + i.work.length}</span>{/if}
                </a>
              {/each}
            </nav>
          </section>
        {/each}

        <Collapsible.Root bind:open={addOpen}>
          <Collapsible.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm"><Plus />Add an item</Button>{/snippet}</Collapsible.Trigger>
          <Collapsible.Content>
            <form onsubmit={addItem} class="mt-2 flex flex-col gap-2">
              <Input bind:value={added.title} placeholder="What to check" required maxlength={200} autocomplete="off" aria-label="What to check" />
              <Input bind:value={added.value} placeholder="Its value, if you know it" autocomplete="off" aria-label="Value" />
              <div><Button type="submit" size="sm">Add</Button></div>
            </form>
          </Collapsible.Content>
        </Collapsible.Root>
      </Card.Content>
    </Card.Root>

    <Card.Root id="website-step" class="lg:sticky lg:top-4 lg:max-h-[calc(100dvh-2rem)] lg:overflow-y-auto">
      <Card.Content>
        {#if item}
          {#key itemKey}
            <StepDetail {site} {item} total={site.items.length} next={site.items[index + 1] || null} onchange={show} onpick={(id) => pick(id)} />
          {/key}
        {:else}
          <p class="text-muted-foreground text-sm">Pick an item on the left to see and change it.</p>
        {/if}
      </Card.Content>
    </Card.Root>
  </div>

  <WorkSheet bind:open={workOpen} {site} {item} onchange={show} />
  <AuditSheet bind:open={auditOpen} {site} ondone={load} />
{/if}
