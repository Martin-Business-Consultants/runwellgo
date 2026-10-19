<script module>
	import { tv } from "tailwind-variants";
	import { cn } from "$lib/utils.js";
	export const buttonVariants = tv({
		base: "relative inline-flex shrink-0 cursor-pointer items-center justify-center gap-2 whitespace-nowrap rounded-lg border font-medium text-base outline-none transition-shadow select-none before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-lg)-1px)] pointer-coarse:after:absolute pointer-coarse:after:size-full pointer-coarse:after:min-h-11 pointer-coarse:after:min-w-11 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background disabled:pointer-events-none disabled:opacity-64 aria-disabled:pointer-events-none aria-disabled:opacity-64 sm:text-sm [&_svg:not([class*='opacity-'])]:opacity-80 [&_svg:not([class*='size-'])]:size-4.5 sm:[&_svg:not([class*='size-'])]:size-4 [&_svg]:pointer-events-none [&_svg]:-mx-0.5 [&_svg]:shrink-0",
		variants: {
			variant: {
				default: "not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] border-primary bg-primary text-primary-foreground shadow-primary/24 shadow-xs hover:bg-primary/90 aria-expanded:bg-primary/90 [:active,[aria-expanded=true]]:inset-shadow-[0_1px_--theme(--color-black/8%)] [:disabled,:active,[aria-expanded=true]]:shadow-none",
				destructive: "not-disabled:inset-shadow-[0_1px_--theme(--color-white/16%)] border-destructive bg-destructive text-white shadow-destructive/24 shadow-xs hover:bg-destructive/90 aria-expanded:bg-destructive/90 [:active,[aria-expanded=true]]:inset-shadow-[0_1px_--theme(--color-black/8%)] [:disabled,:active,[aria-expanded=true]]:shadow-none",
				"destructive-outline": "border-input bg-popover not-dark:bg-clip-padding text-destructive-foreground shadow-xs/5 not-disabled:not-active:not-aria-expanded:before:shadow-[0_1px_--theme(--color-black/4%)] hover:border-destructive/32 hover:bg-destructive/4 aria-expanded:border-destructive/32 aria-expanded:bg-destructive/4 dark:bg-input/32 dark:not-disabled:before:shadow-[0_-1px_--theme(--color-white/2%)] dark:not-disabled:not-active:not-aria-expanded:before:shadow-[0_-1px_--theme(--color-white/6%)] [:disabled,:active,[aria-expanded=true]]:shadow-none",
				outline: "border-input bg-popover not-dark:bg-clip-padding text-foreground shadow-xs/5 not-disabled:not-active:not-aria-expanded:before:shadow-[0_1px_--theme(--color-black/4%)] hover:bg-accent/50 aria-expanded:bg-accent/50 dark:bg-input/32 dark:aria-expanded:bg-input/64 dark:hover:bg-input/64 dark:not-disabled:before:shadow-[0_-1px_--theme(--color-white/2%)] dark:not-disabled:not-active:not-aria-expanded:before:shadow-[0_-1px_--theme(--color-white/6%)] [:disabled,:active,[aria-expanded=true]]:shadow-none",
				secondary: "border-transparent bg-secondary text-secondary-foreground hover:bg-secondary/90 aria-expanded:bg-secondary/90 [:active,[aria-expanded=true]]:bg-secondary/80",
				ghost: "border-transparent text-foreground hover:bg-accent aria-expanded:bg-accent",
				// Indigo means "act here", so a link reads as one.
				link: "border-transparent text-primary underline-offset-4 hover:underline aria-expanded:underline",
			},
			size: {
				default: "h-9 px-[calc(--spacing(3)-1px)] sm:h-8",
				xs: "h-7 gap-1 rounded-md px-[calc(--spacing(2)-1px)] text-sm before:rounded-[calc(var(--radius-md)-1px)] sm:h-6 sm:text-xs [&_svg:not([class*='size-'])]:size-4 sm:[&_svg:not([class*='size-'])]:size-3.5",
				sm: "h-8 gap-1.5 px-[calc(--spacing(2.5)-1px)] sm:h-7",
				lg: "h-10 px-[calc(--spacing(3.5)-1px)] sm:h-9",
				xl: "h-11 px-[calc(--spacing(4)-1px)] text-lg sm:h-10 sm:text-base [&_svg:not([class*='size-'])]:size-5 sm:[&_svg:not([class*='size-'])]:size-4.5",
				icon: "size-9 sm:size-8",
				"icon-xs": "size-7 rounded-md before:rounded-[calc(var(--radius-md)-1px)] sm:size-6 not-in-data-[slot=input-group]:[&_svg:not([class*='size-'])]:size-4 sm:not-in-data-[slot=input-group]:[&_svg:not([class*='size-'])]:size-3.5",
				"icon-sm": "size-8 sm:size-7",
				"icon-lg": "size-10 sm:size-9",
				"icon-xl": "size-11 sm:size-10 [&_svg:not([class*='size-'])]:size-5 sm:[&_svg:not([class*='size-'])]:size-4.5",
			},
		},
		defaultVariants: {
			variant: "default",
			size: "default",
		},
	});

</script>

<script>
	let {
		class: className,
		variant = "default",
		size = "default",
		ref = $bindable(null),
		href = undefined,
		type = "button",
		disabled,
		children,
		...restProps
	} = $props();
</script>

{#if href}
	<a
		bind:this={ref}
		data-slot="button"
		class={cn(buttonVariants({ variant, size }), className)}
		href={disabled ? undefined : href}
		aria-disabled={disabled}
		role={disabled ? "link" : undefined}
		tabindex={disabled ? -1 : undefined}
		{...restProps}
	>
		{@render children?.()}
	</a>
{:else}
	<button
		bind:this={ref}
		data-slot="button"
		class={cn(buttonVariants({ variant, size }), className)}
		{type}
		{disabled}
		{...restProps}
	>
		{@render children?.()}
	</button>
{/if}