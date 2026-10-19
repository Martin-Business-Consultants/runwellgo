<script>
	import { Switch as SwitchPrimitive } from "bits-ui";
	import { cn } from "$lib/utils.js";

	let {
		ref = $bindable(null),
		class: className,
		checked = $bindable(false),
		size = "default",
		...restProps
	} = $props();
</script>

<SwitchPrimitive.Root
	bind:ref
	bind:checked
	data-slot="switch"
	data-size={size}
	class={cn(
		"peer group/switch relative inline-flex h-[calc(var(--thumb-size)+2px)] w-[calc(var(--thumb-size)*2-2px)] shrink-0 items-center rounded-full p-px outline-none transition-[background-color,box-shadow] duration-200 after:absolute after:-inset-x-3 after:-inset-y-2 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-1 focus-visible:ring-offset-background data-disabled:cursor-not-allowed data-[state=checked]:bg-primary data-[state=unchecked]:bg-input data-disabled:opacity-64",
		size === "sm"
			? "[--thumb-size:--spacing(4)] sm:[--thumb-size:--spacing(3)]"
			: "[--thumb-size:--spacing(5)] sm:[--thumb-size:--spacing(4)]",
		className
	)}
	{...restProps}
>
	<SwitchPrimitive.Thumb
		data-slot="switch-thumb"
		class="pointer-events-none block aspect-square h-full origin-left in-[[role=switch]:active,[data-slot=label]:active,[data-slot=field-label]:active]:not-data-[disabled]:scale-x-110 in-[[role=switch]:active,[data-slot=label]:active,[data-slot=field-label]:active]:rounded-[var(--thumb-size)/calc(var(--thumb-size)*1.1)] rounded-(--thumb-size) bg-background shadow-sm/5 will-change-transform [transition:translate_.15s,border-radius_.15s,scale_.1s_.1s,transform-origin_.15s] motion-reduce:transition-none data-[state=checked]:origin-[var(--thumb-size)_50%] data-[state=checked]:translate-x-[calc(var(--thumb-size)-4px)] rtl:origin-right rtl:data-[state=checked]:origin-left rtl:data-[state=checked]:-translate-x-[calc(var(--thumb-size)-4px)]"
	/>
</SwitchPrimitive.Root>