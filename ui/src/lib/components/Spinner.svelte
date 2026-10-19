<script>
  import { Spinner } from "$lib/components/ui/spinner/index.js";
  let { class: klass = "" } = $props();
</script>

<Spinner class="text-muted-foreground {klass}" />
