<script>
	import { LinkPreview as HoverCardPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";
	import HoverCardPortal from "./hover-card-portal.svelte";
	let {
		ref = $bindable(null),
		class: className,
		side = "bottom",
		sideOffset = 4,
		align = "center",
		portalProps,
		...restProps
	} = $props();
</script>

<HoverCardPortal {...portalProps}>
	<HoverCardPrimitive.Content
		bind:ref
		data-slot="hover-card-content"
		{side}
		{sideOffset}
		{align}
		class={cn(
			"relative rounded-lg border bg-popover not-dark:bg-clip-padding text-popover-foreground shadow-lg/5 before:rounded-[calc(var(--radius-lg)-1px)] before:pointer-events-none before:absolute before:inset-0 before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)] data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 data-closed:zoom-out-98 data-open:zoom-in-98 w-64 text-balance p-4 text-sm duration-100 z-50 origin-(--bits-link-preview-content-transform-origin) outline-hidden",
			className
		)}
		{...restProps}
	/>
</HoverCardPortal>