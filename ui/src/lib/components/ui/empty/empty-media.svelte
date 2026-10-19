<script>
	// The icon on a little stack of cards: two tilted copies behind it, as
	// messenger draws it.
	import { cn } from "$lib/utils.js";
	let { ref = $bindable(null), class: className, variant = "default", children, ...restProps } = $props();
	const base = "flex shrink-0 items-center justify-center [&_svg]:pointer-events-none [&_svg]:shrink-0";
	const icon = "bg-card text-foreground relative flex size-9 shrink-0 items-center justify-center rounded-md border shadow-sm/5 not-dark:bg-clip-padding before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-md)-1px)] before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)] [&_svg:not([class*='size-'])]:size-4.5";
	const look = $derived(cn(base, variant === "icon" && icon, className));
</script>

<div bind:this={ref} class="relative mb-6" data-slot="empty-media" data-variant={variant} {...restProps}>
	{#if variant === "icon"}
		<div aria-hidden="true" class={cn(look, "pointer-events-none absolute bottom-px origin-bottom-left -translate-x-0.5 -rotate-10 scale-84 shadow-none")}></div>
		<div aria-hidden="true" class={cn(look, "pointer-events-none absolute bottom-px origin-bottom-right translate-x-0.5 rotate-10 scale-84 shadow-none")}></div>
	{/if}
	<div class={look}>{@render children?.()}</div>
</div>
