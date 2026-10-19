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
	data-slot="sheet-footer"
	class={cn("mt-auto flex flex-col-reverse gap-2 border-t bg-muted/72 p-4 sm:flex-row sm:justify-end", className)}
	{...restProps}
>
	{@render children?.()}
</div>