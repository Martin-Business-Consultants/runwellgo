<script>
  // One engagement as the client sees it: its state and agreed price, the
  // agreement awaiting their decision (and the decision itself, for a
  // contact who can approve), the scope in force, every agreement sent and
  // how it was decided; beside it, the work shared with them, documents and
  // open commitments. Never internal estimates or work kept internal.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Check from "@lucide/svelte/icons/check";
  import FileText from "@lucide/svelte/icons/file-text";
  import RichText from "$lib/components/RichText.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import PortalLayout from "./PortalLayout.svelte";
  import PortalSnapshot from "./PortalSnapshot.svelte";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";
  import { date, bytes, humanize, money } from "$lib/format.js";
  import { portal, pget, ppost } from "./portal.svelte.js";

  let { params } = $props();
  let e = $state(null);
  let missing = $state(false);
  let approverName = $state(""), comment = $state(""), busy = $state(false), error = $state("");

  async function load() {
    if (!(await portal.require())) return;
    try { e = (await pget(`/portal/engagements/${encodeURIComponent(params.ref)}`)).engagement; } catch (err) { if (err.status === 404) missing = true; else toastError(err); }
  }
  load();

  async function decide(decision) {
    error = "";
    if (!approverName.trim()) { error = "Type your name to confirm."; return; }
    busy = true;
    try {
      const res = await ppost(`/portal/engagements/${e.ref}/approvals`, { decision, approver_name: approverName, comment });
      toast(res.notice);
      approverName = ""; comment = "";
      await load();
    } catch (err) { error = errorMessage(err); }
    busy = false;
  }

  function customValue(f) {
    if (f.kind === "money") return money(f.value);
    if (f.kind === "date") return date(f.value);
    if (f.kind === "boolean") return f.value ? "Yes" : "No";
    return String(f.value);
  }
</script>

