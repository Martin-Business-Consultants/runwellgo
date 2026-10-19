<script>
  // A plugin's page under Settings (/settings/plugins/<key>): what it is,
  // whether it's on and what it adds, and, while it's on, its settings
  // screen when it has one (the Rails gear: its settings page, else its
  // details).
  import * as Card from "$lib/components/ui/card/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import PluginScreen, { pluginScreens } from "./PluginScreen.svelte";
  import { get } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";

  let { params } = $props();
  let p = $state(null), settingsScreen = $state("");

  // A screen is made afresh for each address, so its params never change.
  // svelte-ignore state_referenced_locally
  const key = params.key;
  get(`/settings/plugins/${key}`).then(async (r) => {
    p = r;
    page.set(r.name, [{ label: "Settings", href: "/settings" }, { label: "Plugins", href: "/settings/plugins" }]);
    if (r.enabled && r.has_settings) settingsScreen = (await pluginScreens(r.key, get)).settings?.screen || "";
  }).catch(toastError);
</script>

<SettingsLayout title={p?.name || "Plugin"} description={p?.description || ""}>
  {#if !p}
    <Skeleton class="h-32" />
  {:else}
    {#if settingsScreen}
      <Card.Root><Card.Content><PluginScreen plugin={p.key} screen={settingsScreen} /></Card.Content></Card.Root>
    {/if}
    <Card.Root>
      <Card.Header>
        <Card.Title class="flex items-center gap-2">Details <Badge variant={p.enabled ? "default" : "secondary"}>{p.enabled ? "On" : "Off"}</Badge></Card.Title>
        <Card.Description>{[p.version, p.author].filter(Boolean).join(" · ")}</Card.Description>
      </Card.Header>
      <Card.Content>
        <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
          <dt class="text-muted-foreground">Adds</dt><dd class="flex flex-wrap gap-1">{#each p.adds as a (a)}<Badge variant="outline" class="font-normal">{a}</Badge>{:else}—{/each}</dd>
          <dt class="text-muted-foreground">Hears</dt><dd>{p.hooks.join(", ") || "—"}</dd>
          <dt class="text-muted-foreground">Agent tools</dt><dd>{p.tools.join(", ") || "—"}</dd>
          {#if p.homepage}<dt class="text-muted-foreground">Homepage</dt><dd><a href={p.homepage} target="_blank" rel="noopener" class="underline">{p.homepage}</a></dd>{/if}
          {#if p.page_path && p.enabled}<dt class="text-muted-foreground">Page</dt><dd><a href={p.page_path} class="underline">{p.page_path}</a></dd>{/if}
        </dl>
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>
