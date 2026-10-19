<script>
  // Settings › Agent runs: each mention of an agent (or ask over MCP) and
  // what became of it, newest first: yours, or everyone's for those who
  // manage settings. Filters: the agent, the status, who asked. Open one
  // for its transcript. Updates live as runs move on.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import FilterMenu from "$lib/components/FilterMenu.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import AgentAvatar from "./AgentAvatar.svelte";
  import { staff } from "./ai.svelte.js";
  import { router, href, onQuery, go, withQuery } from "$lib/router.svelte.js";
  import { subscribe } from "$lib/live.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { timeAgo, dateTime } from "$lib/format.js";

  let data = $state(null);
  async function load() {
    const q = { agent: router.query.get("agent"), status: router.query.get("status"), user: router.query.get("user") };
    try { data = await staff.get(href("/runs", q)); } catch (e) { toastError(e); }
  }
  onQuery(load);
  $effect(() => subscribe("ai", (topic) => { if (topic === "run" || topic === "resync") load(); }));

  const pick = (param) => (v) => go(withQuery({ [param]: v || null }), undefined, { replace: true });
  export const tone = { done: "secondary", failed: "destructive", cancelled: "outline", waiting: "default", working: "outline", queued: "outline" };
</script>

<SettingsLayout title="Agent runs" description="Each time someone mentions an agent, it runs in the background and replies where it was asked. Open a run to see what it read, did and said, and what it cost.">
  {#if !data}
    <Skeleton class="h-48 w-full" />
  {:else}
    <div class="mb-3 flex flex-wrap gap-2">
      <FilterMenu title="Agent" current={router.query.get("agent") || ""} options={[{ value: "", label: "Every agent" }, ...data.filters.agents]} onpick={pick("agent")} align="start" />
      <FilterMenu title="Status" current={router.query.get("status") || ""} options={[{ value: "", label: "Any status" }, ...data.filters.statuses]} onpick={pick("status")} align="start" />
      {#if data.everyone}
        <FilterMenu title="Asked by" current={router.query.get("user") || ""} options={[{ value: "", label: "Anyone" }, ...data.filters.people]} onpick={pick("user")} align="start" />
      {/if}
    </div>
    {#if !data.runs.length}
      {#if router.query.size}
        <EmptyState icon="ai" title="No runs match." description="Try another agent, status or person." />
      {:else}
        <EmptyState icon="ai" title="No runs yet." description="Mention an agent with @ in a note or in chat, and its run shows here." />
      {/if}
    {:else}
      <Card.Root class="py-0">
        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head class="w-10 sm:w-auto"><span class="sr-only sm:not-sr-only">Agent</span></Table.Head>
              <Table.Head>Asked</Table.Head>
              <Table.Head class="hidden md:table-cell">By</Table.Head>
              <Table.Head>Status</Table.Head>
              <Table.Head class="hidden text-right sm:table-cell">Cost</Table.Head>
              <Table.Head class="hidden md:table-cell">When</Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each data.runs as r (r.id)}
              <Table.Row class="cursor-pointer" onclick={() => go(r.url)}>
                <Table.Cell>
                  <span class="flex items-center gap-2"><AgentAvatar agent={r.agent} size="size-6" /><span class="hidden truncate font-medium sm:inline">@{r.agent.handle || "deleted"}</span><span class="sr-only sm:hidden">@{r.agent.handle}</span></span>
                </Table.Cell>
                <Table.Cell class="max-w-36 sm:max-w-64">
                  <a href={r.url} class="block truncate hover:underline" title={r.prompt}>{r.prompt || "(nothing said)"}</a>
                  {#if r.subject}<span class="text-muted-foreground block truncate text-xs">{r.source === "note" ? "Note on" : r.source === "tool" ? "Asked about" : "In"} {r.subject}</span>{/if}
                </Table.Cell>
                <Table.Cell class="hidden md:table-cell">{r.user.name}</Table.Cell>
                <Table.Cell><Badge variant={tone[r.status]}>{r.status_label}</Badge></Table.Cell>
                <Table.Cell class="hidden text-right tabular-nums sm:table-cell">{r.cost}</Table.Cell>
                <Table.Cell class="text-muted-foreground hidden md:table-cell"><time datetime={r.created_at} title={dateTime(r.created_at)}>{timeAgo(r.created_at)}</time></Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </Card.Root>
    {/if}
  {/if}
</SettingsLayout>
