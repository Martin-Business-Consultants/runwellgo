<script>
  // Settings › Reports: the Google Maps key (sealed, never shown again) and
  // an optional Map ID, the field that holds a client's address and how the
  // lookups went, home (the client marked "This is us", or an override),
  // and what clients see in their portal's progress report. Three tabs
  // (?tab=), each saving its own: Map, Addresses and home, Progress report.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import * as Tabs from "$lib/components/ui/tabs/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Eye from "@lucide/svelte/icons/eye";
  import PlacePicker from "./PlacePicker.svelte";
  import { get, patch, put, del, post, ApiError } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { rp, flag, placeName } from "./reports.js";
  import { pageTabs } from "$lib/page-tabs.svelte.js";

  let s = $state(null), errors = $state({}), keys = $state({ browser: "", server: "" }), mapId = $state(""), busy = $state(false);
  let clients = $state([]), previewId = $state("");
  const tabs = pageTabs(["map", "places", "portal"]);

  async function load() {
    try {
      s = await get(rp("/settings"));
      mapId = s.map_id;
    } catch (e) { toastError(e); }
  }
  load();
  get("/clients", { quiet: true }).then((r) => (clients = (r.clients || []).filter((c) => !c.internal))).catch(() => {});

  async function save(body, ok = "Saved.") {
    busy = true; errors = {};
    try { s = await patch(rp("/settings"), { settings: body }); mapId = s.map_id; toast(ok); }
    catch (e) { if (e instanceof ApiError && e.errors) errors = e.errors; else toastError(e); }
    busy = false;
  }
  async function saveKey(server) {
    busy = true; errors = {};
    try {
      toast((await put(rp("/settings/google_key"), { google: { key: server ? keys.server : keys.browser, server } })).summary);
      keys = { browser: "", server: "" };
      await load();
    } catch (e) { if (e instanceof ApiError && e.errors) errors = { [server ? "server_key" : "key"]: e.errors.key }; else toastError(e); }
    busy = false;
  }
  async function removeKey(server) {
    try { toast((await del(rp("/settings/google_key" + (server ? "?server=1" : "")))).summary); await load(); } catch (e) { toastError(e); }
  }
  async function lookUp() {
    try { toast((await post(rp("/geocode"), {})).summary); setTimeout(load, 4000); } catch (e) { toastError(e); }
  }
  const toggleSection = (key, on) => save({ portal: { on: s.portal.on, sections: on ? [...s.portal.sections, key] : s.portal.sections.filter((k) => k !== key) } });
  const fieldLabel = $derived(s ? (s.address_none ? "None: don’t place clients by address" : s.fields.find((f) => f.key === s.address_field)?.label || "None found") : "");
  const clientWord = session.term("client").toLowerCase();
</script>

