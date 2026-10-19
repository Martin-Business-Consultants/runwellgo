<script>
  // Settings › AI (ai/settings/show): the provider and its key (saved
  // sealed, never shown again), the owner's own address, the models picked
  // from the provider's list, letting clients ask, a monthly budget, a
  // connection test, this month's spend, and the clients kept out of AI.
  // Three tabs (?tab=): Setup (the one form that Save saves, and the
  // check), This month's spend, and Privacy (what it sees, who's kept out).
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Check from "@lucide/svelte/icons/check";
  import { staff, status } from "./ai.svelte.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError, undoable } from "$lib/toast.svelte.js";
  import ModelPicker from "./ModelPicker.svelte";
  import { timeAgo } from "$lib/format.js";
  import { pageTabs } from "$lib/page-tabs.svelte.js";

  let data = $state(null), form = $state({}), errors = $state({}), busy = $state(false), timer = null;
  const tabs = pageTabs(["setup", "usage", "privacy"], { hashes: { models: "setup", test: "setup" } });

  function fill(d, keep = false) {
    data = d;
    if (!keep) {
      const s = d.setting;
      form = { enabled: s.enabled, provider: s.provider || "", api_key: "", api_base: s.api_base || "", model: s.model || "", fast_model: s.fast_model || "",
        budget: s.monthly_budget_cents == null ? "" : (s.monthly_budget_cents / 100).toFixed(2), portal: s.portal };
    }
    clearTimeout(timer);
    if (d.models.loading || d.test.testing) timer = setTimeout(() => staff.get("/settings").then((x) => fill(x, true)).catch(() => {}), 2000);
  }
  const load = (keep) => staff.get("/settings").then((d) => fill(d, keep)).catch(toastError);
  load().then(tabs.scrollToHash);
  $effect(() => () => clearTimeout(timer));

  // cents is the budget as cents ("" for no limit); bad input goes as typed,
  // for the server to say so.
  function cents(v) {
    const t = String(v).trim().replace(/^\$/, "").replace(/,/g, "");
    if (t === "") return "";
    const n = Number(t);
    return Number.isFinite(n) && n >= 0 ? String(Math.round(n * 100)) : t;
  }
  async function save(e) {
    e?.preventDefault();
    busy = true;
    errors = {};
    const body = { enabled: form.enabled, provider: form.provider, api_base: form.api_base, model: form.model, fast_model: form.fast_model, portal: form.portal, monthly_budget_cents: cents(form.budget) };
    if (form.api_key) body.api_key = form.api_key;
    try {
      const r = await staff.patch("/settings", { ai_setting: body });
      toast(r.message);
      await load();
      status.load(true);
    } catch (err) {
      errors = err.errors;
      if (!Object.keys(errors).length) toastError(err); else tabs.invalid("setup");
    }
    busy = false;
  }
  async function act(path) {
    try { toast((await staff.post(path)).message); await load(true); } catch (e) { toastError(e); }
  }

  const provider = $derived(data?.providers.find((p) => p.key === form.provider));
  // The models to pick from: the saved provider's (its own list, or the
  // known ones), or, for one just chosen, its known ones.
  const saved = $derived(data && form.provider === data.setting.provider);
  const options = $derived(saved ? data.model_options.options : data?.known_models[form.provider] || []);
  const modelNote = $derived(saved ? data.model_options.note : "Showing known models: save to fetch the provider’s own list.");
  const HINTS = { anthropic: "Claude, from Anthropic", openai: "GPT models, from OpenAI", gemini: "Google’s Gemini models", openrouter: "Any model, through one key", opencode: "Claude, GPT, Gemini and open models on one key", ollama: "Your own server: nothing leaves it" };
  const shortName = (p) => p?.name.split(" (")[0];

  // A new provider starts from its own recommended models; Undo goes back.
  function changeProvider(v) {
    const before = { provider: data.setting.provider || "", model: form.model, fast_model: form.fast_model };
    if (!before.provider || before.provider === v) return save();
    form.model = "";
    form.fast_model = "";
    save().then(() => undoable(`Models reset to ${shortName(provider)}’s recommended ones.`, async () => {
      Object.assign(form, before);
      await save();
    }));
  }
  const clients = $derived(session.term("client", 2).toLowerCase());
  const err = (k) => errors[k]?.[0];
</script>


