<script>
  // What a plugin asks for, each a box to grant (never more than it asks):
  // reading and writing records, its own storage, tables and files, email,
  // public websites, this server's network, secrets, acting as its caller
  // through the agent tools, clients' portal, live updates, notifications,
  // the numbers for reports, mentions, machines' tokens, content from other
  // sites, and the hosts it may call.
  import * as Field from "$lib/components/ui/field/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";

  let { requested, granted = $bindable({}), id, disabled = false } = $props();
  const CAPS = [
    ["records:read", "Read records", "Clients, contacts, engagements, agreements, work, commitments and requests, as agents see them."],
    ["records:write", "Add to records", "Notes, work, commitments and its own events, recorded as whoever acted."],
    ["storage", "Its own storage", "A private store no other plugin can see."],
    ["tables", "Its own tables", "Tables in the database for its own records, which only it can reach."],
    ["files", "Its own files", "Files kept on its own records, in Runwell's file store."],
    ["mail", "Send email", "Through Settings › Email, at most 200 an hour."],
    ["http:public", "Reach public websites", "Any address on the public internet (a client's own site), never this server's network."],
    ["http:private", "Reach this server’s network", "Any address an owner sets, this server and its local network included (your own model server, say). Never ticked for you: only tick it if you trust the plugin there."],
    ["secrets", "Keep secrets", "Values it must use again (an API key), sealed with this install's key."],
    ["tools", "Act as the person using it", "The same tools as their own AI app over MCP: what their role allows, never more. Changes run only once they approve them, and show in history as theirs."],
    ["portal", "Serve clients in their portal", "Pages and actions for a client's contact signed in to their portal."],
    ["live", "Update open pages live", "Push what changes (a new message, say) to the open tabs of the people it's for, and see who has Runwell open."],
    ["notify", "Notify people", "Put a notification in someone's bell, and email it when they're away."],
    ["metrics", "Read the numbers", "Counts and totals for reports: each client's open and late work, requests, commitments and money agreed, and dated history for trends. Read only."],
    ["mentionables", "Be mentioned", "Offer names people can @mention in notes and chat (an AI agent), hear those mentions as the person who wrote them, and reply where they were made."],
    ["mentions:relay", "Pass mentions on", "Pass an @mention of a plugin's name in its own messages to that plugin, and show its reply."],
    ["machines", "Work with your machines", "Give machines you control (not people) tokens of their own to call its routes; the tokens stop working when it's removed."],
    ["content:external", "Load content from other sites", "Let its own pages load scripts, styles, fonts, images or data from the sites its card lists (a map provider's, say). Untick it and the app loads only from this server."],
  ];
  const asks = $derived(CAPS.filter(([k]) => requested[k]));
  const toggleHost = (host, on) => (granted.http = on ? [...granted.http, host] : granted.http.filter((h) => h !== host));
</script>

<div class="flex flex-col gap-2">
  <p class="text-muted-foreground text-xs font-medium">Asks for</p>
  {#if asks.length || requested.http?.length}
    {#each asks as [k, label, desc] (k)}
      <Field.Field orientation="horizontal">
        <Checkbox id="grant-{id}-{k}" bind:checked={granted[k]} {disabled} />
        <Field.Content><Field.Label for="grant-{id}-{k}">{label}</Field.Label><Field.Description>{desc}</Field.Description></Field.Content>
      </Field.Field>
    {/each}
    {#each requested.http || [] as host (host)}
      <Field.Field orientation="horizontal">
        <Checkbox id="grant-{id}-{host}" checked={granted.http?.includes(host)} onCheckedChange={(v) => toggleHost(host, v)} {disabled} />
        <Field.Content><Field.Label for="grant-{id}-{host}">Call {host}</Field.Label><Field.Description>Requests to this host only.</Field.Description></Field.Content>
      </Field.Field>
    {/each}
  {:else}
    <p class="text-muted-foreground text-sm">Nothing beyond writing to the log.</p>
  {/if}
</div>
