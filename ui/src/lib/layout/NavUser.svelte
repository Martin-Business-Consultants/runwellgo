<script>
  // The person at the sidebar's foot (shadcn's nav-user): their account,
  // settings, help, the theme, and signing out.
  import * as Sidebar from "$lib/components/ui/sidebar/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import * as Avatar from "$lib/components/ui/avatar/index.js";
  import { useSidebar } from "$lib/components/ui/sidebar/context.svelte.js";
  import { setMode, userPrefersMode } from "mode-watcher";
  import ChevronsUpDown from "@lucide/svelte/icons/chevrons-up-down";
  import UserIcon from "@lucide/svelte/icons/user";
  import Settings from "@lucide/svelte/icons/settings";
  import CircleHelp from "@lucide/svelte/icons/circle-help";
  import Sun from "@lucide/svelte/icons/sun";
  import Moon from "@lucide/svelte/icons/moon";
  import Monitor from "@lucide/svelte/icons/monitor";
  import LogOut from "@lucide/svelte/icons/log-out";
  import { session } from "../session.svelte.js";
  import { del } from "../api.js";
  import { go } from "../router.svelte.js";
  import { initials } from "../format.js";

  const sidebar = useSidebar();
  const me = $derived(session.me);

  async function signOut() {
    try { await del("/session"); } finally { session.signedOut(); go("/"); }
  }
</script>

<Sidebar.Menu>
  <Sidebar.MenuItem>
    <DropdownMenu.Root>
      <DropdownMenu.Trigger>
        {#snippet child({ props })}
          <Sidebar.MenuButton size="lg" class="data-[state=open]:bg-sidebar-accent data-[state=open]:text-sidebar-accent-foreground" {...props}>
            <Avatar.Root class="size-8 rounded-lg">
              <Avatar.Image src={`/api/users/${me.id}/avatar`} alt={me.name} />
              <Avatar.Fallback class="rounded-lg">{initials(me.name)}</Avatar.Fallback>
            </Avatar.Root>
            <div class="grid flex-1 text-start text-sm leading-tight">
              <span class="truncate font-medium">{me.name}</span>
              <span class="text-muted-foreground truncate text-xs">{me.email_address}</span>
            </div>
            <ChevronsUpDown class="ms-auto size-4" />
          </Sidebar.MenuButton>
        {/snippet}
      </DropdownMenu.Trigger>
      <DropdownMenu.Content class="w-(--bits-dropdown-menu-anchor-width) min-w-56 rounded-lg" side={sidebar.isMobile ? "bottom" : "right"} align="end" sideOffset={4}>
        <DropdownMenu.Label class="p-0 font-normal">
          <div class="flex items-center gap-2 px-1 py-1.5 text-start text-sm">
            <Avatar.Root class="size-8 rounded-lg">
              <Avatar.Image src={`/api/users/${me.id}/avatar`} alt={me.name} />
              <Avatar.Fallback class="rounded-lg">{initials(me.name)}</Avatar.Fallback>
            </Avatar.Root>
            <div class="grid flex-1 text-start text-sm leading-tight">
              <span class="truncate font-medium">{me.name}</span>
              <span class="text-muted-foreground truncate text-xs capitalize">{me.role}</span>
            </div>
          </div>
        </DropdownMenu.Label>
        <DropdownMenu.Separator />
        <DropdownMenu.Group>
          <DropdownMenu.Item onSelect={() => go("/settings/account")}><UserIcon />Your account</DropdownMenu.Item>
          <DropdownMenu.Item onSelect={() => go("/settings")}><Settings />Settings</DropdownMenu.Item>
          <DropdownMenu.Item onSelect={() => go("/settings/help")}><CircleHelp />Help</DropdownMenu.Item>
        </DropdownMenu.Group>
        <DropdownMenu.Separator />
        <DropdownMenu.RadioGroup value={userPrefersMode.current} onValueChange={(v) => setMode(v)}>
          <DropdownMenu.RadioItem value="light"><Sun />Light</DropdownMenu.RadioItem>
          <DropdownMenu.RadioItem value="dark"><Moon />Dark</DropdownMenu.RadioItem>
          <DropdownMenu.RadioItem value="system"><Monitor />System</DropdownMenu.RadioItem>
        </DropdownMenu.RadioGroup>
        <DropdownMenu.Separator />
        <DropdownMenu.Item onSelect={signOut}><LogOut />Sign out</DropdownMenu.Item>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  </Sidebar.MenuItem>
</Sidebar.Menu>
