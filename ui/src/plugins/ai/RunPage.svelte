<script>
  // One agent run: what it was asked, by whom and where, its status and
  // cost, and its transcript (its answers, and each tool it used with what
  // came back). A change it proposed waits in Approvals; Cancel stops one
  // still going. Updates live.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import AgentAvatar from "./AgentAvatar.svelte";
  import AiText from "./AiText.svelte";
  import { staff } from "./ai.svelte.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { subscribe } from "$lib/live.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { timeAgo, dateTime } from "$lib/format.js";

  let { params } = $props();
  let data = $state(null), missing = $state(false);

  async function load() {
    try {
      data = await staff.get(`/runs/${params.id}`);
      const crumbs = session.can("manage_settings") ? [{ label: "Settings", href: "/settings" }, { label: "Agent runs", href: "/settings/ai/runs" }] : [];
      page.set(`@${data.run.agent.handle || "agent"}’s run`, crumbs);
    } catch (e) {
      if (e.status === 404) missing = true;
      else toastError(e);
    }
  }
  load();
  $effect(() => subscribe("ai", (topic, d) => { if ((topic === "run" && String(d?.id) === String(params.id)) || topic === "resync") load(); }));
  // While it works, read again now and then too (the live stream may be off).
  $effect(() => {
    if (!data || !["queued", "working"].includes(data.run.status)) return;
    const t = setInterval(load, 4000);
    return () => clearInterval(t);
  });

  async function cancel() {
    if (!(await confirm({ title: "Cancel this run?", message: "It stops where it is, and says so where it was asked.", confirmLabel: "Cancel the run", destructive: true }))) return;
    try { data = await staff.post(`/runs/${params.id}/cancel`); toast("Cancelled."); } catch (e) { toastError(e); }
  }
  const tone = { done: "secondary", failed: "destructive", cancelled: "outline", waiting: "default", working: "outline", queued: "outline" };
  const where = (r) => (r.source === "note" ? "a note on" : r.source === "tool" ? "a request about" : "chat,");
</script>

<div class="flex flex-col gap-6">
  {#if missing}
    <h1 class="text-2xl font-semibold">Run not found</h1>
    <p class="text-muted-foreground text-sm">It isn’t there, or it was someone else’s.</p>
  {:else if !data}
    <Skeleton class="h-64 w-full" />
  {:else}
    {@const r = data.run}
    <div class="grid gap-6 lg:grid-cols-3">
      <div class="flex flex-col gap-4 lg:col-span-2">
        <div class="flex items-start gap-3">
          <AgentAvatar agent={r.agent} size="size-10" />
          <div class="min-w-0">
            <h1 class="text-2xl font-semibold tracking-tight">{r.agent.name}</h1>
            <p class="text-muted-foreground text-sm">@{r.agent.handle} · asked by {r.user.name} <time datetime={r.created_at} title={dateTime(r.created_at)}>{timeAgo(r.created_at)}</time></p>
          </div>
        </div>
        {#if data.waiting}
          <Card.Root class="border-primary/40">
            <Card.Header>
              <Card.Title>Waiting for approval</Card.Title>
              <Card.Action><Button size="sm" href="/ai/approvals">Open Approvals</Button></Card.Action>
            </Card.Header>
            <Card.Content class="text-sm">
              <p>It proposed a change that waits for {data.can_decide ? "you" : r.user.name} to approve or decline it.</p>
            </Card.Content>
          </Card.Root>
        {/if}
        <Card.Root>
          <Card.Header><Card.Title>Transcript</Card.Title></Card.Header>
          <Card.Content>
            <ol class="flex flex-col gap-4">
              {#each data.messages as m (m.id)}
                <li class="flex flex-col gap-2">
                  <p class="text-muted-foreground text-xs font-medium">{m.role === "user" ? r.user.name : r.agent.name}</p>
                  {#if m.content}
                    <div class="{m.role === 'user' ? 'bg-muted/50 rounded-lg px-3 py-2' : ''} {m.failed ? 'text-destructive' : ''}"><AiText text={m.content} /></div>
                  {/if}
                  {#each m.tool_calls as c (c.id)}
                    <Collapsible.Root class="rounded-md border text-sm">
                      <Collapsible.Trigger class="hover:bg-muted/50 flex w-full items-center gap-2 px-3 py-2 text-left [&[data-state=open]>svg]:rotate-90">
                        <ChevronRight class="text-muted-foreground size-4 shrink-0 transition-transform motion-reduce:transition-none" />
                        <span class="min-w-0 flex-1 truncate">{c.activity}</span>
                        <Badge variant={c.state === "failed" ? "destructive" : c.state === "pending" ? "default" : "outline"}>{c.state === "pending" ? "Waiting for approval" : c.state === "done" ? "Done" : c.state[0].toUpperCase() + c.state.slice(1)}</Badge>
                      </Collapsible.Trigger>
                      <Collapsible.Content class="text-muted-foreground border-t px-3 py-2 text-xs">
                        {#if c.details?.length}
                          <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">{#each c.details as [k, v], i (i)}<dt class="font-medium">{k}</dt><dd class="break-words">{v}</dd>{/each}</dl>
                        {/if}
                        <p class="break-words">{c.summary || (c.state === "pending" ? "Not run yet." : "Nothing to show.")}</p>
                      </Collapsible.Content>
                    </Collapsible.Root>
                  {/each}
                </li>
              {/each}
            </ol>
          </Card.Content>
        </Card.Root>
      </div>
      <div class="flex flex-col gap-4">
        <Card.Root>
          <Card.Header><Card.Title>Details</Card.Title></Card.Header>
          <Card.Content>
            <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <dt class="text-muted-foreground">Status</dt><dd><Badge variant={tone[r.status]}>{r.status_label}</Badge></dd>
              <dt class="text-muted-foreground">Where</dt>
              <dd class="min-w-0 break-words">{#if r.path}<a href={r.path} class="text-primary underline-offset-4 hover:underline">{where(r)} {r.subject || r.record}</a>{:else}{r.record ? `${where(r)} ${r.subject || r.record}` : "Asked directly"}{/if}</dd>
              <dt class="text-muted-foreground">Cost</dt><dd class="tabular-nums">{r.cost}</dd>
              {#if r.finished_at}<dt class="text-muted-foreground">Finished</dt><dd><time datetime={r.finished_at}>{dateTime(r.finished_at)}</time></dd>{/if}
              {#if r.error}<dt class="text-muted-foreground">Why it stopped</dt><dd class="break-words">{r.error}</dd>{/if}
            </dl>
          </Card.Content>
        </Card.Root>
        {#if data.can_cancel}
          <Card.Root>
            <Card.Header>
              <Card.Title>Still going</Card.Title>
              <Card.Action><Button variant="outline" size="sm" onclick={cancel}>Cancel the run</Button></Card.Action>
            </Card.Header>
            <Card.Content class="text-sm">
              <p class="text-muted-foreground">Cancel it to stop it where it is.</p>
            </Card.Content>
          </Card.Root>
        {/if}
      </div>
    </div>
  {/if}
</div>
