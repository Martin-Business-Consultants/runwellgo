<script>
	import { Select as SelectPrimitive } from "bits-ui";
	import ChevronsUpDownIcon from '@lucide/svelte/icons/chevrons-up-down';
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		children,
		size = "default",
		...restProps
	} = $props();
</script>

<SelectPrimitive.Trigger
	bind:ref
	data-slot="select-trigger"
	data-size={size}
	class={cn(
		"relative inline-flex w-fit min-w-0 select-none items-center justify-between gap-2 whitespace-nowrap rounded-lg border border-input bg-background not-dark:bg-clip-padding text-left text-base text-foreground shadow-xs/5 outline-none ring-ring/24 transition-shadow before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-lg)-1px)] not-data-[disabled]:not-focus-visible:not-aria-invalid:not-data-[state=open]:before:shadow-[0_1px_--theme(--color-black/4%)] pointer-coarse:after:absolute pointer-coarse:after:size-full pointer-coarse:after:min-h-11 focus-visible:border-ring focus-visible:ring-[3px] aria-invalid:border-destructive/36 focus-visible:aria-invalid:border-destructive/64 focus-visible:aria-invalid:ring-destructive/16 data-placeholder:text-muted-foreground/72 data-disabled:pointer-events-none data-disabled:opacity-64 disabled:pointer-events-none disabled:opacity-64 sm:text-sm dark:bg-input/32 dark:aria-invalid:ring-destructive/24 dark:not-data-[disabled]:not-focus-visible:not-aria-invalid:not-data-[state=open]:before:shadow-[0_-1px_--theme(--color-white/6%)] [&_svg:not([class*='opacity-'])]:opacity-80 [&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:shrink-0 [[data-disabled],:focus-visible,[aria-invalid=true],[data-state=open]]:shadow-none *:data-[slot=select-value]:flex *:data-[slot=select-value]:items-center *:data-[slot=select-value]:gap-1.5 *:data-[slot=select-value]:line-clamp-1",
		size === "sm"
			? "min-h-8 gap-1.5 px-[calc(--spacing(2.5)-1px)] sm:min-h-7"
			: size === "lg"
				? "min-h-10 px-[calc(--spacing(3)-1px)] sm:min-h-9"
				: "min-h-9 px-[calc(--spacing(3)-1px)] sm:min-h-8",
		className
	)}
	{...restProps}
>
	{@render children?.()}
	<ChevronsUpDownIcon class="pointer-events-none -me-1 size-4.5 opacity-80 sm:size-4" />
</SelectPrimitive.Trigger>