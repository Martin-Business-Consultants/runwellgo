<script>
  // A searchable select, shadcn's way: a Popover holding a Command list.
  // Type to narrow, arrows and Enter to choose. `multiple` keeps several,
  // shown as chips in the trigger; choosing one again takes it off.
  //   options: [{ value, label, hint? }]
  import * as Popover from "$lib/components/ui/popover/index.js";
  import * as Command from "$lib/components/ui/command/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { cn } from "$lib/utils.js";
  import Icon from "./Icon.svelte";
  let {
    value = $bindable(null),
    options = [],
    multiple = false,
    placeholder = "Choose…",
    emptyText = "Nothing matches.",
    invalid = false,
    clearable = true,
    id = undefined,
    onchange = undefined,
  } = $props();

  let open = $state(false);
  const chosen = $derived(multiple ? (Array.isArray(value) ? value : []) : value == null || value === "" ? [] : [value]);
  const has = (v) => chosen.some((c) => String(c) === String(v));
  const labelOf = (v) => options.find((o) => String(o.value) === String(v))?.label ?? String(v);

  function pick(o) {
    if (multiple) value = has(o.value) ? chosen.filter((v) => String(v) !== String(o.value)) : [...chosen, o.value];
    else { value = o.value; open = false; }
    onchange?.(value);
  }
  function clear() { value = multiple ? [] : null; onchange?.(value); }
</script>

<div class="relative">
  <Popover.Root bind:open>
    <Popover.Trigger {id} role="combobox" aria-expanded={open} aria-invalid={invalid || undefined}
      class={cn(
        "border-input dark:bg-input/30 focus-visible:border-ring focus-visible:ring-ring/50 aria-invalid:border-destructive aria-invalid:ring-destructive/20 flex min-h-9 w-full flex-wrap items-center gap-1 rounded-md border bg-transparent py-1 pl-2.5 pr-9 text-left text-sm shadow-xs outline-none focus-visible:ring-3",
      )}>
      {#if multiple && chosen.length}
        {#each chosen as v (v)}<Badge variant="secondary">{labelOf(v)}</Badge>{/each}
      {:else if !multiple && chosen.length}
        <span class="truncate">{labelOf(chosen[0])}</span>
      {:else}
        <span class="text-muted-foreground truncate">{placeholder}</span>
      {/if}
    </Popover.Trigger>
    <Popover.Content class="w-(--bits-popover-anchor-width) min-w-56 p-0" align="start">
      <Command.Root>
        <Command.Input placeholder="Search…" />
        <Command.List>
          <Command.Empty>{emptyText}</Command.Empty>
          <Command.Group>
            {#each options as o (o.value)}
              <Command.Item value={String(o.value)} keywords={[o.label, o.hint || ""]} onSelect={() => pick(o)} data-checked={has(o.value)}>
                <span class="min-w-0 flex-1">
                  <span class="block truncate">{o.label}</span>
                  {#if o.hint}<span class="text-muted-foreground block truncate text-xs">{o.hint}</span>{/if}
                </span>
              </Command.Item>
            {/each}
          </Command.Group>
        </Command.List>
      </Command.Root>
    </Popover.Content>
  </Popover.Root>
  <span class="text-muted-foreground pointer-events-none absolute inset-y-0 right-2.5 flex items-center">
    {#if clearable && chosen.length}
      <button type="button" class="hover:text-foreground pointer-events-auto rounded-sm" onclick={clear} aria-label="Clear"><Icon name="close" /></button>
    {:else}
      <Icon name="caret-down" />
    {/if}
  </span>
</div>
