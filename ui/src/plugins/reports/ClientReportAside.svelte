<script>
  // "Their progress report", in a client's side column: whether they see
  // it in their portal, the switch for them (people who manage settings),
  // and the preview.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Label } from "$lib/components/ui/label/index.js";
  import Eye from "@lucide/svelte/icons/eye";
  import { get, put } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { rp } from "./reports.js";

  let { record } = $props();
  let v = $state(null), busy = $state(false);
  async function load() {
    try { v = (await get(rp(`/clients/${record.id}/client_report`), { quiet: true })).visible; } catch { v = null; }
  }
  $effect(() => { record.id; load(); });
  async function toggle(on) {
    busy = true;
    try { toast((await put(rp(`/clients/${record.id}/client_report`), { client_report: { hidden: !on } })).summary); await load(); } catch (e) { toastError(e); }
    busy = false;
  }
</script>

{#if v && !record.internal}
  <Card.Root>
    <Card.Header>
      <Card.Title>Their progress report</Card.Title>
      <Card.Description>{v.seen ? "They see it in their portal, under “Your progress”." : v.why}</Card.Description>
    </Card.Header>
    <Card.Content class="flex flex-col gap-3">
      {#if session.can("manage_settings") && v.portal_granted && v.page_on}
        <div class="flex items-center gap-2">
          <Switch id="client-report-{record.id}" checked={!v.hidden} disabled={busy} onCheckedChange={toggle} />
          <Label for="client-report-{record.id}">Show it to {record.name}</Label>
        </div>
      {/if}
      <Button variant="outline" size="sm" class="self-start" href={`/reports/clients/${record.id}/client_report`}><Eye />See it as {record.name} sees it</Button>
    </Card.Content>
  </Card.Root>
{/if}
