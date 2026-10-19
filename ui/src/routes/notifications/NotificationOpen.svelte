<script>
  // An emailed notification's link (/notifications/12): reads it on the
  // way to what it's about.
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import { get } from "$lib/api.js";
  import { go } from "$lib/router.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";

  let { params } = $props();
  (async () => {
    try {
      const r = await get(`/notifications/${params.id}`);
      nav.refresh();
      go(r.path || "/notifications", undefined, { replace: true });
    } catch (e) {
      toastError(e);
      go("/notifications", undefined, { replace: true });
    }
  })();
</script>

<Skeleton class="h-32" />
