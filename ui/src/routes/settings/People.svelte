<script>
  // Settings › People: who is on the team and their roles, two-factor
  // sign-in for everyone, invitations out, and what each role can do.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Table from "$lib/components/ui/table/index.js";
  import * as Avatar from "$lib/components/ui/avatar/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import Check from "@lucide/svelte/icons/check";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import PersonRemoveDialog from "./PersonRemoveDialog.svelte";
  import { get, post, patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { humanize, initials, timeAgo } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";
  import { go } from "$lib/router.svelte.js";

  let data = $state(null);
  let invite = $state({ email_address: "", role: "member" }), inviteErrors = $state({}), inviting = $state(false);
  const roles = ["owner", "manager", "member"];

  const load = () => get("/settings/people").then((d) => (data = d)).catch(toastError);
  load();

  const me = $derived(session.me?.person_id);
  let removing = $state(null), removeOpen = $state(false);
  function askRemove(person) { removing = person; removeOpen = true; }

  async function setRole(person, role) {
    try { const res = await patch(`/settings/people/${person.id}`, { user: { role } }); toast(res.summary); }
    catch (err) { toastError(err); }
    load();
  }
  async function deactivate(person) {
    if (!(await confirm({ title: `Deactivate ${person.name}?`, message: "They’re signed out everywhere, their connected apps stop, and they can’t sign in. Their name stays on their work, and you can reactivate them.", confirmLabel: "Deactivate", destructive: true }))) return;
    try { toast((await post(`/settings/people/${person.id}/deactivation`)).summary); } catch (err) { toastError(err); }
    load();
  }
  async function reactivate(person) {
    try { toast((await del(`/settings/people/${person.id}/deactivation`)).summary); } catch (err) { toastError(err); }
    load();
  }
  async function resetTwoFactor(person) {
    if (!(await confirm({ title: `Reset two-factor sign-in for ${person.name}?`, message: "Use this when they’ve lost their phone and their recovery codes. They’re signed out, sign in with just their password, and set it up again.", confirmLabel: "Reset", destructive: true }))) return;
    try { toast((await del(`/settings/people/${person.id}/two_factor`)).summary); } catch (err) { toastError(err); }
    load();
  }
  async function setRequired(on) {
    try {
      const res = await patch("/settings/two_factor_requirement", { setting: { require_two_factor: on } });
      toast(res.summary);
      session.apply(await get("/session", { quiet: true }));
    } catch (err) { toastError(err); }
    load();
  }
  async function sendInvite(e) {
    e.preventDefault();
    inviting = true; inviteErrors = {};
    try {
      const res = await post("/settings/invitations", { invitation: invite });
      toast(res.summary);
      invite = { email_address: "", role: "member" };
      load();
    } catch (err) { inviteErrors = err.errors; if (!Object.keys(inviteErrors).length) toastError(err); else focusInvalid(); }
    inviting = false;
  }
  async function resend(inv) {
    try { toast((await patch(`/settings/invitations/${inv.id}`)).summary); } catch (err) { toastError(err); }
    load();
  }
  async function withdraw(inv) {
    if (!(await confirm({ title: `Withdraw the invitation to ${inv.email_address}?`, message: "The link they were sent stops working.", confirmLabel: "Withdraw", destructive: true }))) return;
    try { toast((await del(`/settings/invitations/${inv.id}`)).summary); } catch (err) { toastError(err); }
    load();
  }
</script>

<SettingsLayout title="People" description="Who is on the team, their roles, and invitations out.">
  {#if !data}
    <Skeleton class="h-64 w-full" />
  {:else}
    <Card.Root>
      <Card.Header><Card.Title>Team</Card.Title><Card.Description>{data.summary}</Card.Description></Card.Header>
      <Card.Content class="px-0">
        <Table.Root>
          <Table.Header>
            <Table.Row>
              <Table.Head class="ps-6">Person</Table.Head>
              <Table.Head>Sign-in</Table.Head>
              <Table.Head>Role</Table.Head>
              <Table.Head class="pe-6 text-end"><span class="sr-only">Actions</span></Table.Head>
            </Table.Row>
          </Table.Header>
          <Table.Body>
            {#each data.people as p (p.id)}
              <Table.Row class={p.active ? "" : "opacity-60"}>
                <Table.Cell class="ps-6">
                  <div class="flex items-center gap-3">
                    <Avatar.Root class="size-8"><Avatar.Image src={`/api/users/${p.id}/avatar`} alt="" /><Avatar.Fallback>{initials(p.name)}</Avatar.Fallback></Avatar.Root>
                    <div class="min-w-0">
                      <a href={`/settings/people/${p.id}`} class="font-medium hover:underline">{p.name}</a>{#if p.id === me}<span class="text-muted-foreground"> (you)</span>{/if}
                      <p class="text-muted-foreground truncate text-xs">{p.email_address}</p>
                      {#if p.agent}<p class="text-muted-foreground truncate text-xs">Apps they connect act as <strong>{p.agent.name}</strong></p>{/if}
                    </div>
                  </div>
                </Table.Cell>
                <Table.Cell>
                  {#if p.two_factor}
                    <Badge variant="secondary" title="Signs in with a code from an authenticator app">Two-factor</Badge>
                  {:else if data.two_factor_required && p.active}
                    <Badge variant="outline" title="Sets it up on their next page">Two-factor pending</Badge>
                  {/if}
                  {#if !p.active}<Badge variant="outline">Deactivated</Badge>{/if}
                </Table.Cell>
                <Table.Cell>
                  {#if p.active}
                    <Select.Root type="single" value={p.role} onValueChange={(r) => setRole(p, r)}>
                      <Select.Trigger size="sm" class="w-32" aria-label={`Role for ${p.name}`}>{humanize(p.role)}</Select.Trigger>
                      <Select.Content>{#each roles as r (r)}<Select.Item value={r}>{humanize(r)}</Select.Item>{/each}</Select.Content>
                    </Select.Root>
                  {:else}
                    {humanize(p.role)}
                  {/if}
                </Table.Cell>
                <Table.Cell class="pe-6 text-end whitespace-nowrap">
                  {#if !p.active}
                    <Button variant="outline" size="sm" onclick={() => reactivate(p)}>Reactivate</Button>
                  {/if}
                  {#if p.id !== me}
                    <DropdownMenu.Root>
                      <DropdownMenu.Trigger>{#snippet child({ props })}<Button {...props} variant="ghost" size="icon-sm" aria-label={`More for ${p.name}`}><Ellipsis /></Button>{/snippet}</DropdownMenu.Trigger>
                      <DropdownMenu.Content align="end">
                        <DropdownMenu.Item onSelect={() => go(`/settings/people/${p.id}`)}>Open</DropdownMenu.Item>
                        {#if p.two_factor && p.active}
                          <DropdownMenu.Item onSelect={() => resetTwoFactor(p)}>Reset two-factor</DropdownMenu.Item>
                        {/if}
                        {#if p.active}
                          <DropdownMenu.Item onSelect={() => deactivate(p)}>Deactivate…</DropdownMenu.Item>
                        {:else}
                          <DropdownMenu.Item onSelect={() => reactivate(p)}>Reactivate</DropdownMenu.Item>
                        {/if}
                        <DropdownMenu.Separator />
                        <DropdownMenu.Item variant="destructive" onSelect={() => askRemove(p)}>Remove…</DropdownMenu.Item>
                      </DropdownMenu.Content>
                    </DropdownMenu.Root>
                  {/if}
                </Table.Cell>
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
      </Card.Content>
    </Card.Root>

    <Card.Root id="two-factor">
      <Card.Header><Card.Title>Two-factor sign-in</Card.Title></Card.Header>
      <Card.Content>
        <Field.Field orientation="horizontal">
          <Field.Content>
            <Field.Label for="require-2fa">Require it for everyone</Field.Label>
            <Field.Description>After their password, everyone enters a code from an authenticator app. Anyone without it sets it up on their next page. {session.term("client", 2)} sign in by emailed link and aren’t affected.</Field.Description>
          </Field.Content>
          <Switch id="require-2fa" checked={data.two_factor_required} onCheckedChange={setRequired} />
        </Field.Field>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Invite someone</Card.Title>
        <Card.Description>They get an email with a link to choose a name and password. It works once and expires in 7 days.</Card.Description>
      </Card.Header>
      <Card.Content class="flex flex-col gap-4">
        <form onsubmit={sendInvite} class="flex flex-col gap-2">
          <div class="flex flex-wrap items-start gap-2">
            <Input type="email" bind:value={invite.email_address} required autocomplete="off" placeholder="name@example.com" aria-label="Email address" class="min-w-56 flex-1" aria-invalid={!!inviteErrors.email_address} />
            <Select.Root type="single" bind:value={invite.role}>
              <Select.Trigger class="w-32" aria-label="Role">{humanize(invite.role)}</Select.Trigger>
              <Select.Content>{#each roles as r (r)}<Select.Item value={r}>{humanize(r)}</Select.Item>{/each}</Select.Content>
            </Select.Root>
            <Button type="submit" disabled={inviting}>Send invitation</Button>
          </div>
          {#if inviteErrors.email_address}<p class="text-destructive text-sm">Email address {inviteErrors.email_address.join(", ")}</p>{/if}
        </form>
        {#if data.invitations.length}
          <ul class="divide-y rounded-md border">
            {#each data.invitations as inv (inv.id)}
              <li class="flex flex-wrap items-center gap-2 px-3 py-2 text-sm">
                <div class="min-w-0 flex-1">
                  <p class="font-medium">{inv.email_address}</p>
                  <p class="text-muted-foreground text-xs">{humanize(inv.role)} · invited by {inv.invited_by?.name} {timeAgo(inv.sent_at)}{inv.expired ? " · expired" : ""}</p>
                </div>
                <Button variant="outline" size="sm" onclick={() => resend(inv)}>Resend</Button>
                <Button variant="ghost" size="sm" onclick={() => withdraw(inv)}>Withdraw</Button>
              </li>
            {/each}
          </ul>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header><Card.Title>What each role can do</Card.Title></Card.Header>
      <Card.Content class="flex flex-col gap-4">
        <dl class="grid gap-x-4 gap-y-1 text-sm sm:grid-cols-[auto_1fr]">
          {#each Object.entries(data.roles) as [role, description] (role)}
            <dt class="font-medium">{humanize(role)}</dt><dd class="text-muted-foreground">{description}</dd>
          {/each}
        </dl>
        <Table.Root>
          <Table.Header>
            <Table.Row><Table.Head>Permission</Table.Head>{#each roles as r (r)}<Table.Head class="text-center">{humanize(r)}</Table.Head>{/each}</Table.Row>
          </Table.Header>
          <Table.Body>
            {#each Object.entries(data.permissions) as [key, perm] (key)}
              <Table.Row>
                <Table.Cell>{perm.name}</Table.Cell>
                {#each roles as r (r)}
                  <Table.Cell class="text-center">{#if perm.roles.includes(r)}<Check class="inline size-4" /><span class="sr-only">Yes</span>{:else}<span class="text-muted-foreground">—</span>{/if}</Table.Cell>
                {/each}
              </Table.Row>
            {/each}
          </Table.Body>
        </Table.Root>
        <p class="text-muted-foreground text-xs">Everyone can work on {session.term("client", 2).toLowerCase()}, {session.term("engagement", 2).toLowerCase()}, {session.term("work").toLowerCase()}, commitments, requests, notes and documents. Plugins that are on add their own permissions here.</p>
      </Card.Content>
    </Card.Root>
  {/if}
  <PersonRemoveDialog person={removing} bind:open={removeOpen} onremoved={load} ondeactivate={() => removing && deactivate(removing)} />
</SettingsLayout>
