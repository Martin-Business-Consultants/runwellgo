<script>
  // An empty list, as messenger draws it (its EmptyState): an icon on a
  // little stack of cards, a title, what to do about it, and actions, in a
  // dashed box (where records will be). Always an icon: "inbox" when the
  // caller names none. Only as tall as what it says: the page's column is
  // a flex column, and Empty's own flex-1 would stretch it to the bottom.
  //   <EmptyState icon="users" title="No clients yet.">Add one, or …</EmptyState>
  // `icon` is an Icon name or a lucide component. `variant` "card" stands
  // on its own (an index with nothing to list); "inline" sits inside a
  // panel, a tab or a table cell, and is what it becomes there by itself.
  // On its own (not in a card, frame or panel) it sits in a frame, as
  // messenger's sit in their SectionPanel; `section` titles that frame.
  import * as Empty from "$lib/components/ui/empty/index.js";
  import Icon from "./Icon.svelte";
  import * as Frame from "$lib/components/ui/frame/index.js";
  import { onSurface } from "$lib/components/ui/table/context.js";
  import { cn } from "$lib/utils.js";
  let { icon = "inbox", section = "", title = "", description = "", variant = "card", children = undefined, actions = undefined, class: klass = "" } = $props();
  const VARIANTS = {
    card: "bg-card rounded-xl border border-dashed py-12 md:py-12 in-data-[slot=card-content]:bg-transparent in-data-[slot=card-content]:py-8 in-data-[slot=frame-panel]:bg-transparent in-data-[slot=frame-panel]:py-8 in-data-[slot=table-cell]:bg-transparent in-data-[slot=table-cell]:py-8",
    inline: "rounded-xl border border-dashed py-10 md:py-10",
  };
  const surface = onSurface();
  const framed = $derived(variant === "card" && !surface);
</script>

{#snippet empty()}
<Empty.Root class={cn("flex-none gap-5 px-6", VARIANTS[variant], klass)}>
  <Empty.Header class="gap-1.5">
    {#if icon}
      <Empty.Media variant="icon" class="bg-muted text-muted-foreground mb-2 size-10 rounded-xl border">
        {#if typeof icon === "string"}<Icon name={icon} />{:else}{@const C = icon}<C class="size-4.5" />{/if}
      </Empty.Media>
    {/if}
    {#if title}<Empty.Title class="text-sm">{title}</Empty.Title>{/if}
    {#if description || children}
      <Empty.Description class="max-w-sm text-[13px]">{#if children}{@render children()}{:else}{description}{/if}</Empty.Description>
    {/if}
  </Empty.Header>
  {#if actions}<Empty.Content class="flex-row flex-wrap justify-center gap-2">{@render actions()}</Empty.Content>{/if}
</Empty.Root>
{/snippet}

{#if framed}
  <Frame.Root class="min-w-0">
    {#if section}
      <Frame.Header class="py-3 pr-2 pl-4"><Frame.Title>{section}</Frame.Title></Frame.Header>
    {/if}
    {@render empty()}
  </Frame.Root>
{:else}
  {@render empty()}
{/if}
