<script>
  // My day (days/_day): your work on the left, two fifths (a greeting,
  // search and tabs, your priorities in a panel of their own, then the list
  // by when it's due), the piece you picked on the right, three fifths,
  // which stays in view. Work is dragged onto Priorities to add it (or
  // within them to reorder) and back onto the list to take it off. j / k
  // move through the rows (the app's keys: gg and G to the first and last,
  // Enter or o opens the one you're on, x picks it), c copies its brief; picked rows get the
  // bulk bar at the foot of the list. Show, Clients and Engagements are
  // dropdown filters beside the search.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import * as InputGroup from "$lib/components/ui/input-group/index.js";
  import Search from "@lucide/svelte/icons/search";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import Plus from "@lucide/svelte/icons/plus";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import Pin from "@lucide/svelte/icons/pin";
  import PinOff from "@lucide/svelte/icons/pin-off";
  import X from "@lucide/svelte/icons/x";
  import Sortable from "sortablejs";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import FilterMenu from "$lib/components/FilterMenu.svelte";
  import TodoRow from "./TodoRow.svelte";
  import TodoCard from "./TodoCard.svelte";
  import { api, get } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, href, go, withQuery } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { plural } from "$lib/format.js";
  import { keys as appKeys } from "$lib/keys.svelte.js";
  import { BASE, TABS, STATUSES, STATUS_LABELS, statusDot, call, greetingDate, copyBrief, copyPrompt } from "./day.js";

  page.set("My day");

  // A new address draws the page afresh: it starts from the day as it last
  // was, so filtering or opening work doesn't flash the skeleton.
  let data = $state(last);
  let search = $state(router.query.get("q") || "");
  let box = $state(null);
  let prioritiesOpen = $state(lastOpen);
  let keepOpen = false;
  let picked = $state([]);
  let due = $state("");

  const show = $derived(TABS.some(([k]) => k === router.query.get("show")) ? router.query.get("show") : "open");

  // The day as the address has it: the tab, the search, the todo open.
  async function load() {
    if (router.path !== "/my-day") return;
    const q = router.query;
    try {
      const d = await get(BASE + href("/", { show: q.get("show"), q: q.get("q"), client: q.get("client"), engagement: q.get("engagement"), todo: q.get("todo") }));
      prioritiesOpen = keepOpen || !!d.todo?.priority || d.priorities.length === 0;
      keepOpen = false;
      data = last = d;
      lastOpen = prioritiesOpen;
      const ids = rows().map((t) => t.id);
      picked = picked.filter((id) => ids.includes(id));
    } catch (e) { toastError(e); }
  }
  $effect(() => { router.query; load(); });

  // A change to the priorities keeps them unfolded.
  const changed = (open = true) => { keepOpen = open || prioritiesOpen; return load(); };

  // Search as you type, keeping the rest of the address.
  let timer;
  function typed() {
    clearTimeout(timer);
    timer = setTimeout(() => { refocus = true; go(withQuery({ q: search.trim() }), undefined, { replace: true, transition: false }); }, 300);
  }
  $effect(() => {
    if (refocus && box) { box.focus(); box.setSelectionRange(search.length, search.length); }
    refocus = false;
  });

  // The dropdown filters: each a query parameter, "" for all.
  const filter = (param, v) => go(withQuery({ [param]: v || null, ...(param === "client" ? { engagement: null } : {}), todo: null }), undefined, { replace: true });

  const link = (t) => withQuery({ todo: t.id });
  const open = (t) => go(link(t), undefined, { replace: true });

  // Every row in the order it's on the page (the priorities when unfolded).
  function rows() {
    if (!data) return [];
    return [...(prioritiesOpen ? data.priorities : []), ...data.groups.flatMap((g) => g.todos)];
  }
  const pick = (id, on) => (picked = on ? [...new Set([...picked, id])] : picked.filter((x) => x !== id));
  const pickedInOrder = () => { const order = rows().map((t) => t.id); return [...picked].sort((a, b) => order.indexOf(a) - order.indexOf(b)); };

  const late = $derived(data?.late_count || 0);
  const today = $derived(data?.due_today_count || 0);
  const count = $derived(data?.priorities.length || 0);
  const full = $derived(data && count >= data.limit);

  // ---- the bulk bar -------------------------------------------------------

  async function bulk(method, path, body) {
    try {
      const r = await api(method, path, { ids: pickedInOrder(), ...body });
      if (r?.message || r?.summary) (r.alert ? toastError({ message: r.message }) : toast(r.message || r.summary));
      picked = [];
      due = "";
      await changed(path.startsWith(BASE));
    } catch (e) { toastError(e); }
  }

  // ---- dragging -------------------------------------------------------------

  // A drop: onto the priorities adds the work there (before the row it's
  // dropped above) or reorders it; from them onto the list takes it off.
  // The row goes back where Svelte drew it, and the day is read again.
  async function dropped({ item, from, to, oldIndex }) {
    const id = item.dataset.id;
    let next = item.nextElementSibling;
    while (next && !next.matches("[data-row]")) next = next.nextElementSibling;
    item.remove();
    from.insertBefore(item, from.children[oldIndex] ?? null);
    const onto = to.dataset.kind === "priorities", off = from.dataset.kind === "priorities";
    if (onto) {
      const before = next?.dataset.id;
      if (from === to && !before && from.lastElementChild === item) return;
      if (await call("POST", `/priorities/${id}`, before && before !== id ? { before: Number(before) } : {})) changed();
    } else if (off) {
      if (await call("POST", `/priorities/${id}/removal`)) changed();
    }
  }
  function dnd(kind) {
    return (el) => {
      const s = Sortable.create(el, {
        group: { name: "my-day", pull: true, put: kind === "priorities" ? true : (_to, from) => from.el.dataset.kind === "priorities" },
        sort: kind === "priorities",
        draggable: "[data-row]",
        filter: "input, button",
        preventOnFilter: false,
        // Pointer events rather than the browser's own drag: the same in
        // every browser, and a click on a row's link stays a click.
        forceFallback: true,
        fallbackOnBody: true,
        fallbackTolerance: 4,
        animation: 150,
        ghostClass: "opacity-40",
        onEnd: dropped,
      });
      return () => s.destroy();
    };
  }

  // ---- keys -----------------------------------------------------------------

  // The app's keys move through the rows (j / k, gg / G), open one (Enter
  // or o) and pick it (x); c here copies the brief of the one you're on.
  function keys(e) {
    if (e.key !== "c" || e.metaKey || e.ctrlKey || e.altKey || e.target.closest?.("input, textarea, select, [contenteditable], [role=dialog], [role=menu], [role=listbox]")) return;
    const id = Number(appKeys.current()?.dataset.id);
    if (!id) return;
    e.preventDefault();
    copyBrief(id);
  }
