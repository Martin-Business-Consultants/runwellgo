<script>
  // A client's Engagements tab: filtered by derived state (a Show filter,
  // each choice with its count), each with its ref, title, type, price and
  // agreed amount.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import { session } from "$lib/session.svelte.js";
  import QueryFilter from "$lib/components/QueryFilter.svelte";
  import { router, withQuery } from "$lib/router.svelte.js";
  import { money } from "$lib/format.js";

  let { record } = $props();
  // With agreements off nothing is a draft, sent or sent back.
  const agreements = session.me.agreements !== false;
  const filters = [["approved", "Active"], ["open", "All open"], ["draft", "Draft"], ["sent", "Sent"], ["changes_requested", "Changes requested"], ["internal", "Internal"], ["closed", "Closed"], ["all", "All"]]
    .filter(([v]) => agreements || !["draft", "sent", "changes_requested"].includes(v));
  const list = $derived(record.engagements || []);
  const current = $derived(filters.some(([v]) => v === router.query.get("engagements")) ? router.query.get("engagements") : "approved");
  // The same rules as the server's filter (Engagement.filtered).
  const matches = (e, f) => (f === "all" ? true : f === "closed" ? e.closed : f === "open" ? !e.closed : f === "approved" ? e.active : e.state === f);
  const shown = $derived(list.filter((e) => matches(e, current)));
  const amount = (e) => (session.me.prices && agreements && e.agreed_amount_cents ? "Agreed " + money(e.agreed_amount_cents) + (e.shape === "recurring" ? " / period" : "") : "");
  const price = (e) => (session.me.prices && e.price_cents != null ? money(e.price_cents) : "");
</script>

<Card.Root>
  <Card.Header>
    <Card.Title>{session.term("engagement", 2)}</Card.Title>
    <Card.Action class="flex flex-wrap justify-end gap-2">
      <QueryFilter param="engagements" title="Show" options={filters.map(([value, label]) => ({ value, label, count: list.filter((e) => matches(e, value)).length }))} />
      <Button href={`/engagements/new?client_id=${record.id}`} variant="outline" size="sm"><Plus />New {session.term("engagement").toLowerCase()}</Button>
    </Card.Action>
  </Card.Header>
  <Card.Content class="flex flex-col gap-3">
    {#if shown.length}
      <ul class="flex flex-col gap-2 text-sm">
        {#each shown as e (e.id)}
          <li class="flex items-center gap-2">
            <a href={`/engagements/${e.ref}`} class="min-w-0 flex-1 truncate hover:underline">
              <strong>{e.ref}</strong> {e.title}
              <span class="text-muted-foreground text-xs">{[e.type_name, price(e), amount(e)].filter(Boolean).join(" · ")}</span>
            </a>
            <StatusBadge status={e.closed ? e.close_outcome || "closed" : e.state} label={e.state_label} />
          </li>
        {/each}
      </ul>
    {:else if list.length}
      <p class="text-muted-foreground text-sm">None {filters.find(([v]) => v === current)[1].toLowerCase()}. <a class="text-primary underline underline-offset-4" href={withQuery({ engagements: "all" })}>Show all</a></p>
    {:else}
      <p class="text-muted-foreground text-sm">No {session.term("engagement", 2).toLowerCase()} yet.</p>
    {/if}
  </Card.Content>
</Card.Root>
