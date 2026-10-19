<script>
  // A list's search: it updates the ?s= of the address as you type (after a
  // pause), so the search is in the URL and the back button undoes it.
  import Input from "./Input.svelte";
  import { untrack } from "svelte";
  import { router, withQuery, go } from "../router.svelte.js";
  let { placeholder = "Search…", param = "s", label = "Search" } = $props();
  // svelte-ignore state_referenced_locally
  let value = $state(router.query.get(param) || "");
  $effect(() => {
    const now = router.query.get(param) || "";
    if (untrack(() => value.trim()) !== now && !document.activeElement?.matches?.("input[type=search]")) value = now;
  });
  let timer;
  function input() {
    clearTimeout(timer);
    timer = setTimeout(() => { go(withQuery({ [param]: value.trim() || null }), undefined, { replace: true, transition: false }); }, 250);
  }
</script>

<Input type="search" icon="search" class="w-full sm:w-64" bind:value oninput={input} {placeholder} aria-label={label} />
