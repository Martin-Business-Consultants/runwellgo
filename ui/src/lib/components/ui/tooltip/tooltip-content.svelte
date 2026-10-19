<script>
	import { Tooltip as TooltipPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";
	import TooltipPortal from "./tooltip-portal.svelte";
	let {
		ref = $bindable(null),
		class: className,
		sideOffset = 4,
		side = "top",
		children,
		// Messenger's tooltips have no arrow; still taken so callers that pass it don't break.
		arrowClasses: _arrowClasses,
		portalProps,
		...restProps
	} = $props();
</script>

<TooltipPortal {...portalProps}>
	<TooltipPrimitive.Content
		bind:ref
		data-slot="tooltip-content"
		{sideOffset}
		{side}
		class={cn(
			"data-open:animate-in data-open:fade-in-0 data-open:zoom-in-98 data-[state=delayed-open]:animate-in data-[state=delayed-open]:fade-in-0 data-[state=delayed-open]:zoom-in-98 data-closed:animate-out data-closed:fade-out-0 data-closed:zoom-out-98 relative inline-flex items-center gap-1.5 text-balance rounded-md border bg-popover not-dark:bg-clip-padding text-popover-foreground px-2 py-1 text-xs shadow-md/5 before:rounded-[calc(var(--radius-md)-1px)] before:pointer-events-none before:absolute before:inset-0 before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)] has-data-[slot=kbd]:pr-1.5 **:data-[slot=kbd]:relative **:data-[slot=kbd]:isolate **:data-[slot=kbd]:z-50 **:data-[slot=kbd]:rounded-sm z-50 w-fit max-w-xs origin-(--bits-tooltip-content-transform-origin)",
			className
		)}
		{...restProps}
	>
		{@render children?.()}
	</TooltipPrimitive.Content>
</TooltipPortal>