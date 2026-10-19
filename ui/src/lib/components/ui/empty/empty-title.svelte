<script>
	import { cn } from "$lib/utils.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
</script>

<div bind:this={ref} data-slot="empty-title" class={cn("text-xl font-semibold", className)} {...restProps}>
	{@render children?.()}
</div>
