<script>
  // Settings › Webhooks: addresses that hear of events as signed JSON, how
  // each one's deliveries went, and adding another.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import SecretBox from "$lib/components/SecretBox.svelte";
  import { get, post, patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { dateTime, plural } from "$lib/format.js";

  let data = $state(null), edits = $state({}), add = $state({ url: "", description: "", kinds_text: "", include_security: false }), addError = $state(""), errors = $state({});

  function load() {
    return get("/settings/webhooks").then((d) => {
      data = d;
      edits = Object.fromEntries(d.webhooks.map((w) => [w.id, { kinds_text: w.kinds_text, include_security: w.include_security, active: w.active }]));
    }).catch(toastError);
  }
  load();

  async function save(w, e) {
    e.preventDefault();
    errors = {};
    try { toast((await patch(`/settings/webhooks/${w.id}`, { webhook_endpoint: { url: w.url, description: w.description || "", ...edits[w.id] } })).summary); load(); }
    catch (err) { errors = { [w.id]: errorMessage(err) }; }
  }
  async function ping(w) {
    try { toast((await patch(`/settings/webhooks/${w.id}`, { ping: true })).summary); setTimeout(load, 1500); } catch (err) { toastError(err); }
  }
  async function remove(w) {
    if (!(await confirm({ title: "Remove this webhook?", message: `Events stop going to ${w.url}.`, confirmLabel: "Remove", destructive: true }))) return;
    try { toast((await del(`/settings/webhooks/${w.id}`)).summary); load(); } catch (err) { toastError(err); }
  }
  async function create(e) {
    e.preventDefault();
    addError = "";
    try {
      toast((await post("/settings/webhooks", { webhook_endpoint: add })).summary);
      add = { url: "", description: "", kinds_text: "", include_security: false };
      setTimeout(load, 1000);
      load();
    } catch (err) { addError = errorMessage(err); }
  }
</script>

<SettingsLayout title="Webhooks">
  <Card.Root>
    <Card.Header><Card.Title>How webhooks work</Card.Title></Card.Header>
    <Card.Content class="text-muted-foreground flex flex-col gap-2 text-sm">
      <p>Send each event (a {session.term("client").toLowerCase()} added, an agreement approved, {session.term("work").toLowerCase()} done…) as JSON to another tool: Zapier, Make, n8n, a spreadsheet script, your own server.</p>
      <p>Each request is signed with the endpoint’s secret: <code>X-Runwell-Signature</code> is <code>sha256=</code> and the HMAC-SHA256 of <code>X-Runwell-Timestamp</code>, a dot, and the body. Failed deliveries are tried again with growing waits; twenty failures in a row switch an endpoint off.</p>
    </Card.Content>
  </Card.Root>

  {#if !data}
    <Skeleton class="h-48 w-full" />
  {:else}
    {#each data.webhooks as w (w.id)}
      <Card.Root id={`webhook_endpoint_${w.id}`}>
        <Card.Header>
          <Card.Title class="truncate">{w.description || w.url}</Card.Title>
          <Card.Description class="flex flex-wrap items-center gap-2">
            {#if w.active}<Badge>On</Badge>{:else}<Badge variant="destructive">Off</Badge>{/if}
            <code class="truncate">{w.url}</code>
            <span>{w.kinds ? w.kinds_text : "every event"}{w.include_security ? ", with sign-in and security" : ""}{w.failures_in_a_row > 0 ? ` · ${plural(w.failures_in_a_row, "failure")} in a row` : ""}</span>
          </Card.Description>
        </Card.Header>
        <Card.Content class="flex flex-col gap-4">
          <form onsubmit={(e) => save(w, e)}>
            <Field.Group>
              <Field.Field><Field.Label>Secret</Field.Label><SecretBox value={w.secret || "(can’t be read here: remove this webhook and add it again)"} /></Field.Field>
              <Field.Field>
                <Field.Label for={`kinds-${w.id}`}>Only these kinds</Field.Label>
                <Input id={`kinds-${w.id}`} bind:value={edits[w.id].kinds_text} autocomplete="off" placeholder="Blank for every event, or e.g. agreement.approved, todo.*" />
              </Field.Field>
              <Field.Field orientation="horizontal">
                <Checkbox id={`sec-${w.id}`} bind:checked={edits[w.id].include_security} />
                <Field.Label for={`sec-${w.id}`} class="font-normal">Include sign-ins and security changes</Field.Label>
              </Field.Field>
              <Field.Field orientation="horizontal">
                <Checkbox id={`on-${w.id}`} bind:checked={edits[w.id].active} />
                <Field.Label for={`on-${w.id}`} class="font-normal">On</Field.Label>
              </Field.Field>
              {#if errors[w.id]}<p class="text-destructive text-sm">{errors[w.id]}</p>{/if}
              <div class="flex flex-wrap gap-2">
                <Button type="submit">Save</Button>
                <Button variant="outline" onclick={() => ping(w)}>Send a test</Button>
                <Button variant="ghost" class="text-destructive ms-auto" onclick={() => remove(w)}>Remove</Button>
              </div>
            </Field.Group>
          </form>
          {#if w.deliveries.length}
            <Table.Root class="text-xs">
              <Table.Header><Table.Row><Table.Head>When</Table.Head><Table.Head>Event</Table.Head><Table.Head>Result</Table.Head><Table.Head>Tries</Table.Head></Table.Row></Table.Header>
              <Table.Body>
                {#each w.deliveries as d (d.id)}
                  <Table.Row>
                    <Table.Cell class="whitespace-nowrap">{dateTime(d.created_at)}</Table.Cell>
                    <Table.Cell><code>{d.kind}</code></Table.Cell>
                    <Table.Cell>
                      {#if d.ok}<Badge>{d.status_code}</Badge> <span class="text-muted-foreground">{d.duration_ms} ms</span>
                      {:else if d.attempts === 0}<Badge variant="outline">Queued</Badge>
                      {:else}<Badge variant="destructive">{d.status_code || "Failed"}</Badge> <span class="text-muted-foreground inline-block max-w-sm truncate align-middle" title={d.error}>{d.error}</span>{/if}
                    </Table.Cell>
                    <Table.Cell>{d.attempts}</Table.Cell>
                  </Table.Row>
                {/each}
              </Table.Body>
            </Table.Root>
          {/if}
        </Card.Content>
      </Card.Root>
    {/each}

    <Card.Root>
      <Card.Header><Card.Title>Add a webhook</Card.Title></Card.Header>
      <Card.Content>
        <form onsubmit={create}>
          <Field.Group>
            <Field.Field><Field.Label for="hook-url">Address</Field.Label><Input id="hook-url" type="url" bind:value={add.url} required autocomplete="off" placeholder="https://hooks.example.com/runwell" /></Field.Field>
            <Field.Field><Field.Label for="hook-name">Name (optional)</Field.Label><Input id="hook-name" bind:value={add.description} maxlength={120} autocomplete="off" placeholder="Zapier: approvals to the sales sheet" /></Field.Field>
            <Field.Field><Field.Label for="hook-kinds">Only these kinds (optional)</Field.Label><Input id="hook-kinds" bind:value={add.kinds_text} autocomplete="off" placeholder="Blank for every event, or e.g. agreement.approved, todo.*" /></Field.Field>
            <Field.Field orientation="horizontal">
              <Checkbox id="hook-sec" bind:checked={add.include_security} />
              <Field.Label for="hook-sec" class="font-normal">Include sign-ins and security changes</Field.Label>
            </Field.Field>
            {#if addError}<p class="text-destructive text-sm">{addError}</p>{/if}
            <div><Button type="submit">Add webhook</Button></div>
            <Field.Description>Sign-in and security event kinds are in the <a href="/settings/audit_log?area=security" class="underline underline-offset-4">audit log</a>.</Field.Description>
          </Field.Group>
        </form>
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>
