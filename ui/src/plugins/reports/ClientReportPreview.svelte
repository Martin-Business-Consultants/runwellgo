<script>
  // "See it as <client> sees it": the client's progress report exactly as
  // their portal shows it, for the owner, framed like the portal, with
  // whether they see it now and the switch for them.
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import RunwellMark from "$lib/components/RunwellMark.svelte";
  import Eye from "@lucide/svelte/icons/eye";
  import ClientReportView from "./ClientReportView.svelte";
  import { get, put } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { rp } from "./reports.js";

  let { params } = $props();
  let report = $state(null);
  const id = $derived(params.id);

  async function load() {
    try {
      report = await get(rp(`/clients/${id}/client_report`));
      page.set(`As ${report.client.name} sees it`, [{ label: session.term("client", 2), href: "/clients" }, { label: report.client.name, href: `/clients/${id}` }]);
    } catch (e) { toastError(e); }
  }
  $effect(() => { id; load(); });
  async function setHidden(hidden) {
    try { toast((await put(rp(`/clients/${id}/client_report`), { client_report: { hidden } })).summary); load(); } catch (e) { toastError(e); }
  }
</script>

{#if !report}
  <Skeleton class="h-16" /><Skeleton class="h-80" />
{:else}
  <h1 class="sr-only">{report.client.name}’s progress report, as they see it</h1>
  <Alert.Root>
    <Eye />
    <Alert.Title>{report.visible.seen ? `This is what ${report.client.name} sees in their portal.` : `${report.client.name} doesn’t see this now.`}</Alert.Title>
    <Alert.Description>
      <p>{report.visible.seen ? "Their contacts with portal access find it under “Your progress”." : report.visible.why}
        {#if session.can("manage_settings")}<a href="/settings/reports" class="underline underline-offset-2">Choose what clients see</a>.{/if}</p>
      {#if session.can("manage_settings") && report.visible.portal_granted && report.visible.page_on}
        <Button variant="outline" size="sm" class="mt-2" onclick={() => setHidden(!report.visible.hidden)}>{report.visible.hidden ? "Show it to them again" : "Switch it off for them"}</Button>
      {/if}
    </Alert.Description>
  </Alert.Root>

  <!-- Framed like the portal: its bar, then the page. -->
  <div class="bg-muted/40 overflow-hidden rounded-xl border">
    <div class="bg-background flex items-center gap-2 border-b px-4 py-3 text-sm font-medium" aria-hidden="true"><RunwellMark class="size-6" />{session.brand}<span class="text-muted-foreground ms-auto hidden font-normal sm:inline">Your progress</span></div>
    <div class="flex flex-col gap-4 p-4 sm:p-6">
      <div>
        <p class="text-2xl font-semibold tracking-tight">Your progress</p>
        <p class="text-muted-foreground mt-1 text-sm">How the work for {report.client.name} is going.</p>
      </div>
      <ClientReportView {report} term={(k, n) => session.term(k, n)} preview />
    </div>
  </div>
{/if}
