<script>
  // Someone on the team: their details and role, their password, how they
  // sign in (browsers, connected apps, two-factor, recent sign-ins), and
  // their access.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import * as Avatar from "$lib/components/ui/avatar/index.js";
  import * as Collapsible from "$lib/components/ui/collapsible/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Mail from "@lucide/svelte/icons/mail";
  import { get, post, patch, del } from "$lib/api.js";
  import { page } from "$lib/page.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import NotFound from "../NotFound.svelte";
  import { confirm } from "$lib/confirm.svelte.js";
  import { humanize, initials, timeAgo, dateTime } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";
  import { go } from "$lib/router.svelte.js";
  import PersonRemoveDialog from "./PersonRemoveDialog.svelte";

  let { params } = $props();
  // svelte-ignore state_referenced_locally
  const id = params.id;
  let person = $state(null), form = $state({ name: "", email_address: "", role: "member" }), errors = $state({}), busy = $state(false);
  let pw = $state({ password: "", password_confirmation: "" }), pwErrors = $state({});
  const roles = ["owner", "manager", "member"];
  const descriptions = { owner: "Everything, including settings, plugins and people", manager: "Sends agreements, records decisions, triages, closes and deletes", member: "Does the work: updates work, adds notes, documents and commitments" };

  function load() {
    return get(`/settings/people/${id}`).then((p) => {
      person = p;
      form = { name: p.name, email_address: p.email_address, role: p.role };
      page.set(p.display_name, [{ label: "Settings", href: "/settings" }, { label: "People", href: "/settings/people" }]);
    }).catch((e) => { if (e.status === 404 && !person) missing = true; else toastError(e); });
  }
  let missing = $state(false), removeOpen = $state(false);
  const deactivate = () => act(() => post(`/settings/people/${id}/deactivation`), { title: `Deactivate ${person.display_name}?`, message: "They’re signed out everywhere, their connected apps stop, and they can’t sign in. Their name stays on their work, and you can reactivate them.", confirmLabel: "Deactivate", destructive: true });
  load();

  async function save(e) {
    e.preventDefault();
    busy = true; errors = {};
    try { toast((await patch(`/settings/people/${id}`, { user: form })).summary); load(); }
    catch (err) { errors = err.errors; if (!Object.keys(errors).length) toastError(err); else focusInvalid(); }
    busy = false;
  }
  async function act(fn, ask) {
    if (ask && !(await confirm(ask))) return;
    try { toast((await fn()).summary); } catch (err) { toastError(err); }
    load();
  }
  async function setPassword(e) {
    e.preventDefault();
    pwErrors = {};
    try {
      toast((await patch(`/settings/people/${id}/password`, { user: pw })).summary);
      pw = { password: "", password_confirmation: "" };
      load();
    } catch (err) { pwErrors = err.errors; if (!Object.keys(pwErrors).length) toastError(err); else focusInvalid(); }
  }
</script>

