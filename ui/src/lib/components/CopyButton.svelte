<script>
  // Copies text, and says so for a moment.
  import Button from "./Button.svelte";
  let { text, label = "Copy", size = "sm", variant = "plain" } = $props();
  let copied = $state(false);
  async function copy() {
    try { await navigator.clipboard.writeText(text); copied = true; setTimeout(() => (copied = false), 1600); } catch {}
  }
</script>

<Button {variant} {size} icon={copied ? "check" : "clipboard"} onclick={copy} class={copied ? "text-emerald-600 dark:text-emerald-400" : ""}>{copied ? "Copied" : label}</Button>
