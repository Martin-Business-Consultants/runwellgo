<script>
	import { cn } from "$lib/utils.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
</script>

<div bind:this={ref} data-slot="frame-panel-title" class={cn("text-sm font-semibold", className)} {...restProps}>
	{@render children?.()}
</div>
