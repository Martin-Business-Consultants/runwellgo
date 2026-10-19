<script>
  // A plugin's page (/p/<key>/<page>): its nav page, or any of its screens
  // a link leads to, drawn from its description with the address's query.
  import PluginScreen from "./PluginScreen.svelte";
  import { page } from "$lib/page.svelte.js";
  import { router } from "$lib/router.svelte.js";

  let { params } = $props();
  let title = $state("");
  // The query as the page opened; the screen keeps its own after that.
  // svelte-ignore state_referenced_locally
  const query = Object.fromEntries(router.query);

  function loaded(screen, plugin) {
    title = screen.title;
    const up = plugin.nav && plugin.nav.page !== screen.id ? [{ label: plugin.nav.label, href: `/p/${plugin.key}/${plugin.nav.page}` }] : [];
    page.set(screen.title, up);
  }
</script>

<h1 class="text-2xl font-semibold tracking-tight">{title}</h1>
<PluginScreen plugin={params.key} screen={params.page} {query} page onload={loaded} />
