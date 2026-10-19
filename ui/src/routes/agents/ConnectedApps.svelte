<script>
  // Settings › Connected apps, for everyone: the AI apps and tokens acting as
  // you (a personal token is shown once, when it's made), what each did
  // lately, pausing and disconnecting them, and how to connect one. An owner
  // also sees everyone's connections, clients' included, and decides
  // whether clients' own assistants may decide agreements.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import KeyRound from "@lucide/svelte/icons/key-round";
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import SecretBox from "$lib/components/SecretBox.svelte";
  import AgentsToken from "./AgentsToken.svelte";
  import AgentsCode from "./AgentsCode.svelte";
  import { get, post, patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";

  let data = $state(null);
  let newToken = $state(null);
  let name = $state(""), readOnly = $state(false), busy = $state(false);

  async function load() {
    try { data = await get("/settings/connected_apps"); } catch (e) { toastError(e); }
  }
  load();

  async function create(e) {
    e.preventDefault();
    busy = true;
    try {
      const res = await post("/settings/connected_apps", { access_token: { name, read_only: readOnly } });
      newToken = res.new_token;
      data = res;
      name = ""; readOnly = false;
    } catch (err) { toastError(err); }
    busy = false;
  }

  async function disconnect(token) {
    if (!(await confirm({ title: `Disconnect ${token.name}?`, message: "It stops working at once. Anything it already did stays, marked with its name.", confirmLabel: "Disconnect", destructive: true }))) return;
    try { toast((await del(`/settings/connected_apps/${token.id}`)).notice); load(); } catch (e) { toastError(e); }
  }

  async function pause(token) {
    try {
      const res = token.paused ? await del(`/settings/connected_apps/${token.id}/pause`) : await post(`/settings/connected_apps/${token.id}/pause`);
      toast(res.notice);
      load();
    } catch (e) { toastError(e); }
  }

  async function setApprovals(on) {
    try {
      const res = await patch("/settings/client_agent_approvals", { setting: { client_agent_approvals: on } });
      data.client_agent_approvals = res.client_agent_approvals;
      toast(res.notice);
    } catch (e) { toastError(e); load(); }
  }
</script>

<SettingsLayout title="Connected apps" description="The AI apps and tokens acting as you, and how to connect one.">
  {#if !data}
    <Skeleton class="h-64" />
  {:else}
    {#if newToken}
      <Alert.Root>
        <KeyRound />
        <Alert.Title>Your new token</Alert.Title>
        <Alert.Description class="w-full">
          <SecretBox value={newToken.token} note={`Copy it now: it won’t be shown again. It acts as ${data.agent_name}, with your role, until you disconnect it.`} />
        </Alert.Description>
      </Alert.Root>
    {/if}

    <Card.Root>
      <Card.Header>
        <Card.Title>Apps acting as your agent</Card.Title>
        <Card.Description>Everything an app does with these is recorded as <strong>{data.agent_name}</strong>, with the app’s name, so your history shows what you did and what your AI did. Your agent has exactly your role.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if data.tokens.length}
          <ul class="divide-y">
            {#each data.tokens as token (token.id)}
              <AgentsToken {token} onpause={() => pause(token)} ondisconnect={() => disconnect(token)} />
            {/each}
          </ul>
        {:else}
          <p class="text-muted-foreground text-sm">Nothing is connected yet.</p>
        {/if}
      </Card.Content>
    </Card.Root>

    {#if session.can("manage_settings")}
      <Card.Root>
        <Card.Header>
          <Card.Title>{session.term("client", 2)}’ AI</Card.Title>
          <Card.Description>A contact with portal access can connect their own assistant under Connected apps in the portal, or by giving it <code class="font-mono text-xs">{data.portal_mcp_url}</code>. It sees what they see and can send requests, as them.</Card.Description>
        </Card.Header>
        <Card.Content>
          <Field.Field orientation="horizontal">
            <Field.Content>
              <Field.Label for="client-agent-approvals">Their assistants may decide agreements</Field.Label>
              <Field.Description>Only for a contact who can approve, after showing them the agreement and taking their typed name; the decision says it came through their assistant. Off, they decide in the portal or by the emailed link.</Field.Description>
            </Field.Content>
            <Switch id="client-agent-approvals" checked={data.client_agent_approvals} onCheckedChange={setApprovals} />
          </Field.Field>
        </Card.Content>
      </Card.Root>
    {/if}

    {#if data.everyone?.length}
      <Card.Root>
        <Card.Header>
          <Card.Title>Everyone’s connections <span class="text-muted-foreground font-normal tabular-nums">{data.everyone.length}</span></Card.Title>
          <Card.Description>Every app and token connected to Runwell by your team and by {session.term("client", 2).toLowerCase()}’ contacts. Pause one to stop it at once, or disconnect it. Each can make up to {data.rate_limit} calls a minute.</Card.Description>
        </Card.Header>
        <Card.Content>
          <ul class="divide-y">
            {#each data.everyone as token (token.id)}
              <AgentsToken {token} holder onpause={() => pause(token)} ondisconnect={() => disconnect(token)} />
            {/each}
          </ul>
        </Card.Content>
      </Card.Root>
    {/if}

    <Card.Root>
      <Card.Header>
        <Card.Title>Connect an AI app</Card.Title>
        <Card.Description>Runwell speaks MCP at <code class="font-mono text-xs">{data.mcp_url}</code>. Anything you can do here, an AI app connected as you can do too, and it asks before anything reaches a client.</Card.Description>
      </Card.Header>
      <Card.Content class="flex flex-col gap-4">
        <ul class="flex flex-col gap-3 text-sm">
          <li><strong>Claude or ChatGPT:</strong> add a custom connector with that URL. You’ll be asked to approve it here.</li>
          <li><strong>Muse, Grok and assistants that build their own connection:</strong> describe Runwell to it as an MCP server at that URL, and make a personal token below when it asks for a key.</li>
          <li class="flex flex-col gap-2">
            <span><strong>Claude Code:</strong> add it, then run <code class="font-mono text-xs">/mcp</code> in Claude Code to sign in.</span>
            <AgentsCode text={`claude mcp add --transport http runwell ${data.mcp_url}`} />
          </li>
          <li class="flex flex-col gap-2">
            <span><strong>The command line:</strong> install it, then sign in.</span>
            <AgentsCode text={`curl -fsSL ${data.cli_install_url} | sh\nrunwell login ${data.mcp_url.replace(/\/mcp$/, "")}`} />
          </li>
        </ul>
        <form onsubmit={create} class="flex flex-wrap items-center gap-3">
          <Input bind:value={name} class="min-w-48 flex-1" autocomplete="off" placeholder="Name it, like “My laptop” or “Zapier”" aria-label="Token name" />
          <Field.Field orientation="horizontal" class="w-auto">
            <Checkbox id="token-read-only" bind:checked={readOnly} />
            <Field.Label for="token-read-only" class="font-normal">Read only</Field.Label>
          </Field.Field>
          <Button type="submit" disabled={busy}>Make a personal token</Button>
        </form>
        <p class="text-muted-foreground text-xs">A personal token is for scripts and tools that can’t sign in through the browser.</p>
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>
