<script>
  // Settings › Audit log: everything that happened, newest first, filtered
  // by area, period and person, and downloadable as CSV.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Download from "@lucide/svelte/icons/download";
  import { get } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { router, go, withQuery, href, onQuery } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { dateTime } from "$lib/format.js";

  const area = $derived(router.query.get("area") || "all");
  const days = $derived(router.query.get("days") || "30");
  const person = $derived(router.query.get("person") || "");
  const pageNo = $derived(Math.max(1, Number(router.query.get("page")) || 1));
  const per = 50;
  let data = $state(null);

  onQuery(() => get(href("/settings/audit_log", { area, days, person, page: pageNo, per })).then((d) => (data = d)).catch(toastError));
  const csv = $derived(href("/api/settings/audit_log", { area, days, person, format: "csv" }));
  const set = (k, v) => go(withQuery({ [k]: v === "all" && k === "person" ? "" : v }));
</script>

<SettingsLayout title="Audit log" description={`Everything that happened, newest first: sign-ins (and failed ones), two-factor and role changes, settings, and every change of consequence to ${session.term("client", 2).toLowerCase()}, agreements and ${session.term("work").toLowerCase()}. Nothing here can be edited or deleted.`}>
  {#snippet actions()}
    <Button variant="outline" size="sm" href={csv} data-external download><Download />Download CSV</Button>
  {/snippet}
  <div class="flex flex-wrap gap-2">
    <Select.Root type="single" value={area} onValueChange={(v) => set("area", v)}>
      <Select.Trigger size="sm" class="w-52" aria-label="Area">{data?.areas?.[area] || "Area…"}</Select.Trigger>
      <Select.Content>{#each Object.entries(data?.areas || {}) as [k, label] (k)}<Select.Item value={k}>{label}</Select.Item>{/each}</Select.Content>
    </Select.Root>
    <Select.Root type="single" value={days} onValueChange={(v) => set("days", v)}>
      <Select.Trigger size="sm" class="w-40" aria-label="When">{data?.periods?.[days] || "When…"}</Select.Trigger>
      <Select.Content>{#each Object.entries(data?.periods || {}) as [k, label] (k)}<Select.Item value={k}>{label}</Select.Item>{/each}</Select.Content>
    </Select.Root>
    <Select.Root type="single" value={person || "all"} onValueChange={(v) => set("person", v)}>
      <Select.Trigger size="sm" class="w-48" aria-label="Who">{session.me?.team?.find((t) => String(t.id) === person)?.name || data?.person?.name || "Anyone"}</Select.Trigger>
      <Select.Content>
        <Select.Item value="all">Anyone</Select.Item>
        {#each session.me?.team || [] as t (t.id)}<Select.Item value={String(t.id)}>{t.name}</Select.Item>{/each}
      </Select.Content>
    </Select.Root>
  </div>
  {#if !data}
    <Skeleton class="h-96 w-full" />
  {:else if !data.events.length}
    <Card.Root><Card.Content class="text-muted-foreground py-10 text-center text-sm">Nothing in this area and period.</Card.Content></Card.Root>
  {:else}
    <Card.Root class="py-0">
      <Table.Root>
        <Table.Header>
          <Table.Row><Table.Head class="ps-4">When</Table.Head><Table.Head>What</Table.Head><Table.Head>Who</Table.Head><Table.Head>On</Table.Head><Table.Head class="pe-4">Details</Table.Head></Table.Row>
        </Table.Header>
        <Table.Body>
          {#each data.events as e (e.id)}
            <Table.Row>
              <Table.Cell class="ps-4 whitespace-nowrap" title={e.occurred_at}>{dateTime(e.occurred_at)}</Table.Cell>
              <Table.Cell>{e.title}</Table.Cell>
              <Table.Cell>{e.by}{#if e.source === "agent"} <span class="text-muted-foreground">{e.source_label}</span>{/if}</Table.Cell>
              <Table.Cell class="max-w-56 truncate">{e.on}</Table.Cell>
              <Table.Cell class="text-muted-foreground max-w-64 pe-4 text-xs whitespace-normal [overflow-wrap:anywhere]"><span class="line-clamp-3" title={e.details}>{e.details}</span></Table.Cell>
            </Table.Row>
          {/each}
        </Table.Body>
      </Table.Root>
    </Card.Root>
    {#if pageNo > 1 || data.more}
      <div class="flex justify-end gap-2">
        <Button variant="outline" size="sm" disabled={pageNo <= 1} href={pageNo > 1 ? withQuery({ page: pageNo - 1 }) : undefined}>Newer</Button>
        <Button variant="outline" size="sm" disabled={!data.more} href={data.more ? withQuery({ page: pageNo + 1 }) : undefined}>Older</Button>
      </div>
    {/if}
  {/if}
</SettingsLayout>
