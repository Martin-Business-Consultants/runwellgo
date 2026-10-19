<script>
  // An index whose search (?q) found nothing: says so, and clears it.
  import EmptyState from "./EmptyState.svelte";
  import { router, withQuery } from "../router.svelte.js";
</script>

<EmptyState title={`Nothing matches “${router.query.get("q")}”.`}>
  <a class="underline-offset-4 hover:underline" href={withQuery({ q: null })}>Clear the search</a>
</EmptyState>
