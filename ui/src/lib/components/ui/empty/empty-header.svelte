<script>
	import { cn } from "$lib/utils.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
</script>

<div bind:this={ref} data-slot="empty-header" class={cn("flex max-w-sm flex-col items-center text-center", className)} {...restProps}>
	{@render children?.()}
</div>
