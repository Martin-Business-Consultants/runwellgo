<script>
  // A postbox (the editor's side boxes, WordPress's): a card that folds
  // under its title, remembering whether it was open.
  import * as Card from "$lib/components/ui/card/index.js";
  import Icon from "./Icon.svelte";
  let { key = "", title, flush = false, children, class: klass = "" } = $props();
  let open = $state(true);
  $effect.pre(() => {
    try { if (key && localStorage.getItem("lp-postbox-" + key) === "closed") open = false; } catch {}
  });
  function remember() {
    try { if (key) localStorage.setItem("lp-postbox-" + key, open ? "open" : "closed"); } catch {}
  }
</script>

<Card.Root class="min-w-0 gap-0 py-0 {klass}">
  <details class="group" bind:open ontoggle={remember}>
    <summary class="flex select-none items-center justify-between gap-2 px-5 py-3.5 group-open:border-b">
      <Card.Title>{title}</Card.Title>
      <Icon name="caret-down" class="text-muted-foreground transition-transform group-[:not([open])]:-rotate-90" />
    </summary>
    <div class={flush ? "text-sm" : "flex flex-col gap-5 p-5 text-sm"}>{@render children()}</div>
  </details>
</Card.Root>
