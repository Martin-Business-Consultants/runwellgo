<script>
  // The inset's header (shadcn's site-header): the sidebar's toggle, then
  // where you are (the screen's breadcrumbs, the last one the screen
  // itself), and at the far right what plugins put there (the header slot),
  // their quick actions, Search (s or
  // ⌘K) and Add (a), which open their dialogs by window events, and the
  // notifications bell last.
  import * as Breadcrumb from "$lib/components/ui/breadcrumb/index.js";
  import { Separator } from "$lib/components/ui/separator/index.js";
  import { SidebarTrigger } from "$lib/components/ui/sidebar/index.js";
  import { page } from "../page.svelte.js";
  import PluginQuickActions from "../../routes/plugins/PluginQuickActions.svelte";
  import PluginSlot from "$lib/components/PluginSlot.svelte";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Kbd } from "$lib/components/ui/kbd/index.js";
  import SearchIcon from "@lucide/svelte/icons/search";
  import Plus from "@lucide/svelte/icons/plus";
  import NotificationBell from "./NotificationBell.svelte";
</script>

<header class="flex h-12 shrink-0 items-center gap-2 border-b transition-[width,height] ease-linear">
  <div class="flex w-full min-w-0 items-center gap-1 px-4 lg:gap-2 lg:px-6">
    <SidebarTrigger class="-ms-1" />
    <Separator orientation="vertical" class="mx-2 data-[orientation=vertical]:h-4" />
    <Breadcrumb.Root class="min-w-0">
      <Breadcrumb.List class="flex-nowrap">
        {#each page.crumbs as c (c.href)}
          <Breadcrumb.Item class="hidden md:inline-flex">
            <Breadcrumb.Link href={c.href}>{c.label}</Breadcrumb.Link>
          </Breadcrumb.Item>
          <Breadcrumb.Separator class="hidden md:block" />
        {/each}
        <Breadcrumb.Item class="min-w-0">
          <Breadcrumb.Page class="truncate">{page.title}</Breadcrumb.Page>
        </Breadcrumb.Item>
      </Breadcrumb.List>
    </Breadcrumb.Root>
    <div class="ms-auto flex shrink-0 items-center gap-2">
      <PluginSlot name="header" />
      {#key page.record}<PluginQuickActions record={page.record} />{/key}
      <Button variant="outline" size="sm" class="text-muted-foreground w-9 justify-start px-0 sm:w-56 sm:px-2.5" onclick={() => dispatchEvent(new Event("opensearch"))} aria-label="Search">
        <SearchIcon class="mx-auto sm:mx-0" /><span class="hidden font-normal sm:inline">Search…</span><Kbd class="ms-auto hidden sm:inline-flex">⌘K</Kbd>
      </Button>
      <Button size="sm" onclick={() => dispatchEvent(new Event("quickactions"))} aria-label="Add">
        <Plus /><span class="hidden sm:inline">Add</span><Kbd class="bg-primary-foreground/15 text-primary-foreground hidden sm:inline-flex">A</Kbd>
      </Button>
      <NotificationBell />
    </div>
  </div>
</header>