<PortalLayout title={e ? `${e.ref} ${e.title}` : ""} back={{ href: "/portal", label: portal.term("engagement", 2) }}>
  {#if missing}
    <p class="text-muted-foreground text-sm">Couldn’t find that {portal.term("engagement").toLowerCase()}. It may have been closed or removed; your {portal.term("engagement", 2).toLowerCase()} are listed on the <a href="/portal" class="underline underline-offset-4">first page</a>.</p>
  {:else if !e}
    <Skeleton class="h-64" />
  {:else}
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="flex min-w-0 flex-col gap-4 lg:col-span-2">
        <Card.Root>
          <Card.Header><Card.Title>About this {portal.term("engagement").toLowerCase()}</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3">
            <div class="flex flex-wrap items-center gap-3">
              <Badge variant={e.active ? "default" : "secondary"}>{e.state_label}</Badge>
              <span class="text-muted-foreground text-sm">{e.label}</span>
              {#if e.priced}<strong class="text-sm">Agreed {e.agreed}{e.per_period ? " per period" : ""}</strong>{/if}
            </div>
            {#if e.description_html}<RichText html={e.description_html} />{/if}
            {#if e.custom_fields?.length}
              <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1 text-sm">
                {#each e.custom_fields as f (f.key)}
                  <dt class="text-muted-foreground">{f.label}</dt>
                  <dd>{#if f.kind === "link"}<a href={f.value} target="_blank" rel="noopener" class="underline">{f.value}</a>{:else}{customValue(f)}{/if}</dd>
                {/each}
              </dl>
            {/if}
          </Card.Content>
        </Card.Root>

        {#if e.awaiting_your_decision}
          {@const pending = e.awaiting_your_decision}
          <Card.Root class="border-primary">
            <Card.Header>
              <Card.Title>Awaiting your decision</Card.Title>
              <Card.Description>{pending.label}{pending.sent_at ? ` · sent ${portal.dateTime(pending.sent_at)}` : ""}</Card.Description>
            </Card.Header>
            <Card.Content class="flex flex-col gap-4">
              <PortalSnapshot entry={pending} compact />
              {#if pending.you_can_decide}
                <p class="text-muted-foreground text-sm">Review the terms above. Approving records your name and the time.</p>
                <Field.Group>
                  <Field.Field data-invalid={!!error}>
                    <Field.Label for="approver-name">Your name</Field.Label>
                    <Input id="approver-name" bind:value={approverName} autocomplete="name" placeholder="Type your full name to confirm" />
                    {#if error}<Field.Error>{error}</Field.Error>{/if}
                  </Field.Field>
                  <Field.Field>
                    <Field.Label>Comment (optional)</Field.Label>
                    <TiptapEditor bind:value={comment} format="html" tools="email" minHeight="min-h-24" />
                  </Field.Field>
                  <div class="flex flex-wrap justify-center gap-2">
                    <Button disabled={busy} onclick={() => decide("approved")}><Check />Approve</Button>
                    <Button variant="outline" disabled={busy} onclick={() => decide("changes_requested")}>Request changes</Button>
                  </div>
                </Field.Group>
              {:else}
                <p class="text-muted-foreground text-sm">Someone at your company who can approve agreements needs to decide on this.</p>
              {/if}
            </Card.Content>
          </Card.Root>
        {/if}

        <Card.Root>
          <Card.Header><Card.Title>Scope in force</Card.Title></Card.Header>
          <Card.Content>
            {#if e.scope_in_force.length}
              <ul class="divide-y text-sm">
                {#each e.scope_in_force as item, i (i)}
                  <li class="flex items-center gap-3 py-2">
                    <span class="min-w-0 flex-1">{item.description}</span>
                    {#if e.priced}<span class="whitespace-nowrap tabular-nums">{item.price}</span>{/if}
                    <StatusBadge status={item.delivery} />
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="text-muted-foreground text-sm">Nothing approved yet.</p>
            {/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header><Card.Title>Agreement history</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3">
            {#each e.agreements as entry, i (i)}
              <div class="bg-muted/50 flex flex-col gap-3 rounded-lg p-4">
                <PortalSnapshot {entry} />
                {#if entry.version?.approval}
                  {@const ap = entry.version.approval}
                  <div class="flex flex-col gap-2">
                    <div class="flex flex-wrap items-center gap-2 text-sm">
                      <StatusBadge status={ap.decision} />
                      <span>by {ap.approver_name}{#if ap.method === "agent"}, through their agent ({ap.via}){/if} on {portal.dateTime(ap.decided_at)}</span>
                    </div>
                    {#if ap.comment}<RichText html={ap.comment} />{/if}
                  </div>
                {/if}
              </div>
            {:else}
              <p class="text-muted-foreground text-sm">Nothing has been sent yet.</p>
            {/each}
          </Card.Content>
        </Card.Root>
      </div>

      <div class="flex flex-col gap-4">
        <Card.Root>
          <Card.Header><Card.Title>{portal.term("work")}</Card.Title></Card.Header>
          <Card.Content>
            {#if e.work.length}
              <ul class="divide-y text-sm">
                {#each e.work as w, i (i)}
                  <li class="flex items-center gap-3 py-2">
                    <div class="min-w-0 flex-1">
                      <div class="truncate font-medium">{w.title}</div>
                      {#if w.due_on}<div class="text-muted-foreground text-xs">due {date(w.due_on)}</div>{/if}
                    </div>
                    <StatusBadge status={w.status} label={humanize(w.status)} />
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="text-muted-foreground text-sm">No work to show yet.</p>
            {/if}
          </Card.Content>
        </Card.Root>

        {#if e.documents.length}
          <Card.Root>
            <Card.Header><Card.Title>Documents</Card.Title></Card.Header>
            <Card.Content>
              <ul class="flex flex-col gap-2 text-sm">
                {#each e.documents as d, i (i)}
                  <li>
                    <a href={d.url} download data-external class="hover:bg-muted -mx-2 flex items-center gap-2 rounded-md px-2 py-1">
                      <FileText class="text-muted-foreground size-4 shrink-0" />
                      <span class="min-w-0 flex-1">
                        <span class="block truncate">{d.filename}</span>
                        <span class="text-muted-foreground block text-xs">{bytes(d.byte_size)} · {portal.dateTime(d.created_at)}</span>
                      </span>
                    </a>
                  </li>
                {/each}
              </ul>
            </Card.Content>
          </Card.Root>
        {/if}

        <Card.Root>
          <Card.Header><Card.Title>Commitments</Card.Title></Card.Header>
          <Card.Content>
            {#if e.commitments.length}
              <ul class="divide-y text-sm">
                {#each e.commitments as c, i (i)}
                  <li class="flex items-center gap-3 py-2">
                    <div class="min-w-0 flex-1">
                      <div class="font-medium">{c.description}</div>
                      <div class="text-muted-foreground text-xs">{c.owner_name} · due {date(c.due_on)}</div>
                    </div>
                    {#if c.overdue}<Badge variant="destructive">Overdue</Badge>{/if}
                  </li>
                {/each}
              </ul>
            {:else}
              <p class="text-muted-foreground text-sm">No open commitments.</p>
            {/if}
          </Card.Content>
        </Card.Root>
      </div>
    </div>
  {/if}
</PortalLayout>