<SettingsLayout title="AI" description="An assistant on every page (press i) that answers from your records and does what you ask, as you: it can do only what you can, and shows every change for you to approve first. It also suggests on records: what a request should become, the scope of an agreement, the work a scope item needs, promises in a note, the day ahead. It uses your own account with an AI provider; nothing is sent anywhere until it’s on. For coding, use an AI on your own computer connected over MCP (Settings › Connected apps).">
  {#if !data}
    <Skeleton class="h-96 w-full" />
  {:else}
    <Tabs.Root value={tabs.value} onValueChange={tabs.set} class="gap-6">
      <Tabs.List aria-label="AI">
        <Tabs.Trigger value="setup">Setup</Tabs.Trigger>
        <Tabs.Trigger value="usage">This month <span class="text-muted-foreground text-xs tabular-nums">{data.spent}</span></Tabs.Trigger>
        <Tabs.Trigger value="privacy">Privacy{#if data.kept_out.length} <span class="text-muted-foreground text-xs tabular-nums">{data.kept_out.length} kept out</span>{/if}</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="setup" class="flex flex-col gap-6">
        <form onsubmit={save} class="flex flex-col gap-6">
          <Card.Root>
            <Card.Header><Card.Title>Provider</Card.Title></Card.Header>
            <Card.Content>
              <Field.Group>
                <Field.Field orientation="horizontal">
                  <Field.Content>
                    <Field.Label for="ai-enabled">Switch AI on</Field.Label>
                    <Field.Description>For everyone on the team. Turn it off and the assistant and suggestions disappear; chats stay.</Field.Description>
                  </Field.Content>
                  <Switch id="ai-enabled" bind:checked={form.enabled} />
                </Field.Field>
                <Field.Field data-invalid={err("provider") ? true : undefined}>
                  <Field.Label for="ai-provider">Provider</Field.Label>
                  <Select.Root type="single" bind:value={form.provider} onValueChange={changeProvider}>
                    <Select.Trigger id="ai-provider" class="w-full">{shortName(provider) || "Choose…"}</Select.Trigger>
                    <Select.Content>
                      {#each data.providers as p (p.key)}
                        <Select.Item value={p.key} label={shortName(p)}>
                          <span class="flex flex-col"><span>{shortName(p)}</span><span class="text-muted-foreground text-xs">{HINTS[p.key] || p.name}</span></span>
                        </Select.Item>
                      {/each}
                    </Select.Content>
                  </Select.Root>
                  {#if err("provider")}<Field.Error>Provider {err("provider")}</Field.Error>{/if}
                </Field.Field>
                <Field.Field data-invalid={err("api_key") ? true : undefined}>
                  <Field.Label for="ai-key">API key</Field.Label>
                  <Input id="ai-key" type="password" bind:value={form.api_key} autocomplete="new-password" placeholder={data.setting.api_key_saved ? "Saved; paste a new one to replace it" : "From your provider’s console"} />
                  {#if err("api_key")}<Field.Error>API key {err("api_key")}</Field.Error>{/if}
                  <Field.Description>Kept sealed with this install’s key; it’s never shown again.</Field.Description>
                </Field.Field>
                <Field.Field data-invalid={err("api_base") ? true : undefined}>
                  <Field.Label for="ai-base">Address (your own server, or a compatible one; leave blank otherwise)</Field.Label>
                  <Input id="ai-base" type="url" bind:value={form.api_base} autocomplete="off" placeholder={provider?.base || "https://api.openai.com/v1"} />
                  {#if err("api_base")}<Field.Error>Address {err("api_base")}</Field.Error>{/if}
                  {#if (form.api_base || form.provider === "ollama") && !data.private_network}
                    <Field.Description>Runwell reaches your own server only once an owner allows it: tick “Reach this server’s network” for AI in <a href="/settings/plugins" class="text-primary underline underline-offset-4">Settings › Plugins</a>.</Field.Description>
                  {/if}
                </Field.Field>
              </Field.Group>
            </Card.Content>
          </Card.Root>

          <Card.Root id="models">
            <Card.Header>
              <Card.Title>Models</Card.Title>
              <Card.Description>The main model answers the Ask panel and most suggestions; the fast one does small jobs, like summing up the day. With OpenCode Zen they can be from different families: Runwell reaches each the way Zen serves it.</Card.Description>
            </Card.Header>
            <Card.Content class="flex flex-col gap-3">
              {#if !form.provider}
                <p class="text-muted-foreground text-sm">Choose a provider first, and its models show here.</p>
              {:else}
                <div class="grid gap-4 sm:grid-cols-2">
                  <Field.Field>
                    <Field.Label for="ai-model">Model</Field.Label>
                    {#key form.provider}<ModelPicker id="ai-model" bind:value={form.model} {options} role="main" blank={`Recommended (${provider?.model})`} />{/key}
                  </Field.Field>
                  <Field.Field>
                    <Field.Label for="ai-fast">Fast model, for small jobs</Field.Label>
                    {#key form.provider}<ModelPicker id="ai-fast" bind:value={form.fast_model} {options} role="fast" blank={`Recommended (${provider?.fast})`} />{/key}
                  </Field.Field>
                </div>
                {#if data.models.loading && saved}
                  <p class="text-muted-foreground text-xs">Fetching {data.models.provider_name}’s models…</p>
                {:else if modelNote}
                  <p class="text-muted-foreground text-xs">{modelNote}</p>
                {:else}
                  <p class="text-muted-foreground text-xs">{options.length} {options.length === 1 ? "model" : "models"} from {data.models.provider_name}. A model a provider lists may still be unavailable: Test the connection after choosing.</p>
                {/if}
              {/if}
            </Card.Content>
          </Card.Root>

          <!-- Who may ask and what it may spend, side by side. -->
          <div class="grid gap-6 md:grid-cols-2">
            <Card.Root>
              <Card.Header><Card.Title>{session.term("client", 2)}</Card.Title></Card.Header>
              <Card.Content>
                <Field.Field orientation="horizontal">
                  <Field.Content>
                    <Field.Label for="ai-portal">Let {clients} ask too</Field.Label>
                    <Field.Description>An Ask button in their portal. It sees only what their portal shows, can send a request for the team (they approve it first), and never decides on an agreement.</Field.Description>
                  </Field.Content>
                  <Switch id="ai-portal" bind:checked={form.portal} />
                </Field.Field>
              </Card.Content>
            </Card.Root>

            <Card.Root>
              <Card.Header><Card.Title>Budget</Card.Title></Card.Header>
              <Card.Content>
                <Field.Field data-invalid={err("monthly_budget_cents") ? true : undefined} class="max-w-xs">
                  <Field.Label for="ai-budget">Monthly budget, in dollars (optional)</Field.Label>
                  <Input id="ai-budget" bind:value={form.budget} inputmode="decimal" autocomplete="off" placeholder="No limit" />
                  {#if err("monthly_budget_cents")}<Field.Error>Monthly budget {err("monthly_budget_cents")}</Field.Error>{/if}
                  <Field.Description>When this month’s spend reaches it, AI pauses until next month.</Field.Description>
                </Field.Field>
              </Card.Content>
            </Card.Root>
          </div>

          <div><Button type="submit" disabled={busy}>Save</Button></div>
        </form>

        <Card.Root id="test">
          <Card.Header><Card.Title>Check</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            <div class="flex flex-wrap gap-2">
              <Button variant="outline" size="sm" onclick={() => act("/settings/test")} disabled={!data.ready || data.test.testing}>Test the connection</Button>
              <Button variant="outline" size="sm" onclick={() => act("/settings/models")} disabled={!data.provider || data.models.loading}>Refresh the models</Button>
            </div>
            {#if data.test.testing}
              <p class="text-muted-foreground">Asking {data.test.provider_name}…</p>
            {:else if data.test.results?.length}
              {#each data.test.results as r (r.model)}
                {#if r.ok}
                  <p class="flex items-center gap-1.5"><Check class="size-4 shrink-0" />{r.route} answered in {r.seconds}s.</p>
                {:else}
                  <p class="text-destructive break-words">{r.route}: {r.error}</p>
                {/if}
              {/each}
              <p class="text-muted-foreground text-xs">Tested {timeAgo(data.test.at)}.</p>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <Tabs.Content value="usage">
        <Card.Root>
          <Card.Header><Card.Title>This month</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            <p><strong class="tabular-nums">{data.spent}</strong>{#if data.budget}{" "}of {data.budget}{/if}</p>
            {#if data.by_person.length}
              <ul class="flex flex-col divide-y">
                {#each data.by_person as p, i (i)}<li class="flex items-center gap-2 py-1.5"><span class="flex-1">{p.name}</span><span class="text-muted-foreground tabular-nums">{p.cost}</span></li>{/each}
              </ul>
              <p class="text-muted-foreground text-xs">Ask panel {data.by_kind.ask || "$0.000"} · suggestions {data.by_kind.suggestion || "$0.000"}{#if data.by_kind.agent} · agents {data.by_kind.agent}{/if}{#if data.by_kind.plugin} · other plugins {data.by_kind.plugin}{/if}</p>
            {:else}
              <p class="text-muted-foreground">Nothing yet.</p>
            {/if}
            {#if data.suggestions_used.length}<p class="text-muted-foreground text-xs">Suggestions used: {data.suggestions_used.join(" · ")}</p>{/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <Tabs.Content value="privacy" class="flex flex-col gap-6">
        <Card.Root>
          <Card.Header><Card.Title>What it sees</Card.Title></Card.Header>
          <Card.Content>
            <ul class="flex list-disc flex-col gap-1 ps-5 text-sm">
              <li>Only what the person using it may see, through the same actions an AI connected over MCP uses.</li>
              <li>Each record it reads, and your question, go to your provider. Choose Ollama to keep everything on your own server.</li>
              <li>A {session.term("client").toLowerCase()} marked “Keep out of AI” (in its side column) gets no assistant or suggestions on its records, and its records are left out of what the AI reads.</li>
              <li>Every change it makes was approved by a person, and shows in history as their agent “via AI”.</li>
            </ul>
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header><Card.Title>Kept out of AI</Card.Title></Card.Header>
          <Card.Content class="text-sm">
            {#if data.kept_out.length}
              <ul class="flex flex-col gap-1">{#each data.kept_out as c (c.id)}<li><a href={`/clients/${c.id}`} class="text-primary underline-offset-4 hover:underline">{c.display_name}</a></li>{/each}</ul>
            {:else}
              <p class="text-muted-foreground">No {clients}. Keep one out from the switch in its side column.</p>
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>
    </Tabs.Root>
  {/if}
</SettingsLayout>
