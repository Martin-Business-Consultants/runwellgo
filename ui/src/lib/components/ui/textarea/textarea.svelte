<script>
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		value = $bindable(),
		class: className,
		"data-slot": dataSlot = "textarea",
		...restProps
	} = $props();
</script>

<!-- Messenger's textarea-control surface, on the <textarea> itself. -->
<textarea
	bind:this={ref}
	data-slot={dataSlot}
	class={cn(
		"field-sizing-content flex min-h-18 w-full rounded-lg border border-input bg-background not-dark:bg-clip-padding px-[calc(--spacing(3)-1px)] py-[calc(--spacing(1.5)-1px)] text-base text-foreground shadow-xs/5 ring-ring/24 outline-none transition-shadow placeholder:text-muted-foreground/72 focus-visible:border-ring focus-visible:ring-[3px] aria-invalid:border-destructive/36 focus-visible:aria-invalid:border-destructive/64 focus-visible:aria-invalid:ring-destructive/16 disabled:cursor-not-allowed disabled:opacity-64 [&:is(:disabled,:focus-visible,[aria-invalid=true])]:shadow-none max-sm:min-h-21 sm:text-sm dark:bg-input/32 dark:aria-invalid:ring-destructive/24",
		className
	)}
	bind:value
	{...restProps}></textarea>
