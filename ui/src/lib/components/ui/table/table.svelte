<script>
	import { cn } from "$lib/utils.js";
	import { tableVariant } from "./context.js";
	let {
		ref = $bindable(null),
		class: className,
		variant = tableVariant(),
		children,
		...restProps
	} = $props();
</script>

<div data-slot="table-container" data-variant={variant} class="relative w-full overflow-x-auto">
	<table
		bind:this={ref}
		data-slot="table"
		class={cn("w-full caption-bottom text-sm in-data-[variant=card]:border-separate in-data-[variant=card]:border-spacing-0", className)}
		{...restProps}
	>
		{@render children?.()}
	</table>
</div>
