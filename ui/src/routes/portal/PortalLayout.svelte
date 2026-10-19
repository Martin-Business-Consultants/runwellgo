<script>
  // The portal's frame: a simple top bar (the install's name, the portal's
  // pages, the contact and Sign out) over the page; no staff sidebar. It
  // waits for the contact (signing them in first when there's none).
  import { Button } from "$lib/components/ui/button/index.js";
  import { Separator } from "$lib/components/ui/separator/index.js";
  import RunwellMark from "$lib/components/RunwellMark.svelte";
  import LogOut from "@lucide/svelte/icons/log-out";
  import Spinner from "$lib/components/Spinner.svelte";
  import { session } from "$lib/session.svelte.js";
  import { router, go } from "$lib/router.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { cn } from "$lib/utils.js";
  import { portal, pdel } from "./portal.svelte.js";
  import PluginSlot from "$lib/components/PluginSlot.svelte";

  let { title = "", back = undefined, children } = $props();
  portal.require();

  $effect(() => { document.title = title ? `${title} · ${session.brand}` : session.brand; });

  const links = $derived([
    { href: "/portal", label: portal.term("engagement", 2), active: router.path === "/portal" || router.path.startsWith("/portal/engagements") },
    { href: "/portal/work", label: portal.term("work"), active: router.path === "/portal/work" },
    { href: "/portal/requests", label: "Requests", active: router.path.startsWith("/portal/requests") },
    { href: "/portal/connected_apps", label: "Connected apps", active: router.path === "/portal/connected_apps" },
  ]);

  async function signOut() {
    try { await pdel("/portal/session"); } catch (e) { toastError(e); }
    portal.signedOut();
    go("/portal/session/new");
  }
</script>

<div class="bg-muted/40 flex min-h-dvh flex-col">
  <header class="bg-background border-b print:hidden">
    <div class="mx-auto flex w-full max-w-5xl flex-wrap items-center gap-x-6 gap-y-2 px-4 py-3">
      <a href="/portal" class="flex items-center gap-2 font-medium">
        <RunwellMark class="size-6" />
        {session.brand}
      </a>
      {#if portal.me}
        <nav class="flex flex-wrap items-center gap-1 text-sm" aria-label="Portal">
          {#each links as l (l.href)}
            <a href={l.href} aria-current={l.active ? "page" : undefined}
              class={cn("hover:bg-muted rounded-md px-2 py-1.5", l.active ? "bg-muted font-medium" : "text-muted-foreground")}>{l.label}</a>
          {/each}
          <PluginSlot name="portal_nav" />
        </nav>
        <div class="ms-auto flex items-center gap-3 text-sm">
          <PluginSlot name="portal_header" />
          <span class="text-muted-foreground hidden sm:inline">{portal.me.me.name} · {portal.me.me.company}</span>
          <Separator orientation="vertical" class="hidden h-5 sm:block" />
          <Button variant="ghost" size="sm" onclick={signOut}><LogOut />Sign out</Button>
        </div>
      {/if}
    </div>
  </header>
  <main class="mx-auto flex w-full max-w-5xl flex-1 flex-col gap-4 px-4 py-6">
    {#if !portal.me}
      <div class="flex flex-1 items-center justify-center"><Spinner /></div>
    {:else}
      {#if title}
        <div class="flex flex-col gap-1">
          {#if back}<a href={back.href} class="text-muted-foreground hover:text-foreground text-sm">← {back.label}</a>{/if}
          <h1 class="text-2xl font-semibold tracking-tight">{title}</h1>
        </div>
      {/if}
      {@render children()}
    {/if}
  </main>
</div>
