<script>
  // A section in one piece (messenger's SectionPanel): a frame whose header
  // (title, a line under it, actions) and footer sit on the tint, and whose
  // body is a white panel.
  //   <Frame title="Contacts" description="Who we talk to">
  //     {#snippet actions()}<Button size="sm">Add</Button>{/snippet}
  //     …
  //   </Frame>
  // `flush` drops the panel's padding (a list or table edge to edge);
  // `bare` leaves the body on the tint with no panel, for content that
  // brings its own (a Table, which draws as cards in a frame, or several
  // Frame.Panel).
  import * as Frame from "$lib/components/ui/frame/index.js";
  import { cn } from "$lib/utils.js";
  let {
    title = "",
    description = "",
    header = undefined,
    actions = undefined,
    footer = undefined,
    flush = false,
    bare = false,
    scroll = false,
    class: klass = "",
    panelClass = "",
    children,
    ...rest
  } = $props();
</script>

<Frame.Root class={cn("min-w-0", klass)} {...rest}>
  {#if title || description || header || actions}
    <Frame.Header class="flex-row flex-wrap items-center justify-between gap-x-4 gap-y-2 py-3 pr-2 pl-4">
      <div class="min-w-0">
        {#if header}{@render header()}{:else}
          {#if title}<Frame.Title>{title}</Frame.Title>{/if}
          {#if description}<Frame.Description>{description}</Frame.Description>{/if}
        {/if}
      </div>
      {#if actions}<div class="ml-auto flex shrink-0 items-center gap-2">{@render actions()}</div>{/if}
    </Frame.Header>
  {/if}
  {#if bare}
    {@render children?.()}
  {:else}
    <Frame.Panel class={cn("min-w-0", flush && "p-0", scroll && "overflow-x-auto", panelClass)}>{@render children?.()}</Frame.Panel>
  {/if}
  {#if footer}
    <Frame.Footer class="text-muted-foreground px-4 py-3 text-sm">{@render footer()}</Frame.Footer>
  {/if}
</Frame.Root>
