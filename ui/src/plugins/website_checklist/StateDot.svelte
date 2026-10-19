<script>
  // A step's state in the list: a dot, lit green and glowing once confirmed
  // (or a recommendation once approved), amber while pending.
  let { state, title = "" } = $props();
  const classes = {
    confirmed: "bg-emerald-500 shadow-[0_0_0_2px_rgb(16_185_129/0.2),0_0_8px_1px_rgb(16_185_129/0.55)]",
    approved: "bg-emerald-500 shadow-[0_0_0_2px_rgb(16_185_129/0.2),0_0_8px_1px_rgb(16_185_129/0.55)]",
    pending: "bg-amber-400",
    not_applicable: "bg-muted-foreground/30",
    rejected: "bg-muted-foreground/30",
  };
</script>

<span class="inline-block size-2 shrink-0 rounded-full {classes[state] || 'bg-muted-foreground/60'}" title={title || undefined} aria-hidden="true"></span>
