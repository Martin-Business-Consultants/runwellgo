<script>
  // The older name for Frame: a section with a header, a body and a footer.
  // `padded` is Frame's not-`flush`.
  import Frame from "./Frame.svelte";
  let { padded = false, ...rest } = $props();
</script>

<Frame flush={!padded} {...rest} />
