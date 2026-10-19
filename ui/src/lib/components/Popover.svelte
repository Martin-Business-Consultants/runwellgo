<script>
  // shadcn's Popover: something that opens under its trigger (a menu, a
  // filter). It closes on Escape, a click outside, and when a link in it is
  // followed. The trigger snippet gets { props } to spread on its button.
  import * as P from "$lib/components/ui/popover/index.js";
  let { open = $bindable(false), align = "start", width = "w-56", trigger, children, class: klass = "" } = $props();
</script>

<P.Root bind:open>
  <P.Trigger class={klass}>
    {#snippet child({ props })}{@render trigger({ props, open, toggle: () => (open = !open) })}{/snippet}
  </P.Trigger>
  <P.Content {align} class="max-h-[70dvh] overflow-y-auto p-1 {width}" onclick={(e) => { if (e.target.closest("a,[data-close]")) open = false; }}>
    {@render children()}
  </P.Content>
</P.Root>
