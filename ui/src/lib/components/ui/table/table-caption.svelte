<script>
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		class: className,
		children,
		...restProps
	} = $props();
</script>

<caption
	bind:this={ref}
	data-slot="table-caption"
	class={cn("text-muted-foreground in-data-[variant=card]:my-4 mt-4 text-sm", className)}
	{...restProps}
>
	{@render children?.()}
</caption>