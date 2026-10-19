<script>
	import { cn } from "$lib/utils.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
</script>

<header bind:this={ref} data-slot="frame-panel-header" class={cn("flex flex-col px-5 py-4", className)} {...restProps}>
	{@render children?.()}
</header>
