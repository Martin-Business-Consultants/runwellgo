<script>
  // Signed-out screens (shadcn's login block): the install's name over a
  // card.
  import * as Card from "$lib/components/ui/card/index.js";
  import RunwellMark from "$lib/components/RunwellMark.svelte";
  import { session } from "$lib/session.svelte.js";
  let { title, description = "", children, footer = undefined } = $props();
</script>

<main class="bg-muted flex min-h-svh flex-col items-center justify-center gap-6 p-6 md:p-10">
  <div class="flex w-full max-w-sm flex-col gap-6">
    <a href="/" class="flex items-center gap-2 self-center font-medium">
      <RunwellMark class="size-7" />
      {session.brand}
    </a>
    <Card.Root>
      <Card.Header class="text-center">
        <Card.Title class="text-xl"><h1>{title}</h1></Card.Title>
        {#if description}<Card.Description>{description}</Card.Description>{/if}
      </Card.Header>
      <Card.Content>{@render children()}</Card.Content>
    </Card.Root>
    {#if footer}<div class="text-muted-foreground text-center text-xs text-balance">{@render footer()}</div>{/if}
  </div>
</main>
