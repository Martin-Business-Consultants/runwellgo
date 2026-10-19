<script>
  // Quick actions (a, from anywhere): a Command menu of what to add (a
  // note, a document, a commitment, work, a request), then the action's
  // form in a dialog, on the record on screen or another one picked at its
  // top. The picker starts with the record on screen and the most recently
  // changed few; typing asks the server for matches, so the page never
  // carries every record.
  import * as Command from "$lib/components/ui/command/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Popover from "$lib/components/ui/popover/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Label } from "$lib/components/ui/label/index.js";
  import ChevronsUpDown from "@lucide/svelte/icons/chevrons-up-down";
  import MessageSquare from "@lucide/svelte/icons/message-square";
  import Paperclip from "@lucide/svelte/icons/paperclip";
  import Bookmark from "@lucide/svelte/icons/bookmark";
  import CircleCheck from "@lucide/svelte/icons/circle-check";
  import Inbox from "@lucide/svelte/icons/inbox";
  import Plus from "@lucide/svelte/icons/plus";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import { get, post } from "../api.js";
  import { session } from "../session.svelte.js";
  import { nav } from "../nav.svelte.js";
  import { router, go } from "../router.svelte.js";
  import { toast, toastError } from "../toast.svelte.js";
  import { focusInvalid } from "$lib/forms.js";

  const icons = { "message-square": MessageSquare, paperclip: Paperclip, bookmark: Bookmark, "circle-check": CircleCheck, inbox: Inbox };
  const kinds = ["internal", "call", "meeting", "email"];

  let menu = $state(false), open = $state(false), picking = $state(false), busy = $state(false);
  let actions = $state([]), action = $state(null);
  let record = $state(null), records = $state([]), q = $state("");
  let form = $state({}), files = $state(null), errors = $state({});
  let timer;

  // The record the screen is about, as "Type:id" (an engagement by its ref).
  const onScreen = $derived.by(() => {
    const m = router.path.match(/^\/(clients|engagements|work|requests|scope_items)\/([^/]+)$/);
    if (!m || m[2] === "new") return null;
    const type = { clients: "Client", engagements: "Engagement", work: "Todo", requests: "Request", scope_items: "ScopeItem" }[m[1]];
    return `${type}:${m[2]}`;
  });

  const typing = (e) => ["INPUT", "TEXTAREA", "SELECT"].includes(e.target.tagName) || e.target.isContentEditable;
  function onkeydown(e) {
    if (e.key !== "a" || e.metaKey || e.ctrlKey || e.altKey || typing(e) || document.querySelector("[role=dialog]")) return;
    e.preventDefault();
    showMenu();
  }
  async function showMenu() {
    if (!actions.length) {
      try { actions = (await get("/quick_actions")).actions; } catch (e) { toastError(e); return; }
    }
    menu = true;
  }

  function blank(key) {
    const week = new Date(Date.now() + 7 * 86400000);
    const ymd = `${week.getFullYear()}-${String(week.getMonth() + 1).padStart(2, "0")}-${String(week.getDate()).padStart(2, "0")}`;
    return {
      note: { body: "", kind: "internal", source: "" },
      document: {},
      commitment: { owner: "us", description: "", due_on: ymd, source: "" },
      work: { title: "", description: "", owner_id: String(session.me.person_id), due_on: "", other_owner_ids: [] },
      request: { subject: "", body: "", source: "email" },
    }[key] || {};
  }

  async function start(a) {
    menu = false;
    action = a; form = blank(a.key); files = null; errors = {}; q = ""; record = null; records = [];
    try {
      const res = await get(`/quick_actions/new?kind=${a.key}${onScreen ? "&record=" + encodeURIComponent(onScreen) : ""}`);
      record = res.record; records = res.records;
    } catch (e) { toastError(e); }
    open = true;
  }

  function search() {
    clearTimeout(timer);
    timer = setTimeout(async () => {
      try { records = (await get(`/quick_actions/records?types=${action.types.join(",")}&q=${encodeURIComponent(q.trim())}`)).records; } catch { records = []; }
    }, 180);
  }
  const pick = (r) => { record = r; picking = false; };
  const recordPath = (r) => ({ Client: `/clients/${r.id}`, Engagement: `/engagements/${r.ref}`, Todo: `/work/${r.id}`, Request: `/requests/${r.id}`, ScopeItem: `/scope_items/${r.id}` })[r.type];

  async function submit(e) {
    e.preventDefault();
    if (!record && action.key !== "request") { toastError(new Error("Choose where it goes first.")); return; }
    busy = true; errors = {};
    try {
      let res, to = record ? recordPath(record) : null;
      switch (action.key) {
        case "note":
          res = await post("/notes", { record: record.value, note: form });
          break;
        case "document": {
          const data = new FormData();
          data.append("record", record.value);
          for (const f of files || []) data.append("files", f);
          res = await post("/documents", data);
          break;
        }
        case "commitment":
          res = await post(record.type === "Engagement" ? `/engagements/${record.ref}/commitments` : `/clients/${record.id}/commitments`, { commitment: form });
          break;
        case "work":
          res = await post(`/engagements/${record.ref}/work`, { todo: { ...form, owner_id: form.owner_id === "none" ? null : Number(form.owner_id), due_on: form.due_on || null } });
          break;
        case "request":
          res = await post("/requests", { request: { ...form, client_id: record?.id || null } });
          to = `/requests/${res.request.id}`;
          break;
      }
      toast(res?.summary || "Added.");
      open = false;
      nav.refresh();
      if (to && to !== router.path) go(to);
    } catch (err) { errors = err.errors || {}; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
  const toggleOther = (id, on) => (form.other_owner_ids = on ? [...form.other_owner_ids, id] : form.other_owner_ids.filter((x) => x !== id));
  const placeholder = $derived(action ? "Find a " + action.types.map((t) => ({ Client: session.term("client"), Engagement: session.term("engagement"), ScopeItem: session.term("scope_item"), Todo: session.term("work"), Request: "request" })[t].toLowerCase()).join(" or ") + "…" : "");
  const people = $derived(session.me.team.map((p) => ({ value: String(p.id), label: p.name })));
</script>

<!-- The sidebar's "Add" item opens it too (a quick-actions event). -->
<svelte:window {onkeydown} onquickactions={showMenu} />

<Command.Dialog bind:open={menu} title="Quick actions" description="Add something to the record on screen, or another">
  <Command.Input placeholder="Add…" />
  <Command.List>
    <Command.Empty>Nothing matches.</Command.Empty>
    <Command.Group heading="Add">
      {#each actions as a (a.key)}
        {@const I = icons[a.icon] || Plus}
        <Command.Item value={a.title} onSelect={() => start(a)}><I />{a.key === "work" ? "Add " + session.term("work").toLowerCase() : a.key === "commitment" ? "Add a " + session.term("commitment").toLowerCase() : a.title}</Command.Item>
      {/each}
    </Command.Group>
  </Command.List>
</Command.Dialog>

<Dialog.Root bind:open>
  <Dialog.Content class="sm:max-w-lg">
    {#if action}
      <form onsubmit={submit} class="flex flex-col gap-4">
        <Dialog.Header>
          <Dialog.Title>{action.key === "work" ? "Add " + session.term("work").toLowerCase() : action.key === "commitment" ? "Add a " + session.term("commitment").toLowerCase() : action.title}</Dialog.Title>
          <Dialog.Description>On the record you pick.</Dialog.Description>
        </Dialog.Header>

        <Field.Field>
          <Field.Label>{action.key === "request" ? `${session.term("client")} (optional)` : "On"}</Field.Label>
          <Popover.Root bind:open={picking}>
            <Popover.Trigger>
              {#snippet child({ props })}
                <Button {...props} variant="outline" class="w-full justify-between font-normal" role="combobox" aria-expanded={picking}>
                  <span class="truncate">{#if record}<span class="text-muted-foreground me-1 text-xs">{record.kind}</span>{record.label}{:else}Choose…{/if}</span>
                  <ChevronsUpDown class="opacity-50" />
                </Button>
              {/snippet}
            </Popover.Trigger>
            <Popover.Content class="w-(--bits-popover-anchor-width) p-0" align="start">
              <Command.Root shouldFilter={false}>
                <Command.Input bind:value={q} oninput={search} placeholder={placeholder} />
                <Command.List>
                  <Command.Empty>Nothing matches.</Command.Empty>
                  {#each records as r (r.value)}
                    <Command.Item value={r.value} onSelect={() => pick(r)}>
                      <span class="text-muted-foreground text-xs whitespace-nowrap">{r.kind}</span><span class="truncate">{r.label}</span>
                    </Command.Item>
                  {/each}
                </Command.List>
              </Command.Root>
            </Popover.Content>
          </Popover.Root>
        </Field.Field>

        {#if action.key === "note"}
          <Field.Field data-invalid={!!errors.body}>
            <Field.Label for="qa-note-body">Note</Field.Label>
            <TiptapEditor id="qa-note-body" bind:value={form.body} format="html" placeholder="What was said or decided…" minHeight="min-h-28" />
            {#if errors.body}<Field.Error>Body {errors.body[0]}</Field.Error>{/if}
          </Field.Field>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field>
              <Field.Label for="qa-note-kind">Kind</Field.Label>
              <Select.Root type="single" bind:value={form.kind}>
                <Select.Trigger id="qa-note-kind" class="w-full capitalize">{form.kind}</Select.Trigger>
                <Select.Content>{#each kinds as k (k)}<Select.Item value={k} class="capitalize">{k}</Select.Item>{/each}</Select.Content>
              </Select.Root>
            </Field.Field>
            <Field.Field>
              <Field.Label for="qa-note-source">Source</Field.Label>
              <Input id="qa-note-source" bind:value={form.source} placeholder="call with Ann 24 Sep" autocomplete="off" />
            </Field.Field>
          </div>
        {:else if action.key === "document"}
          <Field.Field>
            <Field.Label for="qa-files">Files</Field.Label>
            <Input id="qa-files" type="file" multiple bind:files required />
            <Field.Description>Files start internal; share one with the client from its record.</Field.Description>
          </Field.Field>
        {:else if action.key === "commitment"}
          <RadioGroup.Root bind:value={form.owner} class="flex gap-4">
            <div class="flex items-center gap-2"><RadioGroup.Item value="us" id="qa-owner-us" /><Label for="qa-owner-us">We will</Label></div>
            <div class="flex items-center gap-2"><RadioGroup.Item value="client" id="qa-owner-client" /><Label for="qa-owner-client">They will</Label></div>
          </RadioGroup.Root>
          <Field.Field data-invalid={!!errors.description}>
            <Field.Label for="qa-commitment-what">What</Field.Label>
            <Input id="qa-commitment-what" bind:value={form.description} required placeholder="send the invoice" autocomplete="off" />
            {#if errors.description}<Field.Error>Description {errors.description[0]}</Field.Error>{/if}
          </Field.Field>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field data-invalid={!!errors.due_on}>
              <Field.Label for="qa-commitment-due">By</Field.Label>
              <DatePicker id="qa-commitment-due" bind:value={form.due_on} clearable={false} />
            </Field.Field>
            <Field.Field>
              <Field.Label for="qa-commitment-source">Where it was said</Field.Label>
              <Input id="qa-commitment-source" bind:value={form.source} placeholder="optional" autocomplete="off" />
            </Field.Field>
          </div>
        {:else if action.key === "work"}
          <Field.Field data-invalid={!!errors.title}>
            <Field.Label for="qa-work-title">What</Field.Label>
            <Input id="qa-work-title" bind:value={form.title} required placeholder="What needs doing…" autocomplete="off" />
            {#if errors.title}<Field.Error>Title {errors.title[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="qa-work-details">Details (optional)</Field.Label>
            <TiptapEditor id="qa-work-details" bind:value={form.description} format="html" minHeight="min-h-20" />
          </Field.Field>
          <div class="grid gap-4 sm:grid-cols-2">
            <Field.Field>
              <Field.Label for="qa-work-lead">Lead</Field.Label>
              <Select.Root type="single" bind:value={form.owner_id}>
                <Select.Trigger id="qa-work-lead" class="w-full">{people.find((p) => p.value === form.owner_id)?.label || "Unassigned"}</Select.Trigger>
                <Select.Content><Select.Item value="none">Unassigned</Select.Item>{#each people as p (p.value)}<Select.Item value={p.value}>{p.label}</Select.Item>{/each}</Select.Content>
              </Select.Root>
            </Field.Field>
            <Field.Field>
              <Field.Label for="qa-work-due">Due</Field.Label>
              <DatePicker id="qa-work-due" bind:value={form.due_on} />
            </Field.Field>
          </div>
          <Field.Set>
            <Field.Legend variant="label">Also on it</Field.Legend>
            <div class="flex flex-wrap gap-x-4 gap-y-2">
              {#each session.me.team as p (p.id)}
                <div class="flex items-center gap-2">
                  <Checkbox id={`qa-other-${p.id}`} checked={form.other_owner_ids.includes(p.id)} onCheckedChange={(v) => toggleOther(p.id, v)} />
                  <Label for={`qa-other-${p.id}`} class="font-normal">{p.name}</Label>
                </div>
              {/each}
            </div>
          </Field.Set>
        {:else if action.key === "request"}
          <Field.Field data-invalid={!!errors.subject}>
            <Field.Label for="qa-request-subject">Subject</Field.Label>
            <Input id="qa-request-subject" bind:value={form.subject} required placeholder="Can we add a newsletter signup?" autocomplete="off" />
            {#if errors.subject}<Field.Error>Subject {errors.subject[0]}</Field.Error>{/if}
          </Field.Field>
          <Field.Field>
            <Field.Label for="qa-request-body">Details</Field.Label>
            <TiptapEditor id="qa-request-body" bind:value={form.body} format="html" minHeight="min-h-20" />
          </Field.Field>
          <Field.Field data-invalid={!!errors.source}>
            <Field.Label for="qa-request-source">Source</Field.Label>
            <Input id="qa-request-source" bind:value={form.source} required placeholder="email: Re: newsletter, or call 9/24" autocomplete="off" />
            {#if errors.source}<Field.Error>Source {errors.source[0]}</Field.Error>{/if}
          </Field.Field>
        {/if}

        <Dialog.Footer>
          <Button type="button" variant="ghost" onclick={() => (open = false)}>Cancel</Button>
          <Button type="submit" disabled={busy}>
            {#if action.key === "note"}Add note{:else if action.key === "document"}Upload{:else if action.key === "commitment"}Add {session.term("commitment").toLowerCase()}{:else if action.key === "work"}Add {session.term("work").toLowerCase()}{:else if action.key === "request"}Log request{:else}Add{/if}
          </Button>
        </Dialog.Footer>
      </form>
    {/if}
  </Dialog.Content>
</Dialog.Root>
