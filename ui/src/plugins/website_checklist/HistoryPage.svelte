<script>
  // The websites' history (the Rails audits/index): every change to all
  // websites, one (?site_id=) or one step and its webhooks (&item_id=),
  // newest first: who made it (a person, or their agent), when, and each
  // field's before and after. Download CSV takes the lot.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Download from "@lucide/svelte/icons/download";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import { get } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { router, href, withQuery, onQuery } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { dateTime } from "$lib/format.js";
  import { wc } from "./wc.js";

  const siteID = $derived(router.query.get("site_id") || "");
  const itemID = $derived(router.query.get("item_id") || "");
  let data = $state(null);

  const title = $derived(data?.item ? `History of ${data.item.title}` : data?.site ? `History of ${data.site.name}` : "Websites’ history");
  $effect(() => {
    if (!data) return;
    const crumbs = [{ label: "Websites", href: "/websites" }];
    if (data.site) crumbs.push({ label: data.site.name, href: href(`/websites/${data.site.id}`, { item: data.item?.id }) });
    page.set(title, crumbs);
  });
  page.set("Websites’ history", [{ label: "Websites", href: "/websites" }]);

  onQuery(() => get(wc("/websites/history", { site_id: siteID, item_id: itemID, before: router.query.get("before") })).then((r) => (data = r)).catch(toastError));
  const csv = $derived("/api" + wc("/websites/history", { site_id: siteID, item_id: itemID, format: "csv" }));
  const what = $derived(data?.item ? "this step" : data?.site ? "this website and its checklist" : "a website or its checklist");
  const link = (a) => a.record_type !== "Site" && a.item_id && a.site_id && a.action !== "deleted";
  const upper = (s) => s.charAt(0).toUpperCase() + s.slice(1);
</script>

<div class="flex flex-wrap items-center justify-between gap-2">
  <h1 class="text-2xl font-semibold tracking-tight">{title}</h1>
  <Button variant="outline" size="sm" href={csv} download data-external><Download />Download CSV</Button>
</div>

{#if !data}
  <Skeleton class="h-48" />
{:else if !data.audits.length}
  <Card.Root><Card.Content class="text-muted-foreground text-sm">No changes yet. Every change to {what} shows here: who made it, when, and what it was before and after.</Card.Content></Card.Root>
{:else}
  <Card.Root class="py-0">
    <Table.Root>
      <Table.Header>
        <Table.Row><Table.Head>When</Table.Head><Table.Head>Who</Table.Head><Table.Head>What</Table.Head><Table.Head>Change</Table.Head></Table.Row>
      </Table.Header>
      <Table.Body>
        {#each data.audits as a (a.id)}
          <Table.Row class="align-top">
            <Table.Cell class="text-sm whitespace-nowrap">{dateTime(a.created_at)}</Table.Cell>
            <Table.Cell class="text-sm"><span class="inline-flex items-center gap-2"><PersonAvatar person={a.actor} fallback="Runwell" /><span>{a.who}</span></span></Table.Cell>
            <Table.Cell class="text-sm whitespace-normal">
              {#if link(a)}<a class="underline-offset-4 hover:underline" href={href(`/websites/${a.site_id}`, { item: a.item_id })}>{a.subject}</a>{:else}{a.subject}{/if}
              {#if !data.site}<div class="text-muted-foreground text-xs">{a.site_name}</div>{/if}
            </Table.Cell>
            <Table.Cell class="text-sm whitespace-normal">
              <strong>{upper(a.action)}</strong>
              {#if a.changes.length}
                <dl class="mt-1 grid grid-cols-[auto_minmax(0,1fr)] gap-x-4 gap-y-1">
                  {#each a.changes as c, i (i)}
                    <dt class="text-muted-foreground">{c.field}</dt>
                    <dd class="flex flex-wrap gap-x-2 gap-y-1 break-words whitespace-pre-line">
                      {#if c.before}<del class="bg-red-500/10">{c.before}</del>{/if}
                      {#if c.after}<ins class="bg-emerald-500/15 no-underline">{c.after}</ins>{:else}<span class="text-muted-foreground">(blank)</span>{/if}
                    </dd>
                  {/each}
                </dl>
              {/if}
            </Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Root>
  {#if data.older_before}
    <div><Button variant="outline" size="sm" href={withQuery({ before: data.older_before })}>Older changes</Button></div>
  {/if}
{/if}
