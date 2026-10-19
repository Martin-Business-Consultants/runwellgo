<script>
  // The app. Signed out, the public screens (sign in, the first owner's
  // sign-up, emailed links); the client portal and approval pages, which
  // have their own layout; signed in on staff, shadcn's sidebar around the
  // routed screen.
  import { ModeWatcher } from "mode-watcher";
  import * as Sidebar from "$lib/components/ui/sidebar/index.js";
  import { session } from "./lib/session.svelte.js";
  import { router, go } from "./lib/router.svelte.js";
  import { appearance } from "./lib/appearance.svelte.js";
  import { nav } from "./lib/nav.svelte.js";
  import { routes, publicRoutes } from "./routes.js";
  import AppSidebar from "./lib/layout/AppSidebar.svelte";
  import SiteHeader from "./lib/layout/SiteHeader.svelte";
  import GlobalSearch from "./lib/layout/GlobalSearch.svelte";
  import QuickActions from "./lib/layout/QuickActions.svelte";
  import KeyboardHelp from "./lib/layout/KeyboardHelp.svelte";
  import Toasts from "./lib/components/Toasts.svelte";
  import ConfirmHost from "./lib/components/ConfirmHost.svelte";
  import Spinner from "./lib/components/Spinner.svelte";
  import SignIn from "./routes/SignIn.svelte";
  import Signup from "./routes/Signup.svelte";
  import NotFound from "./routes/NotFound.svelte";

  let searching = $state(false);
  // The sidebar remembers being folded in a cookie (shadcn's Sidebar).
  const sidebarOpen = !document.cookie.split("; ").includes("sidebar_state=false");

  session.load();
  appearance.load();
  const open = $derived(router.match(publicRoutes));
  const matched = $derived(router.match(routes));
  $effect(() => { if (session.me) { router.path; nav.refresh(); } });
  // A new screen, or another page of a list, starts at the top.
  $effect(() => { router.path; router.query.get("page"); window.scrollTo(0, 0); });
  // Settings › Appearance: a person's own text size, and custom CSS on
  // client pages (the portal, approvals) only when it's meant for them too.
  $effect(() => appearance.personal(session.me?.text_size));
  $effect(() => appearance.useCustomCSS(!(open && /^\/(portal|approve)/.test(router.path)) || appearance.everywhere));
  // The install requires two-factor sign-in and this person hasn't set it up.
  $effect(() => {
    const me = session.me;
    if (me && me.two_factor_required && !me.two_factor && !me.agent && router.path !== "/settings/two_factor/new") go("/settings/two_factor/new", undefined, { replace: true });
  });
</script>

<ModeWatcher />
{#if open}
  <!-- Approval pages, the portal, emailed links: their own screens, signed in or not. -->
  {#key router.path}<open.component params={open.params} />{/key}
{:else if !session.ready}
  <div class="flex min-h-dvh items-center justify-center"><Spinner /></div>
{:else if !session.me}
  {#if session.needsSetup}<Signup />{:else}<SignIn />{/if}
{:else}
  <a href="#main" data-external class="bg-background fixed start-2 top-2 z-[100] -translate-y-[200%] rounded-md px-3 py-2 text-sm font-medium shadow-md focus:translate-y-0">Skip to main content</a>
  <Sidebar.Provider open={sidebarOpen}>
    <AppSidebar />
    <Sidebar.Inset class="min-w-0">
      <SiteHeader />
      <div id="main" tabindex="-1" class="mx-auto flex w-full max-w-[80rem] flex-1 flex-col gap-4 px-4 py-6 outline-none lg:px-6">
        <!-- A new path makes the screen afresh; a new query on the same path
             doesn't: the screen reads it (onQuery) and nothing flickers. -->
        {#key router.path}
          {#if matched}
            <matched.component params={matched.params} {...matched.props} />
          {:else}
            <NotFound />
          {/if}
        {/key}
      </div>
    </Sidebar.Inset>
  </Sidebar.Provider>
  <GlobalSearch bind:open={searching} />
  <QuickActions />
  <KeyboardHelp />
{/if}
<Toasts />
<ConfirmHost />
