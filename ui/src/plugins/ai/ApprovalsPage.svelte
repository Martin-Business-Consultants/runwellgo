<script>
  // Approvals: what an AI or a machine made that waits for a person before
  // it counts: changes Ask and the agents propose, and what other plugins
  // ask (a pull request a runner opened). Each says what it is, who or what
  // asked, the record it's on, a summary and its details, with Approve or
  // Decline (and a note) in place. j/k move, Enter approves, x declines.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ExternalLink from "@lucide/svelte/icons/external-link";
  import CheckCheck from "@lucide/svelte/icons/check-check";
  import IndexToolbar from "$lib/components/IndexToolbar.svelte";
  import ApprovalMarkdown from "./ApprovalMarkdown.svelte";
  import AiText from "./AiText.svelte";
  import { staff } from "./ai.svelte.js";
  import { page } from "$lib/page.svelte.js";
  import { router, href, onQuery } from "$lib/router.svelte.js";
  import { subscribe } from "$lib/live.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { timeAgo, dateTime } from "$lib/format.js";

  page.set("Approvals");
  let data = $state(null), notes = $state({}), noting = $state({}), busy = $state({});
  const item = $derived(Number(router.query.get("item")) || 0);

  async function load() {
    const q = { status: router.query.get("status"), source: router.query.get("source"), person: router.query.get("person") };
    try { data = await staff.get(href("/approvals", q)); } catch (e) { toastError(e); }
  }
  onQuery(load);
  $effect(() => subscribe("ai", (topic) => { if (topic === "approval" || topic === "resync") load(); }));
  // The one a notification opened, brought into view once.
  let shown = 0;
  $effect(() => {
    if (!data || !item || shown === item) return;
    shown = item;
    queueMicrotask(() => document.getElementById(`approval-${item}`)?.scrollIntoView({ block: "center" }));
  });

  async function decide(a, decision) {
    if (busy[a.id]) return;
    busy[a.id] = true;
    try {
      const r = await staff.post(`/approvals/${a.id}/decision`, { decision, note: notes[a.id] || "" });
      r.approval.outcome === "failed" ? toastError(new Error(r.message)) : toast(r.message);
      await load();
      nav.refresh?.();
    } catch (e) { toastError(e); }
    busy[a.id] = false;
  }

  const filters = $derived(data ? [
    { param: "status", title: "Status", current: router.query.get("status") || "waiting", options: data.filters.statuses },
    ...(data.filters.sources.length > 1 ? [{ param: "source", title: "From", current: router.query.get("source") || "", options: [{ value: "", label: "From anywhere" }, ...data.filters.sources] }] : []),
    ...(data.filters.people.length > 1 ? [{ param: "person", title: "Asked for", current: router.query.get("person") || "", options: [{ value: "", label: "Anyone" }, ...data.filters.people] }] : []),
  ] : []);
  const outcome = (a) => [STATUS[a.status], a.decided_by && `by ${a.decided_by.name}`, a.decided_at && timeAgo(a.decided_at)].filter(Boolean).join(" ") +
    (a.outcome === "failed" ? ", but it failed" : "") + (a.outcome_text ? `: ${a.outcome_text}` : ".");
  const linkLabel = (a) => (a.link.startsWith("/ai/runs/") ? "The agent’s run" : a.kind === "pull_request" ? "Open the pull request" : a.link.startsWith("/") ? "Open it" : "Open the link");
  const STATUS = { waiting: "Waiting", approved: "Approved", declined: "Declined", withdrawn: "Withdrawn" };
</script>

<h1 class="sr-only">Approvals</h1>
<IndexToolbar {filters} />

{#if !data}
  <Skeleton class="h-40 w-full" />
{:else if !data.approvals.length}
  <Card.Root>
    <Card.Content class="flex flex-col items-start gap-2 text-sm">
      <CheckCheck class="text-muted-foreground size-6" />
      <p class="font-medium">Nothing waits on you.</p>
      <p class="text-muted-foreground">Changes your agents and Ask propose land here, as does anything another plugin needs a person to approve, like a pull request a runner opened.</p>
    </Card.Content>
  </Card.Root>
{:else}
  <p class="text-muted-foreground mb-3 hidden text-xs sm:block">j and k move between them, Enter approves, x declines.</p>
  <ul class="flex flex-col gap-3">
    {#each data.approvals as a (a.id)}
      <li id="approval-{a.id}" data-nav-item class="rounded-xl data-[nav-current]:ring-ring/50 data-[nav-current]:ring-3 {a.id === item ? 'ring-primary/40 ring-2' : ''}">
        <Card.Root class="gap-3">
          <Card.Header class="gap-1.5">
            <div class="flex flex-wrap items-center gap-2 text-xs">
              <Badge variant="secondary">{a.source_name}</Badge>
              {#if a.status !== "waiting"}<Badge variant="outline">{STATUS[a.status]}{a.outcome === "failed" ? ", failed" : ""}</Badge>{/if}
              <span class="text-muted-foreground">
                {#if a.requested_by}{a.source === "agent" || a.source === "ask" ? "For" : "Asked by"} {a.requested_by.name} · {/if}<time datetime={a.created_at} title={dateTime(a.created_at)}>{timeAgo(a.created_at)}</time>
              </span>
            </div>
            <Card.Title class="text-base leading-snug">{a.title}</Card.Title>
            <div class="text-muted-foreground flex flex-wrap gap-x-3 gap-y-1 text-sm">
              {#if a.record}
                {#if a.record_path}<a href={a.record_path} class="text-primary underline-offset-4 hover:underline">{a.record}</a>{:else}<span>{a.record}</span>{/if}
              {/if}
              {#if a.link}
                <a href={a.link} class="text-primary inline-flex items-center gap-1 underline-offset-4 hover:underline" target={a.link.startsWith("/") ? undefined : "_blank"} rel="noopener noreferrer">
                  {linkLabel(a)}{#if !a.link.startsWith("/")}<ExternalLink class="size-3.5" />{/if}
                </a>
              {/if}
            </div>
          </Card.Header>
          <Card.Content class="flex flex-col gap-3">
            {#if a.summary}<AiText text={a.summary} />{/if}
            {#if a.details_markdown}
              <div class="bg-muted/30 rounded-md border p-3"><ApprovalMarkdown text={a.details_markdown} /></div>
            {/if}
            {#if a.status !== "waiting"}
              <p class="text-sm {a.outcome === 'failed' ? 'text-destructive' : 'text-muted-foreground'}">
                {outcome(a)}
                {#if a.note}<span class="block">Note: {a.note}</span>{/if}
              </p>
            {:else if a.can_decide}
              {#if noting[a.id]}
                <Textarea bind:value={notes[a.id]} rows={2} placeholder="A note for whoever asked (optional)" aria-label="Note" />
              {/if}
              <div class="flex flex-wrap items-center gap-2">
                <Button size="sm" data-nav-open disabled={busy[a.id]} onclick={() => decide(a, "approve")}>Approve</Button>
                <Button size="sm" variant="outline" data-nav-select disabled={busy[a.id]} onclick={() => decide(a, "decline")}>Decline</Button>
                {#if !noting[a.id]}<Button size="sm" variant="ghost" onclick={() => (noting[a.id] = true)}>Add a note</Button>{/if}
              </div>
            {:else}
              <p class="text-muted-foreground text-sm">Waiting for someone else to decide.</p>
            {/if}
          </Card.Content>
        </Card.Root>
      </li>
    {/each}
  </ul>
{/if}
