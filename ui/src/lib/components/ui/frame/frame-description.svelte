<script>
	import { cn } from "$lib/utils.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
</script>

<div bind:this={ref} data-slot="frame-panel-description" class={cn("text-muted-foreground text-sm", className)} {...restProps}>
	{@render children?.()}
</div>
