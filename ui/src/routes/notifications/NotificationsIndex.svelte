<script>
  // Notifications (Fizzy's tray as a page): what's new for you, with Mark
  // all as read, then what you've seen. Opening one reads it on the way to
  // what it's about; the button on each marks it read or unread.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import CheckCheck from "@lucide/svelte/icons/check-check";
  import Check from "@lucide/svelte/icons/check";
  import EyeOff from "@lucide/svelte/icons/eye-off";
  import BellOff from "@lucide/svelte/icons/bell-off";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import { get, post, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { go } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { timeAgo } from "$lib/format.js";

  let data = $state(null);
  page.set("Notifications", [{ label: "Home", href: "/" }]);

  async function load() {
    try { data = await get("/notifications"); } catch (e) { toastError(e); }
    nav.refresh();
  }
  load();

  async function open(n) {
    try {
      const r = await get(`/notifications/${n.id}`);
      nav.refresh();
      go(r.path || n.path);
    } catch (e) { toastError(e); }
  }

  async function toggle(n) {
    try {
      if (n.read) await del(`/notifications/${n.id}/reading`);
      else await post(`/notifications/${n.id}/reading`);
      await load();
    } catch (e) { toastError(e); }
  }

  async function readAll() {
    try { const r = await post("/notifications/bulk_reading"); toast(r.summary); await load(); } catch (e) { toastError(e); }
  }
</script>

<h1 class="sr-only">Notifications</h1>

{#snippet item(n)}
  <li class="flex items-start gap-3 py-3 first:pt-0 last:pb-0">
    <button type="button" class="hover:bg-muted/50 -m-2 flex min-w-0 flex-1 items-start gap-3 rounded-md p-2 text-start" onclick={() => open(n)}>
      <PersonAvatar person={n.creator} size="size-8" />
      <span class="min-w-0 flex-1">
        <span class="text-muted-foreground flex items-center gap-2 text-xs">
          <span class="text-foreground font-medium">{n.target?.ref}</span>
          <span class="truncate">{n.target?.name}</span>
          <span class="ms-auto shrink-0">{n.creator_familiar_name} · {timeAgo(n.created_at)}</span>
        </span>
        <span class="block truncate text-sm {n.read ? '' : 'font-semibold'}">{n.title}</span>
        {#if n.excerpt}<span class="text-muted-foreground block truncate text-sm">{n.excerpt}</span>{/if}
      </span>
    </button>
    <Button variant="ghost" size="icon" class="size-8 shrink-0" title={n.read ? "Mark as unread" : "Mark as read"} aria-label={n.read ? "Mark as unread" : "Mark as read"} onclick={() => toggle(n)}>
      {#if n.read}<EyeOff />{:else}<Check />{/if}
    </Button>
  </li>
{/snippet}

{#if !data}
  <Skeleton class="h-48" />
{:else}
  <div class="mx-auto flex w-full max-w-3xl flex-col gap-4">
    <Card.Root>
      <Card.Header>
        <Card.Title>New for you {#if data.unread.length}<span class="text-muted-foreground font-normal">{data.unread.length}</span>{/if}</Card.Title>
        {#if data.unread.length}
          <Card.Action><Button variant="outline" size="sm" onclick={readAll}><CheckCheck />Mark all as read</Button></Card.Action>
        {/if}
      </Card.Header>
      <Card.Content>
        {#if !data.unread.length}
          <EmptyState icon={BellOff} title="Nothing new for you" />
        {:else}
          <ul class="flex flex-col divide-y">{#each data.unread as n (n.id)}{@render item(n)}{/each}</ul>
        {/if}
      </Card.Content>
    </Card.Root>

    {#if data.read.length}
      <Card.Root>
        <Card.Header><Card.Title>Previously seen</Card.Title></Card.Header>
        <Card.Content>
          <ul class="flex flex-col divide-y">{#each data.read as n (n.id)}{@render item(n)}{/each}</ul>
        </Card.Content>
      </Card.Root>
    {/if}
  </div>
{/if}
