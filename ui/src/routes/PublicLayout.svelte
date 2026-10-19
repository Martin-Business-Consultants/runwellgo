<script>
  // Signed-out screens (shadcn's login block): the install's name over a
  // card.
  import * as Card from "$lib/components/ui/card/index.js";
  import RunwellMark from "$lib/components/RunwellMark.svelte";
  import { session } from "$lib/session.svelte.js";
  let { title, description = "", audience = "", children, footer = undefined } = $props();
  const audiences = [
    { key: "team", label: "Team", href: "/" },
    { key: "client", label: "Client", href: "/portal/session/new" },
  ];
</script>

<main class="bg-muted flex min-h-svh flex-col items-center justify-center gap-6 p-6 md:p-10">
  <div class="flex w-full max-w-sm flex-col gap-6">
    <a href="/" class="flex items-center gap-2 self-center font-medium">
      <RunwellMark class="size-7" />
      {session.brand}
    </a>
    {#if audience}
      <nav aria-label="Who's signing in" class="bg-background grid grid-cols-2 gap-1 rounded-xl border p-1 text-sm">
        {#each audiences as a (a.key)}
          <a href={a.href} aria-current={a.key === audience ? "page" : undefined}
            class="text-muted-foreground hover:text-foreground aria-[current=page]:bg-muted aria-[current=page]:text-foreground rounded-lg px-3 py-1.5 text-center font-medium transition-colors">{a.label}</a>
        {/each}
      </nav>
    {/if}
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
