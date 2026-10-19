<script>
  // Work from a website's page (the Rails Work sheet): a sheet on the right
  // to add a piece of work on one of the client's open engagements, about
  // the step on screen (or the site as a whole), with a lead, a due date and
  // details (with @mentions). It's linked back to the step, which lists it.
  import * as Sheet from "$lib/components/ui/sheet/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import RichTextEditor from "$lib/components/RichTextEditor.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import { post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { wc } from "./wc.js";
  import { focusInvalid } from "$lib/forms.js";

  let { open = $bindable(false), site, item = null, onchange } = $props();
  const word = $derived(session.term("work"));
  const team = $derived(site.people || []);
  let form = $state({ engagement_id: "", item_id: "", title: "", description: "", owner_id: "", due_on: "" });
  let errors = $state({}), busy = $state(false);

  // Each time it opens: about the step on screen, led by whoever opened it.
  $effect(() => {
    if (!open) return;
    form = { engagement_id: site.engagements[0] ? String(site.engagements[0].id) : "", item_id: item ? String(item.id) : "", title: "", description: "",
      owner_id: session.me.id ? String(session.me.id) : "", due_on: "" };
    errors = {};
  });
  const about = $derived(site.items.find((i) => String(i.id) === form.item_id));
  const engagement = $derived(site.engagements.find((e) => String(e.id) === form.engagement_id));

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    const todo = { ...form, engagement_id: Number(form.engagement_id), item_id: form.item_id ? Number(form.item_id) : null, owner_id: form.owner_id ? Number(form.owner_id) : null };
    try { const r = await post(wc(`/websites/${site.id}/todos`), { todo }); toast(r.message); open = false; onchange(r); }
    catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
</script>

<Sheet.Root bind:open>
  <Sheet.Content side="right" class="w-full overflow-y-auto data-[side=right]:sm:max-w-xl">
    <Sheet.Header>
      <Sheet.Title>Add {word.toLowerCase()} · {site.client.display_name}</Sheet.Title>
    </Sheet.Header>
    <div class="px-4 pb-4">
      {#if !site.engagements.length}
        <p class="text-sm">{site.client.display_name} has no open {session.term("engagement", 2).toLowerCase()} to put {word.toLowerCase()} on. <a class="underline underline-offset-4" href={`/clients/${site.client.id}`}>Start one on their page</a>.</p>
      {:else}
        <form onsubmit={save}>
          <Field.Group class="gap-4">
            <Field.Field>
              <Field.Label for="work-engagement">{session.term("engagement")}</Field.Label>
              <Select.Root type="single" bind:value={form.engagement_id}>
                <Select.Trigger id="work-engagement" class="w-full">{engagement ? `${engagement.ref} · ${engagement.title}` : "Choose…"}</Select.Trigger>
                <Select.Content>{#each site.engagements as en (en.id)}<Select.Item value={String(en.id)}>{en.ref} · {en.title}</Select.Item>{/each}</Select.Content>
              </Select.Root>
            </Field.Field>
            <Field.Field>
              <Field.Label for="work-about">About</Field.Label>
              <Select.Root type="single" bind:value={form.item_id}>
                <Select.Trigger id="work-about" class="w-full">{about ? about.title : "The website as a whole"}</Select.Trigger>
                <Select.Content class="max-h-72">
                  <Select.Item value="">The website as a whole</Select.Item>
                  {#each site.items as i (i.id)}<Select.Item value={String(i.id)}>{i.title}</Select.Item>{/each}
                </Select.Content>
              </Select.Root>
            </Field.Field>
            <Field.Field data-invalid={!!errors.title}>
              <Field.Label for="work-title">What needs doing</Field.Label>
              <Input id="work-title" bind:value={form.title} required autocomplete="off" aria-invalid={!!errors.title}
                placeholder={about ? `e.g. Get ${about.title.toLowerCase()} access from the client` : "What needs doing…"} />
              {#if errors.title}<Field.Error>Title {errors.title[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field>
              <Field.Label for="work-details">Details</Field.Label>
              <RichTextEditor id="work-details" bind:value={form.description} placeholder="Optional. @ to mention someone. A link back to this step is added." />
            </Field.Field>
            <div class="grid gap-4 sm:grid-cols-2">
              <Field.Field data-invalid={!!errors.owner}>
                <Field.Label for="work-lead">Lead</Field.Label>
                <Select.Root type="single" bind:value={form.owner_id}>
                  <Select.Trigger id="work-lead" class="w-full">{team.find((p) => String(p.id) === form.owner_id)?.name || "Unassigned"}</Select.Trigger>
                  <Select.Content class="max-h-72">
                    <Select.Item value="">Unassigned</Select.Item>
                    {#each team as p (p.id)}<Select.Item value={String(p.id)}>{p.name}</Select.Item>{/each}
                  </Select.Content>
                </Select.Root>
                {#if errors.owner}<Field.Error>Lead {errors.owner[0]}</Field.Error>{/if}
              </Field.Field>
              <Field.Field data-invalid={!!errors.due_on}>
                <Field.Label for="work-due">Due</Field.Label>
                <DatePicker id="work-due" bind:value={form.due_on} />
                {#if errors.due_on}<Field.Error>Due {errors.due_on[0]}</Field.Error>{/if}
              </Field.Field>
            </div>
            <div><Button type="submit" disabled={busy}>Add {word.toLowerCase()}</Button></div>
          </Field.Group>
        </form>
      {/if}
    </div>
  </Sheet.Content>
</Sheet.Root>
