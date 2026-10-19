<script>
  // One row of a menu: a link or a button, in shadcn's menu-item look.
  import { cn } from "$lib/utils.js";
  import Icon from "./Icon.svelte";
  let { href = null, icon = null, danger = false, selected = false, onclick = undefined, children, trailing = undefined } = $props();
  const cls = $derived(cn(
    "hover:bg-accent hover:text-accent-foreground flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm outline-none focus-visible:bg-accent [&_svg]:text-muted-foreground",
    danger && "text-destructive hover:bg-destructive/10 hover:text-destructive",
    selected && "bg-accent font-medium",
  ));
</script>

{#if href}
  <a {href} class={cls} aria-current={selected ? "true" : undefined}>
    {#if icon}<Icon name={icon} />{/if}<span class="min-w-0 flex-1 truncate">{@render children()}</span>{@render trailing?.()}
  </a>
{:else}
  <button type="button" class={cls} data-close {onclick}>
    {#if icon}<Icon name={icon} />{/if}<span class="min-w-0 flex-1 truncate">{@render children()}</span>{@render trailing?.()}
  </button>
{/if}
