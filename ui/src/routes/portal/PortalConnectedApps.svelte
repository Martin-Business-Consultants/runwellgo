<script>
  // The portal's Connected apps: the contact's own AI (Muse, Claude,
  // ChatGPT…), connected by a key made here (shown once) or by signing in
  // from the app; disconnect any.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import KeyRound from "@lucide/svelte/icons/key-round";
  import SecretBox from "$lib/components/SecretBox.svelte";
  import AgentsToken from "../agents/AgentsToken.svelte";
  import AgentsCode from "../agents/AgentsCode.svelte";
  import PortalLayout from "./PortalLayout.svelte";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { portal, pget, ppost, pdel } from "./portal.svelte.js";

  let data = $state(null), newToken = $state(null);
  let name = $state(""), readOnly = $state(false), busy = $state(false);

  async function load() {
    if (!(await portal.require())) return;
    try { data = await pget("/portal/connected_apps"); } catch (e) { toastError(e); }
  }
  load();

  async function create(e) {
    e.preventDefault();
    busy = true;
    try {
      const res = await ppost("/portal/connected_apps", { access_token: { name, read_only: readOnly } });
      newToken = res.new_token; data = res; name = ""; readOnly = false;
    } catch (err) { toastError(err); }
    busy = false;
  }

  async function disconnect(token) {
    if (!(await confirm({ title: `Disconnect ${token.name}?`, message: "It stops working at once. Anything it already did stays.", confirmLabel: "Disconnect", destructive: true }))) return;
    try { toast((await pdel(`/portal/connected_apps/${token.id}`)).notice); load(); } catch (e) { toastError(e); }
  }
</script>

<PortalLayout title="Connected apps">
  {#if !data}
    <Skeleton class="h-48" />
  {:else}
    {#if newToken}
      <Alert.Root>
        <KeyRound />
        <Alert.Title>Your new token</Alert.Title>
        <Alert.Description class="w-full">
          <SecretBox value={newToken.token} note="Copy it now: it won’t be shown again. It acts as you, in this portal only, until you disconnect it." />
        </Alert.Description>
      </Alert.Root>
    {/if}

    <Card.Root>
      <Card.Header>
        <Card.Title>Your AI, connected</Card.Title>
        <Card.Description>
          Your own assistant (Muse, Claude, ChatGPT and the like) can see what you see here and ask us for things, as you. Everything it does is recorded as done through it.
          {#if data.can_approve}
            {data.client_agent_approvals ? "It can decide on an agreement too, but only after showing it to you and taking your typed name." : "Agreements are decided here in the portal, not by an assistant."}
          {/if}
        </Card.Description>
      </Card.Header>
      <Card.Content>
        {#if data.tokens.length}
          <ul class="divide-y">
            {#each data.tokens as token (token.id)}
              <AgentsToken token={{ ...token, created_at: null }} ondisconnect={() => disconnect(token)} />
            {/each}
          </ul>
        {:else}
          <p class="text-muted-foreground text-sm">Nothing is connected yet.</p>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Connect one</Card.Title>
        <Card.Description>Give your assistant this address. If it can sign in by itself, it sends you here to approve it, after the usual emailed sign-in link.</Card.Description>
      </Card.Header>
      <Card.Content class="flex flex-col gap-4">
        <AgentsCode text={data.mcp_url} />
        <p class="text-sm">If it asks for a key instead (Muse does, for a server you describe to it), make one here and paste it in.</p>
        <form onsubmit={create} class="flex flex-wrap items-center gap-3">
          <Input bind:value={name} class="min-w-48 flex-1" autocomplete="off" placeholder="Name it, like “Muse”" aria-label="Token name" />
          <Field.Field orientation="horizontal" class="w-auto">
            <Checkbox id="portal-token-read-only" bind:checked={readOnly} />
            <Field.Label for="portal-token-read-only" class="font-normal">Read only</Field.Label>
          </Field.Field>
          <Button type="submit" disabled={busy}>Make a key</Button>
        </form>
      </Card.Content>
    </Card.Root>
  {/if}
</PortalLayout>
