<script>
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		class: className,
		children,
		...restProps
	} = $props();
</script>

<td bind:this={ref} data-slot="table-cell" class={cn("bg-clip-padding p-2.5 in-data-[slot=table-footer]:py-3.5 align-middle whitespace-nowrap in-data-[variant=card]:first:ps-[calc(--spacing(2.5)-1px)] in-data-[variant=card]:last:pe-[calc(--spacing(2.5)-1px)] has-[[role=checkbox]]:w-px last:has-[[role=checkbox]]:ps-0 first:has-[[role=checkbox]]:pe-0", className)} {...restProps}>
	{@render children?.()}
</td>