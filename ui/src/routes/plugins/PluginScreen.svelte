<script module>
  // A plugin's screens and the routes the person may write to, fetched once
  // per plugin while the app is open.
  const defs = new Map();
  export function pluginScreens(key, get) {
    if (!defs.has(key)) defs.set(key, get(`/plugin_screens/${key}`, { quiet: true }).then((r) => r.plugin).catch((e) => { defs.delete(key); throw e; }));
    return defs.get(key);
  }
  export const forgetPluginScreens = () => defs.clear();
</script>

<script>
  // A plugin's screen, drawn from its manifest's description: data, never
  // code (docs/plugins.md). It calls the screen's source route with its
  // query (record=Client:12 on a panel) and draws the parts the screen has,
  // in order: text, stats, detail, form, table, from shadcn's components.
  // Filters set a query parameter; forms post to the plugin's routes and
  // mark the fields a 422 names; row and header actions open a screen or
  // call a route, confirming first when the screen says so. Only what the
  // person may call is offered.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Textarea } from "$lib/components/ui/textarea/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import EllipsisVertical from "@lucide/svelte/icons/ellipsis-vertical";
  import StatusBadge from "$lib/components/StatusBadge.svelte";
  import DatePicker from "$lib/components/DatePicker.svelte";
  import { api, get } from "$lib/api.js";
  import { href, go, withQuery } from "$lib/router.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { date, dateTime, timeAgo } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";

  // plugin: its key; screen: the screen's id; query: its parameters (the
  // page's query string, or record= on a panel); page: drawn as a page of
  // its own (filters change the address); onload(screen, plugin) when it's
  // known; onsaved after a form is saved.
  let { plugin, screen, query = {}, page = false, onload = undefined, onsaved = undefined } = $props();

  let def = $state(null), meta = $state(null), data = $state(null), missing = $state(false);
  let values = $state({}), errors = $state({}), busy = $state(false);
  // svelte-ignore state_referenced_locally
  let q = $state({ ...query });

  const route = (path, params = {}) => {
    const s = new URLSearchParams(Object.entries(params).filter(([, v]) => v !== "" && v != null)).toString();
    return `/x/${plugin}${path}${s ? (path.includes("?") ? "&" : "?") + s : ""}`;
  };
  const canWrite = (method, path) => !!meta?.writes.includes(`${(method || "POST").toUpperCase()} ${path.split("?")[0]}`);
  const screenHref = (id, params = {}) => href(`/p/${plugin}/${id}`, params);
  // {id: "id"} → {id: row.id}: an action or link's params, read from a row.
  const paramsFrom = (map, row) => Object.fromEntries(Object.entries(map || {}).map(([k, field]) => [k, row?.[field]]));
  const today = () => { const d = new Date(); return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`; };

  function initial(form, answer) {
    const v = { ...((form.record && answer?.[form.record]) || {}) };
    for (const f of form.fields) {
      if (v[f.name] == null) v[f.name] = f.type === "checkbox" ? false : f.default === "today" && f.type === "date" ? today() : "";
    }
    return v;
  }

  async function load({ keepValues = false } = {}) {
    try {
      meta = await pluginScreens(plugin, get);
      def = meta.screens.find((s) => s.id === screen) || null;
      if (!def) { missing = true; return; }
      onload?.(def, meta);
      data = await get(route(def.source, q));
      if (def.form && !keepValues) values = initial(def.form, data);
    } catch (e) {
      if (e.status === 404 || e.status === 403) missing = true;
      else toastError(e);
    }
  }
  load();

  function setParam(param, value) {
    // On a page the address keeps the filter too (the screen isn't made
    // afresh for it: it reads again here).
    if (page) go(withQuery({ [param]: value }));
    q = { ...q, [param]: value };
    load();
  }

  async function submit(e) {
    e.preventDefault();
    const fm = def.form;
    busy = true; errors = {};
    try {
      const res = await api((fm.method || "POST").toUpperCase(), route(fm.save, q), { [fm.record || "values"]: $state.snapshot(values) });
      toast(res?.message || "Saved.");
      // A log starts again empty; a record being edited keeps what was typed.
      await load({ keepValues: !fm.reset });
      onsaved?.(res);
    } catch (err) {
      errors = err.errors || {};
      if (!Object.keys(errors).length) toastError(err); else focusInvalid();
    }
    busy = false;
  }

  async function run(a, row) {
    if (a.screen) return go(screenHref(a.screen, paramsFrom(a.params, row)));
    if (a.confirm && !(await confirm({ title: a.label + "?", message: a.confirm, confirmLabel: a.label, destructive: !!a.danger }))) return;
    try {
      const res = await api((a.method || "POST").toUpperCase(), route(a.post, { ...q, ...paramsFrom(a.params, row) }), a.body || undefined);
      toast(res?.message || "Done.");
      load({ keepValues: true });
    } catch (e) { toastError(e); }
  }

  const offered = (a, row) => (a.screen || canWrite(a.method, a.post)) && (!a.show_if || !row || row[a.show_if]);
  const filterOptions = (f) => f.options || data?.[f.options_from] || [];
  const fid = (f) => `plugin-${plugin}-${screen}-${f.name}`;
</script>

{#snippet value(v, it)}
  {#if v == null || v === ""}<span class="text-muted-foreground">—</span>
  {:else if it.badge}<StatusBadge status={v} />
  {:else if it.date}{date(v)}
  {:else if it.time}<span title={dateTime(v)}>{timeAgo(v)}</span>
  {:else}{v}{/if}
{/snippet}

{#if missing}
  <p class="text-muted-foreground text-sm">This isn't here: the plugin may be off, or your role can't open it.</p>
{:else if !def || !data}
  <Skeleton class="h-32" />
{:else}
  <div class="flex flex-col gap-4">
    {#if def.filters?.length || def.actions?.length}
      <div class="flex flex-wrap items-center justify-between gap-2">
        <div class="flex flex-wrap items-center gap-3">
          {#each def.filters || [] as f (f.param)}
            {@const options = filterOptions(f)}
            {#if options.length}
              <div class="flex items-center gap-1" role="group" aria-label={f.label}>
                {#each options as o (o.value)}
                  {@const current = (q[f.param] ?? data[f.param] ?? options[0].value) === o.value}
                  <Button size="sm" variant={current ? "secondary" : "ghost"} aria-pressed={current} onclick={() => setParam(f.param, o.value)}>{o.label}</Button>
                {/each}
              </div>
            {/if}
          {/each}
        </div>
        <div class="flex flex-wrap gap-2">
          {#each (def.actions || []).filter((a) => offered(a)) as a (a.label)}
            <Button size="sm" variant={a.danger ? "destructive" : "outline"} onclick={() => run(a, data)}>{a.label}</Button>
          {/each}
        </div>
      </div>
    {/if}

    {#if def.text && data[def.text]}
      <p class="text-muted-foreground text-sm whitespace-pre-line">{data[def.text]}</p>
    {/if}

    {#if def.stats && data[def.stats]?.length}
      <div class="grid grid-cols-2 gap-3 sm:grid-cols-3 lg:grid-cols-4">
        {#each data[def.stats] as s, i (i)}
          <Card.Root class="gap-1 py-3">
            <Card.Content class="px-4">
              <p class="text-muted-foreground truncate text-xs">{s.label}</p>
              <p class="text-lg font-semibold tabular-nums">{s.value}</p>
              {#if s.hint}<p class="text-muted-foreground text-xs">{s.hint}</p>{/if}
            </Card.Content>
          </Card.Root>
        {/each}
      </div>
    {/if}

    {#if def.detail && data[def.detail.items]?.length}
      <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
        {#each data[def.detail.items] as it, i (i)}
          <dt class="text-muted-foreground">{it.label}</dt>
          <dd class="min-w-0 break-words {it.multiline ? 'whitespace-pre-wrap' : ''}">
            {#if it.href && it.value}<a href={it.href} class="hover:underline">{it.value}</a>{:else}{@render value(it.value, it)}{/if}
          </dd>
        {/each}
      </dl>
    {/if}

    {#if def.form && canWrite(def.form.method, def.form.save)}
      {@const fm = def.form}
      <form onsubmit={submit}>
        <Field.Group class="gap-4">
          {#if errors.base}<p class="text-destructive text-sm">{errors.base.join(" ")}</p>{/if}
          <div class="grid gap-4 {fm.fields.length > 1 ? 'sm:grid-cols-2' : ''}">
            {#each fm.fields as f (f.name)}
              <Field.Field data-invalid={!!errors[f.name]} class={f.type === "textarea" || (fm.fields.length % 2 === 1 && f === fm.fields.at(-1)) ? "sm:col-span-2" : ""}
                orientation={f.type === "checkbox" ? "horizontal" : undefined}>
                {#if f.type === "checkbox"}
                  <Checkbox id={fid(f)} bind:checked={values[f.name]} />
                  <Field.Label for={fid(f)}>{f.label}</Field.Label>
                {:else}
                  <Field.Label for={fid(f)}>{f.label}</Field.Label>
                  {#if f.type === "textarea"}
                    <Textarea id={fid(f)} bind:value={values[f.name]} placeholder={f.placeholder} required={f.required} aria-invalid={!!errors[f.name]} />
                  {:else if f.type === "date"}
                    <DatePicker id={fid(f)} bind:value={values[f.name]} invalid={!!errors[f.name]} clearable={!f.required} />
                  {:else if f.type === "select"}
                    {@const options = f.options || data[f.options_from] || []}
                    <Select.Root type="single" bind:value={values[f.name]}>
                      <Select.Trigger id={fid(f)} class="w-full" aria-invalid={!!errors[f.name]}>{options.find((o) => o.value === values[f.name])?.label || f.placeholder || "Choose…"}</Select.Trigger>
                      <Select.Content>{#each options as o (o.value)}<Select.Item value={o.value}>{o.label}</Select.Item>{/each}</Select.Content>
                    </Select.Root>
                  {:else}
                    <Input id={fid(f)} type={f.type || "text"} bind:value={values[f.name]} placeholder={f.placeholder} required={f.required} autocomplete="off" aria-invalid={!!errors[f.name]} />
                  {/if}
                {/if}
                {#if f.help}<Field.Description>{f.help}</Field.Description>{/if}
                {#if errors[f.name]}<Field.Error>{errors[f.name][0].match(/^[A-Z]/) ? errors[f.name][0] : `${f.label} ${errors[f.name][0]}`}</Field.Error>{/if}
              </Field.Field>
            {/each}
          </div>
          <div><Button type="submit" size="sm" disabled={busy}>{fm.submit || "Save"}</Button></div>
        </Field.Group>
      </form>
    {/if}

    {#if def.table}
      {@const t = def.table}
      {@const rows = data[t.rows] || []}
      {#if !rows.length}
        <p class="text-muted-foreground text-sm">{t.empty || "Nothing here yet."}</p>
      {:else}
        <div class="overflow-x-auto rounded-md border">
          <Table.Root>
            <Table.Header>
              <Table.Row>
                {#each t.columns as c (c.key)}<Table.Head class={c.num ? "text-end" : ""}>{c.label}</Table.Head>{/each}
                {#if t.row_actions?.length}<Table.Head class="w-10"><span class="sr-only">Actions</span></Table.Head>{/if}
              </Table.Row>
            </Table.Header>
            <Table.Body>
              {#each rows as r, ri (r[t.key || "id"] ?? ri)}
                {@const acts = (t.row_actions || []).filter((a) => offered(a, r))}
                <Table.Row>
                  {#each t.columns as c, ci (c.key)}
                    <Table.Cell class="{c.num ? 'text-end tabular-nums' : ''} {ci === 0 ? 'font-medium' : ''}">
                      {#if c.link}<a class="hover:underline" href={screenHref(c.link.screen, paramsFrom(c.link.params, r))}>{r[c.key] ?? ""}</a>
                      {:else if c.href && r[c.href]}<a class="hover:underline" href={r[c.href]}>{r[c.key] ?? ""}</a>
                      {:else}{@render value(r[c.key], c)}{/if}
                      {#if c.sub && r[c.sub]}<p class="text-muted-foreground line-clamp-1 text-xs font-normal">{r[c.sub]}</p>{/if}
                    </Table.Cell>
                  {/each}
                  {#if t.row_actions?.length}
                    <Table.Cell>
                      {#if acts.length}
                        <DropdownMenu.Root>
                          <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm"><EllipsisVertical /><span class="sr-only">Actions</span></Button>{/snippet}</DropdownMenu.Trigger>
                          <DropdownMenu.Content align="end">
                            {#each acts as a (a.label)}<DropdownMenu.Item variant={a.danger ? "destructive" : "default"} onSelect={() => run(a, r)}>{a.label}</DropdownMenu.Item>{/each}
                          </DropdownMenu.Content>
                        </DropdownMenu.Root>
                      {/if}
                    </Table.Cell>
                  {/if}
                </Table.Row>
              {/each}
            </Table.Body>
          </Table.Root>
        </div>
      {/if}
    {/if}
  </div>
{/if}
