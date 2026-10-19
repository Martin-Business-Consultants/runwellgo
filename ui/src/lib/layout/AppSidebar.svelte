<script>
  // The sidebar: shadcn's Sidebar as it comes. The install's name at the
  // top, the core's sections with what belongs to each nested under it
  // (lib/nav.svelte.js), plugins' pages under Plugins, and the person
  // at the foot (their account, settings, help, the theme, sign out). It
  // folds to icons (⌘B, or the header's toggle); on a narrow screen it's a
  // sheet.
  import * as Sidebar from "$lib/components/ui/sidebar/index.js";
  import { useSidebar } from "$lib/components/ui/sidebar/context.svelte.js";
  import NavUser from "./NavUser.svelte";
  import { coreNav, nav } from "../nav.svelte.js";
  import { session } from "../session.svelte.js";
  import { router } from "../router.svelte.js";
  import { appearance } from "../appearance.svelte.js";
  import RunwellMark from "../components/RunwellMark.svelte";

  const sidebar = useSidebar();
  const items = $derived(coreNav());
  const current = (i) => (i.exact ? router.path === i.href : router.path === i.href || router.path.startsWith(i.href + "/"));
  const close = () => sidebar.setOpenMobile(false);
</script>

<Sidebar.Root collapsible="icon" variant="inset" role="navigation" aria-label="Runwell">
  <Sidebar.Header>
    <Sidebar.Menu>
      <Sidebar.MenuItem>
        <Sidebar.MenuButton size="lg">
          {#snippet child({ props })}
            <a href="/" onclick={close} {...props}>
              {#if appearance.logoUrl}
                <!-- The install's logo says who it is: no name beside it. -->
                <img src={appearance.logoUrl} alt={session.brand} class="h-8 max-w-full min-w-8 object-contain object-left" />
              {:else}
                <RunwellMark class="size-8" />
                <div class="grid flex-1 text-start text-sm leading-tight">
                  <span class="truncate font-medium">{session.brand}</span>
                  {#if session.brand !== "Runwell"}<span class="text-muted-foreground truncate text-xs">Runwell</span>{/if}
                </div>
              {/if}
            </a>
          {/snippet}
        </Sidebar.MenuButton>
      </Sidebar.MenuItem>
    </Sidebar.Menu>
  </Sidebar.Header>
  <Sidebar.Content>
    <Sidebar.Group>
      <Sidebar.Menu>
        {#each items as i (i.href)}
          <Sidebar.MenuItem>
            <Sidebar.MenuButton isActive={current(i)} tooltipContent={i.label}>
              {#snippet child({ props })}
                <a href={i.href} onclick={close} {...props}><i.icon /><span>{i.label}</span></a>
              {/snippet}
            </Sidebar.MenuButton>
            {#if i.badge && nav.counts[i.badge]}
              <Sidebar.MenuBadge>{nav.counts[i.badge]}</Sidebar.MenuBadge>
            {/if}
            {#if i.children?.length}
              <!-- What belongs to it, under it (hidden when the sidebar is folded to icons). -->
              <Sidebar.MenuSub>
                {#each i.children as c (c.href)}
                  <Sidebar.MenuSubItem>
                    <Sidebar.MenuSubButton isActive={current(c)}>
                      {#snippet child({ props })}
                        <a href={c.href} onclick={close} {...props}>
                          <span>{c.label}</span>
                          {#if c.badge && nav.counts[c.badge]}<span class="text-muted-foreground ms-auto text-xs tabular-nums">{nav.counts[c.badge]}</span>{/if}
                        </a>
                      {/snippet}
                    </Sidebar.MenuSubButton>
                  </Sidebar.MenuSubItem>
                {/each}
              </Sidebar.MenuSub>
            {/if}
          </Sidebar.MenuItem>
        {/each}
      </Sidebar.Menu>
    </Sidebar.Group>
    {#if nav.plugins.length}
      <Sidebar.Group>
        <Sidebar.GroupLabel>Plugins</Sidebar.GroupLabel>
        <Sidebar.Menu>
          {#each nav.plugins as p (p.href)}
            <Sidebar.MenuItem>
              <Sidebar.MenuButton isActive={router.path === p.href || router.path.startsWith(p.href + "/")} tooltipContent={p.label}>
                {#snippet child({ props })}
                  <a href={p.href} onclick={close} {...props}><p.icon /><span>{p.label}</span>{#if p.badge}<span class="sr-only">, {p.badge} unread</span>{/if}</a>
                {/snippet}
              </Sidebar.MenuButton>
              {#if p.badge}<Sidebar.MenuBadge>{p.badge > 99 ? "99+" : p.badge}</Sidebar.MenuBadge>{/if}
            </Sidebar.MenuItem>
          {/each}
        </Sidebar.Menu>
      </Sidebar.Group>
    {/if}
  </Sidebar.Content>
  <Sidebar.Footer>
    <NavUser />
  </Sidebar.Footer>
  <Sidebar.Rail />
</Sidebar.Root>
