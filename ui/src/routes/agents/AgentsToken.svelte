<script>
  // One connection: what it is, whose (holder: an owner's list of
  // everyone's), whether it's paused, Pause/Resume and Disconnect, and its
  // recent calls under Activity.
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import ChevronRight from "@lucide/svelte/icons/chevron-right";
  import { timeAgo } from "$lib/format.js";

  let { token, holder = false, onpause = undefined, ondisconnect } = $props();
  const calls = $derived(token.calls || []);
</script>

<li class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
  <div class="flex flex-wrap items-center gap-3">
    <div class="min-w-0 flex-1">
      <div class="flex items-center gap-2">
        <span class="truncate text-sm font-medium">{token.name}</span>
        {#if token.paused}<Badge variant="secondary">Paused</Badge>{/if}
      </div>
      <p class="text-muted-foreground text-xs">
        {holder ? `${token.holder} · ` : ""}{token.kind_label}{token.read_only ? " · read only" : ""}{token.created_at ? ` · made ${timeAgo(token.created_at)}` : ""} · {token.last_used_at ? `last used ${timeAgo(token.last_used_at)}` : "never used"}
      </p>
    </div>
    <div class="flex items-center gap-2">
      {#if onpause}<Button variant="outline" size="sm" onclick={onpause}>{token.paused ? "Resume" : "Pause"}</Button>{/if}
      <Button variant="ghost" size="sm" class="text-destructive" onclick={ondisconnect}>Disconnect</Button>
    </div>
  </div>
  {#if calls.length}
    <Collapsible.Root>
      <Collapsible.Trigger class="text-muted-foreground hover:text-foreground group flex items-center gap-1 text-xs">
        <ChevronRight class="size-3 transition-transform group-data-[state=open]:rotate-90" />Activity: the last {calls.length === 1 ? "call" : `${calls.length} calls`}
      </Collapsible.Trigger>
      <Collapsible.Content>
        <ul class="mt-2 flex flex-col gap-1 text-xs">
          {#each calls as call (call.id)}
            <li class="flex items-center gap-2">
              <span class="text-muted-foreground shrink-0 whitespace-nowrap">{timeAgo(call.created_at)}</span>
              <code class="bg-muted shrink-0 rounded px-1 font-mono">{call.tool}</code>
              {#if call.status === "error"}
                <span class="text-destructive shrink-0">{call.code || "error"}</span>
              {:else if call.status === "needs_confirmation"}
                <span class="text-muted-foreground shrink-0">asked first</span>
              {/if}
              <span class="text-muted-foreground min-w-0 flex-1 truncate">{call.summary || ""}</span>
              {#if call.duration_ms != null}<span class="text-muted-foreground shrink-0 tabular-nums">{call.duration_ms} ms</span>{/if}
            </li>
          {/each}
        </ul>
      </Collapsible.Content>
    </Collapsible.Root>
  {/if}
</li>
