<script>
  // How strong a password looks: length and variety, as a bar and a word.
  let { password = "" } = $props();
  const score = $derived.by(() => {
    const p = password || "";
    if (!p) return 0;
    let s = p.length >= 10 ? 1 : 0;
    if (p.length >= 14) s++;
    if (/[a-z]/.test(p) && /[A-Z]/.test(p)) s++;
    if (/\d/.test(p)) s++;
    if (/[^A-Za-z0-9]/.test(p)) s++;
    if (/^(.)\1+$/.test(p) || /^(password|1234567890|qwerty)/i.test(p)) s = 0;
    return Math.min(4, s);
  });
  const words = ["Too short", "Weak", "Fair", "Good", "Strong"];
  const tones = ["bg-destructive", "bg-destructive", "bg-amber-500", "bg-emerald-500", "bg-emerald-600"];
</script>

{#if password}
  <div class="flex items-center gap-3" aria-live="polite">
    <div class="bg-muted flex h-1.5 flex-1 gap-1 overflow-hidden rounded-full">
      {#each [1, 2, 3, 4] as i}<span class="h-full flex-1 {score >= i ? tones[score] : ''}"></span>{/each}
    </div>
    <span class="text-muted-foreground w-16 text-right text-xs">{words[score]}</span>
  </div>
{/if}
