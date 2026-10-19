<script>
  let { href = null, danger = false, onclick = undefined, children } = $props();
  const cls = $derived(`${danger ? "text-destructive" : "text-muted-foreground hover:text-foreground"} underline-offset-4 hover:underline`);
</script>

{#if href}<a {href} class={cls}>{@render children()}</a>{:else}<button type="button" class={cls} {onclick}>{@render children()}</button>{/if}
