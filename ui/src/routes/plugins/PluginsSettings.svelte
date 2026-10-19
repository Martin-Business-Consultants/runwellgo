<script>
  // Settings › Plugins: the plugins installed, each switched on or off here
  // (granting what it asks for), opened for its details or settings, or
  // removed; the ones this build has that aren't installed yet; and the
  // recent installs and removals, each a tab (?tab=). Plugins are built
  // into Runwell, so they update with it.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Settings from "@lucide/svelte/icons/settings";
  import Plus from "@lucide/svelte/icons/plus";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import PluginGrants from "./PluginGrants.svelte";
  import { get, post, patch, del } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { timeAgo } from "$lib/format.js";
  import { afterSwitch } from "./switch.js";
  import { pageTabs } from "$lib/page-tabs.svelte.js";

  let data = $state(null), grants = $state({}), installing = $state(""), repo = $state(""), checking = $state(false);
  let removing = $state(null), purge = $state(false), folds = $state({});
  const tabs = pageTabs(() => (data?.changes.length ? ["installed", "add", "changes"] : ["installed", "add"]));

  async function load() {
    try {
      // The grants first: a plugin just installed must have its own before
      // the list that draws it does.
      const d = await get("/settings/plugins");
      grants = Object.fromEntries(d.plugins.map((p) => [p.key, p.enabled ? structuredClone(p.granted) : { ...structuredClone(p.requested), "http:private": false }]));
      data = d;
    } catch (e) { toastError(e); }
  }
  load();

  async function toggle(p, on) {
    try {
      const r = await patch(`/settings/plugins/${p.key}`, { plugin: { enabled: on, granted: on ? grants[p.key] : undefined } });
      toast(r.summary);
      await afterSwitch();
      load();
    } catch (e) { toastError(e); load(); }
  }
  async function saveGrants(p) {
    try { const r = await patch(`/settings/plugins/${p.key}`, { plugin: { granted: grants[p.key] } }); toast(r.summary); await afterSwitch(); load(); } catch (e) { toastError(e); }
  }
  async function install(key) {
    installing = key;
    try { const r = await post("/settings/plugins/installations", { repo: key }); toast(r.summary); repo = ""; await load(); tabs.set("installed"); } catch (e) { toastError(e); }
    installing = "";
  }
  async function check() {
    checking = true;
    try { toast((await post("/settings/plugins/check", {})).summary); } catch (e) { toastError(e); }
    checking = false;
  }
  async function remove() {
    const p = removing;
    removing = null;
    try { const r = await del(`/settings/plugins/${p.key}${purge ? "?purge_data=1" : ""}`); toast(r.summary); await afterSwitch(); load(); } catch (e) { toastError(e); }
    purge = false;
  }
  // What it asks for: each capability, and each host it calls.
  const grantCount = (p) => Object.entries(p.requested).filter(([k, v]) => k !== "http" && v).length + (p.requested.http?.length || 0);
  const changed = (p) => JSON.stringify(grants[p.key]) !== JSON.stringify(p.granted);
</script>

