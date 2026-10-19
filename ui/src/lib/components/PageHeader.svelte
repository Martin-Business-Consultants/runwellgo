<script>
  // An admin page's title row: the title (and an icon), its actions at the
  // right, and the CLI command that does the same, to copy.
  import Icon from "./Icon.svelte";
  import CopyButton from "./CopyButton.svelte";
  let { title, icon = null, subtitle = "", cli = "", actions = undefined, children = undefined } = $props();
</script>

<div class="flex flex-col gap-2">
  <div class="flex flex-wrap items-center justify-between gap-x-3 gap-y-2">
    <div class="min-w-0">
      <h1 class="flex items-center gap-2 text-2xl font-semibold tracking-tight">
        {#if icon}<Icon name={icon} size="lg" class="text-muted-foreground" />{/if}
        <span class="truncate">{title}</span>
      </h1>
      {#if subtitle}<p class="text-muted-foreground mt-0.5 text-sm">{subtitle}</p>{/if}
    </div>
    {#if actions}<div class="ml-auto flex flex-wrap items-center justify-end gap-2">{@render actions()}</div>{/if}
  </div>
  {#if cli}
    <p class="text-muted-foreground flex flex-wrap items-center gap-2 text-xs">
      <span>From the CLI or MCP:</span>
      <code class="bg-muted text-foreground rounded-sm px-1 py-0.5 font-mono">{cli}</code>
      <CopyButton text={cli} />
    </p>
  {/if}
  {@render children?.()}
</div>
