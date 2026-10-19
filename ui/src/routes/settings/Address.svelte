<script>
  // Settings › Address: where people reach this install (every link Runwell
  // sends points there) and its time zone.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import { get, patch } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";

  let data = $state(null), url = $state(""), zone = $state(""), zones = $state([]), error = $state("");

  function fill(d) {
    data = d;
    url = d.host === "localhost" ? "" : d.url;
    zone = d.time_zone;
  }
  get("/settings/address").then(fill).catch(toastError);
  get("/time_zones").then((r) => (zones = r.zones)).catch(() => {});

  async function save(body) {
    error = "";
    try { const res = await patch("/settings/address", { setting: body }); fill(res); toast(res.summary); }
    catch (err) { error = errorMessage(err); }
  }
</script>

<SettingsLayout title="Address">
  {#if !data}
    <Skeleton class="h-64 w-full" />
  {:else}
    <Card.Root>
      <Card.Header>
        <Card.Title>Where people reach Runwell</Card.Title>
        <Card.Description>Every link Runwell sends points here: approval links, portal sign-ins, invitations and notifications. Use the address your team and {session.term("client", 2).toLowerCase()} type to open it.</Card.Description>
      </Card.Header>
      <Card.Content class="flex flex-col gap-3 text-sm">
        {#if !data.set}
          <p class="text-destructive">Not set yet, so links use {data.url}{data.last_seen_host ? ", the address you’ve been using" : ", which nobody else can open"}. Save it below to keep it.</p>
        {/if}
        <form onsubmit={(e) => { e.preventDefault(); save({ app_url: url }); }} class="flex flex-wrap gap-2">
          <Input type="url" bind:value={url} required placeholder={`https://${location.host}`} autocomplete="off" aria-label="Runwell’s address" class="min-w-64 flex-1" />
          <Button type="submit">Save</Button>
        </form>
        {#if error}<p class="text-destructive">{error}</p>{/if}
        {#if data.app_host_env}<p class="text-muted-foreground text-xs">The server’s APP_HOST is {data.app_host_env}; the address saved here takes its place.</p>{/if}
        {#if data.app_url_env}<p class="text-muted-foreground text-xs">The server’s APP_URL ({data.app_url_env}) is used for links while it’s set.</p>{/if}
      </Card.Content>
    </Card.Root>

    <Card.Root id="time_zone">
      <Card.Header>
        <Card.Title>Your time zone</Card.Title>
        <Card.Description>Times on your pages, in emails and in what agents read are shown here. A {session.term("client").toLowerCase()} in another zone can have its own; theirs is used in their portal and emails.</Card.Description>
      </Card.Header>
      <Card.Content class="flex flex-col gap-3 text-sm">
        <form onsubmit={(e) => { e.preventDefault(); save({ time_zone: zone }); }} class="flex flex-wrap gap-2">
          <Select.Root type="single" bind:value={zone}>
            <Select.Trigger class="min-w-64 flex-1" aria-label="Time zone">{zones.find((z) => z.name === zone)?.label || zone}</Select.Trigger>
            <Select.Content class="max-h-72">{#each zones as z (z.name)}<Select.Item value={z.name}>{z.label}</Select.Item>{/each}</Select.Content>
          </Select.Root>
          <Button type="submit">Save</Button>
        </form>
        {#if !data.time_zone_saved}<p class="text-muted-foreground text-xs">Not saved yet: {data.time_zone} is the {data.time_zone_default_from === "TIME_ZONE" ? "server’s TIME_ZONE" : "default"}.</p>{/if}
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>
