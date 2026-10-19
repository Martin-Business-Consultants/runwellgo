<script>
	import { Dialog as SheetPrimitive } from "bits-ui";
	import XIcon from '@lucide/svelte/icons/x';
	import { Button } from "$lib/components/ui/button/index.js";
	import { cn } from "$lib/utils.js";
	import SheetOverlay from "./sheet-overlay.svelte";
	import SheetPortal from "./sheet-portal.svelte";
	let {
		ref = $bindable(null),
		class: className,
		side = "right",
		showCloseButton = true,
		portalProps,
		children,
		...restProps
	} = $props();
</script>

<SheetPortal {...portalProps}>
	<SheetOverlay />
	<SheetPrimitive.Content
		bind:ref
		data-slot="sheet-content"
		data-side={side}
		class={cn(
			"bg-popover not-dark:bg-clip-padding text-popover-foreground fixed z-50 flex flex-col gap-4 text-sm shadow-lg/5 transition duration-200 ease-in-out before:pointer-events-none before:absolute before:inset-0 before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)] max-sm:before:hidden data-[side=bottom]:inset-x-0 data-[side=bottom]:bottom-0 data-[side=bottom]:h-auto data-[side=bottom]:max-h-[calc(100dvh-3rem)] data-[side=bottom]:border-t data-[side=left]:inset-y-0 data-[side=left]:left-0 data-[side=left]:h-full data-[side=left]:w-[calc(100%-(--spacing(12)))] data-[side=left]:border-e data-[side=right]:inset-y-0 data-[side=right]:right-0 data-[side=right]:h-full data-[side=right]:w-[calc(100%-(--spacing(12)))] data-[side=right]:border-s data-[side=top]:inset-x-0 data-[side=top]:top-0 data-[side=top]:h-auto data-[side=top]:border-b data-[side=left]:max-w-md data-[side=right]:max-w-md data-open:animate-in data-open:fade-in-0 data-[side=bottom]:data-open:slide-in-from-bottom-8 data-[side=left]:data-open:slide-in-from-left-8 data-[side=right]:data-open:slide-in-from-right-8 data-[side=top]:data-open:slide-in-from-top-8 data-closed:animate-out data-closed:fade-out-0 data-[side=bottom]:data-closed:slide-out-to-bottom-8 data-[side=left]:data-closed:slide-out-to-left-8 data-[side=right]:data-closed:slide-out-to-right-8 data-[side=top]:data-closed:slide-out-to-top-8",
			className
		)}
		{...restProps}
	>
		{@render children?.()}
		{#if showCloseButton}
			<SheetPrimitive.Close data-slot="sheet-close">
				{#snippet child({ props })}
					<Button variant="ghost" class="absolute end-2 top-2" size="icon-sm" {...props}>
						<XIcon  />
						<span class="sr-only">Close</span>
					</Button>
				{/snippet}
			</SheetPrimitive.Close>
		{/if}
	</SheetPrimitive.Content>
</SheetPortal>