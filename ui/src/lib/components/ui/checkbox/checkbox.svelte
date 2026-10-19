<script>
	import { Checkbox as CheckboxPrimitive } from "bits-ui";
	import CheckIcon from '@lucide/svelte/icons/check';
	import MinusIcon from '@lucide/svelte/icons/minus';
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		checked = $bindable(false),
		indeterminate = $bindable(false),
		class: className,
		...restProps
	} = $props();
</script>

<CheckboxPrimitive.Root
	bind:ref
	data-slot="checkbox"
	class={cn(
		"peer relative inline-flex size-4.5 shrink-0 items-center justify-center rounded-[.25rem] border border-input bg-background not-dark:bg-clip-padding shadow-xs/5 outline-none ring-ring transition-shadow before:pointer-events-none before:absolute before:inset-0 before:rounded-[3px] not-data-[disabled]:not-data-[state=checked]:not-aria-invalid:before:shadow-[0_1px_--theme(--color-black/4%)] after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:ring-2 focus-visible:ring-offset-1 focus-visible:ring-offset-background aria-invalid:border-destructive/36 focus-visible:aria-invalid:border-destructive/64 focus-visible:aria-invalid:ring-destructive/48 data-disabled:cursor-not-allowed data-disabled:opacity-64 disabled:cursor-not-allowed disabled:opacity-64 sm:size-4 dark:not-data-[state=checked]:bg-input/32 dark:aria-invalid:ring-destructive/24 dark:not-data-[disabled]:not-data-[state=checked]:not-aria-invalid:before:shadow-[0_-1px_--theme(--color-white/6%)] [[data-disabled],[data-state=checked],[aria-invalid=true]]:shadow-none",
		className
	)}
	bind:checked
	bind:indeterminate
	{...restProps}
>
	{#snippet children({ checked, indeterminate })}
		{#if checked || indeterminate}
			<span
				data-slot="checkbox-indicator"
				class={cn(
					"absolute -inset-px flex items-center justify-center rounded-[.25rem] text-primary-foreground",
					indeterminate ? "text-foreground" : "bg-primary"
				)}
			>
				{#if indeterminate}
					<MinusIcon class="size-3.5 sm:size-3" strokeWidth={3} />
				{:else}
					<CheckIcon class="size-3.5 sm:size-3" strokeWidth={3} />
				{/if}
			</span>
		{/if}
	{/snippet}
</CheckboxPrimitive.Root>