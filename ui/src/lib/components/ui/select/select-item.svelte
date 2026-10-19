<script>
	import { Select as SelectPrimitive } from "bits-ui";
	import CheckIcon from '@lucide/svelte/icons/check';
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		value,
		label,
		children: childrenProp,
		...restProps
	} = $props();
</script>

<SelectPrimitive.Item
	bind:ref
	{value}
	{label}
	data-slot="select-item"
	class={cn(
		"relative flex min-h-8 w-full cursor-default select-none items-center gap-2 rounded-sm py-1 ps-2 pe-8 text-base outline-none data-highlighted:bg-accent data-highlighted:text-accent-foreground data-disabled:pointer-events-none data-disabled:opacity-64 sm:min-h-7 sm:text-sm [&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0 *:[span]:last:flex *:[span]:last:items-center *:[span]:last:gap-2",
		className
	)}
	{...restProps}
>
	{#snippet children({ selected, highlighted })}
		<span class="pointer-events-none absolute end-2 flex size-4.5 items-center justify-center sm:size-4">
			{#if selected}
				<CheckIcon class="pointer-events-none" />
			{/if}
		</span>
		<span class="wrap-anywhere flex min-w-0 flex-1 gap-2">
			{#if childrenProp}
				{@render childrenProp({ selected, highlighted })}
			{:else}
				{label || value}
			{/if}
		</span>
	{/snippet}
</SelectPrimitive.Item>