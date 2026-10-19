<script>
  // shadcn's Select over a list of choices. options: [{value, label}] or
  // strings; `placeholder` adds an empty first choice, which reads back as
  // null. Values match whatever their type (an id 3 or "3"), and the chosen
  // option's own value is what's bound; onchange(value) follows a choice.
  import * as Select from "$lib/components/ui/select/index.js";
  let { value = $bindable(""), options = [], placeholder = null, invalid = false, disabled = false, class: klass = "", onchange = undefined, ...rest } = $props();
  const NONE = "\u0000none";
  const opts = $derived(options.map((o) => (typeof o === "object" ? o : { value: o, label: o })));
  const current = $derived(value == null || value === "" ? (placeholder !== null ? NONE : "") : String(value));
  const label = $derived(opts.find((o) => String(o.value) === current)?.label ?? "");
  function change(v) {
    value = v === NONE ? null : (opts.find((o) => String(o.value) === v)?.value ?? v);
    onchange?.(value);
  }
</script>

<Select.Root type="single" value={current} onValueChange={change} {disabled}>
  <Select.Trigger class="w-full {klass}" aria-invalid={invalid || undefined} {...rest}>
    <span data-slot="select-value" class={label ? "" : "text-muted-foreground"}>{label || placeholder || ""}</span>
  </Select.Trigger>
  <Select.Content>
    {#if placeholder !== null}<Select.Item value={NONE} label={placeholder} />{/if}
    {#each opts as o (o.value)}<Select.Item value={String(o.value)} label={o.label} disabled={o.disabled} />{/each}
  </Select.Content>
</Select.Root>