</script>

<script module>
  // The day as last drawn, whether Priorities were unfolded, and whether the
  // search box had focus when its typing moved the address.
  let last = null, lastOpen = false, refocus = false;
</script>

<svelte:window onkeydown={keys} />

{#snippet row(t, number)}
  <TodoRow todo={t} today={data.today} {number} link={link(t)} current={t.id === data.todo?.id}
    picked={picked.includes(t.id)} picking={picked.length > 0} onpick={pick} />
{/snippet}

{#if !data}
  <div class="grid gap-6 lg:grid-cols-5">
    <div class="flex flex-col gap-4 lg:col-span-2"><Skeleton class="h-16" /><Skeleton class="h-24" /><Skeleton class="h-64" /></div>
    <Skeleton class="h-96 lg:col-span-3" />
  </div>
{:else}
  <div class="grid items-start gap-6 lg:grid-cols-5">
    <div class="flex min-w-0 flex-col gap-4 lg:col-span-2">
      <header>
        <h1 class="text-2xl font-semibold tracking-tight">{greetingDate(data.today)}</h1>
        <p class="text-muted-foreground text-sm">
          {plural(count, "priority", "priorities")}
          {#if late} · <span class="text-destructive font-medium">{late} late</span>{/if}
          {#if today} · {today} due today{/if}
          {#if !late && !today} · nothing late{/if}
        </p>
      </header>

      <div class="flex flex-col gap-2">
        <InputGroup.Root>
          <InputGroup.Addon><Search /></InputGroup.Addon>
          <InputGroup.Input type="search" bind:ref={box} bind:value={search} oninput={typed} placeholder="Search your work…" aria-label="Search your work" autocomplete="off" />
        </InputGroup.Root>
        <div class="flex flex-wrap gap-2">
          <FilterMenu title="Show" align="start" current={show} options={TABS.map(([value, label]) => ({ value, label }))} onpick={(v) => filter("show", v === "open" ? "" : v)} />
          <FilterMenu title="Clients" align="start" current={String(data.client ?? "")} onpick={(v) => filter("client", v)}
            options={[{ value: "", label: "All clients" }, ...data.clients.map((c) => ({ value: String(c.id), label: c.label, count: c.count }))]} />
          <FilterMenu title="Engagements" align="start" current={String(data.engagement ?? "")} onpick={(v) => filter("engagement", v)}
            options={[{ value: "", label: "All engagements" }, ...data.engagements.map((e) => ({ value: String(e.id), label: e.label, count: e.count }))]} />
        </div>
      </div>

      <Collapsible.Root bind:open={prioritiesOpen}>
        <Card.Root class="gap-0 py-0 {full ? 'border-primary/40 bg-primary/5' : ''}">
          <Collapsible.Trigger class="flex w-full items-center gap-2 px-4 py-3 text-start">
            {#if prioritiesOpen}<ChevronDown class="size-4 shrink-0" />{:else}<ChevronRight class="size-4 shrink-0" />{/if}
            <strong class="text-sm">Priorities</strong>
            <span class="text-muted-foreground text-xs tabular-nums">{count} of {data.limit}</span>
            <Progress value={count} max={data.limit} class="ms-auto w-24" aria-label={`${count} of ${data.limit} places used`} />
          </Collapsible.Trigger>
          <Collapsible.Content>
            <div class="flex flex-col gap-1 px-2 pb-3">
              {#if !count}
                <p class="text-muted-foreground px-2 pb-1 text-sm">Today's few that matter most, in the order you'll do them. Drag work up from the list, or pick it and add it.</p>
              {/if}
              <nav class="flex min-h-2 flex-col gap-0.5" aria-label="Priorities" data-kind="priorities" {@attach dnd("priorities")}>
                {#each data.priorities as t, i (t.id)}{@render row(t, i + 1)}{/each}
              </nav>
              {#if full}
                <p class="text-muted-foreground rounded-md border border-dashed px-3 py-2 text-xs">Full: finish or remove one to add another.</p>
              {:else}
                <p class="text-muted-foreground flex items-center gap-1 rounded-md border border-dashed px-3 py-2 text-xs">
                  <Plus class="size-3.5" /> Drag work here <span>· {plural(data.limit - count, "place")} left</span>
                </p>
              {/if}
            </div>
          </Collapsible.Content>
        </Card.Root>
      </Collapsible.Root>

      <Card.Root class="relative gap-4 py-4">
        <Card.Content class="flex flex-col gap-4 px-2">
          {#each data.groups as g (g.key)}
            <section aria-label={g.title}>
              <header class="flex items-center gap-2 px-2 pb-1">
                <h2 class="text-xs font-semibold tracking-wide uppercase {g.key === 'late' ? 'text-destructive' : 'text-muted-foreground'}">{g.title}</h2>
                <span class="rounded-full px-1.5 text-xs tabular-nums {g.key === 'late' ? 'bg-destructive/10 text-destructive' : 'bg-muted text-muted-foreground'}">{g.todos.length}</span>
              </header>
              <nav class="flex flex-col gap-0.5" aria-label={g.title} data-kind="list" {@attach dnd("list")}>
                {#each g.todos as t (t.id)}{@render row(t, null)}{/each}
              </nav>
            </section>
          {/each}
          {#if !data.groups.length}
            <div class="text-muted-foreground px-2 text-sm" data-kind="list" {@attach dnd("list")}>
              {#if data.q}
                Nothing of yours matches “{data.q}”.
              {:else if show === "open" && !data.client && !data.engagement}
                Nothing open is yours. Work you lead or are on shows here.
              {:else}
                Nothing here. <a class="text-foreground underline-offset-4 hover:underline" href="/my-day">Show everything open</a>
              {/if}
            </div>
          {/if}
        </Card.Content>

        {#if picked.length}
          <div class="bg-background/95 sticky bottom-4 mx-2 flex flex-wrap items-center gap-2 rounded-lg border px-3 py-2 text-sm shadow-lg">
            <span class="font-medium tabular-nums">{picked.length} picked</span>
            <Button variant="outline" size="sm" onclick={() => copyPrompt(pickedInOrder())}><Sparkles />Copy for AI</Button>
            <Button variant="outline" size="sm" onclick={() => bulk("POST", BASE + "/bulk/priorities")}><Pin />Prioritize</Button>
            <Button variant="outline" size="sm" onclick={() => bulk("DELETE", BASE + "/bulk/priorities")}><PinOff />Take off</Button>
            <DropdownMenu.Root>
              <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Status<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
              <DropdownMenu.Content>
                {#each STATUSES as s (s)}
                  <DropdownMenu.Item onSelect={() => bulk("PATCH", "/bulk/work", { todo: { status: s } })}><span class="size-2.5 rounded-full {statusDot[s]}" aria-hidden="true"></span>{STATUS_LABELS[s]}</DropdownMenu.Item>
                {/each}
              </DropdownMenu.Content>
            </DropdownMenu.Root>
            <div class="w-36"><DatePicker bind:value={due} placeholder="Due" class="h-8" onchange={(v) => bulk("PATCH", "/bulk/work", { todo: { due_on: v } })} /></div>
            <DropdownMenu.Root>
              <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm">Lead<ChevronDown /></Button>{/snippet}</DropdownMenu.Trigger>
              <DropdownMenu.Content class="max-h-72">
                <DropdownMenu.Item onSelect={() => bulk("PATCH", "/bulk/work", { todo: { owner_id: session.me.person_id ?? session.me.id } })}>Me</DropdownMenu.Item>
                <DropdownMenu.Separator />
                {#each session.me.team || [] as p (p.id)}
                  <DropdownMenu.Item onSelect={() => bulk("PATCH", "/bulk/work", { todo: { owner_id: p.id } })}><PersonAvatar person={p} size="size-5" />{p.name}</DropdownMenu.Item>
                {/each}
              </DropdownMenu.Content>
            </DropdownMenu.Root>
            <Button variant="ghost" size="sm" class="ms-auto" onclick={() => (picked = [])}><X />Clear</Button>
          </div>
        {/if}
      </Card.Root>
    </div>

    <div class="min-w-0 lg:sticky lg:top-4 lg:col-span-3 lg:-m-1 lg:max-h-[calc(100svh-6rem)] lg:overflow-y-auto lg:p-1">
      {#if data.todo}
        {#key data.todo.id}
          <TodoCard todo={data.todo} {count} limit={data.limit} changed={() => changed(false)} />
        {/key}
      {:else}
        <Card.Root><Card.Content class="text-muted-foreground text-sm">Nothing to work on. Pick a piece of work on the left when there is.</Card.Content></Card.Root>
      {/if}
    </div>
  </div>
{/if}