{#snippet actions()}
  <Button variant="outline" size="sm" class="shrink-0" onclick={check} disabled={checking}><RefreshCw />Check for updates</Button>
{/snippet}

<SettingsLayout title="Plugins" description="Plugins add to Runwell without changing it. Each is built into Runwell and updates with it; it starts off, and can do only what it asks for and you grant. Switching one off hides everything it adds and keeps its data." {actions}>
  {#if !data}
    <Skeleton class="h-48" />
  {:else}
    <Tabs.Root value={tabs.value} onValueChange={tabs.set} class="gap-6">
      <Tabs.List aria-label="Plugins">
        <Tabs.Trigger value="installed">Installed <span class="text-muted-foreground text-xs tabular-nums">{data.plugins.length}</span></Tabs.Trigger>
        <Tabs.Trigger value="add">Add a plugin <span class="text-muted-foreground text-xs tabular-nums">{data.available.length}</span></Tabs.Trigger>
        {#if data.changes.length}<Tabs.Trigger value="changes">Recent changes</Tabs.Trigger>{/if}
      </Tabs.List>
      <Tabs.Content value="installed" class="flex flex-col gap-3">
        <h2 class="sr-only">Installed plugins</h2>
        {#if data.plugins.length}
          <div class="grid gap-4 xl:grid-cols-2">
            {#each data.plugins as p (p.key)}
              {@const shown = folds[p.key] ?? (!p.enabled || changed(p))}
              <Card.Root>
                <Card.Header>
                  <Card.Title class="flex items-center gap-2">{p.name}
                    {#if !p.built_in}<Badge variant="destructive">Not in this build</Badge>{:else}<Badge variant={p.enabled ? "default" : "secondary"}>{p.enabled ? "On" : "Off"}</Badge>{/if}
                  </Card.Title>
                  <Card.Description>{[p.version, p.author].filter(Boolean).join(" · ")}</Card.Description>
                  <Card.Action class="flex items-center gap-1">
                    <Button href="/settings/plugins/{p.key}" variant="ghost" size="icon-sm" title={p.name + (p.enabled && p.has_settings ? " settings" : " details")}><Settings /><span class="sr-only">{p.name} settings</span></Button>
                    <Switch checked={p.enabled} disabled={!p.built_in} onCheckedChange={(v) => toggle(p, v)} aria-label="Switch {p.name} on or off" />
                  </Card.Action>
                </Card.Header>
                <Card.Content class="flex flex-col gap-3 text-sm">
                  <p>{p.description}</p>
                  {#if p.homepage}<p class="text-xs"><a href={p.homepage} target="_blank" rel="noopener" class="underline">About this plugin</a></p>{/if}
                  <!-- What it adds and asks for, folded once it's on (its
                       grants are chosen before switching it on, so a plugin
                       that's off, or one with unsaved changes, shows them). -->
                  <Collapsible.Root open={shown} onOpenChange={(o) => (folds[p.key] = o)}>
                    <Collapsible.Trigger class="text-muted-foreground hover:text-foreground flex items-center gap-1 text-xs font-medium">
                      <ChevronDown class={["size-3.5", shown && "rotate-180"]} />What it adds and asks for
                      <span class="tabular-nums">· {grantCount(p)} {grantCount(p) === 1 ? "permission" : "permissions"}</span>
                    </Collapsible.Trigger>
                    <Collapsible.Content class="flex flex-col gap-3 pt-3">
                      {#if p.adds.length}
                        <div class="flex flex-wrap gap-1">{#each p.adds as a (a)}<Badge variant="outline" class="font-normal">{a}</Badge>{/each}</div>
                      {/if}
                      {#if p.built_in}<PluginGrants requested={p.requested} bind:granted={grants[p.key]} id={p.key} />{/if}
                    </Collapsible.Content>
                  </Collapsible.Root>
                </Card.Content>
                <Card.Footer class="justify-between gap-2">
                  <Button variant="ghost" size="sm" class="text-destructive" onclick={() => (removing = p)}>Remove</Button>
                  {#if p.enabled && changed(p)}<Button size="sm" onclick={() => saveGrants(p)}>Save permissions</Button>{/if}
                </Card.Footer>
              </Card.Root>
            {/each}
          </div>
        {:else}
          <p class="text-muted-foreground text-sm">No plugins installed yet. <a href="/settings/plugins?tab=add" class="text-primary underline-offset-4 hover:underline">Add one</a>.</p>
        {/if}
        {#each Object.entries(data.failed_to_load) as [name, error] (name)}
          <p class="text-destructive text-sm"><strong>{name}</strong> is installed but didn’t load: {error}</p>
        {/each}
      </Tabs.Content>

      <Tabs.Content value="add" class="flex flex-col gap-3">
        <h2 class="sr-only">Add a plugin</h2>
        {#if data.available.length}
          <div class="overflow-hidden rounded-lg border">
            <Table.Root>
              <Table.Header>
                <Table.Row>
                  <Table.Head>Plugin</Table.Head>
                  <Table.Head class="hidden md:table-cell">What it does</Table.Head>
                  <Table.Head class="w-20">Version</Table.Head>
                  <Table.Head class="w-28"><span class="sr-only">Install</span></Table.Head>
                </Table.Row>
              </Table.Header>
              <Table.Body>
                {#each data.available as m (m.key)}
                  <Table.Row>
                    <Table.Cell class="align-top font-medium">
                      {m.name}
                      <p class="text-muted-foreground mt-1 text-sm font-normal whitespace-normal md:hidden">{m.description}</p>
                    </Table.Cell>
                    <Table.Cell class="text-muted-foreground hidden align-top text-sm whitespace-normal md:table-cell">{m.description}</Table.Cell>
                    <Table.Cell class="text-muted-foreground align-top text-sm tabular-nums">{m.version}</Table.Cell>
                    <Table.Cell class="text-end align-top">
                      <Button size="sm" variant="outline" disabled={installing === m.key} onclick={() => install(m.key)}><Plus />Install</Button>
                    </Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          </div>
        {:else}
          <p class="text-muted-foreground text-sm">Every plugin in this build is installed.</p>
        {/if}
        <form class="flex flex-wrap gap-2" onsubmit={(e) => { e.preventDefault(); install(repo); }}>
          <Field.Field class="min-w-64 flex-1">
            <Field.Label for="plugin-repo" class="sr-only">Plugin</Field.Label>
            <Input id="plugin-repo" bind:value={repo} required autocomplete="off" placeholder="time_tracking, or owner/runwell-time-tracking" />
            <Field.Description>Not listed? Install one built into Runwell by its key, or by its repository (owner/name).</Field.Description>
          </Field.Field>
          <Button type="submit" variant="outline" disabled={!!installing}>Install</Button>
        </form>
      </Tabs.Content>

      {#if data.changes.length}
        <Tabs.Content value="changes" class="flex flex-col gap-3">
          <h2 class="sr-only">Recent changes</h2>
          <ul class="divide-y rounded-md border text-sm">
            {#each data.changes as c (c.id)}
              <li class="flex flex-wrap items-baseline gap-x-2 px-3 py-2">
                <strong class="font-medium">{c.summary}</strong>
                <span class="text-muted-foreground">{c.to_version || ""}{c.requested_by ? ` · ${c.requested_by.name}` : ""} · {c.status} · {timeAgo(c.created_at)}</span>
              </li>
            {/each}
          </ul>
        </Tabs.Content>
      {/if}
    </Tabs.Root>
  {/if}
</SettingsLayout>

<Dialog.Root open={!!removing} onOpenChange={(v) => { if (!v) removing = null; }}>
  <Dialog.Content>
    <Dialog.Header>
      <Dialog.Title>Remove {removing?.name}?</Dialog.Title>
      <Dialog.Description>It stops at once. Its data stays, so installing it again brings it back.</Dialog.Description>
    </Dialog.Header>
    <Field.Field orientation="horizontal">
      <Checkbox id="plugin-purge" bind:checked={purge} />
      <Field.Label for="plugin-purge">Delete its data too, for good</Field.Label>
    </Field.Field>
    <Dialog.Footer>
      <Button variant="ghost" onclick={() => (removing = null)}>Cancel</Button>
      <Button variant="destructive" onclick={remove}>Remove</Button>
    </Dialog.Footer>
  </Dialog.Content>
</Dialog.Root>
