<script>
  // shadcn's Alert in a tone: info, success, warning, danger.
  import * as Alert from "$lib/components/ui/alert/index.js";
  import Icon from "./Icon.svelte";
  let { tone = "info", icon = null, children, class: klass = "" } = $props();
  const tones = {
    info: "",
    success: "border-emerald-500/30 text-emerald-700 dark:text-emerald-400",
    warning: "border-amber-500/30 text-amber-700 dark:text-amber-400",
    danger: "border-destructive/30",
  };
  const icons = { info: "help-circle", success: "check-circle", warning: "bell-alert", danger: "close-circle" };
</script>

<Alert.Root variant={tone === "danger" ? "destructive" : "default"} class="{tones[tone]} {klass}" role={tone === "danger" ? "alert" : "status"}>
  <Icon name={icon || icons[tone]} />
  <Alert.Description class="text-current">{@render children()}</Alert.Description>
</Alert.Root>
