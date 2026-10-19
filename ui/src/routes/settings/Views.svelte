<script>
  // Settings › Views: whether lists open as cards or as a table.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import LayoutGrid from "@lucide/svelte/icons/layout-grid";
  import Table from "@lucide/svelte/icons/table";
  import { get, patch } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let view = $state(null);
  get("/settings/views").then((v) => (view = v.index_view)).catch(toastError);

  async function set(value) {
    try {
      const res = await patch("/settings/views", { index_view: value });
      view = res.index_view; toast(res.summary);
      session.apply(await get("/session", { quiet: true }));
    } catch (err) { toastError(err); }
  }
</script>

<SettingsLayout title="Views">
  <Card.Root>
    <Card.Header>
      <Card.Title>How lists open</Card.Title>
      <Card.Description>{session.term("client", 2)}, {session.term("engagement", 2).toLowerCase()}, {session.term("work").toLowerCase()}, commitments and requests open as cards or as a table. Anyone can switch a list with its View menu; this sets where it starts.</Card.Description>
    </Card.Header>
    <Card.Content>
      {#if view}
        <RadioGroup.Root value={view} onValueChange={set} class="grid max-w-md grid-cols-2 gap-3">
          {#each [["cards", "Cards", LayoutGrid], ["table", "Table", Table]] as [value, label, Icon] (value)}
            <Field.Label for={`view-${value}`} class="w-full has-data-[state=checked]:border-primary flex cursor-pointer flex-col items-center gap-2 rounded-md border p-4">
              <RadioGroup.Item {value} id={`view-${value}`} class="sr-only" />
              <Icon class="size-5" /><span>{label}</span>
            </Field.Label>
          {/each}
        </RadioGroup.Root>
      {/if}
    </Card.Content>
  </Card.Root>
</SettingsLayout>
