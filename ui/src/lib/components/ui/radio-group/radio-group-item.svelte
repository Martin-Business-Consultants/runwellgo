<script>
	import { RadioGroup as RadioGroupPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		...restProps
	} = $props();
</script>

<RadioGroupPrimitive.Item
	bind:ref
	data-slot="radio-group-item"
	class={cn(
		"group/radio-group-item peer relative inline-flex size-4.5 shrink-0 items-center justify-center rounded-full border border-input bg-background not-dark:bg-clip-padding shadow-xs/5 outline-none transition-shadow before:pointer-events-none before:absolute before:inset-0 before:rounded-full not-data-[disabled]:not-data-[state=checked]:not-aria-invalid:before:shadow-[0_1px_--theme(--color-black/4%)] after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background aria-invalid:border-destructive/36 focus-visible:aria-invalid:border-destructive/64 focus-visible:aria-invalid:ring-destructive/48 data-disabled:cursor-not-allowed data-disabled:opacity-64 disabled:cursor-not-allowed disabled:opacity-64 sm:size-4 dark:not-data-[state=checked]:bg-input/32 dark:aria-invalid:ring-destructive/24 dark:not-data-[disabled]:not-data-[state=checked]:not-aria-invalid:before:shadow-[0_-1px_--theme(--color-white/6%)] [[data-disabled],[data-state=checked],[aria-invalid=true]]:shadow-none",
		className
	)}
	{...restProps}
>
	{#snippet children({ checked })}
		{#if checked}
			<span
				data-slot="radio-group-indicator"
				class="absolute -inset-px flex size-4.5 items-center justify-center rounded-full bg-primary before:size-2 before:rounded-full before:bg-primary-foreground sm:size-4 sm:before:size-1.5"
			></span>
		{/if}
	{/snippet}
</RadioGroupPrimitive.Item>