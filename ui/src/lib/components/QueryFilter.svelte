<script>
  // A filter kept in the address (?param=value), for a list inside a page
  // (a client's engagements, an engagement's work) as IndexToolbar's are for
  // an index: a FilterMenu, so a long list of choices is searched. Choosing
  // the default takes the parameter off.
  //   options: [{value, label, count}]; fallback: the value when none is set.
  import FilterMenu from "./FilterMenu.svelte";
  import { router, withQuery, go } from "../router.svelte.js";

  let { param, options = [], fallback = options[0]?.value, title = "Show", align = "end" } = $props();
  const current = $derived(options.some((o) => o.value === router.query.get(param)) ? router.query.get(param) : fallback);
</script>

<FilterMenu {title} {options} {current} {align} onpick={(v) => go(withQuery({ [param]: v === fallback ? null : v }), undefined, { replace: true })} />
