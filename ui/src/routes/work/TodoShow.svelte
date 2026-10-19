<script>
  // A piece of work: its title with Copy for AI and Edit, then the working
  // column (where it sits, its description, and the tabs areas add: notes,
  // documents, plugins' panels) beside the side column (Details changed in
  // place, Make this a commitment, History).
  import * as Card from "$lib/components/ui/card/index.js";
  import ConnectionsButton from "$lib/components/ConnectionsButton.svelte";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import RichText from "$lib/components/RichText.svelte";
  import CustomFieldValues from "$lib/components/CustomFieldValues.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import Time from "$lib/components/Time.svelte";
  import StatusPicker from "./StatusPicker.svelte";
  import OwnerPicker from "./OwnerPicker.svelte";
  import MakeCommitment from "./MakeCommitment.svelte";
  import { get, patch } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, withQuery, go } from "$lib/router.svelte.js";
  import { tabsFor } from "$lib/record-tabs.js";
  import { toastError } from "$lib/toast.svelte.js";
  import NotFound from "../NotFound.svelte";
  import { date } from "$lib/format.js";
  import { copyBrief } from "./work.js";

  let { params } = $props();
  let todo = $state(null);
  const tabs = tabsFor("Todo");
  const tab = $derived(tabs.some((t) => t.key === router.query.get("tab")) ? router.query.get("tab") : tabs[0]?.key);

  function show(t) {
    todo = t;
    const up = t.scope_item ? { label: t.scope_item.display_name, href: `/engagements/${t.engagement.ref}?tab=work&item=${t.scope_item.id}` } : { label: t.engagement.ref, href: `/engagements/${t.engagement.ref}` };
    page.set(t.title, [{ label: session.term("work"), href: "/work" }, up], `Todo:${t.id}`);
  }
  async function load() {
    try { show((await get(`/work/${params.id}`)).todo); } catch (e) { if (e.status === 404 && !todo) missing = true; else toastError(e); }
  }
  let missing = $state(false);
  load();
  async function change(fields) {
    try { show((await patch(`/work/${todo.id}`, { todo: fields })).todo); } catch (e) { toastError(e); }
  }
</script>

<svelte:window onkeydown={(e) => {
  if (!todo || e.metaKey || e.ctrlKey || e.altKey || e.target.closest?.("input, textarea, select, [contenteditable], [role=dialog], [role=menu]")) return;
  if (e.key === "e") go(`/work/${todo.id}/edit`);
  else if (e.key === "c") copyBrief(todo);
}} />

