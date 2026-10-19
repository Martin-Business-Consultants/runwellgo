<script>
  // A filter in a toolbar's own slot: a FilterMenu on one query parameter,
  // "" (the first choice) for none. A searchable combobox past five choices.
  import FilterMenu from "./FilterMenu.svelte";
  import { router, withQuery, go } from "../router.svelte.js";
  let { title, param, options = [] } = $props();
  const current = $derived(router.query.get(param) || "");
</script>

<FilterMenu title={title?.replace(/…$/, "")} {options} {current} onpick={(v) => go(withQuery({ [param]: v || null }))} />
