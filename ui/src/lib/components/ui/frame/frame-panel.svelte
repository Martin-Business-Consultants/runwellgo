<script>
	import { cn } from "$lib/utils.js";
	import { setTableVariant } from "../table/context.js";
	let { ref = $bindable(null), class: className, children, ...restProps } = $props();
	setTableVariant("default");
</script>

<div
	bind:this={ref}
	data-slot="frame-panel"
	class={cn("bg-card relative rounded-xl border bg-clip-padding p-5 shadow-xs/5 before:pointer-events-none before:absolute before:inset-0 before:rounded-[calc(var(--radius-xl)-1px)] before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)]", className)}
	{...restProps}
>
	{@render children?.()}
</div>