<SettingsLayout title="Reports" description="The map, where your {session.term('client', 2).toLowerCase()} are, and the progress report they see in their portal.">
  {#if !s}
    <Skeleton class="h-48" /><Skeleton class="h-48" />
  {:else}
    <Tabs.Root value={tabs.value} onValueChange={tabs.set} class="gap-6">
      <Tabs.List aria-label="Reports">
        <Tabs.Trigger value="map">Map</Tabs.Trigger>
        <Tabs.Trigger value="places">Addresses and home</Tabs.Trigger>
        <Tabs.Trigger value="portal">Progress report{#if s.portal.on}<span class="text-muted-foreground text-xs">On</span>{/if}</Tabs.Trigger>
      </Tabs.List>
      <Tabs.Content value="map">
        <Card.Root>
          <Card.Header>
            <Card.Title>Map</Card.Title>
            <Card.Description>With a Google Maps key, Reports shows a street-level map. Without one, it shows the built-in globe, which works offline.</Card.Description>
          </Card.Header>
          <Card.Content class="flex flex-col gap-5">
            {#if !s.secrets_granted || !s.content_granted}
              <Alert.Root>
                <Alert.Title>Reports needs two more permissions for Google Maps</Alert.Title>
                <Alert.Description>In <a href="/settings/plugins" class="underline underline-offset-2">Settings › Plugins</a>, tick {!s.secrets_granted ? "“Keep secrets”" : ""}{!s.secrets_granted && !s.content_granted ? " and " : ""}{!s.content_granted ? "“Load content from other sites”" : ""} for Reports.</Alert.Description>
              </Alert.Root>
            {/if}
            <Field.Field data-invalid={errors.key ? true : undefined}>
              <Field.Label for="reports-key">Google Maps key</Field.Label>
              <div class="flex flex-wrap gap-2">
                <Input id="reports-key" type="password" class="min-w-0 flex-1 basis-60" bind:value={keys.browser} autocomplete="new-password" placeholder={s.google_key_set ? "Saved; paste a new one to replace it" : "AIza…"} />
                <Button onclick={() => saveKey(false)} disabled={busy || !keys.browser.trim()}>Save key</Button>
                {#if s.google_key_set}<Button variant="ghost" onclick={() => removeKey(false)}>Remove</Button>{/if}
              </div>
              {#if errors.key}<Field.Error>The key {errors.key}</Field.Error>{/if}
              <Field.Description>Kept sealed and never shown here again. The map runs in the browser, so the browser is sent this key to load it: in Google Cloud, enable the Maps JavaScript API for it and restrict it to this site’s address ({location.origin}/*).</Field.Description>
            </Field.Field>
            <Field.Field data-invalid={errors.server_key ? true : undefined}>
              <Field.Label for="reports-server-key">Key for looking up addresses <span class="text-muted-foreground font-normal">(optional)</span></Field.Label>
              <div class="flex flex-wrap gap-2">
                <Input id="reports-server-key" type="password" class="min-w-0 flex-1 basis-60" bind:value={keys.server} autocomplete="new-password" placeholder={s.google_server_key_set ? "Saved; paste a new one to replace it" : "Uses the Maps key when blank"} />
                <Button variant="outline" onclick={() => saveKey(true)} disabled={busy || !keys.server.trim()}>Save key</Button>
                {#if s.google_server_key_set}<Button variant="ghost" onclick={() => removeKey(true)}>Remove</Button>{/if}
              </div>
              {#if errors.server_key}<Field.Error>The key {errors.server_key}</Field.Error>{/if}
              <Field.Description>A second key, used only by this server for the Geocoding API, so the one the browser sees can’t look addresses up. Restrict it to the Geocoding API (and this server’s IP address).</Field.Description>
            </Field.Field>
            <Field.Field data-invalid={errors.map_id ? true : undefined}>
              <Field.Label for="reports-map-id">Map ID <span class="text-muted-foreground font-normal">(optional)</span></Field.Label>
              <div class="flex flex-wrap gap-2">
                <Input id="reports-map-id" class="min-w-0 flex-1 basis-60" bind:value={mapId} placeholder="For a map style of your own" />
                <Button variant="outline" onclick={() => save({ map_id: mapId })} disabled={busy || mapId === s.map_id}>Save</Button>
              </div>
              {#if errors.map_id}<Field.Error>The Map ID {errors.map_id}</Field.Error>{/if}
              <Field.Description>A vector map style from Google Cloud’s Map Management. Light and dark follow your theme either way.</Field.Description>
            </Field.Field>
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <Tabs.Content value="places" class="flex flex-col gap-6">
        <Card.Root>
          <Card.Header>
            <Card.Title>Addresses</Card.Title>
            <Card.Description>A {clientWord} goes on the map where someone put them by hand; else at their address; else, roughly, at their time zone’s city.</Card.Description>
          </Card.Header>
          <Card.Content class="flex flex-col gap-4">
            <Field.Field data-invalid={errors.address_field ? true : undefined}>
              <Field.Label for="reports-address-field">Which field holds a {clientWord}’s address</Field.Label>
              <Select.Root type="single" value={s.address_none ? "none" : s.address_field} onValueChange={(v) => save({ address_field: v }, "Saved. Looking addresses up again.")}>
                <Select.Trigger id="reports-address-field" class="w-full sm:w-80">{fieldLabel}</Select.Trigger>
                <Select.Content>
                  {#each s.fields as f (f.key)}<Select.Item value={f.key} label={f.label}>{f.label}</Select.Item>{/each}
                  <Select.Item value="none" label="None">None: don’t place {session.term("client", 2).toLowerCase()} by address</Select.Item>
                </Select.Content>
              </Select.Root>
              {#if errors.address_field}<Field.Error>The field {errors.address_field}</Field.Error>{/if}
              <Field.Description>
                {#if !s.fields.length}No {clientWord} fields yet: <a href="/settings/fields" class="underline underline-offset-2">add an Address field</a>.
                {:else if s.address_auto}Picked for you, as it says address. {/if}
                Looked up with {s.geocoder}: {s.geocoded} found{#if s.not_found}, {s.not_found} not found{/if}{#if s.failed}, {s.failed} failed (tried again within the hour){/if}.
              </Field.Description>
            </Field.Field>
            <Button variant="outline" size="sm" class="self-start" onclick={lookUp}>Look them all up again</Button>
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header>
            <Card.Title>Home</Card.Title>
            <Card.Description>Where the arcs on the map start: the {clientWord} marked “This is us”, at its address.</Card.Description>
          </Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            <p><span class="font-medium">{flag(s.home.country_code)} {placeName(s.home, { country: true })}</span>{#if s.home.from === "internal"}<span class="text-muted-foreground">, where <a class="underline underline-offset-2" href={`/clients/${s.home.client_id}`}>{s.home.client_name}</a> is.</span>
              {:else if s.home.from === "override"}<span class="text-muted-foreground">, set here.</span>
              {:else}<span class="text-muted-foreground">, your time zone’s city.</span>{/if}
            </p>
            {#if s.home.from !== "internal"}
              <p class="text-muted-foreground">
                {#if s.home.client_id}<a class="underline underline-offset-2" href={`/clients/${s.home.client_id}`}>{s.home.client_name}</a> has no address on the map yet: add one and home follows it. Until then, you can set home here.
                {:else}No {clientWord} is marked “This is us”. Mark yours (on its page, Edit) and give it an address, or set home here.{/if}
              </p>
              <PlacePicker path="/home" place={s.home.from === "override" ? s.home : null} onsaved={load} />
            {/if}
          </Card.Content>
        </Card.Root>
      </Tabs.Content>

      <Tabs.Content value="portal">
        <Card.Root>
          <Card.Header>
            <Card.Title>Clients’ progress report</Card.Title>
            <Card.Description>A page in each {clientWord}’s portal, “Your progress”, with what you choose here. Never their health score, your notes or anything about others.</Card.Description>
          </Card.Header>
          <Card.Content class="flex flex-col gap-4">
            {#if !s.portal_granted}
              <Alert.Root><Alert.Description>Reports may not serve clients yet: in <a href="/settings/plugins" class="underline underline-offset-2">Settings › Plugins</a>, tick “Serve clients in their portal” for Reports.</Alert.Description></Alert.Root>
            {/if}
            <Field.Field orientation="horizontal">
              <Switch id="reports-portal-on" checked={s.portal.on} disabled={busy} onCheckedChange={(on) => save({ portal: { on, sections: s.portal.sections } }, on ? "Clients see their progress report." : "Clients no longer see their progress report.")} />
              <Field.Label for="reports-portal-on">Show {session.term("client", 2).toLowerCase()} their progress report</Field.Label>
            </Field.Field>
            <Field.Set disabled={!s.portal.on}>
              <Field.Legend variant="label">What it shows</Field.Legend>
              <Field.Group class="gap-3">
                {#each s.sections as sec (sec.key)}
                  <Field.Field orientation="horizontal">
                    <Checkbox id="reports-section-{sec.key}" checked={s.portal.sections.includes(sec.key)} disabled={busy || !s.portal.on} onCheckedChange={(v) => toggleSection(sec.key, v)} />
                    <Field.Content><Field.Label for="reports-section-{sec.key}">{sec.label}</Field.Label><Field.Description>{sec.help}</Field.Description></Field.Content>
                  </Field.Field>
                {/each}
              </Field.Group>
            </Field.Set>
            {#if s.hidden_clients.length}
              <p class="text-muted-foreground text-sm">Switched off for {#each s.hidden_clients as c, i (c.id)}{i ? ", " : ""}<a class="underline underline-offset-2" href={c.path}>{c.name}</a>{/each}. Switch it back on from their page.</p>
            {:else}
              <p class="text-muted-foreground text-sm">Switch it off for one {clientWord} from their page.</p>
            {/if}
            <div class="flex flex-wrap items-center gap-2">
              <Select.Root type="single" bind:value={previewId}>
                <Select.Trigger class="w-full sm:w-64" aria-label="A {clientWord} to preview">{clients.find((c) => String(c.id) === previewId)?.name || `Choose a ${clientWord}`}</Select.Trigger>
                <Select.Content>{#each clients as c (c.id)}<Select.Item value={String(c.id)} label={c.name}>{c.name}</Select.Item>{/each}</Select.Content>
              </Select.Root>
              <Button variant="outline" href={previewId ? `/reports/clients/${previewId}/client_report` : undefined} disabled={!previewId}><Eye />See it as {clients.find((c) => String(c.id) === previewId)?.name || "they"} see{previewId ? "s" : ""} it</Button>
            </div>
          </Card.Content>
        </Card.Root>
      </Tabs.Content>
    </Tabs.Root>
  {/if}
</SettingsLayout>
