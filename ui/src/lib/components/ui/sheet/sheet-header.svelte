<script>
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		class: className,
		children,
		...restProps
	} = $props();
</script>

<div
	bind:this={ref}
	data-slot="sheet-header"
	class={cn("flex flex-col gap-2 p-4 pe-12", className)}
	{...restProps}
>
	{@render children?.()}
</div>