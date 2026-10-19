<script>
  // Old portal addresses: /portal/todos is the work now, /portal/me the
  // portal's home.
  import { router, go } from "$lib/router.svelte.js";
  go(router.path === "/portal/todos" ? "/portal/work" : "/portal", undefined, { replace: true });
</script>
