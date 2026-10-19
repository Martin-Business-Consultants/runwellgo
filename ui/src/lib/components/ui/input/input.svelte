<script module>
	// Messenger wraps the field in a styled span; here the surface sits on
	// the <input> itself, so callers keep bind:value and their classes on it.
	// (An <input> can't carry the ::before highlight, so it keeps the shadow.)
	export const inputClass =
		"h-9 w-full min-w-0 rounded-lg border border-input bg-background not-dark:bg-clip-padding px-[calc(--spacing(3)-1px)] text-base text-foreground shadow-xs/5 ring-ring/24 outline-none [transition:box-shadow_.15s,border-color_.15s,background-color_5000000s_ease-in-out_0s] placeholder:text-muted-foreground/72 autofill:[-webkit-text-fill-color:var(--foreground)] focus-visible:border-ring focus-visible:ring-[3px] aria-invalid:border-destructive/36 focus-visible:aria-invalid:border-destructive/64 focus-visible:aria-invalid:ring-destructive/16 disabled:cursor-not-allowed disabled:opacity-64 [&:is(:disabled,:focus-visible,[aria-invalid=true])]:shadow-none sm:h-8 sm:text-sm dark:bg-input/32 dark:aria-invalid:ring-destructive/24";
</script>

<script>
	import { cn } from "$lib/utils.js";
	let {
		ref = $bindable(null),
		value = $bindable(),
		type,
		files = $bindable(),
		class: className,
		"data-slot": dataSlot = "input",
		...restProps
	} = $props();
</script>

{#if type === "file"}
	<input
		bind:this={ref}
		data-slot={dataSlot}
		class={cn(
			inputClass,
			"leading-8.5 text-muted-foreground sm:leading-7.5 file:me-3 file:h-full file:border-0 file:bg-transparent file:font-medium file:text-foreground file:text-sm",
			className
		)}
		type="file"
		bind:files
		bind:value
		{...restProps}
	/>
{:else}
	<input
		bind:this={ref}
		data-slot={dataSlot}
		class={cn(
			inputClass,
			type === "search" &&
				"[&::-webkit-search-cancel-button]:appearance-none [&::-webkit-search-decoration]:appearance-none [&::-webkit-search-results-button]:appearance-none [&::-webkit-search-results-decoration]:appearance-none",
			className
		)}
		{type}
		bind:value
		{...restProps}
	/>
{/if}