<SettingsLayout title={person?.display_name || "Person"}>
  {#if missing}
    <NotFound title="Couldn’t find that person" back={{ href: "/settings/people", label: "Everyone on the team" }} />
  {:else if !person}
    <Skeleton class="h-64 w-full" />
  {:else}
    <div class="flex items-center gap-4">
      <Avatar.Root class="size-14 rounded-lg"><Avatar.Image src={`/api/users/${person.id}/avatar`} alt="" /><Avatar.Fallback class="rounded-lg">{initials(person.display_name)}</Avatar.Fallback></Avatar.Root>
      <div class="flex flex-col gap-1">
        {#if person.you}<p class="text-muted-foreground text-sm">This is you.</p>{/if}
        <div class="flex flex-wrap gap-1">
          <Badge variant="secondary">{humanize(person.role)}</Badge>
          <Badge variant="outline">{person.active ? "Active" : "Deactivated"}</Badge>
          {#if person.two_factor}<Badge>Two-factor</Badge>{/if}
        </div>
      </div>
    </div>
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="flex flex-col gap-4 lg:col-span-2">
        <Card.Root>
          <Card.Header><Card.Title>Details</Card.Title></Card.Header>
          <Card.Content>
            <form onsubmit={save}>
              <Field.Group>
                {#if errors.base}<p class="text-destructive text-sm">{errors.base.join(" ")}</p>{/if}
                <Field.Field data-invalid={!!errors.name}>
                  <Field.Label for="person-name">Name</Field.Label>
                  <Input id="person-name" bind:value={form.name} required autocomplete="off" />
                  {#if errors.name}<Field.Error>Name {errors.name[0]}</Field.Error>{/if}
                </Field.Field>
                <Field.Field data-invalid={!!errors.email_address}>
                  <Field.Label for="person-email">Email address</Field.Label>
                  <Input id="person-email" type="email" bind:value={form.email_address} required autocomplete="off" />
                  <Field.Description>They sign in with it, and Runwell’s email goes there.{person.you ? "" : " Changing it signs them out everywhere."}</Field.Description>
                  {#if errors.email_address}<Field.Error>Email address {errors.email_address[0]}</Field.Error>{/if}
                </Field.Field>
                <Field.Field>
                  <Field.Label for="person-role">Role</Field.Label>
                  <Select.Root type="single" bind:value={form.role}>
                    <Select.Trigger id="person-role" class="w-48">{humanize(form.role)}</Select.Trigger>
                    <Select.Content>{#each roles as r (r)}<Select.Item value={r}>{humanize(r)}</Select.Item>{/each}</Select.Content>
                  </Select.Root>
                  <Field.Description>{descriptions[form.role]}</Field.Description>
                </Field.Field>
                <div><Button type="submit" disabled={busy}>Save</Button></div>
              </Field.Group>
            </form>
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header><Card.Title>Password</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            {#if person.you}
              <p>Change your own in <a href="/settings/account" class="underline underline-offset-4">Your account</a>.</p>
            {:else}
              <div><Button variant="outline" size="sm" onclick={() => act(() => post(`/settings/people/${id}/password`))}><Mail />Email them a link to choose one</Button></div>
              <Collapsible.Root>
                <Collapsible.Trigger class="text-sm underline-offset-4 hover:underline">Set a new password for them</Collapsible.Trigger>
                <Collapsible.Content>
                  <form onsubmit={setPassword} class="mt-3 flex flex-col gap-3">
                    <div class="grid gap-3 sm:grid-cols-2">
                      <Input type="password" bind:value={pw.password} required maxlength={72} autocomplete="new-password" placeholder="New password" aria-label="New password" aria-invalid={!!pwErrors.password} />
                      <Input type="password" bind:value={pw.password_confirmation} required maxlength={72} autocomplete="new-password" placeholder="The same again" aria-label="Confirm the new password" aria-invalid={!!pwErrors.password_confirmation} />
                    </div>
                    {#if pwErrors.password}<p class="text-destructive">Password {pwErrors.password[0]}</p>{/if}
                    {#if pwErrors.password_confirmation}<p class="text-destructive">Password confirmation {pwErrors.password_confirmation[0]}</p>{/if}
                    <p class="text-muted-foreground text-xs">For someone who can’t get the email. They’re signed out everywhere; tell them the new one yourself.</p>
                    <div><Button type="submit" size="sm">Set password</Button></div>
                  </form>
                </Collapsible.Content>
              </Collapsible.Root>
            {/if}
          </Card.Content>
        </Card.Root>
      </div>

      <div class="flex flex-col gap-4">
        <Card.Root>
          <Card.Header><Card.Title>Signed in</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            {#if person.sessions.length}
              <ul class="flex flex-col gap-2">
                {#each person.sessions as s (s.id)}
                  <li><p class="truncate">{s.user_agent || "A browser"}</p><p class="text-muted-foreground text-xs">{s.ip_address} · since {dateTime(s.created_at)}</p></li>
                {/each}
              </ul>
              {#if !person.you}<div><Button variant="outline" size="sm" onclick={() => act(() => del(`/settings/people/${id}/sessions`))}>Sign them out everywhere</Button></div>{/if}
            {:else}
              <p class="text-muted-foreground">Not signed in anywhere.</p>
            {/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header><Card.Title>Connected apps</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            {#if person.connected_apps.length}
              <ul class="flex flex-col gap-2">
                {#each person.connected_apps as t (t.id)}
                  <li><p class="truncate font-medium">{t.name}</p><p class="text-muted-foreground text-xs">{t.kind === "oauth" ? "Connected app" : "Personal token"}{t.read_only ? " · read only" : ""} · {t.last_used_at ? `last used ${timeAgo(t.last_used_at)}` : "never used"}</p></li>
                {/each}
              </ul>
              <div><Button variant="ghost" size="sm" class="text-destructive" onclick={() => act(() => del(`/settings/people/${id}/connections`), { title: `Cut off every app ${person.display_name} connected?`, message: "Their personal tokens and connected apps stop working at once. Anything they already did stays.", confirmLabel: "Cut them off", destructive: true })}>Cut off every app</Button></div>
            {:else}
              <p class="text-muted-foreground">No apps or scripts connected.</p>
            {/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header><Card.Title>Two-factor sign-in</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            {#if person.two_factor}
              <p>On: a code from an authenticator app after their password.</p>
              {#if !person.you && person.active}
                <div><Button variant="ghost" size="sm" class="text-destructive" onclick={() => act(() => del(`/settings/people/${id}/two_factor`), { title: `Reset two-factor sign-in for ${person.display_name}?`, message: "Use this when they’ve lost their phone and their recovery codes. They’re signed out, sign in with just their password, and set it up again.", confirmLabel: "Reset", destructive: true })}>Reset two-factor sign-in</Button></div>
              {/if}
            {:else if person.two_factor_required}
              <p class="text-muted-foreground">Not set up yet: they set it up on their next page.</p>
            {:else}
              <p class="text-muted-foreground">Off.</p>
            {/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header><Card.Title>Recent sign-ins</Card.Title></Card.Header>
          <Card.Content class="flex flex-col gap-3 text-sm">
            {#if person.recent_sign_ins.length}
              <ul class="flex flex-col gap-2">
                {#each person.recent_sign_ins as s (s.id)}
                  <li><p>{s.ok ? "Signed in" : "Failed to sign in"}{s.method ? ` with ${s.method}` : ""}</p><p class="text-muted-foreground text-xs">{dateTime(s.at)}{s.ip ? ` · ${s.ip}` : ""}</p></li>
                {/each}
              </ul>
            {:else}
              <p class="text-muted-foreground">None recorded.</p>
            {/if}
            <a href={`/settings/audit_log?person=${person.id}`} class="text-sm underline-offset-4 hover:underline">Everything in the audit log</a>
          </Card.Content>
        </Card.Root>

        {#if !person.you}
          <Card.Root id="access">
            <Card.Header><Card.Title>Access</Card.Title></Card.Header>
            <Card.Content class="flex flex-col gap-3 text-sm">
              {#if !person.active}
                <p>Deactivated: they can’t sign in, and their name stays on their work.</p>
                <div><Button variant="outline" size="sm" onclick={() => act(() => del(`/settings/people/${id}/deactivation`))}>Reactivate</Button></div>
              {:else}
                <p>Deactivating signs them out and keeps them out; their name stays on their work, and you can reactivate them.</p>
                <div><Button variant="outline" size="sm" onclick={deactivate}>Deactivate {person.display_name}</Button></div>
              {/if}
              <p class="border-t pt-3">Leaving for good? Remove them and hand their work to someone else.</p>
              <div><Button variant="destructive" size="sm" onclick={() => (removeOpen = true)}>Remove…</Button></div>
            </Card.Content>
          </Card.Root>
        {/if}
      </div>
    </div>
    {#if !person.you}
      <PersonRemoveDialog person={{ id: person.id, name: person.display_name }} bind:open={removeOpen} onremoved={() => go("/settings/people")} ondeactivate={person.active ? deactivate : undefined} />
    {/if}
  {/if}
</SettingsLayout>
