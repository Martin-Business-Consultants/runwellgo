<script>
  // A settings page: Settings' own nav down the left, in its groups and
  // anchored while the page scrolls (on a narrow screen, across the top),
  // the page beside it. Every settings screen wraps its
  // content in it: <SettingsLayout title="Names" description="…">…</SettingsLayout>.
  import { settingsPages } from "../settings-nav.js";
  import { session } from "../session.svelte.js";
  import { router } from "../router.svelte.js";
  import { page } from "../page.svelte.js";
  import { cn } from "$lib/utils.js";

  let { title, description = "", children, actions = undefined } = $props();
  // svelte-ignore state_referenced_locally
  page.set(title, [{ label: "Settings", href: "/settings" }]);
  const groups = $derived(settingsPages((p) => session.can(p)));
  // A page the person's role can't open (reached by its address) says so
  // in place of a blank form.
  const here = (gs) => gs.some(([, list]) => list.some((p) => p.href === router.path));
  const allowed = $derived(here(groups) || !here(settingsPages(() => true)));
</script>

<div class="flex flex-col gap-6 lg:flex-row lg:gap-10">
  <!-- Anchored: it stays in view (scrolling on its own if it's taller than
       the window) while the page beside it scrolls. -->
  <nav class="flex shrink-0 gap-1 overflow-x-auto lg:sticky lg:top-4 lg:max-h-[calc(100svh-6rem)] lg:w-52 lg:flex-col lg:self-start lg:overflow-y-auto lg:pe-1" aria-label="Settings">
    {#each groups as [group, list] (group)}
      <h2 class="text-muted-foreground hidden px-2 pt-4 pb-1 text-xs font-semibold first:pt-0 lg:block">{group}</h2>
      {#each list as p (p.href)}
        <a href={p.href} aria-current={router.path === p.href ? "page" : undefined}
          class={cn("hover:bg-muted rounded-md px-2 py-1.5 text-sm whitespace-nowrap", router.path === p.href ? "bg-muted font-medium" : "text-muted-foreground")}>{p.label}</a>
      {/each}
    {/each}
  </nav>
  <div class="flex min-w-0 flex-1 flex-col gap-6">
    <div class="flex items-start justify-between gap-4">
      <div>
        <h1 class="text-2xl font-semibold tracking-tight">{title}</h1>
        {#if description}<p class="text-muted-foreground mt-1 text-sm">{description}</p>{/if}
      </div>
      {@render actions?.()}
    </div>
    {#if allowed}
      {@render children()}
    {:else}
      <p class="text-muted-foreground text-sm">Your role can’t open this page. An owner can change roles in Settings › People.</p>
    {/if}
  </div>
</div>
