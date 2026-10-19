<script>
  // The Webhooks step's rows (the Rails repeater): each webhook folds open to
  // its name, URL, and the keys and values it's set up with, each list added
  // to and taken from as you go. Removing a saved one marks it to delete on
  // save (_destroy); a new one just goes.
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import Plus from "@lucide/svelte/icons/plus";
  import Trash from "@lucide/svelte/icons/trash-2";
  import X from "@lucide/svelte/icons/x";
  import { tick } from "svelte";

  let { rows = $bindable([]) } = $props();
  let list = $state();

  async function addWebhook() {
    rows.push({ id: null, name: "", url: "", pairs: [], _destroy: false, open: true });
    await tick();
    list?.querySelector("[data-row]:last-of-type input")?.focus();
  }
  function removeWebhook(i) {
    if (rows[i].id) rows[i]._destroy = true;
    else rows.splice(i, 1);
  }
</script>

<div class="flex flex-col gap-2" bind:this={list}>
  <span class="text-sm font-medium">Webhooks</span>
  {#each rows as w, i (w)}
    {#if !w._destroy}
      <Collapsible.Root bind:open={w.open} class="rounded-lg border" data-row>
        <Collapsible.Trigger class="flex w-full min-w-0 items-center gap-2 px-3 py-2 text-start text-sm">
          <ChevronDown class="text-muted-foreground size-4 shrink-0 transition-transform {w.open ? '' : '-rotate-90'}" />
          <strong class="truncate">{w.name || "New webhook"}</strong>
          <span class="text-muted-foreground min-w-0 truncate text-xs">{w.url}</span>
        </Collapsible.Trigger>
        <Collapsible.Content class="flex flex-col gap-3 border-t px-3 py-3">
          <Field.Field>
            <Field.Label for={`wh-name-${i}`}>Name</Field.Label>
            <Input id={`wh-name-${i}`} bind:value={w.name} autocomplete="off" placeholder="Zapier to the CRM" />
          </Field.Field>
          <Field.Field>
            <Field.Label for={`wh-url-${i}`}>Webhook URL</Field.Label>
            <Input id={`wh-url-${i}`} type="url" bind:value={w.url} autocomplete="off" placeholder="https://hooks.zapier.com/hooks/catch/…" />
          </Field.Field>
          <div class="flex flex-col gap-2">
            <span class="text-sm font-medium">Keys and values</span>
            {#each w.pairs as p, j (p)}
              <div class="grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)_auto] items-center gap-2">
                <Input bind:value={p.key} autocomplete="off" placeholder="Key" aria-label="Key" />
                <Input bind:value={p.value} autocomplete="off" placeholder="Value" aria-label="Value" />
                <Button type="button" variant="ghost" size="icon-sm" title="Remove" onclick={() => w.pairs.splice(j, 1)}><X /><span class="sr-only">Remove</span></Button>
              </div>
            {/each}
            <div><Button type="button" variant="outline" size="sm" onclick={() => w.pairs.push({ key: "", value: "" })}><Plus />Add a key</Button></div>
          </div>
          <div><Button type="button" variant="outline" size="sm" onclick={() => removeWebhook(i)}><Trash />Remove this webhook</Button></div>
        </Collapsible.Content>
      </Collapsible.Root>
    {/if}
  {/each}
  <div><Button type="button" variant="outline" size="sm" onclick={addWebhook}><Plus />Add a webhook</Button></div>
</div>
