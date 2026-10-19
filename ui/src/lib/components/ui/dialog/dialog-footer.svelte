<script>
	import { Dialog as DialogPrimitive } from "bits-ui";
	import { Button } from "$lib/components/ui/button/index.js";
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		class: className,
		children,
		showCloseButton = false,
		...restProps
	} = $props();
</script>

<div
	bind:this={ref}
	data-slot="dialog-footer"
	class={cn("-mx-6 -mb-6 flex flex-col-reverse gap-2 border-t bg-muted/72 px-6 py-4 sm:flex-row sm:justify-end sm:rounded-b-[calc(var(--radius-2xl)-1px)]", className)}
	{...restProps}
>
	{@render children?.()}
	{#if showCloseButton}
		<DialogPrimitive.Close>
			{#snippet child({ props })}
				<Button variant="outline" {...props}>Close</Button>
			{/snippet}
		</DialogPrimitive.Close>
	{/if}
</div>