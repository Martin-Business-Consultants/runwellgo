<script>
  // A list's filter as an outline button showing what's chosen. Up to five
  // choices open as a menu; more open as a combobox, a search box over the
  // list, so a long list of clients is found by typing.
  //   options: [{value, label, count}]; current: the chosen value;
  //   onpick(value): a choice was made.
  import { Button } from "$lib/components/ui/button/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import * as Popover from "$lib/components/ui/popover/index.js";
  import * as Command from "$lib/components/ui/command/index.js";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import Check from "@lucide/svelte/icons/check";
  import { cn } from "$lib/utils.js";

  let { title = "", options = [], current = "", onpick, align = "end" } = $props();
  const LONG = 5;

  let open = $state(false);
  const chosen = $derived(options.find((o) => String(o.value) === String(current)) || options[0]);
  const pick = (v) => { open = false; if (String(v) !== String(current)) onpick(v); };
</script>

{#snippet trigger(props)}
  <Button {...props} variant="outline" size="sm" class="max-w-full min-w-0" title={title || undefined}
    aria-label={title ? `${title}: ${chosen?.label ?? ""}` : undefined}>
    <span class="truncate">{chosen?.label}</span>{#if chosen?.count != null}<span class="text-muted-foreground text-xs tabular-nums">{chosen.count}</span>{/if}
    <ChevronDown class="text-muted-foreground" />
  </Button>
{/snippet}

{#snippet choice(o)}
  <span class="min-w-0 flex-1 truncate" title={o.label}>{o.label}</span>{#if o.count != null}<span class="text-muted-foreground text-xs tabular-nums">{o.count}</span>{/if}
{/snippet}

{#if options.length > LONG}
  <Popover.Root bind:open>
    <Popover.Trigger>{#snippet child({ props })}{@render trigger(props)}{/snippet}</Popover.Trigger>
    <Popover.Content class="w-72 max-w-[calc(100vw-2rem)] p-0" {align}>
      <Command.Root>
        <Command.Input placeholder={title && !/^show/i.test(title) ? `Search ${title.toLowerCase()}…` : "Search…"} />
        <Command.List class="max-h-72">
          <Command.Empty>Nothing matches.</Command.Empty>
          <Command.Group heading={title || undefined}>
            {#each options as o (o.value)}
              {@const on = String(o.value) === String(current)}
              <Command.Item value={String(o.value)} keywords={[o.label]} onSelect={() => pick(o.value)} aria-selected={on}>
                <Check class={cn("size-4", on ? "opacity-100" : "opacity-0")} />{@render choice(o)}
              </Command.Item>
            {/each}
          </Command.Group>
        </Command.List>
      </Command.Root>
    </Popover.Content>
  </Popover.Root>
{:else}
  <DropdownMenu.Root>
    <DropdownMenu.Trigger>{#snippet child({ props })}{@render trigger(props)}{/snippet}</DropdownMenu.Trigger>
    <DropdownMenu.Content {align} class="max-w-[min(22rem,calc(100vw-2rem))] min-w-48">
      {#if title}<DropdownMenu.Label>{title}</DropdownMenu.Label>{/if}
      <DropdownMenu.RadioGroup value={String(current)} onValueChange={pick}>
        {#each options as o (o.value)}
          <DropdownMenu.RadioItem value={String(o.value)}>{@render choice(o)}</DropdownMenu.RadioItem>
        {/each}
      </DropdownMenu.RadioGroup>
    </DropdownMenu.Content>
  </DropdownMenu.Root>
{/if}
