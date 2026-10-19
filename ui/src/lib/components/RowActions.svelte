<script>
  // WordPress's row actions: quiet links under a row's title that show when
  // the row is hovered or focused (always, on a touch screen).
  let { children } = $props();
</script>

<div class="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs leading-4 [@media(any-hover:hover)]:invisible group-hover:visible group-focus-within:visible">
  {@render children()}
</div>
