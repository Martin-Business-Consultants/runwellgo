<script>
  import Icon from "./Icon.svelte";
  let { icon = null, title = "", children = undefined, actions = undefined, class: klass = "" } = $props();
</script>

<div class="text-muted-foreground flex flex-col items-center gap-2 px-4 py-12 text-center {klass}">
  {#if icon}<span class="bg-muted mb-1 flex size-10 items-center justify-center rounded-full"><Icon name={icon} size="md" /></span>{/if}
  {#if title}<p class="text-foreground text-sm font-medium">{title}</p>{/if}
  {#if children}<div class="max-w-md text-sm">{@render children()}</div>{/if}
  {#if actions}<div class="mt-2 flex items-center gap-2">{@render actions()}</div>{/if}
</div>
