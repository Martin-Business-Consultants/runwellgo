<script>
  // shadcn's Card as a panel: an optional header (title and actions), the
  // body (flush, or padded), and a muted footer.
  import * as Card from "$lib/components/ui/card/index.js";
  let { title = "", padded = false, scroll = false, header = undefined, actions = undefined, footer = undefined, children, class: klass = "" } = $props();
</script>

<Card.Root class="min-w-0 gap-0 py-0 {scroll ? 'overflow-x-auto' : ''} {klass}">
  {#if title || header || actions}
    <div class="flex flex-wrap items-center justify-between gap-x-4 gap-y-2 border-b px-6 py-4">
      {#if header}{@render header()}{:else}<Card.Title>{title}</Card.Title>{/if}
      {#if actions}<div class="flex items-center gap-2">{@render actions()}</div>{/if}
    </div>
  {/if}
  <div class={padded ? "p-6" : ""}>{@render children?.()}</div>
  {#if footer}
    <Card.Footer class="text-muted-foreground bg-muted/50 border-t px-6 py-3 text-xs">{@render footer()}</Card.Footer>
  {/if}
</Card.Root>
