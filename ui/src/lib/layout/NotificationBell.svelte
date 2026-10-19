<script>
  // The bell at the far right of the header: how many notifications are
  // new (the dot and count), and a menu of them. Opening one reads it on the
  // way to what it's about; All notifications is the full list.
  import * as Popover from "$lib/components/ui/popover/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Bell from "@lucide/svelte/icons/bell";
  import CheckCheck from "@lucide/svelte/icons/check-check";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import { get, post } from "$lib/api.js";
  import { nav } from "../nav.svelte.js";
  import { go } from "../router.svelte.js";
  import { toast, toastError } from "../toast.svelte.js";
  import { timeAgo } from "../format.js";

  let open = $state(false), list = $state(null);
  const count = $derived(nav.counts.notifications || 0);

  async function load() {
    try { list = (await get("/notifications/tray", { quiet: true })).notifications; } catch (e) { toastError(e); }
  }
  $effect(() => { if (open) load(); });

  async function view(n) {
    open = false;
    try {
      const r = await get(`/notifications/${n.id}`);
      nav.refresh();
      go(r.path || n.path);
    } catch (e) { toastError(e); }
  }
  async function readAll() {
    try { toast((await post("/notifications/bulk_reading")).summary); list = []; nav.refresh(); } catch (e) { toastError(e); }
  }
</script>

<Popover.Root bind:open>
  <Popover.Trigger>
    {#snippet child({ props })}
      <Button {...props} variant="ghost" size="icon" class="relative size-8" aria-label={count ? `Notifications, ${count} new` : "Notifications"}>
        <Bell />
        {#if count}
          <span class="bg-primary text-primary-foreground absolute -end-0.5 -top-0.5 flex h-4 min-w-4 items-center justify-center rounded-full px-1 text-[10px] font-semibold tabular-nums">{count > 99 ? "99+" : count}</span>
        {/if}
      </Button>
    {/snippet}
  </Popover.Trigger>
  <Popover.Content align="end" class="w-96 max-w-[calc(100vw-2rem)] p-0">
    <div class="flex items-center gap-2 border-b px-4 py-3">
      <h2 class="text-sm font-semibold">Notifications</h2>
      {#if list?.length}
        <Button variant="ghost" size="sm" class="ms-auto h-7" onclick={readAll}><CheckCheck />Mark all as read</Button>
      {/if}
    </div>
    {#if list === null}
      <p class="text-muted-foreground px-4 py-6 text-center text-sm">Loading…</p>
    {:else if !list.length}
      <p class="text-muted-foreground px-4 py-6 text-center text-sm">Nothing new for you.</p>
    {:else}
      <ul class="max-h-96 divide-y overflow-y-auto">
        {#each list as n (n.id)}
          <li>
            <button type="button" class="hover:bg-muted/50 flex w-full items-start gap-3 px-4 py-3 text-start" onclick={() => view(n)}>
              <PersonAvatar person={n.creator} size="size-7" />
              <span class="min-w-0 flex-1">
                <span class="block truncate text-sm font-medium">{n.title}</span>
                <span class="text-muted-foreground block truncate text-xs">{[n.target?.ref, n.target?.name].filter(Boolean).join(" ")} · {timeAgo(n.created_at)}</span>
              </span>
            </button>
          </li>
        {/each}
      </ul>
    {/if}
    <a href="/notifications" class="text-primary block border-t px-4 py-2.5 text-center text-sm hover:underline" onclick={() => (open = false)}>All notifications</a>
  </Popover.Content>
</Popover.Root>
