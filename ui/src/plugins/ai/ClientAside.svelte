<script>
  // A client's side column (the client_aside slot): the week with them,
  // and the switch that keeps them out of AI (a person's call). Not on
  // the install's own internal client.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import Sparkles from "@lucide/svelte/icons/sparkles";
  import SuggestionCard from "./SuggestionCard.svelte";
  import { staff, status } from "./ai.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { record, reload } = $props();
  let excluded = $state(null), busy = $state(false), round = $state(0);
  status.load();
  const look = () => staff.get(`/status?record=Client:${record.id}`).then((s) => (excluded = !!s.excluded)).catch(() => {});
  look();

  async function flip() {
    busy = true;
    try {
      const r = excluded ? await staff.del(`/clients/${record.id}/exclusion`) : await staff.post(`/clients/${record.id}/exclusion`);
      toast(r.message);
      excluded = r.excluded;
      round++;
    } catch (e) { toastError(e); }
    busy = false;
  }
</script>

{#if status.ready && !record.internal && excluded !== null}
  <Card.Root>
    <Card.Header><Card.Title>AI</Card.Title></Card.Header>
    <Card.Content class="flex flex-col gap-3">
      {#if !excluded}{#key round}<SuggestionCard kind="client_week" record={`Client:${record.id}`} onused={reload} />{/key}{/if}
      <div>
        <Button variant="ghost" size="sm" class={excluded ? "" : "text-muted-foreground"} onclick={flip} disabled={busy}>
          <Sparkles />{excluded ? "Kept out of AI: let it back" : "Keep out of AI"}
        </Button>
      </div>
    </Card.Content>
  </Card.Root>
{/if}
