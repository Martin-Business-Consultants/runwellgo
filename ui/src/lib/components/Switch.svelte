<script>
  // shadcn's Switch, with its label and description; onchange(checked)
  // follows a flip.
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Label } from "$lib/components/ui/label/index.js";
  let { checked = $bindable(false), label = "", description = "", disabled = false, onchange = undefined, id = undefined } = $props();
  const uid = $props.id();
  const sid = $derived(id || `s-${uid}`);
</script>

<div class="flex items-center gap-3">
  <Switch id={sid} bind:checked {disabled} onCheckedChange={(v) => onchange?.(v)} aria-label={label || undefined} />
  {#if label || description}
    <Label for={sid} class="flex flex-col items-start gap-0.5">
      {#if label}<span>{label}</span>{/if}
      {#if description}<span class="text-muted-foreground text-xs font-normal">{description}</span>{/if}
    </Label>
  {/if}
</div>
