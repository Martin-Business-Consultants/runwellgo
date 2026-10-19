<script>
  // Settings › Sign-in: let people sign in with Google, Microsoft, GitHub or
  // single sign-on as well, or only through them. For each, the owner
  // registers an app with the provider (the callback address is here to
  // copy) and saves its keys.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import SecretBox from "$lib/components/SecretBox.svelte";
  import { get, patch } from "$lib/api.js";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";

  let data = $state(null), forms = $state({}), errors = $state({});

  function load() {
    return get("/settings/sign_in").then((d) => {
      data = d;
      forms = Object.fromEntries(d.providers.map((p) => [p.key, { client_id: p.client_id, client_secret: "", tenant_id: p.tenant_id, issuer: p.issuer, label: p.label, enabled: p.enabled }]));
    }).catch(toastError);
  }
  load();

  async function save(p, e) {
    e.preventDefault();
    errors = {};
    try { toast((await patch(`/settings/sign_in_providers/${p.key}`, { sign_in_provider: forms[p.key] })).summary); load(); }
    catch (err) { errors = { [p.key]: errorMessage(err) }; }
  }
  async function setOnly(on) {
    try { toast((await patch("/settings/providers_only", { setting: { providers_only: on } })).summary); }
    catch (err) { toastError(err); }
    load();
  }
</script>

<SettingsLayout title="Sign-in" description="Everyone signs in with a password, and can switch on emailed links in Settings › Your account. Here you can add Google, Microsoft, GitHub or single sign-on as well. People connect their own accounts, or are matched by an address the provider has verified. Nobody joins this way: accounts still start with an invitation, and two-factor sign-in still applies.">
  {#if !data}
    <Skeleton class="h-96 w-full" />
  {:else}
    <Card.Root id="providers-only">
      <Card.Content>
        <Field.Field orientation="horizontal">
          <Field.Content>
            <Field.Label for="providers-only-switch">Only sign in through these</Field.Label>
            <Field.Description>Passwords and emailed links stop working for everyone but owners, who keep their password in case a provider is down. Switch on at least one provider below first.</Field.Description>
          </Field.Content>
          <Switch id="providers-only-switch" checked={data.providers_only} onCheckedChange={setOnly} />
        </Field.Field>
      </Card.Content>
    </Card.Root>

    {#each data.providers as p (p.key)}
      <Card.Root id={p.key}>
        <Card.Header>
          <Card.Title>{p.name}</Card.Title>
          <Card.Action>{#if p.usable}<Badge>On</Badge>{:else}<Badge variant="outline">Off</Badge>{/if}</Card.Action>
        </Card.Header>
        <Card.Content class="flex flex-col gap-4">
          <ol class="flex list-decimal flex-col gap-2 ps-5 text-sm">
            {#if p.console}
              <li>Create an OAuth app in <a href={p.console} target="_blank" rel="noopener" class="underline underline-offset-4">the {p.name} console</a>.</li>
            {:else}
              <li>In your identity provider (Okta, Auth0, Keycloak, JumpCloud, Google Workspace or any other that speaks OpenID Connect), add a web application.</li>
            {/if}
            <li><div class="flex flex-col gap-2">Give it this callback (redirect) address:<SecretBox value={p.callback_url} /></div></li>
            <li>Paste its client ID and secret here, and switch it on.</li>
          </ol>
          <form onsubmit={(e) => save(p, e)}>
            <Field.Group>
              {#if p.key === "openid_connect"}
                <div class="grid gap-4 sm:grid-cols-2">
                  <Field.Field><Field.Label for={`${p.key}-label`}>Button name</Field.Label><Input id={`${p.key}-label`} bind:value={forms[p.key].label} autocomplete="off" placeholder="Okta" /></Field.Field>
                  <Field.Field><Field.Label for={`${p.key}-issuer`}>Issuer address</Field.Label><Input id={`${p.key}-issuer`} type="url" bind:value={forms[p.key].issuer} autocomplete="off" spellcheck="false" placeholder="https://acme.okta.com" /></Field.Field>
                </div>
              {/if}
              <div class="grid gap-4 sm:grid-cols-2">
                <Field.Field><Field.Label for={`${p.key}-id`}>Client ID</Field.Label><Input id={`${p.key}-id`} bind:value={forms[p.key].client_id} autocomplete="off" spellcheck="false" /></Field.Field>
                <Field.Field><Field.Label for={`${p.key}-secret`}>Client secret</Field.Label><Input id={`${p.key}-secret`} type="password" bind:value={forms[p.key].client_secret} autocomplete="new-password" placeholder={p.secret_saved ? "Saved: leave blank to keep it" : ""} /></Field.Field>
              </div>
              {#if p.key === "entra_id"}
                <Field.Field><Field.Label for={`${p.key}-tenant`}>Tenant ID (optional)</Field.Label><Input id={`${p.key}-tenant`} bind:value={forms[p.key].tenant_id} autocomplete="off" spellcheck="false" placeholder="common: any Microsoft work or personal account" /></Field.Field>
              {/if}
              <Field.Field orientation="horizontal">
                <Checkbox id={`${p.key}-enabled`} bind:checked={forms[p.key].enabled} />
                <Field.Label for={`${p.key}-enabled`} class="font-normal">Show “Continue with {p.name}” on the sign-in page</Field.Label>
              </Field.Field>
              {#if errors[p.key]}<p class="text-destructive text-sm">{errors[p.key]}</p>{/if}
              <div><Button type="submit">Save</Button></div>
            </Field.Group>
          </form>
        </Card.Content>
      </Card.Root>
    {/each}
  {/if}
</SettingsLayout>
