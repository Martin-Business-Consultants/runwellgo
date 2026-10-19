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
	data-slot="alert-dialog-footer"
	class={cn(
		"-mx-6 -mb-6 flex flex-col-reverse gap-2 border-t bg-muted/72 px-6 py-4 sm:rounded-b-[calc(var(--radius-2xl)-1px)] group-data-[size=sm]/alert-dialog-content:grid group-data-[size=sm]/alert-dialog-content:grid-cols-2 sm:flex-row sm:justify-end",
		className
	)}
	{...restProps}
>
	{@render children?.()}
</div>