<script>
  // Settings › Email: who mail comes from, the outgoing mail server, a test
  // message, and where clients email requests (and how that mail comes in).
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Send from "@lucide/svelte/icons/send";
  import SecretBox from "$lib/components/SecretBox.svelte";
  import { get, patch, post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";
  import { timeAgo } from "$lib/format.js";

  let data = $state(null), sender = $state({}), smtp = $state({}), requests = $state(""), ingress = $state("");
  let to = $state(session.me?.email_address || ""), errors = $state({}), poll = null;

  function fill(d) {
    data = d;
    sender = { mail_from_name: d.from_name || "", mail_from_email: d.from_email || "" };
    smtp = { smtp_address: d.smtp.address || "", smtp_port: d.smtp.port ?? "", smtp_security: d.smtp.security, smtp_username: d.smtp.username || "", smtp_password: "" };
    requests = d.requests_email || "";
    ingress = d.inbound_saved || "";
  }
  const load = () => get("/settings/email").then(fill).catch(toastError);
  load();

  async function save(path, body, key) {
    errors = {};
    try { const res = await patch(path, body); fill(res); toast(res.summary); }
    catch (err) { errors = { [key]: errorMessage(err) }; }
  }
  async function sendTest(e) {
    e.preventDefault();
    try { toast((await post("/settings/email/test_message", { to })).summary); } catch (err) { toastError(err); return; }
    // The message goes out from a job: look again until it's gone or failed.
    clearInterval(poll);
    let tries = 0;
    poll = setInterval(async () => {
      tries++;
      const d = await get("/settings/email").catch(() => null);
      if (d) data.test_email = d.test_email;
      if (!d || d.test_email.sent_at || d.test_email.error || tries > 60) clearInterval(poll);
    }, 2000);
  }
  $effect(() => () => clearInterval(poll));

  const test = $derived(data?.test_email || {});
  const recent = (iso) => iso && Date.now() - new Date(iso).getTime() < 5 * 60 * 1000;
  const ingressNames = { "": "Nothing yet", relay: "Relay (Cloudflare Email Routing, your mail server)", postmark: "Postmark", sendgrid: "SendGrid" };
</script>

<SettingsLayout title="Email">
  {#if !data}
    <Skeleton class="h-96 w-full" />
  {:else}
    <Card.Root>
      <Card.Header>
        <Card.Title>Who mail comes from</Card.Title>
        <Card.Description>The sender on every email Runwell sends: approval links, portal sign-ins, invitations and notifications. Use an address on a domain your mail server can send for, or mail lands in spam. Leave a field blank to use {data.default_sender}.</Card.Description>
      </Card.Header>
      <Card.Content>
        <form onsubmit={(e) => { e.preventDefault(); save("/settings/email", { setting: sender }, "sender"); }}>
          <Field.Group>
            <div class="grid gap-4 sm:grid-cols-2">
              <Field.Field>
                <Field.Label for="from-name">From name</Field.Label>
                <Input id="from-name" bind:value={sender.mail_from_name} autocomplete="off" placeholder={data.default_name} />
              </Field.Field>
              <Field.Field>
                <Field.Label for="from-email">From email</Field.Label>
                <Input id="from-email" type="email" bind:value={sender.mail_from_email} autocomplete="off" placeholder={data.default_email} />
              </Field.Field>
            </div>
            {#if errors.sender}<p class="text-destructive text-sm">{errors.sender}</p>{/if}
            <div><Button type="submit">Save</Button></div>
          </Field.Group>
        </form>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>How it goes out</Card.Title></Card.Header>
      <Card.Content class="flex flex-col gap-4 text-sm">
        <p>
          {#if data.smtp_from_env}Through the mail server at {data.smtp_env_address}, from the server’s SMTP settings.
          {:else if data.smtp.address}Through the mail server at {data.smtp.address}, saved below.
          {:else}No mail server yet, so mail isn’t sent. Give one below, or let a plugin send it (<a href="/settings/plugins" class="underline underline-offset-4">Plugins</a>).{/if}
          {#if data.smtp_from_env || data.smtp.address} A plugin that sends mail, when it’s on, sends instead.{/if}
        </p>
        {#if !data.smtp_from_env}
          <Collapsible.Root open={!data.smtp.address}>
            <Collapsible.Trigger>
              {#snippet child({ props })}<Button variant="outline" size="sm" {...props}>{data.smtp.address ? "Change the mail server" : "Add a mail server"}</Button>{/snippet}
            </Collapsible.Trigger>
            <Collapsible.Content>
              <form class="mt-4" onsubmit={(e) => { e.preventDefault(); save("/settings/email/smtp", { smtp }, "smtp"); }}>
                <Field.Group>
                  <div class="grid gap-4 sm:grid-cols-[1fr_7rem_14rem]">
                    <Field.Field><Field.Label for="smtp-address">Server</Field.Label><Input id="smtp-address" bind:value={smtp.smtp_address} autocomplete="off" placeholder="smtp.example.com" /></Field.Field>
                    <Field.Field><Field.Label for="smtp-port">Port</Field.Label><Input id="smtp-port" type="number" min={1} max={65535} bind:value={smtp.smtp_port} placeholder="587" /></Field.Field>
                    <Field.Field>
                      <Field.Label for="smtp-security">Security</Field.Label>
                      <Select.Root type="single" bind:value={smtp.smtp_security}>
                        <Select.Trigger id="smtp-security" class="w-full">{data.smtp_securities[smtp.smtp_security]}</Select.Trigger>
                        <Select.Content>{#each Object.entries(data.smtp_securities) as [k, label] (k)}<Select.Item value={k}>{label}</Select.Item>{/each}</Select.Content>
                      </Select.Root>
                    </Field.Field>
                  </div>
                  <div class="grid gap-4 sm:grid-cols-2">
                    <Field.Field><Field.Label for="smtp-user">Username</Field.Label><Input id="smtp-user" bind:value={smtp.smtp_username} autocomplete="off" /></Field.Field>
                    <Field.Field><Field.Label for="smtp-pass">Password</Field.Label><Input id="smtp-pass" type="password" bind:value={smtp.smtp_password} autocomplete="new-password" placeholder={data.smtp.password_saved ? "Saved: leave blank to keep it" : ""} /></Field.Field>
                  </div>
                  <Field.Description>The password is stored encrypted. Save, then send a test email below to check it.</Field.Description>
                  {#if errors.smtp}<p class="text-destructive text-sm">{errors.smtp}</p>{/if}
                  <div><Button type="submit">Save</Button></div>
                </Field.Group>
              </form>
            </Collapsible.Content>
          </Collapsible.Root>
        {/if}
        <form onsubmit={sendTest} class="flex flex-wrap items-center gap-2">
          <Input type="email" bind:value={to} autocomplete="off" placeholder="you@example.com" aria-label="Send to" class="max-w-xs" />
          <Button type="submit" variant="outline"><Send />Send a test email</Button>
        </form>
        {#if test.error}
          <p class="text-destructive">Couldn’t send to {test.to}: {test.error}</p>
        {:else if test.sent_at}
          <p class="text-muted-foreground">Sent a test email to {test.to} {timeAgo(test.sent_at)}.</p>
        {:else if recent(test.requested_at)}
          <p class="text-muted-foreground">Sending a test email to {test.to}…</p>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root id="requests_by_email">
      <Card.Header>
        <Card.Title>{session.term("client", 2)} email requests</Card.Title>
        <Card.Description>Mail sent to this address becomes a request to triage: the subject is its title, the message its description (without the quoted mail and signature), and attachments its documents. A known contact gets a short reply saying it came in; their replies to that land on the same request as notes. Automatic replies are ignored.</Card.Description>
      </Card.Header>
      <Card.Content class="flex flex-col gap-4 text-sm">
        <form onsubmit={(e) => { e.preventDefault(); save("/settings/email", { setting: { requests_email: requests } }, "requests"); }} class="flex flex-wrap items-end gap-2">
          <Field.Field class="min-w-64 flex-1">
            <Field.Label for="requests-email">Requests address</Field.Label>
            <Input id="requests-email" type="email" bind:value={requests} autocomplete="off" placeholder="requests@yourcompany.com" />
          </Field.Field>
          <Button type="submit">Save</Button>
        </form>
        {#if errors.requests}<p class="text-destructive">{errors.requests}</p>{/if}
        {#if data.inbound_ingress}
          <p><Badge>Receiving</Badge> Mail comes in through {data.inbound_name}. Forward the requests address there (plus addresses too, for replies), to:</p>
          <SecretBox value={data.inbound_url} />
          <p class="text-muted-foreground">{data.inbound_hint}</p>
        {:else}
          <p><Badge variant="outline">Off</Badge> Runwell can’t receive mail on its own: a service (Cloudflare Email Routing, Postmark, SendGrid) or your own mail server forwards the address here. Choose it below, or let a plugin set it up.</p>
        {/if}
        {#if !data.inbound_from_env}
          <form onsubmit={(e) => { e.preventDefault(); save("/settings/email/inbound", { inbound: { ingress } }, "inbound"); }} class="flex flex-wrap items-end gap-2">
            <Field.Field class="min-w-64 flex-1">
              <Field.Label for="ingress">Comes in through</Field.Label>
              <Select.Root type="single" bind:value={ingress}>
                <Select.Trigger id="ingress" class="w-full">{ingressNames[ingress] || ingress}</Select.Trigger>
                <Select.Content>{#each Object.entries(ingressNames) as [k, label] (k)}<Select.Item value={k}>{label}</Select.Item>{/each}</Select.Content>
              </Select.Root>
            </Field.Field>
            <Button type="submit">Save</Button>
          </form>
          {#if data.inbound_password}
            <Field.Field>
              <Field.Label>Password</Field.Label>
              <SecretBox value={data.inbound_password} />
              <Field.Description>The service signs in with it as <code>actionmailbox</code>. It was made for this install; it isn’t shown anywhere else.</Field.Description>
            </Field.Field>
          {/if}
        {/if}
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>
