<script>
  // Help › Connecting an MCP: plugging an AI assistant into Runwell, the
  // command line, and clients' own assistants.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import SecretBox from "$lib/components/SecretBox.svelte";
  import { get } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";

  let help = $state(null);
  get("/settings/help").then((h) => (help = h)).catch(() => {});
  const origin = location.origin;
  const mcp = $derived(help?.mcp_url || origin + "/mcp");
  const portalMcp = $derived(help?.portal_mcp_url || origin + "/portal/mcp");
  const cli = $derived(help?.cli_install_url || origin + "/install/cli");
  const agent = $derived(help?.agent_name || "your agent");
  const client = $derived(session.term("client").toLowerCase());
  const clients = $derived(session.term("client", 2).toLowerCase());
  const json = $derived(JSON.stringify({ mcpServers: { runwell: { url: mcp, headers: { Authorization: "Bearer rw_your_token" } } } }, null, 2));
</script>

<SettingsLayout title="Connecting an MCP">
  <Card.Root>
    <Card.Header><Card.Title>What it is</Card.Title></Card.Header>
    <Card.Content class="flex flex-col gap-3 text-sm leading-relaxed">
      <p>Runwell speaks MCP, the way AI assistants plug into the systems people use. Connect yours once and it can read and do anything you can here, as <strong>{agent}</strong>, with your role: no more, no less.</p>
      <p>Runwell’s address for assistants:</p>
      <SecretBox value={mcp} />
      <p class="text-muted-foreground">Everything it does is marked in the history as {agent}, with the assistant’s name. Anything that reaches a {client} (sending an agreement, emailing a link, showing something in their portal) is shown to you first and done only when you agree. You can see what each connection did, pause it or disconnect it in <a href="/settings/connected_apps" class="underline underline-offset-4">Connected apps</a>.</p>
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header><Card.Title>Claude and ChatGPT</Card.Title></Card.Header>
    <Card.Content class="text-sm leading-relaxed">
      <ol class="list-decimal space-y-1 ps-5">
        <li>In Claude, open Settings → Connectors and add a custom connector. In ChatGPT, add it under Settings → Connectors (or Apps).</li>
        <li>Paste the address above and save.</li>
        <li>It opens Runwell and asks you to approve it. Tick <em>Read only</em> if it should only look.</li>
        <li>That’s it: it keeps its own access fresh. Ask it “What needs me in Runwell today?”</li>
      </ol>
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header><Card.Title>Muse, Grok and assistants that build their own connection</Card.Title><Card.Description>Some assistants connect to anything you describe, and ask for a key rather than signing in.</Card.Description></Card.Header>
    <Card.Content class="text-sm leading-relaxed">
      <ol class="list-decimal space-y-1 ps-5">
        <li>In <a href="/settings/connected_apps" class="underline underline-offset-4">Connected apps</a>, make a personal token named after the assistant (“Muse”). Copy it: it’s shown once.</li>
        <li>Tell the assistant: “Connect to Runwell, an MCP server over Streamable HTTP at {mcp}. It needs a bearer token.”</li>
        <li>When it asks for the key, paste the token into its secure credentials prompt, not into the chat.</li>
        <li>Ask it to save the connection as a skill, so it’s there next time.</li>
      </ol>
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header><Card.Title>Claude Code, Cursor and other coding tools</Card.Title></Card.Header>
    <Card.Content class="flex flex-col gap-3 text-sm leading-relaxed">
      <p>Claude Code: add Runwell, then run <code>/mcp</code> in Claude Code to sign in.</p>
      <SecretBox value={`claude mcp add --transport http runwell ${mcp}`} />
      <p>A tool configured with JSON takes the address and, if it can’t sign in by itself, a personal token:</p>
      <pre class="bg-muted overflow-x-auto rounded-md border p-3 font-mono text-xs">{json}</pre>
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header><Card.Title>The command line</Card.Title><Card.Description>For you, for scripts, and for any assistant that runs commands. Every Runwell tool is a command (<code>runwell tools</code> lists them).</Card.Description></Card.Header>
    <Card.Content class="flex flex-col gap-3 text-sm">
      <pre class="bg-muted overflow-x-auto rounded-md border p-3 font-mono text-xs">{`curl -fsSL ${cli} | sh\nrunwell login\nrunwell agent setup   # teaches Claude Code to use it`}</pre>
      <p class="text-muted-foreground"><code>runwell login --read-only</code> signs in to look only; <code>runwell doctor</code> checks everything.</p>
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header><Card.Title>Your {clients}’ assistants</Card.Title></Card.Header>
    <Card.Content class="flex flex-col gap-3 text-sm leading-relaxed">
      <p>A contact with portal access can connect their own assistant to their portal, from <strong>Connected apps</strong> in the portal, or by giving it:</p>
      <SecretBox value={portalMcp} />
      <p>It sees what they see there and can send you requests, as them; never anything internal or another {client}’s.
        {#if help?.client_agent_approvals}A contact who can approve may have it decide an agreement, after it shows them the agreement and takes their typed name.{:else}Agreements are decided in the portal or by the emailed link, not by an assistant.{/if}
        {#if session.can("manage_settings")}Change that in <a href="/settings/connected_apps" class="underline underline-offset-4">Connected apps</a>.{/if}
      </p>
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header><Card.Title>If something’s wrong</Card.Title></Card.Header>
    <Card.Content class="text-sm leading-relaxed">
      <ul class="list-disc space-y-1 ps-5">
        <li><strong>It says it isn’t signed in</strong> (<code>auth</code>): the token was disconnected or has expired. Connect again, or make a new token.</li>
        <li><strong>It says it’s paused</strong> (<code>paused</code>): someone paused it in Connected apps. Resume it there.</li>
        <li><strong>It says it can’t</strong> (<code>forbidden</code>): your role doesn’t allow that. It can ask Runwell (<code>me</code>) what your role can do.</li>
        <li><strong>It’s told to slow down</strong> (<code>rate_limited</code>): each connection may make {help?.rate_limit || 120} calls a minute. It waits and carries on.</li>
        <li><strong>It can’t find something by name</strong> (<code>ambiguous</code>): the name fits more than one record. It should ask you which.</li>
        <li><strong>A tool you expect is missing</strong>: it belongs to a plugin that’s off, or your role doesn’t have it.</li>
      </ul>
    </Card.Content>
  </Card.Root>
</SettingsLayout>
