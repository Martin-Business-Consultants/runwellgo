<script>
  // A client's Websites tab (the Rails client_panel slot): their websites,
  // each with its checklist's count and what's left, and Add a website for
  // those who may.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import { get } from "$lib/api.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { wc, summaryClass } from "./wc.js";

  let { record } = $props();
  let data = $state(null);
  $effect(() => {
    get(wc("/websites", { client_id: record.id, show: "all" })).then((r) => (data = r)).catch(toastError);
  });
</script>

<Card.Root>
  <Card.Content class="flex flex-col gap-3">
    {#if !data}
      <Skeleton class="h-16" />
    {:else if data.websites.length}
      <ul class="flex flex-col gap-3">
        {#each data.websites as s (s.id)}
          <li class="flex flex-wrap items-baseline gap-x-3 gap-y-1">
            <a href={`/websites/${s.id}`} class="font-medium underline-offset-4 hover:underline">{s.name}</a>
            {#if s.host}<span class="text-muted-foreground text-sm">{s.host}</span>{/if}
            <Badge variant="outline" class={summaryClass(s.checklist)}>{s.checklist_text}</Badge>
            {#if s.needs_attention}<span class="text-muted-foreground text-xs">{s.needs_attention}</span>{/if}
          </li>
        {/each}
      </ul>
    {:else}
      <p class="text-muted-foreground text-sm">No websites yet: add one to keep how it’s set up (form emails, webhooks, sitemap, sign-off) in one place.</p>
    {/if}
    {#if data?.can_manage}
      <div><Button href={`/websites/new?client_id=${record.id}`} variant="outline" size="sm"><Plus />Add a website</Button></div>
    {/if}
  </Card.Content>
</Card.Root>
