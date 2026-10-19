<script>
  // One record in the connections map: its kind, its name and its state;
  // the one asked from ringed, the ones it doesn't touch faded.
  import { Handle, Position } from "@xyflow/svelte";
  import { cn } from "$lib/utils.js";
  let { data } = $props();
</script>

<Handle type="target" position={Position.Left} class="!border-0 !bg-transparent" isConnectable={false} />
<div
  class={cn(
    "bg-card text-card-foreground flex w-60 cursor-pointer flex-col gap-0.5 rounded-lg border px-3 py-2 text-start transition-opacity",
    data.focus && "border-primary ring-primary/30 ring-3",
    data.faded && "opacity-35",
  )}
  title={data.label}
>
  <span class="flex items-center gap-1.5 text-[11px] font-medium" style:color={data.color}>
    <span class="size-2 shrink-0 rounded-full" style:background={data.color} aria-hidden="true"></span>{data.kind}
  </span>
  <span class="truncate text-sm font-medium">{data.label}</span>
  {#if data.detail}<span class="text-muted-foreground truncate text-xs">{data.detail}</span>{/if}
</div>
<Handle type="source" position={Position.Right} class="!border-0 !bg-transparent" isConnectable={false} />
