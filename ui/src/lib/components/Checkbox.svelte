<script>
  // shadcn's Checkbox with its label beside it, and an optional line under;
  // onchange(checked) follows a click.
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Label } from "$lib/components/ui/label/index.js";
  let { checked = $bindable(false), label = "", description = "", onchange = undefined, class: klass = "", id = undefined, ...rest } = $props();
  const uid = $props.id();
  const cid = $derived(id || `c-${uid}`);
</script>

<div class="flex gap-2 {description ? 'items-start' : 'items-center'} {klass}">
  <Checkbox id={cid} bind:checked onCheckedChange={(v) => onchange?.(v)} class={description ? "mt-0.5" : ""} {...rest} />
  <Label for={cid} class="flex flex-col items-start gap-0.5 font-normal">
    <span>{label}</span>
    {#if description}<span class="text-muted-foreground text-xs">{description}</span>{/if}
  </Label>
</div>
