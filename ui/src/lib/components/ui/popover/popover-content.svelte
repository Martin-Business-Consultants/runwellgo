<script>
	import { Popover as PopoverPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";
	import PopoverPortal from "./popover-portal.svelte";
	let {
		ref = $bindable(null),
		class: className,
		sideOffset = 4,
		align = "center",
		portalProps,
		...restProps
	} = $props();
</script>

<PopoverPortal {...portalProps}>
	<PopoverPrimitive.Content
		bind:ref
		data-slot="popover-content"
		{sideOffset}
		{align}
		class={cn(
			"relative rounded-lg border bg-popover not-dark:bg-clip-padding text-popover-foreground shadow-lg/5 before:rounded-[calc(var(--radius-lg)-1px)] before:pointer-events-none before:absolute before:inset-0 before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)] has-data-[slot=calendar]:rounded-xl has-data-[slot=calendar]:p-2 has-data-[slot=calendar]:before:rounded-[calc(var(--radius-xl)-1px)] data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-98 data-open:zoom-in-98 flex flex-col gap-4 p-4 text-sm duration-100 z-50 w-72 max-w-(--bits-popover-content-available-width) max-h-(--bits-popover-content-available-height) overflow-y-auto origin-(--bits-popover-content-transform-origin) outline-hidden",
			className
		)}
		{...restProps}
	/>
</PopoverPortal>