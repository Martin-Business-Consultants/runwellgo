<script>
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  let { value = $bindable(""), invalid = false, mono = false, rows = 4, class: klass = "", ...rest } = $props();
</script>

<Textarea bind:value {rows} class="{mono ? 'font-mono text-xs' : ''} {klass}" aria-invalid={invalid || undefined} {...rest} />
