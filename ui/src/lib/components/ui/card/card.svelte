<script>
	import { cn } from "$lib/utils.js";
	import { setTableVariant } from "../table/context.js";
	let {
		ref = $bindable(null),
		class: className,
		children,
		size = "default",
		...restProps
	} = $props();
	setTableVariant("card");
	// A frame keeps its own height (h-fit): in a grid beside a taller
	// column it would otherwise stretch, and the tint would show the gap.
</script>

<div
	bind:this={ref}
	data-slot="card"
	data-size={size}
	class={cn(
		"group/card text-card-foreground relative flex flex-col rounded-2xl text-sm",
		"card-solid:bg-card card-solid:gap-(--card-spacing) card-solid:py-(--card-spacing) card-solid:border card-solid:bg-clip-padding card-solid:shadow-xs/5 card-solid:before:pointer-events-none card-solid:before:absolute card-solid:before:inset-0 card-solid:before:rounded-[calc(var(--radius-2xl)-1px)] card-solid:before:shadow-[0_1px_--theme(--color-black/4%)] dark:card-solid:before:shadow-[0_-1px_--theme(--color-white/6%)] [--card-spacing:--spacing(6)] [--card-edge:0px] data-[size=sm]:[--card-spacing:--spacing(4)]",
		"card-framed:bg-muted/72 card-framed:gap-0 card-framed:h-fit card-framed:p-1 card-framed:[--card-spacing:--spacing(4)] card-framed:[--card-edge:--spacing(3)] transform-gpu",
		className
	)}
	{...restProps}
>
	{@render children?.()}
</div>
