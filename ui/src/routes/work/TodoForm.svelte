<script>
  // Changing a piece of work (/work/:id/edit), or adding one to an
  // engagement (/work/new?engagement=WO-12): the form in the working column,
  // its status, due date, people, scope item and visibility down the side,
  // then what it belongs to, Make this a commitment, and Remove.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Label } from "$lib/components/ui/label/index.js";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import CustomFieldInputs from "$lib/components/CustomFieldInputs.svelte";
  import MakeCommitment from "./MakeCommitment.svelte";
  import NotFound from "../NotFound.svelte";
  import { get, post, patch, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { router, go, href } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { humanize, date } from "$lib/format.js";
  import { STATUSES } from "./work.js";
  import { focusInvalid } from "$lib/forms.js";

  let { params } = $props();
  // A screen is made afresh for each path, so its params never change.
  // svelte-ignore state_referenced_locally
  const id = params.id;
  const editing = !!id;
  const initialRef = router.query.get("engagement") || "";
  let ref = $state(initialRef);
  let form = $state({ title: "", description: "", status: "planned", due_on: "", owner_id: "", other_owner_ids: [], scope_item_id: "", client_visible: true, custom_fields: {} });
  let missing = $state(false);
  let todo = $state(null), items = $state([]), errors = $state({}), busy = $state(false), loaded = $state(!editing);
  const team = $derived(session.me.team || []);
  const back = $derived(editing ? `/work/${id}` : ref ? `/engagements/${ref}?tab=work` : "/work");
  const word = session.term("work").toLowerCase();

  page.set(editing ? "Edit" : "Add " + word, [{ label: session.term("work"), href: "/work" }]);
  if (editing) {
    get(`/work/${id}`).then(({ todo: t }) => {
      todo = t;
      items = t.agreed_items;
      form = { title: t.title, description: t.description_html || "", status: t.status, due_on: t.due_on || "", owner_id: t.owner ? String(t.owner.id) : "",
        other_owner_ids: t.other_owners.map((p) => p.id), scope_item_id: t.scope_item ? String(t.scope_item.id) : "", client_visible: t.client_visible, custom_fields: { ...t.custom_fields } };
      page.set("Edit", [{ label: session.term("work"), href: "/work" }, { label: t.title, href: `/work/${id}` }]);
      loaded = true;
    }).catch((e) => { if (e.status === 404) missing = true; else toastError(e); });
  } else if (initialRef) {
    loadItems();
  }
  async function loadItems() {
    try {
      const r = await get(href("/work", { engagement: ref, status: "all" }));
      items = r.scope_items || [];
      const item = router.query.get("item");
      if (item && items.some((i) => String(i.id) === item)) form.scope_item_id = item;
    } catch (e) { items = []; toastError(e); }
  }

  const others = (pid, on) => (form.other_owner_ids = on ? [...form.other_owner_ids, pid] : form.other_owner_ids.filter((x) => x !== pid));
  const nameOf = (pid) => team.find((p) => String(p.id) === String(pid))?.name;

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    const body = { ...form, owner_id: form.owner_id || null, scope_item_id: form.scope_item_id || null, other_owner_ids: form.other_owner_ids.filter((x) => String(x) !== String(form.owner_id)) };
    try {
      const r = editing ? await patch(`/work/${id}`, { todo: body }) : await post(`/engagements/${encodeURIComponent(ref)}/work`, { todo: body });
      toast(editing ? "Changes saved." : r.summary);
      go(`/work/${r.todo.id}`);
    } catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
  async function remove() {
    if (!(await confirm({ title: `Remove ${todo.title}?`, message: "It comes off the engagement and the client’s portal. Its history stays.", confirmLabel: "Remove", destructive: true }))) return;
    try { await del(`/work/${id}`); toast(`Removed ${todo.title}.`); go(`/engagements/${todo.engagement.ref}?tab=work`); } catch (e) { toastError(e); }
  }
</script>

<svelte:window onkeydown={(e) => { if (e.key === "Escape" && !document.querySelector("[role=dialog],[role=listbox],[role=menu]")) go(back); }} />
{#if missing}
  <NotFound title="Couldn’t find it" back={{ href: "/work", label: `All ${session.term("work", 2).toLowerCase()}` }} />
{:else}
<form onsubmit={save} class="grid items-start gap-4 lg:grid-cols-3">
  <Card.Root class="lg:col-span-2">
    <Card.Header><Card.Title class="text-xl">{editing ? `Edit ${todo?.title || ""}` : "Add " + word}</Card.Title></Card.Header>
    <Card.Content>
      {#if loaded}
        <Field.Group>
          {#if errors.base}<p class="text-destructive text-sm">{errors.base.join(" ")}</p>{/if}
          {#if !editing}
            <Field.Field>
              <Field.Label for="todo-ref">{session.term("engagement")}</Field.Label>
              <Input id="todo-ref" bind:value={ref} required placeholder="WO-12" autocomplete="off" onchange={loadItems} />
            </Field.Field>
          {/if}
          <Field.Field data-invalid={!!errors.title}>
            <Field.Label for="todo-title">Title</Field.Label>
            <Input id="todo-title" bind:value={form.title} required autofocus autocomplete="off" placeholder="What needs doing…" aria-invalid={!!errors.title} />
            {#if errors.title}<Field.Error>Title {errors.title[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="todo-description">Description</Field.Label>
            <TiptapEditor id="todo-description" bind:value={form.description} format="html" />
          </Field.Field>
          <CustomFieldInputs model="Todo" bind:values={form.custom_fields} />
        </Field.Group>
      {/if}
    </Card.Content>
    <Card.Footer class="gap-2">
      <Button type="submit" disabled={busy || !loaded}>{editing ? "Save changes" : "Add " + word}</Button>
      <Button href={back} variant="ghost">Cancel</Button>
    </Card.Footer>
  </Card.Root>

  <div class="flex flex-col gap-4">
    {#if loaded}
      <Card.Root>
        <Card.Header><Card.Title>Status and people</Card.Title></Card.Header>
        <Card.Content>
          <Field.Group>
            <Field.Field data-invalid={!!errors.status}>
              <Field.Label for="todo-status">Status</Field.Label>
              <Select.Root type="single" bind:value={form.status}>
                <Select.Trigger id="todo-status" class="w-full">{humanize(form.status)}</Select.Trigger>
                <Select.Content>{#each STATUSES as s (s)}<Select.Item value={s}>{humanize(s)}</Select.Item>{/each}</Select.Content>
              </Select.Root>
              {#if errors.status}<Field.Error>Status {errors.status[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field>
              <Field.Label for="todo-due">Due</Field.Label>
              <DatePicker id="todo-due" bind:value={form.due_on} placeholder="No date" />
            </Field.Field>
            <Field.Field>
              <Field.Label for="todo-lead">Lead</Field.Label>
              <Select.Root type="single" bind:value={form.owner_id}>
                <Select.Trigger id="todo-lead" class="w-full">{nameOf(form.owner_id) || "Unassigned"}</Select.Trigger>
                <Select.Content>
                  <Select.Item value="">Unassigned</Select.Item>
                  {#each team as p (p.id)}<Select.Item value={String(p.id)}>{p.name}</Select.Item>{/each}
                </Select.Content>
              </Select.Root>
            </Field.Field>
            <Field.Set>
              <Field.Legend variant="label">Also on it</Field.Legend>
              <div class="flex flex-col gap-2">
                {#each team.filter((p) => String(p.id) !== String(form.owner_id)) as p (p.id)}
                  <Label class="flex items-center gap-2 font-normal">
                    <Checkbox checked={form.other_owner_ids.includes(p.id)} onCheckedChange={(v) => others(p.id, v)} />
                    <PersonAvatar person={p} size="size-5" />{p.name}
                  </Label>
                {/each}
              </div>
            </Field.Set>
            <Field.Field>
              <Field.Label for="todo-item">{session.term("scope_item")}</Field.Label>
              <Select.Root type="single" bind:value={form.scope_item_id}>
                <Select.Trigger id="todo-item" class="w-full"><span class="truncate">{items.find((i) => String(i.id) === form.scope_item_id)?.description || "None"}</span></Select.Trigger>
                <Select.Content>
                  <Select.Item value="">None</Select.Item>
                  {#each items as item (item.id)}<Select.Item value={String(item.id)}>{item.description}</Select.Item>{/each}
                </Select.Content>
              </Select.Root>
            </Field.Field>
            <Field.Field orientation="horizontal">
              <Switch id="todo-visible" bind:checked={form.client_visible} />
              <Field.Label for="todo-visible">Visible to the client in the portal</Field.Label>
            </Field.Field>
          </Field.Group>
        </Card.Content>
      </Card.Root>
    {/if}
    {#if todo}
      <Card.Root>
        <Card.Header><Card.Title>Details</Card.Title></Card.Header>
        <Card.Content>
          <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt class="text-muted-foreground">{session.term("engagement")}</dt><dd><a class="hover:underline" href={`/engagements/${todo.engagement.ref}`}>{todo.engagement.display_name}</a></dd>
            <dt class="text-muted-foreground">{session.term("client")}</dt><dd><a class="hover:underline" href={`/clients/${todo.client.id}`}>{todo.client.display_name}</a></dd>
            <dt class="text-muted-foreground">Added</dt><dd>{date(todo.created_at)}{todo.created_by ? ` by ${todo.created_by.name}` : ""}</dd>
            {#if todo.completed_at}<dt class="text-muted-foreground">Done</dt><dd>{date(todo.completed_at)}</dd>{/if}
          </dl>
        </Card.Content>
      </Card.Root>
      {#if todo.status !== "done"}<MakeCommitment {todo} />{/if}
      {#if session.can("delete_records")}
        <Card.Root>
          <Card.Header><Card.Title>Remove</Card.Title><Card.Description>It comes off the {session.term("engagement").toLowerCase()} and the client’s portal. Its history stays.</Card.Description></Card.Header>
          <Card.Content><Button type="button" variant="destructive" size="sm" onclick={remove}>Remove this {word}</Button></Card.Content>
        </Card.Root>
      {/if}
    {:else if !editing}
      <Card.Root>
        <Card.Header><Card.Title>Where work comes from</Card.Title></Card.Header>
        <Card.Content class="text-muted-foreground text-sm">Approving an agreement adds one per {session.term("scope_item").toLowerCase()}. Add anything else here; it shows in the client’s portal unless you make it internal.</Card.Content>
      </Card.Root>
    {/if}
  </div>
</form>
{/if}
