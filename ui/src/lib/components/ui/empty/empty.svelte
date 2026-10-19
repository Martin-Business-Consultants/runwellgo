<script>
	import { cn } from "$lib/utils.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
</script>

<div bind:this={ref} data-slot="empty" class={cn("flex min-w-0 flex-1 flex-col items-center justify-center gap-6 text-balance px-6 py-12 text-center md:py-20", className)} {...restProps}>
	{@render children?.()}
</div>
