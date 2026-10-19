<script>
  // The plugins' quick actions (Log time…): a menu of them for the record
  // on screen (record: "Client:12", or none), each opening its form in a
  // dialog. The header (or a record page) renders
  // <PluginQuickActions record={…} />; it draws nothing when there are none.
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Zap from "@lucide/svelte/icons/zap";
  import PluginScreen from "./PluginScreen.svelte";
  import { extensions } from "./extensions.svelte.js";
  import { pluginIcon } from "./icons.js";

  let { record = "" } = $props();
  let open = $state(null);
  const type = $derived(record.split(":")[0]);
  // Those for this kind of record, or for none.
  const actions = $derived(extensions.quickActions.filter((a) => !record || !a.types.length || a.types.includes(type)));
</script>

{#if actions.length}
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="outline" size="sm"><Zap />Quick actions</Button>{/snippet}</DropdownMenu.Trigger>
    <DropdownMenu.Content align="end">
      {#each actions as a (a.key + a.screen)}
        {@const Icon = pluginIcon(a.icon)}
        <DropdownMenu.Item onSelect={() => (open = a)}><Icon />{a.label}</DropdownMenu.Item>
      {/each}
    </DropdownMenu.Content>
  </DropdownMenu.Root>
  <Dialog.Root open={!!open} onOpenChange={(v) => { if (!v) open = null; }}>
    <Dialog.Content>
      {#if open}
        <Dialog.Header><Dialog.Title>{open.label}</Dialog.Title></Dialog.Header>
        <PluginScreen plugin={open.key} screen={open.screen} query={record && open.types.includes(type) ? { record } : {}} onsaved={() => (open = null)} />
      {/if}
    </Dialog.Content>
  </Dialog.Root>
{/if}
