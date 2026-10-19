<script>
  // The portal's "Your progress": the client's own report (what the owner
  // chose to show them), printable or saved as a PDF from the browser.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Printer from "@lucide/svelte/icons/printer";
  import PortalLayout from "../../routes/portal/PortalLayout.svelte";
  import ClientReportView from "./ClientReportView.svelte";
  import { portal, pget } from "../../routes/portal/portal.svelte.js";

  let report = $state(null), missing = $state(false);
  async function load() {
    if (!(await portal.require())) return;
    try { report = await pget("/x/reports/portal/report", { quiet: true }); } catch { missing = true; }
  }
  load();
</script>

<PortalLayout title="Your progress">
  {#if missing}
    <Card.Root><Card.Content class="text-muted-foreground py-10 text-center text-sm">There’s no progress report here. <a class="text-primary underline-offset-4 hover:underline" href="/portal">See your {portal.term("engagement", 2).toLowerCase()}</a>.</Card.Content></Card.Root>
  {:else if !report}
    <Skeleton class="h-24" /><Skeleton class="h-64" />
  {:else}
    <div class="-mt-2 flex flex-wrap items-center gap-x-4 gap-y-2">
      <p class="text-muted-foreground min-w-0 flex-1 text-sm">How the work for {report.client.name} is going.</p>
      <Button variant="outline" size="sm" class="print:hidden" onclick={() => window.print()}><Printer />Print or save as PDF</Button>
    </div>
    <ClientReportView {report} term={(k, n) => portal.term(k, n)} />
  {/if}
</PortalLayout>
