<script>
  // The client's approval page, from the emailed link (/approve/<token>):
  // the version exactly as it was sent, then the decision (approve, or ask
  // for changes, with their typed name), or the decision already made. No
  // sidebar: a centered column of cards, like the signed-out screens.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Check from "@lucide/svelte/icons/check";
  import RunwellMark from "$lib/components/RunwellMark.svelte";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import { get, post } from "$lib/api.js";
  import { money } from "$lib/format.js";

  let { params } = $props();
  let data = $state(null), failed = $state(""), error = $state("");
  let name = $state(""), comment = $state(""), busy = $state(false);
  document.title = "Review your agreement";

  // A screen is made afresh for each address, so its params never change.
  // svelte-ignore state_referenced_locally
  get(`/approve/${params.token}`, { quiet: true }).then((r) => (data = r)).catch((e) => (failed = e.message));

  async function decide(decision) {
    error = "";
    busy = true;
    try { data = await post(`/approve/${params.token}`, { decision, approver_name: name, comment }, { quiet: true }); } catch (e) { error = e.message; }
    busy = false;
  }
  const change = $derived(data && data.version.kind !== "initial");
</script>

<main class="bg-muted flex min-h-svh flex-col items-center gap-6 p-6 md:p-10">
  <div class="flex w-full max-w-2xl flex-col gap-6">
    <div class="flex items-center gap-2 self-center font-medium">
      <RunwellMark class="size-7" />
      {data?.brand || "Runwell"}
    </div>

    {#if failed}
      <Card.Root>
        <Card.Header><Card.Title><h1>Couldn’t open this approval</h1></Card.Title></Card.Header>
        <Card.Content><p class="text-sm">{failed}</p></Card.Content>
      </Card.Root>
    {:else if !data}
      <Skeleton class="h-64" />
    {:else}
      <p class="text-muted-foreground text-center text-sm">Prepared for {data.contact.name}</p>
      {#if data.notice}<Alert.Root><Check /><Alert.Title>{data.notice}</Alert.Title></Alert.Root>{/if}

      <Card.Root>
        <Card.Header>
          <Card.Description>For {data.snapshot.client}</Card.Description>
          <Card.Title class="text-xl"><h1>{data.snapshot.engagement?.ref} · {data.snapshot.engagement?.title}</h1></Card.Title>
          <Card.Description>{data.version.label}{data.version.sent_at_text ? ` · sent ${data.version.sent_at_text}` : ""}</Card.Description>
        </Card.Header>
        <Card.Content class="flex flex-col gap-3">
          {#if data.version.summary}<p class="font-semibold">{data.version.summary}</p>{/if}
          {#if data.version.reason}<div class="prose-admin text-sm">{@html data.version.reason}</div>{/if}
          {#if data.snapshot.description && !change}<div class="prose-admin text-muted-foreground text-sm">{@html data.snapshot.description}</div>{/if}
          {#if data.snapshot.items.length}
            <ul class="divide-y text-sm">
              {#each data.snapshot.items as it, i (i)}
                <li class="flex items-center gap-2 py-2">
                  <span class="min-w-0 flex-1">{it.description}</span>
                  {#if data.priced}<span class="tabular-nums">{money(it.price_cents)}</span>{/if}
                </li>
              {/each}
              {#if data.priced}
                <li class="flex items-center gap-2 py-2 font-semibold">
                  <span class="flex-1">{data.total_label}</span>
                  <span class="tabular-nums">{money(data.total_cents)}</span>
                </li>
              {/if}
            </ul>
          {:else if data.version.cadence && data.priced}
            <p class="text-sm">New price: <strong>{money(data.total_cents)}</strong> {data.version.cadence}</p>
          {/if}
          {#if data.version.content_hash}<p class="text-muted-foreground text-xs">ref {data.version.content_hash}</p>{/if}
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header><Card.Title>{data.decision ? "Decision" : "Your decision"}</Card.Title></Card.Header>
        <Card.Content>
          {#if data.decision}
            {@const d = data.decision}
            <div class="flex flex-col gap-2 text-sm">
              <div class="flex flex-wrap items-center gap-2">
                <StatusBadge status={d.decision} />
                <span>by {d.approver_name}{#if d.method === "agent"}, through their agent ({d.via}){/if} on {d.decided_at_text}</span>
              </div>
              {#if d.comment}<div class="prose-admin">{@html d.comment}</div>{/if}
            </div>
          {:else if !data.usable}
            <p class="text-muted-foreground text-sm">This link is no longer valid. Ask us for a new one if you still need to decide.</p>
          {:else if data.contact.can_approve}
            <form onsubmit={(e) => e.preventDefault()} class="flex flex-col gap-4">
              <p class="text-muted-foreground text-sm">Approving records your name, the time, and the exact terms shown above.</p>
              {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
              <Field.Group>
                <Field.Field>
                  <Field.Label for="approver-name">Your name</Field.Label>
                  <Input id="approver-name" bind:value={name} required autocomplete="name" placeholder="Type your full name to confirm" />
                </Field.Field>
                <Field.Field>
                  <Field.Label for="approver-comment">Comment (optional)</Field.Label>
                  <TiptapEditor id="approver-comment" format="html" tools="email" bind:value={comment} minHeight="min-h-20" />
                </Field.Field>
              </Field.Group>
              <div class="flex flex-wrap justify-center gap-2">
                <Button type="button" disabled={busy} onclick={() => decide("approved")}><Check />Approve</Button>
                <Button type="button" variant="outline" disabled={busy} onclick={() => decide("changes_requested")}>Request changes</Button>
              </div>
            </form>
          {:else}
            <p class="text-muted-foreground text-sm">This link was sent to you for review. Someone at your company who can approve agreements needs to decide.</p>
          {/if}
        </Card.Content>
      </Card.Root>
    {/if}
  </div>
</main>
