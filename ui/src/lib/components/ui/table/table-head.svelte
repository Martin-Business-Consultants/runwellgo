<script>
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		class: className,
		children,
		...restProps
	} = $props();
</script>

<th bind:this={ref} data-slot="table-head" class={cn("text-muted-foreground h-10 px-2.5 text-left align-middle font-medium leading-none whitespace-nowrap has-[[role=checkbox]]:w-px last:has-[[role=checkbox]]:ps-0 first:has-[[role=checkbox]]:pe-0", className)} {...restProps}>
	{@render children?.()}
</th>