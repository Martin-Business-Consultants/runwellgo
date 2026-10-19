<script>
  // Settings › Fields: what the install records about clients, engagements
  // and work beyond the basics. Information only; archiving one hides it
  // and keeps its values.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import Plus from "@lucide/svelte/icons/plus";
  import { get, post, patch, del } from "$lib/api.js";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";

  let data = $state(null), edits = $state({}), adding = $state({}), errors = $state({});

  function load() {
    return get("/settings/fields").then((d) => {
      data = d;
      edits = Object.fromEntries(d.fields.map((f) => [f.id, { label: f.label, choices: (f.choices || []).join(", "), listed: f.listed, client_visible: f.client_visible }]));
      for (const m of d.models) adding[m.model] ??= { label: "", kind: "text", choices: "", listed: false, client_visible: false };
    }).catch(toastError);
  }
  load();

  const fieldsOf = (model) => data.fields.filter((f) => f.model_type === model);
  const kindLabel = (k) => data.kinds.find((x) => x.key === k)?.label || k;

  async function change(f, body) {
    try { toast((await patch(`/settings/fields/${f.id}`, { field: body })).summary); } catch (err) { toastError(err); }
    load();
  }
  async function archive(f) {
    if (!(await confirm({ title: `Archive ${f.label}?`, message: "It’s archived: hidden from forms, pages and agents, with its values kept. You can restore it.", confirmLabel: "Archive", destructive: true }))) return;
    try { toast((await del(`/settings/fields/${f.id}`)).summary); } catch (err) { toastError(err); }
    load();
  }
  async function add(model, e) {
    e.preventDefault();
    errors = {};
    try {
      toast((await post("/settings/fields", { field: { model_type: model, ...adding[model] } })).summary);
      adding[model] = { label: "", kind: "text", choices: "", listed: false, client_visible: false };
      load();
    } catch (err) { errors = { [model]: errorMessage(err) }; }
  }
</script>

<SettingsLayout title="Fields" description="What you record beyond the basics, like a property address or a matter number. Fields are information only: nothing in Runwell acts on them. Every field is optional. Archiving one hides it and keeps its values.">
  {#if !data}
    <Skeleton class="h-96 w-full" />
  {:else}
    {#each data.models as m (m.model)}
      {@const fields = fieldsOf(m.model)}
      {@const active = fields.filter((f) => !f.archived)}
      <Card.Root>
        <Card.Header><Card.Title>{m.name}</Card.Title></Card.Header>
        <Card.Content class="flex flex-col gap-3">
          {#if fields.length}
            <ul class="divide-y rounded-md border">
              {#each fields as f (f.id)}
                <li class="flex flex-col gap-2 px-3 py-2">
                  <div class="flex items-center gap-2 text-sm">
                    <div class="min-w-0 flex-1">
                      <p class="font-medium">{f.label}</p>
                      <p class="text-muted-foreground truncate text-xs">{kindLabel(f.kind)} · {f.key}{f.kind === "choice" ? " · " + (f.choices || []).join(", ") : ""}{f.listed ? " · listed" : ""}{f.client_visible ? " · shown to clients" : ""}</p>
                    </div>
                    {#if f.archived}
                      <Badge variant="outline">Archived</Badge>
                      <Button variant="outline" size="sm" onclick={() => change(f, { archived: false })}>Restore</Button>
                    {:else}
                      <Button variant="ghost" size="icon-sm" disabled={f.id === active[0]?.id} aria-label={`Move ${f.label} up`} onclick={() => change(f, { position: f.position - 1 })}><ArrowUp /></Button>
                      <Button variant="ghost" size="sm" onclick={() => archive(f)}>Archive</Button>
                    {/if}
                  </div>
                  {#if !f.archived && edits[f.id]}
                    <Collapsible.Root>
                      <Collapsible.Trigger class="text-muted-foreground text-xs underline-offset-4 hover:underline">Change</Collapsible.Trigger>
                      <Collapsible.Content>
                        <form class="mt-2 flex flex-col gap-3" onsubmit={(e) => { e.preventDefault(); change(f, edits[f.id]); }}>
                          <Input bind:value={edits[f.id].label} required autocomplete="off" aria-label="Label" />
                          {#if f.kind === "choice"}<Input bind:value={edits[f.id].choices} autocomplete="off" aria-label="Choices, separated by commas" />{/if}
                          <Field.Field orientation="horizontal">
                            <Checkbox id={`listed-${f.id}`} bind:checked={edits[f.id].listed} />
                            <Field.Label for={`listed-${f.id}`} class="font-normal">A column in the {m.name.toLowerCase()} table</Field.Label>
                          </Field.Field>
                          <Field.Field orientation="horizontal">
                            <Checkbox id={`visible-${f.id}`} bind:checked={edits[f.id].client_visible} />
                            <Field.Label for={`visible-${f.id}`} class="font-normal">Shown to the client in the portal</Field.Label>
                          </Field.Field>
                          <div><Button type="submit" size="sm">Save</Button></div>
                        </form>
                      </Collapsible.Content>
                    </Collapsible.Root>
                  {/if}
                </li>
              {/each}
            </ul>
          {:else}
            <p class="text-muted-foreground text-sm">No fields yet.</p>
          {/if}
          <Collapsible.Root>
            <Collapsible.Trigger>
              {#snippet child({ props })}<Button variant="outline" size="sm" {...props}><Plus />Add a field</Button>{/snippet}
            </Collapsible.Trigger>
            <Collapsible.Content>
              <form class="mt-3 flex flex-col gap-3" onsubmit={(e) => add(m.model, e)}>
                {#if errors[m.model]}<p class="text-destructive text-sm">{errors[m.model]}</p>{/if}
                <div class="flex flex-wrap gap-2">
                  <Input bind:value={adding[m.model].label} required autocomplete="off" placeholder="Label" aria-label="Label" class="min-w-48 flex-1" />
                  <Select.Root type="single" bind:value={adding[m.model].kind}>
                    <Select.Trigger class="w-40" aria-label="Kind">{kindLabel(adding[m.model].kind)}</Select.Trigger>
                    <Select.Content>{#each data.kinds as k (k.key)}<Select.Item value={k.key}>{k.label}</Select.Item>{/each}</Select.Content>
                  </Select.Root>
                </div>
                {#if adding[m.model].kind === "choice"}
                  <Input bind:value={adding[m.model].choices} autocomplete="off" placeholder="Choices: Small, Medium, Large" aria-label="Choices" />
                {/if}
                <Field.Field orientation="horizontal">
                  <Checkbox id={`new-listed-${m.model}`} bind:checked={adding[m.model].listed} />
                  <Field.Label for={`new-listed-${m.model}`} class="font-normal">A column in the {m.name.toLowerCase()} table</Field.Label>
                </Field.Field>
                <Field.Field orientation="horizontal">
                  <Checkbox id={`new-visible-${m.model}`} bind:checked={adding[m.model].client_visible} />
                  <Field.Label for={`new-visible-${m.model}`} class="font-normal">Shown to the client in the portal</Field.Label>
                </Field.Field>
                <p class="text-muted-foreground text-xs">The kind can’t change once the field has values.</p>
                <div><Button type="submit" size="sm">Add field</Button></div>
              </form>
            </Collapsible.Content>
          </Collapsible.Root>
        </Card.Content>
      </Card.Root>
    {/each}
  {/if}
</SettingsLayout>
