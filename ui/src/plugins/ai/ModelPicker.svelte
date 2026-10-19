<script>
  // A model, picked from a list rather than typed: the provider's own
  // models (or the known ones while those can't be had), each with its
  // context size and price where known and the recommended one marked,
  // plus "Another model…" which shows a box for a name the list doesn't
  // have (the one place a model is typed; Test checks it).
  //   options: [{id, label, context, price, recommended}]
  //   extra: [{value, label, hint}] offered first (an agent's "Main model")
  //   value: the model's id, or "" for blank (the recommended one)
  import Combobox from "$lib/components/Combobox.svelte";
  import { Input } from "$lib/components/ui/input/index.js";

  let { value = $bindable(""), options = [], extra = [], role = "main", blank = "Recommended", id = undefined, invalid = false } = $props();

  const OTHER = "__other";
  const known = (v) => !v || extra.some((o) => o.value === v) || options.some((o) => o.id === v);
  // svelte-ignore state_referenced_locally
  let other = $state(!known(value));
  let picked = $state(null);

  const recommended = (o) => o.recommended === role || o.recommended === "both";
  const choices = $derived([
    ...extra,
    ...options.map((o) => ({ value: o.id, label: o.label + (recommended(o) ? " · recommended" : ""), hint: [o.context, o.price].filter(Boolean).join(" · ") || undefined })),
    { value: OTHER, label: "Another model…", hint: "One the list doesn’t have: type its name" },
  ]);

  $effect(() => {
    picked = other ? OTHER : value || null;
  });

  function change(v) {
    if (v === OTHER) {
      other = true;
      if (known(value)) value = "";
      return;
    }
    other = false;
    value = v ?? "";
  }
</script>

<div class="flex flex-col gap-2">
  <Combobox {id} value={picked} options={choices} placeholder={blank} {invalid} onchange={change} emptyText="No model matches: choose Another model…" />
  {#if other}
    <Input bind:value aria-label="Model name" autocomplete="off" spellcheck="false" placeholder="The model’s name, as the provider writes it" />
  {/if}
</div>
