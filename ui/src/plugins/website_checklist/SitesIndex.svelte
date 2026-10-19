<script>
  // Every client's websites (the Rails sites/index): those with steps still
  // to do first (Needs attention), or all; each with its checklist's count
  // and what's left, and Edit and Delete for those who may.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import EllipsisVertical from "@lucide/svelte/icons/ellipsis-vertical";
  import History from "@lucide/svelte/icons/history";
  import IndexToolbar from "$lib/components/IndexToolbar.svelte";
  import { get, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, href, onQuery } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { wc, summaryClass } from "./wc.js";

  page.set("Websites");
  let data = $state(null);
  const clientID = $derived(router.query.get("client_id") || "");

  async function load() {
    try { data = await get(wc("/websites", { show: router.query.get("show"), client_id: clientID })); } catch (e) { toastError(e); }
  }
  onQuery(load);

  async function remove(s) {
    if (!(await confirm({ title: `Delete ${s.name}?`, message: "This forgets the website and its checklist.", confirmLabel: "Delete", destructive: true }))) return;
    try { toast((await del(wc(`/websites/${s.id}`))).message); load(); } catch (e) { toastError(e); }
  }
</script>

<IndexToolbar
  filters={data ? [{ param: "show", current: data.show, title: "Show…", options: [
    { value: "attention", label: "Needs attention", count: data.counts.attention },
    { value: "all", label: "All", count: data.counts.all },
  ] }] : []}
  newText={data?.can_manage ? "New website" : ""} newHref={data?.can_manage ? href("/websites/new", { client_id: clientID }) : ""}>
  <Button href="/websites/history" variant="outline" size="sm"><History />History</Button>
</IndexToolbar>

{#if !data}
  <Skeleton class="h-48" />
{:else}
  <Card.Root class="py-0">
    <Table.Root>
      <Table.Header>
        <Table.Row>
          <Table.Head>Website</Table.Head>
          <Table.Head>{session.term("client")}</Table.Head>
          <Table.Head>Checklist</Table.Head>
          <Table.Head>To do</Table.Head>
          <Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>
        </Table.Row>
      </Table.Header>
      <Table.Body>
        {#each data.websites as s (s.id)}
          <Table.Row>
            <Table.Cell class="max-w-72">
              <a href={`/websites/${s.id}`} class="block hover:underline">
                <span class="font-medium">{s.name}</span>
                {#if s.host}<span class="text-muted-foreground block truncate text-xs">{s.host}</span>{/if}
              </a>
            </Table.Cell>
            <Table.Cell><a href={`/clients/${s.client.id}?tab=website_checklist`} class="hover:underline">{s.client.display_name}</a></Table.Cell>
            <Table.Cell><Badge variant="outline" class={summaryClass(s.checklist)}>{s.checklist_text}</Badge></Table.Cell>
            <Table.Cell class="text-sm">{s.needs_attention || "Nothing"}</Table.Cell>
            <Table.Cell>
              {#if data.can_manage}
                <DropdownMenu.Root>
                  <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><EllipsisVertical /><span class="sr-only">Actions for {s.name}</span></Button>{/snippet}</DropdownMenu.Trigger>
                  <DropdownMenu.Content align="end">
                    <DropdownMenu.Item onSelect={() => router.navigate(`/websites/${s.id}/edit`)}>Edit</DropdownMenu.Item>
                    <DropdownMenu.Item variant="destructive" onSelect={() => remove(s)}>Delete</DropdownMenu.Item>
                  </DropdownMenu.Content>
                </DropdownMenu.Root>
              {/if}
            </Table.Cell>
          </Table.Row>
        {:else}
          <Table.Row>
            <Table.Cell colspan={5} class="text-muted-foreground text-sm whitespace-normal">
              {#if data.show === "attention" && data.counts.all > 0}
                Every website is live, every step confirmed. <a class="text-foreground underline-offset-4 hover:underline" href={href("/websites", { show: "all", client_id: clientID })}>Show all</a>
              {:else}
                No websites yet. Add a {session.term("client").toLowerCase()}’s website to gather what it needs to go live, step by step.
              {/if}
            </Table.Cell>
          </Table.Row>
        {/each}
      </Table.Body>
    </Table.Root>
  </Card.Root>
{/if}
