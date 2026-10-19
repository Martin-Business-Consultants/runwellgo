<script>
  // Settings › Names: the install's own words for its core things and its
  // engagement types (with their ref prefixes), whether agreements carry
  // prices, and starting setups to begin from again.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import { get, patch } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";

  const TERMS = ["client", "engagement", "scope_item", "work", "commitment"];
  const LABELS = ["project", "work_order", "service"];
  let data = $state(null), form = $state(null), busy = $state(false);

  function fill(d) {
    data = d;
    const t = d.terminology || {};
    form = {
      ...Object.fromEntries(TERMS.map((k) => [k, { one: t[k]?.one || "", other: t[k]?.other || "" }])),
      labels: Object.fromEntries(LABELS.map((l) => {
        const x = t.labels?.[l] || {};
        return [l, { one: x.one || "", other: x.other || "", prefix: x.prefix || "", enabled: x.enabled !== false }];
      })),
    };
  }
  get("/settings/names").then(fill).catch(toastError);

  async function refreshSession() {
    try { session.apply(await get("/session", { quiet: true })); } catch { /* stays */ }
  }

  async function save(e) {
    e.preventDefault();
    busy = true;
    try { const res = await patch("/settings/names", { terminology: form }); fill(res); toast(res.summary); refreshSession(); }
    catch (err) { toastError(err); }
    busy = false;
  }
  async function setPrices(on) {
    try { const res = await patch("/settings/prices", { setting: { prices: on } }); toast(res.summary); data.prices = res.prices; refreshSession(); }
    catch (err) { toastError(err); }
  }
  async function start(s) {
    if (!(await confirm({ title: `Start from ${s.name.toLowerCase()}?`, message: "Every name above, and the prices switch, is replaced with this setup’s. Your records stay as they are; only the words change.", confirmLabel: "Use these" }))) return;
    try { const res = await patch("/settings/start", { start: s.key }); fill(res); toast(res.summary); refreshSession(); }
    catch (err) { toastError(err); }
  }
</script>

<SettingsLayout title="Names" description="Use your own words for the core things. Leave a field blank to keep the default.">
  {#if !form}
    <Skeleton class="h-96 w-full" />
  {:else}
    <form onsubmit={save}>
      <Card.Root>
        <Card.Header><Card.Title>What things are called</Card.Title></Card.Header>
        <Card.Content>
          <Field.Group>
            {#each TERMS as key (key)}
              <Field.Field>
                <Field.Label>{data.defaults.terms[key].one}</Field.Label>
                <div class="grid gap-2 sm:grid-cols-2">
                  <Input bind:value={form[key].one} placeholder={data.defaults.terms[key].one} autocomplete="off" aria-label="Singular" />
                  <Input bind:value={form[key].other} placeholder={data.defaults.terms[key].other} autocomplete="off" aria-label="Plural" />
                </div>
              </Field.Field>
            {/each}
            <Field.Separator />
            <Field.Set>
              <Field.Legend>{data.terms.engagement.one} types</Field.Legend>
              <Field.Description>Each type has its own name and ref prefix. A new prefix applies to new refs only; existing refs keep theirs.</Field.Description>
              {#each LABELS as l (l)}
                <div class="flex items-center gap-2">
                  <Switch bind:checked={form.labels[l].enabled} aria-label={`Use ${data.defaults.labels[l].one}`} />
                  <Input bind:value={form.labels[l].one} placeholder={data.defaults.labels[l].one} autocomplete="off" aria-label="Singular" class="flex-1" />
                  <Input bind:value={form.labels[l].other} placeholder={data.defaults.labels[l].other} autocomplete="off" aria-label="Plural" class="flex-1" />
                  <Input bind:value={form.labels[l].prefix} placeholder={data.defaults.labels[l].prefix} autocomplete="off" maxlength={4} aria-label="Ref prefix" class="w-20" />
                </div>
              {/each}
            </Field.Set>
          </Field.Group>
        </Card.Content>
        <Card.Footer><Button type="submit" disabled={busy}>Save names</Button></Card.Footer>
      </Card.Root>
    </form>

    <Card.Root id="prices">
      <Card.Header><Card.Title>Prices</Card.Title></Card.Header>
      <Card.Content>
        <Field.Field orientation="horizontal">
          <Field.Content>
            <Field.Label for="prices">Agreements have prices</Field.Label>
            <Field.Description>On, {data.terms.scope_item.other.toLowerCase()} take a price and agreements show totals, wherever they have some: one with no prices reads as a plain list of what’s agreed. Off, money is hidden everywhere, for planning without it: a home, a wedding, internal work.</Field.Description>
          </Field.Content>
          <Switch id="prices" checked={data.prices} onCheckedChange={setPrices} />
        </Field.Field>
      </Card.Content>
    </Card.Root>

    <Card.Root id="start">
      <Card.Header>
        <Card.Title>Start from</Card.Title>
        <Card.Description>Replace every name above, and the prices switch, with a starting setup’s. Your records stay as they are; only the words change.</Card.Description>
      </Card.Header>
      <Card.Content class="grid gap-3 sm:grid-cols-3">
        {#each data.starts as s (s.key)}
          <div class="flex flex-col gap-3 rounded-md border p-4">
            <div class="flex-1"><p class="font-medium">{s.name}</p><p class="text-muted-foreground text-sm">{s.description}</p></div>
            <div><Button variant="outline" size="sm" onclick={() => start(s)}>Use these</Button></div>
          </div>
        {/each}
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>
