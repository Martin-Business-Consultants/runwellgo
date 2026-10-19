<script>
  // shadcn's Input, optionally with a leading icon; `invalid` marks it.
  import { Input } from "$lib/components/ui/input/index.js";
  import * as InputGroup from "$lib/components/ui/input-group/index.js";
  import Icon from "./Icon.svelte";
  let { value = $bindable(""), type = "text", icon = null, invalid = false, mono = false, class: klass = "", ref = $bindable(null), ...rest } = $props();
</script>

{#if icon}
  <InputGroup.Root class={klass}>
    <InputGroup.Addon><Icon name={icon} /></InputGroup.Addon>
    <InputGroup.Input bind:ref {type} bind:value class={mono ? "font-mono" : ""} aria-invalid={invalid || undefined} {...rest} />
  </InputGroup.Root>
{:else}
  <Input bind:ref {type} bind:value class="{mono ? 'font-mono' : ''} {klass}" aria-invalid={invalid || undefined} {...rest} />
{/if}
