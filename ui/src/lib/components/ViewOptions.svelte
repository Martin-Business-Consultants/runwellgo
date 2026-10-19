<script>
  // A list's view options (WordPress's Screen Options), remembered per
  // person: which columns show, rows a page, compact or comfortable rows.
  //   columns: [{ key, label }] (the ones that can be hidden)
  import * as Popover from "$lib/components/ui/popover/index.js";
  import * as ToggleGroup from "$lib/components/ui/toggle-group/index.js";
  import { buttonVariants } from "$lib/components/ui/button/index.js";
  import Checkbox from "./Checkbox.svelte";
  import Select from "./Select.svelte";
  import Icon from "./Icon.svelte";
  let { view, columns = [], onchange = () => {} } = $props();
  const change = (c) => { view.update(c); onchange(c); };
</script>

<Popover.Root>
  <Popover.Trigger class={buttonVariants({ variant: "outline", size: "sm" })} aria-label="View options"><Icon name="sliders" /><span class="hidden sm:inline">View</span></Popover.Trigger>
  <Popover.Content align="end" class="flex w-64 flex-col gap-4">
    {#if columns.length}
      <div class="flex flex-col gap-2">
        <p class="text-muted-foreground text-xs font-medium">Columns</p>
        {#each columns as c (c.key)}
          <Checkbox label={c.label} checked={!view.hidden.includes(c.key)}
            onchange={(on) => change({ hidden: on ? view.hidden.filter((k) => k !== c.key) : [...view.hidden, c.key] })} />
        {/each}
      </div>
    {/if}
    <div class="flex flex-col gap-1.5">
      <p class="text-muted-foreground text-xs font-medium">Rows a page</p>
      <Select options={[10, 25, 50, 100].map((n) => ({ value: n, label: String(n) }))} value={view.per} onchange={(v) => change({ per: v })} aria-label="Rows a page" />
    </div>
    <div class="flex flex-col gap-1.5">
      <p class="text-muted-foreground text-xs font-medium">Density</p>
      <ToggleGroup.Root type="single" variant="outline" size="sm" value={view.density} onValueChange={(v) => v && change({ density: v })}>
        <ToggleGroup.Item value="compact">Compact</ToggleGroup.Item>
        <ToggleGroup.Item value="comfortable">Comfortable</ToggleGroup.Item>
      </ToggleGroup.Root>
    </div>
  </Popover.Content>
</Popover.Root>
