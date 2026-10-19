<script>
  // A record's custom field values, as a Details list (dt/dd pairs): each
  // active field that has a value, by its label.
  import { onMount } from "svelte";
  import { customFields } from "../custom-fields.svelte.js";
  import { money, date } from "../format.js";
  let { model, values = {} } = $props();
  let fields = $state([]);
  onMount(async () => { fields = await customFields(model); });
  const show = (f, v) => f.kind === "money" ? money(v) : f.kind === "date" ? date(v) : f.kind === "boolean" ? (v ? "Yes" : "No") : String(v);
</script>

{#each fields.filter((f) => values[f.key] != null && values[f.key] !== "") as f (f.id)}
  <dt class="text-muted-foreground">{f.label}</dt>
  <dd>{#if f.kind === "link"}<a class="underline-offset-4 hover:underline" href={values[f.key]} target="_blank" rel="noopener noreferrer">{values[f.key]}</a>{:else}{show(f, values[f.key])}{/if}</dd>
{/each}
