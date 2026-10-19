<script>
  // Settings › AI agents: the named assistants people @mention in a note or
  // in chat. Each has an identity (name, @handle, picture or initials, what
  // it's for), its behaviour (instructions, model), what it may use (read
  // only, or propose changes the person who asked approves; some actions
  // only; some clients' records only), who may mention it, and how much a
  // run may spend and how many steps it takes. Pause one and it answers
  // mentions with a polite no.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Sheet from "$lib/components/ui/sheet/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Combobox from "$lib/components/Combobox.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import Plus from "@lucide/svelte/icons/plus";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import Bot from "@lucide/svelte/icons/bot";
  import AgentAvatar from "./AgentAvatar.svelte";
  import ModelPicker from "./ModelPicker.svelte";
  import { staff } from "./ai.svelte.js";
  import { post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";

  let data = $state(null), open = $state(false), form = $state({}), errors = $state({}), busy = $state(false), editing = $state(null);
  let picture = $state(null);

  const load = () => staff.get("/agents?form=1").then((d) => (data = d)).catch(toastError);
  load();

  const ROLES = [["owner", "Owners"], ["manager", "Managers"], ["member", "Members"]];
  const blank = () => ({ name: "", handle: "", description: "", instructions: "", model: "main", tools: "read", allowed_tools: [], client_ids: [], roles: [], budget: "", max_steps: 8, paused: false, everyone: true, allClients: true });

  function edit(a = null) {
    editing = a;
    errors = {};
    form = a
      ? { ...blank(), ...a, budget: a.run_budget_cents == null ? "" : (a.run_budget_cents / 100).toFixed(2), everyone: !a.roles.length, allClients: !a.client_ids.length, client_ids: a.client_ids.map(String) }
      : blank();
    open = true;
  }

  // The handle follows the name until it's typed.
  let handleTyped = false;
  function nameChanged() {
    if (!editing && !handleTyped) form.handle = form.name.toLowerCase().normalize("NFKD").replace(/[^a-z0-9]+/g, "").slice(0, 30);
  }

  function cents(v) {
    const t = String(v).trim().replace(/^\$/, "");
    if (t === "") return "";
    const n = Number(t);
    return Number.isFinite(n) && n > 0 ? String(Math.round(n * 100)) : t;
  }

  async function save(e) {
    e?.preventDefault();
    busy = true;
    errors = {};
    const body = { name: form.name, handle: form.handle, description: form.description, instructions: form.instructions, model: form.model || "main", tools: form.tools,
      allowed_tools: form.allowed_tools, client_ids: form.allClients ? [] : form.client_ids, roles: form.everyone ? [] : form.roles, run_budget_cents: cents(form.budget),
      max_steps: Number(form.max_steps), paused: form.paused };
    try {
      const r = editing ? await staff.patch(`/agents/${editing.id}`, { agent: body }) : await staff.post("/agents", { agent: body });
      if (picture) {
        await post(`/x/ai/agents/${r.agent.id}/avatar?filename=${encodeURIComponent(picture.name)}`, picture);
        picture = null;
      }
      toast(r.message);
      open = false;
      await load();
    } catch (err) {
      errors = err.errors || {};
      if (!Object.keys(errors).length) toastError(err);
    }
    busy = false;
  }

  async function pause(a, paused) {
    try { toast((await staff.patch(`/agents/${a.id}`, { agent: { paused } })).message); await load(); } catch (e) { toastError(e); }
  }
  async function remove(a) {
    if (!(await confirm({ title: `Delete ${a.name}?`, message: `Mentions of @${a.handle} stop working. Its past runs stay, with what they cost.`, confirmLabel: "Delete", destructive: true }))) return;
    try { toast((await staff.del(`/agents/${a.id}`)).message); await load(); } catch (e) { toastError(e); }
  }
  async function dropPicture() {
    try { const r = await staff.del(`/agents/${editing.id}/avatar`); editing = { ...editing, avatar_url: null }; toast(r.message); await load(); } catch (e) { toastError(e); }
  }

  const err = (k) => errors[k]?.[0];
  const toolOptions = $derived((data?.form?.tools || []).filter((t) => form.tools === "propose" || t.read_only).map((t) => ({ value: t.name, label: t.title || t.name, hint: `${t.name}${t.read_only ? "" : " · changes something"}` })));
  const clientOptions = $derived((data?.form?.clients || []).map((c) => ({ value: String(c.id), label: c.name })));
  const modelExtra = $derived(data?.form ? [{ value: "main", label: "The main model", hint: data.form.main }, { value: "fast", label: "The fast model", hint: data.form.fast }] : []);
  const clientsWord = $derived(session.term("client", 2).toLowerCase());
  const preview = $derived(picture ? URL.createObjectURL(picture) : editing?.avatar_url);
</script>

{#snippet actions()}
  <Button size="sm" onclick={() => edit()} data-nav-new><Plus />New agent</Button>
{/snippet}

<SettingsLayout title="AI agents" description="Agents are assistants with a name that anyone allowed can @mention in a note on a record or in chat. Each one works in the background as the person who mentioned it, sees only what they can, and replies where it was mentioned. A change it proposes waits in that person’s Approvals until they approve it." {actions}>
  {#if !data}
    <Skeleton class="h-48 w-full" />
  {:else if !data.agents.length}
    <EmptyState icon={Bot} title="No agents yet." description="Make one, say a researcher who reads a record and sums it up, and mention it with @ in a note or in chat.">
      {#snippet actions()}<Button size="sm" onclick={() => edit()}><Plus />New agent</Button>{/snippet}
    </EmptyState>
  {:else}
    <ul class="grid gap-3 md:grid-cols-2">
      {#each data.agents as a (a.id)}
        <li>
          <Card.Root class="card-framed:h-full">
            <Card.Content class="flex flex-1 flex-row items-start gap-3">
              <AgentAvatar agent={a} size="size-10" />
              <div class="min-w-0 flex-1">
                <div class="flex flex-wrap items-center gap-2">
                  <button type="button" class="truncate text-sm font-semibold hover:underline" onclick={() => edit(a)}>{a.name}</button>
                  <span class="text-muted-foreground text-sm">@{a.handle}</span>
                  {#if a.paused}<Badge variant="outline">Paused</Badge>{/if}
                </div>
                <p class="text-muted-foreground mt-1 line-clamp-2 text-sm">{a.description || "No description."}</p>
                <p class="text-muted-foreground mt-2 text-xs">
                  {a.tools === "propose" ? "Reads, and proposes changes" : "Only reads"} ·
                  {a.model === "main" ? "Main model" : a.model === "fast" ? "Fast model" : a.model} ·
                  {a.roles.length ? a.roles.map((r) => ROLES.find((x) => x[0] === r)?.[1]).join(", ") : "Everyone"} may mention it
                </p>
              </div>
              <DropdownMenu.Root>
                <DropdownMenu.Trigger>
                  {#snippet child({ props })}<Button {...props} variant="ghost" size="icon" class="size-8" aria-label={`Actions for ${a.name}`}><Ellipsis /></Button>{/snippet}
                </DropdownMenu.Trigger>
                <DropdownMenu.Content align="end">
                  <DropdownMenu.Item onclick={() => edit(a)}>Edit</DropdownMenu.Item>
                  <DropdownMenu.Item onclick={() => pause(a, !a.paused)}>{a.paused ? "Resume" : "Pause"}</DropdownMenu.Item>
                  <DropdownMenu.Item href={`/settings/ai/runs?agent=${a.id}`}>Its runs</DropdownMenu.Item>
                  <DropdownMenu.Separator />
                  <DropdownMenu.Item variant="destructive" onclick={() => remove(a)}>Delete</DropdownMenu.Item>
                </DropdownMenu.Content>
              </DropdownMenu.Root>
            </Card.Content>
          </Card.Root>
        </li>
      {/each}
    </ul>
  {/if}
</SettingsLayout>

<Sheet.Root bind:open>
  <Sheet.Content side="right" class="w-full gap-0 overflow-y-auto p-0 data-[side=right]:sm:max-w-xl">
    <Sheet.Header class="border-b">
      <Sheet.Title>{editing ? `Edit ${editing.name}` : "New agent"}</Sheet.Title>
      <Sheet.Description>People mention it as @{form.handle || "handle"}.</Sheet.Description>
    </Sheet.Header>
    <form onsubmit={save} class="flex flex-col gap-6 p-4">
      <Field.Set>
        <Field.Legend>Identity</Field.Legend>
        <Field.Group>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field data-invalid={err("name") ? true : undefined}>
              <Field.Label for="agent-name">Name</Field.Label>
              <Input id="agent-name" bind:value={form.name} oninput={nameChanged} autocomplete="off" placeholder="Researcher" />
              {#if err("name")}<Field.Error>Name {err("name")}</Field.Error>{/if}
            </Field.Field>
            <Field.Field data-invalid={err("handle") ? true : undefined}>
              <Field.Label for="agent-handle">Handle</Field.Label>
              <Input id="agent-handle" bind:value={form.handle} oninput={() => (handleTyped = true)} autocomplete="off" spellcheck="false" placeholder="researcher" />
              {#if err("handle")}<Field.Error>Handle {err("handle")}</Field.Error>{:else}<Field.Description>What people type after @.</Field.Description>{/if}
            </Field.Field>
          </div>
          <Field.Field>
            <Field.Label for="agent-picture">Picture</Field.Label>
            <div class="flex items-center gap-3">
              <AgentAvatar agent={{ name: form.name || "Agent", avatar_url: preview }} size="size-10" />
              <Input id="agent-picture" type="file" accept="image/png,image/jpeg,image/gif,image/webp" class="max-w-64" onchange={(e) => (picture = e.currentTarget.files?.[0] || null)} />
              {#if editing?.avatar_url && !picture}<Button type="button" variant="ghost" size="sm" onclick={dropPicture}>Use initials</Button>{/if}
            </div>
            <Field.Description>Optional: without one it shows its initials.</Field.Description>
          </Field.Field>
          <Field.Field>
            <Field.Label for="agent-description">What it’s for</Field.Label>
            <Input id="agent-description" bind:value={form.description} autocomplete="off" placeholder="Reads a record and sums up where things stand" />
            <Field.Description>Shown beside it when people mention it.</Field.Description>
          </Field.Field>
        </Field.Group>
      </Field.Set>

      <Field.Set>
        <Field.Legend>Behaviour</Field.Legend>
        <Field.Group>
          <Field.Field data-invalid={err("instructions") ? true : undefined}>
            <Field.Label for="agent-instructions">Instructions</Field.Label>
            <Textarea id="agent-instructions" bind:value={form.instructions} rows={6} placeholder="You research our records. When asked about a piece of work, read it and its notes, then answer in three bullets: where it stands, what's blocking it, what's next." />
            {#if err("instructions")}<Field.Error>Instructions {err("instructions")}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="agent-model">Model</Field.Label>
            <ModelPicker id="agent-model" bind:value={form.model} options={data?.form?.models?.options || []} extra={modelExtra} blank="The main model" />
            {#if data?.form?.models?.note}<Field.Description>{data.form.models.note}</Field.Description>{/if}
          </Field.Field>
        </Field.Group>
      </Field.Set>

      <Field.Set>
        <Field.Legend>Tools and records</Field.Legend>
        <Field.Group>
          <RadioGroup.Root bind:value={form.tools} class="gap-3">
            <Field.Field orientation="horizontal">
              <RadioGroup.Item value="read" id="agent-read" />
              <Field.Content><Field.Label for="agent-read">Only read</Field.Label><Field.Description>It looks things up and answers; it can’t change anything.</Field.Description></Field.Content>
            </Field.Field>
            <Field.Field orientation="horizontal">
              <RadioGroup.Item value="propose" id="agent-propose" />
              <Field.Content><Field.Label for="agent-propose">Read, and propose changes</Field.Label><Field.Description>Each change waits in the Approvals of the person who mentioned it, and runs as them once they approve it.</Field.Description></Field.Content>
            </Field.Field>
          </RadioGroup.Root>
          <Field.Field>
            <Field.Label for="agent-tools">Only these actions (optional)</Field.Label>
            <Combobox id="agent-tools" multiple bind:value={form.allowed_tools} options={toolOptions} placeholder="Every action the person may use" />
            <Field.Description>Leave empty for all of the person’s actions{form.tools === "read" ? " that only read" : ""}.</Field.Description>
          </Field.Field>
          <Field.Field orientation="horizontal">
            <Switch id="agent-all-clients" bind:checked={form.allClients} />
            <Field.Content><Field.Label for="agent-all-clients">Every {session.term("client").toLowerCase()}’s records</Field.Label><Field.Description>Those kept out of AI are always left out.</Field.Description></Field.Content>
          </Field.Field>
          {#if !form.allClients}
            <Field.Field>
              <Field.Label for="agent-clients">Only these {clientsWord}</Field.Label>
              <Combobox id="agent-clients" multiple bind:value={form.client_ids} options={clientOptions} placeholder={`Choose ${clientsWord}…`} />
            </Field.Field>
          {/if}
        </Field.Group>
      </Field.Set>

      <Field.Set>
        <Field.Legend>Who and how much</Field.Legend>
        <Field.Group>
          <Field.Field orientation="horizontal">
            <Switch id="agent-everyone" bind:checked={form.everyone} />
            <Field.Content><Field.Label for="agent-everyone">Everyone may mention it</Field.Label></Field.Content>
          </Field.Field>
          {#if !form.everyone}
            <div class="flex flex-wrap gap-4" role="group" aria-label="Roles that may mention it">
              {#each ROLES as [r, label] (r)}
                <Field.Field orientation="horizontal" class="w-auto">
                  <Checkbox id="agent-role-{r}" checked={form.roles.includes(r)} onCheckedChange={(v) => (form.roles = v ? [...form.roles, r] : form.roles.filter((x) => x !== r))} />
                  <Field.Label for="agent-role-{r}">{label}</Field.Label>
                </Field.Field>
              {/each}
            </div>
            {#if err("roles")}<p class="text-destructive text-sm">Roles {err("roles")}</p>{/if}
          {/if}
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field data-invalid={err("run_budget_cents") ? true : undefined}>
              <Field.Label for="agent-budget">Most one run may spend, in dollars</Field.Label>
              <Input id="agent-budget" bind:value={form.budget} inputmode="decimal" autocomplete="off" placeholder="No limit but the monthly one" />
              {#if err("run_budget_cents")}<Field.Error>Budget {err("run_budget_cents")}</Field.Error>{/if}
            </Field.Field>
            <Field.Field data-invalid={err("max_steps") ? true : undefined}>
              <Field.Label for="agent-steps">Most steps in one run</Field.Label>
              <Input id="agent-steps" type="number" min="1" max="30" bind:value={form.max_steps} />
              {#if err("max_steps")}<Field.Error>Steps {err("max_steps")}</Field.Error>{:else}<Field.Description>Each step is one answer from the model.</Field.Description>{/if}
            </Field.Field>
          </div>
          <Field.Field orientation="horizontal">
            <Switch id="agent-paused" bind:checked={form.paused} />
            <Field.Content><Field.Label for="agent-paused">Paused</Field.Label><Field.Description>Mentions are answered with a polite no until it’s resumed.</Field.Description></Field.Content>
          </Field.Field>
        </Field.Group>
      </Field.Set>

      <div class="flex gap-2">
        <Button type="submit" disabled={busy}>{editing ? "Save" : "Create agent"}</Button>
        <Button type="button" variant="ghost" onclick={() => (open = false)}>Cancel</Button>
      </div>
    </form>
  </Sheet.Content>
</Sheet.Root>
