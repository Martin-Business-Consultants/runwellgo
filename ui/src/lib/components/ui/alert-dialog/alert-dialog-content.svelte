<script>
	import { AlertDialog as AlertDialogPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";
	import AlertDialogOverlay from "./alert-dialog-overlay.svelte";
	import AlertDialogPortal from "./alert-dialog-portal.svelte";
	let {
		ref = $bindable(null),
		class: className,
		size = "default",
		portalProps,
		...restProps
	} = $props();
</script>

<AlertDialogPortal {...portalProps}>
	<AlertDialogOverlay />
	<AlertDialogPrimitive.Content
		bind:ref
		data-slot="alert-dialog-content"
		data-size={size}
		class={cn(
			"bg-popover not-dark:bg-clip-padding text-popover-foreground rounded-2xl border shadow-lg/5 before:rounded-[calc(var(--radius-2xl)-1px)] before:pointer-events-none before:absolute before:inset-0 before:shadow-[0_1px_--theme(--color-black/4%)] dark:before:shadow-[0_-1px_--theme(--color-white/6%)] data-open:animate-in data-closed:animate-out data-closed:fade-out-0 data-open:fade-in-0 sm:data-closed:zoom-out-98 sm:data-open:zoom-in-98 gap-6 p-6 text-sm duration-200 ease-in-out max-w-[calc(100%_-_2rem)] data-[size=sm]:sm:max-w-xs data-[size=default]:sm:max-w-lg group/alert-dialog-content fixed top-1/2 left-1/2 z-50 grid w-full -translate-x-1/2 -translate-y-1/2 outline-none max-sm:inset-x-0 max-sm:top-auto max-sm:bottom-0 max-sm:left-0 max-sm:max-w-none max-sm:max-h-[calc(100dvh-3rem)] max-sm:translate-x-0 max-sm:translate-y-0 max-sm:overflow-y-auto max-sm:rounded-none max-sm:border-x-0 max-sm:border-b-0 max-sm:before:hidden max-sm:data-open:slide-in-from-bottom-4 max-sm:data-closed:slide-out-to-bottom-4",
			className
		)}
		{...restProps}
	/>
</AlertDialogPortal>