{#if missing}
  <NotFound title="Couldn’t find it" back={{ href: "/work", label: `All ${session.term("work", 2).toLowerCase()}` }} />
{:else if !todo}
  <Skeleton class="h-64" />
{:else}
  <div class="flex flex-wrap items-start justify-between gap-x-4 gap-y-2">
    <div class="min-w-0">
      <h1 class="text-2xl font-semibold tracking-tight">{todo.title}</h1>
      <p class="text-muted-foreground text-sm">
        <a class="hover:underline" href={`/engagements/${todo.engagement.ref}`}>{todo.engagement.display_name}</a> ›
        {#if todo.scope_item}<a class="hover:underline" href={`/engagements/${todo.engagement.ref}?tab=work&item=${todo.scope_item.id}`}>{todo.scope_item.display_name}</a>{:else}Other work{/if}
      </p>
    </div>
    <div class="flex shrink-0 gap-2">
      <Button variant="outline" size="sm" title="Copy a brief for your AI (c)" onclick={() => copyBrief(todo)}><Sparkles />Copy for AI</Button>
      <ConnectionsButton record={`Todo:${todo.id}`} name={todo.title} />
      <Button href={`/work/${todo.id}/edit`} variant="outline" size="sm" title="Edit (e)"><Pencil />Edit</Button>
    </div>
  </div>

  <div class="grid gap-4 lg:grid-cols-3">
    <div class="flex min-w-0 flex-col gap-4 lg:col-span-2">
      <Card.Root>
        <Card.Header><Card.Title>Description</Card.Title></Card.Header>
        <Card.Content>
          {#if todo.description_html}<RichText html={todo.description_html} />{:else}<p class="text-muted-foreground text-sm">No description.</p>{/if}
        </Card.Content>
      </Card.Root>
      {#if tabs.length}
        <Tabs.Root value={tab} onValueChange={(v) => go(withQuery({ tab: v }), undefined, { replace: true })}>
          <Tabs.List>
            {#each tabs as t (t.key)}
              {@const n = t.count?.(todo)}
              <Tabs.Trigger value={t.key}>{t.label()}{#if n}<span class="text-muted-foreground ms-1 text-xs tabular-nums">{n}</span>{/if}</Tabs.Trigger>
            {/each}
          </Tabs.List>
          {#each tabs as t (t.key)}
            <Tabs.Content value={t.key} class="mt-2"><t.component record={todo} reload={load} /></Tabs.Content>
          {/each}
        </Tabs.Root>
      {/if}
    </div>

    <div class="flex flex-col gap-4">
      <Card.Root>
        <Card.Header><Card.Title>Details</Card.Title></Card.Header>
        <Card.Content>
          <dl class="grid grid-cols-[auto_minmax(0,1fr)] items-center gap-x-4 gap-y-2 text-sm">
            <dt class="text-muted-foreground">Status</dt><dd><StatusPicker {todo} labelled onchange={show} /></dd>
            <dt class="text-muted-foreground">On it</dt><dd class="min-w-0"><OwnerPicker {todo} labelled onchange={show} /></dd>
            <dt class="text-muted-foreground">Due</dt>
            <dd class="flex min-w-0 flex-wrap items-center gap-2">
              <div class="w-44"><DatePicker value={todo.due_on || ""} placeholder="No date" class="h-8" onchange={(v) => change({ due_on: v })} /></div>
              {#if todo.overdue}<Badge variant="destructive">Overdue</Badge>{/if}
            </dd>
            <dt class="text-muted-foreground">{session.term("scope_item")}</dt>
            <dd>
              <Select.Root type="single" value={todo.scope_item ? String(todo.scope_item.id) : ""} onValueChange={(v) => change({ scope_item_id: v || null })}>
                <Select.Trigger class="h-8 w-full"><span class="truncate">{todo.scope_item?.display_name || "Other work"}</span></Select.Trigger>
                <Select.Content>
                  <Select.Item value="">Other work</Select.Item>
                  {#each todo.agreed_items as item (item.id)}<Select.Item value={String(item.id)}>{item.description}</Select.Item>{/each}
                </Select.Content>
              </Select.Root>
            </dd>
            <dt class="text-muted-foreground">{session.term("client")}</dt><dd><a class="hover:underline" href={`/clients/${todo.client.id}`}>{todo.client.display_name}</a></dd>
            <dt class="text-muted-foreground">Client sees it</dt>
            <dd class="flex items-center gap-2"><Switch checked={todo.client_visible} onCheckedChange={(v) => change({ client_visible: v })} aria-label="Visible to the client in the portal" /><span class="text-muted-foreground">{todo.client_visible ? "Yes, in the portal" : "No, internal"}</span></dd>
            {#if todo.completed_at}<dt class="text-muted-foreground">Done</dt><dd>{date(todo.completed_at)}</dd>{/if}
            <CustomFieldValues model="Todo" values={todo.custom_fields} />
          </dl>
        </Card.Content>
      </Card.Root>

      {#if todo.status !== "done"}<MakeCommitment {todo} />{/if}

      <Card.Root>
        <Card.Header><Card.Title>History</Card.Title></Card.Header>
        <Card.Content>
          {#if !todo.history?.length}
            <p class="text-muted-foreground text-sm">Nothing yet.</p>
          {:else}
            <ol class="flex flex-col gap-3">
              {#each todo.history as e (e.id)}
                <li class="text-sm">
                  <p class="font-medium">{e.title}</p>
                  <p class="text-muted-foreground text-xs">by {e.by} · {e.source_label} · <Time value={e.occurred_at} /></p>
                  {#if e.details}<p class="text-muted-foreground text-xs">{e.details}</p>{/if}
                </li>
              {/each}
            </ol>
          {/if}
        </Card.Content>
      </Card.Root>
    </div>
  </div>
{/if}
