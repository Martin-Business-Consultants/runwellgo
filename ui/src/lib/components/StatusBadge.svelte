<script>
  // A record's status as a shadcn Badge: filled for the ones that are live
  // (active, in progress), outline for the rest, red for trouble.
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { humanize } from "../format.js";
  let { status, label = undefined, class: klass = "" } = $props();
  const live = ["active", "in_progress", "in progress", "open", "approved", "sent", "awaiting_client", "awaiting client", "internal"];
  const trouble = ["blocked", "overdue", "declined", "changes_requested", "cancelled", "missed", "broken"];
  const done = ["done", "closed", "completed", "kept", "resolved", "dismissed", "former"];
  const s = $derived(String(status || "").toLowerCase());
  const variant = $derived(trouble.includes(s) ? "destructive" : live.includes(s) ? "default" : done.includes(s) ? "secondary" : "outline");
</script>

<Badge {variant} class={klass}>{label || humanize(status)}</Badge>
