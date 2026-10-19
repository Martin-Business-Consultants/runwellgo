<script>
	// Always composited: an animating sibling (a tab indicator) otherwise
	// promotes the translucent background to a GPU layer mid-animation and
	// its shade flickers.
	import { cn } from "$lib/utils.js";
	import { setTableVariant } from "../table/context.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
	setTableVariant("card");
</script>

<div
	bind:this={ref}
	data-slot="frame"
	class={cn("bg-muted/72 relative flex h-fit transform-gpu flex-col rounded-2xl p-1 *:[[data-slot=frame-panel]+[data-slot=frame-panel]]:mt-1", className)}
	{...restProps}
>
	{@render children?.()}
</div>
