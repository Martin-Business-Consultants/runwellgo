<script>
  // The custom fields' inputs in a record's form (Settings › Fields): bind
  // values (key → value) and send them as custom_fields. Money is typed in
  // dollars and kept as whole cents.
  import { onMount } from "svelte";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import DatePicker from "./DatePicker.svelte";
  import { customFields } from "../custom-fields.svelte.js";
  let { model, values = $bindable({}) } = $props();
  let fields = $state([]);
  onMount(async () => { fields = await customFields(model); });
  const dollars = (c) => (c === "" || c == null ? "" : (Number(c) / 100).toFixed(2));
</script>

{#each fields as f (f.id)}
  <Field.Field orientation={f.kind === "boolean" ? "horizontal" : "vertical"}>
    <Field.Label for={"cf-" + f.key}>{f.label}</Field.Label>
    {#if f.kind === "long_text"}
      <Textarea id={"cf-" + f.key} bind:value={values[f.key]} />
    {:else if f.kind === "boolean"}
      <Switch id={"cf-" + f.key} checked={!!values[f.key]} onCheckedChange={(v) => (values[f.key] = v)} />
    {:else if f.kind === "date"}
      <DatePicker id={"cf-" + f.key} bind:value={values[f.key]} />
    {:else if f.kind === "money"}
      <Input id={"cf-" + f.key} inputmode="decimal" value={dollars(values[f.key])} onchange={(e) => (values[f.key] = e.target.value === "" ? "" : Math.round(parseFloat(e.target.value.replace(/[^0-9.-]/g, "")) * 100))} placeholder="0.00" />
    {:else if f.kind === "choice"}
      <Select.Root type="single" value={values[f.key] ?? ""} onValueChange={(v) => (values[f.key] = v)}>
        <Select.Trigger id={"cf-" + f.key} class="w-full">{values[f.key] || "Choose…"}</Select.Trigger>
        <Select.Content>
          <Select.Item value="">None</Select.Item>
          {#each f.choices as c (c)}<Select.Item value={c}>{c}</Select.Item>{/each}
        </Select.Content>
      </Select.Root>
    {:else}
      <Input id={"cf-" + f.key} type={f.kind === "number" ? "number" : f.kind === "link" ? "url" : "text"} bind:value={values[f.key]} />
    {/if}
  </Field.Field>
{/each}
