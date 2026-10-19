<script>
  // Settings › Export: everything Runwell holds as one .zip, made in the
  // background and emailed when ready, kept for a week.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Share from "@lucide/svelte/icons/share";
  import { get, post } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { dateTime, plural, bytes } from "$lib/format.js";

  let data = $state(null), busy = $state(false), timer = null;

  function load() {
    return get("/settings/exports").then((d) => {
      data = d;
      clearTimeout(timer);
      if (d.exports.some((x) => x.state === "queued" || x.state === "working")) timer = setTimeout(load, 2000);
    }).catch(toastError);
  }
  load();
  $effect(() => () => clearTimeout(timer));

  async function create() {
    busy = true;
    try { toast((await post("/settings/exports")).summary); load(); } catch (err) { toastError(err); }
    busy = false;
  }
</script>

<SettingsLayout title="Export">
  <Card.Root>
    <Card.Header>
      <Card.Title>Export everything</Card.Title>
      <Card.Description>One .zip of everything Runwell holds: every table (yours and your plugins’) as JSON and CSV, and every file. It’s yours to keep, move or open in a spreadsheet. Passwords, tokens and other secrets are left out. It’s built in the background and emailed to you when ready, then kept for a week.</Card.Description>
    </Card.Header>
    <Card.Footer><Button onclick={create} disabled={busy}><Share />Make an export</Button></Card.Footer>
  </Card.Root>
  {#if !data}
    <Skeleton class="h-32 w-full" />
  {:else if data.exports.length}
    <Card.Root>
      <Card.Header><Card.Title>Recent exports</Card.Title></Card.Header>
      <Card.Content>
        <ul class="divide-y rounded-md border">
          {#each data.exports as x (x.id)}
            <li class="flex items-center gap-3 px-3 py-2 text-sm">
              <div class="min-w-0 flex-1">
                <p class="font-medium">{dateTime(x.created_at)}</p>
                <p class="text-muted-foreground text-xs">By {x.by} ·
                  {#if x.state === "ready"}{(x.rows_count ?? 0).toLocaleString()} records, {plural(x.files_count ?? 0, "file")}, {bytes(x.byte_size)}
                  {:else if x.state === "failed"}failed: {x.error}
                  {:else}being made…{/if}
                </p>
              </div>
              {#if x.state === "ready" && x.download_url}<Button variant="outline" size="sm" href={`/api/settings/exports/${x.id}/download`} data-external download>Download</Button>{/if}
            </li>
          {/each}
        </ul>
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>
