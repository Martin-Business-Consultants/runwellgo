<script>
  // Settings › Your account, for everyone: choices that are yours alone.
  // Your name, address, password and picture; how you sign in (an emailed
  // link, Google…, two-factor); your text size; and your menu.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as Avatar from "$lib/components/ui/avatar/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import ArrowDown from "@lucide/svelte/icons/arrow-down";
  import Lock from "@lucide/svelte/icons/lock";
  import RecoveryCodes from "./RecoveryCodes.svelte";
  import { api, get, patch, post, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { router } from "$lib/router.svelte.js";
  import { toast, toastError, toasts } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { appearance } from "$lib/appearance.svelte.js";
  import { nav } from "$lib/nav.svelte.js";
  import { initials, timeAgo, date, plural } from "$lib/format.js";
  import { focusInvalid } from "$lib/forms.js";

  let account = $state(null);
  let profile = $state({ name: "", email_address: "", password_challenge: "" });
  let profileErrors = $state({}), savingProfile = $state(false);
  let pw = $state({ password_challenge: "", password: "", password_confirmation: "" });
  let pwErrors = $state({}), savingPw = $state(false);
  let avatarVersion = $state(Date.now());
  let codes = $state(null);
  let askPassword = $state(null), passwordInput = $state(""), passwordBusy = $state(false);

  const notice = router.query.get("notice"), alert = router.query.get("alert");
  if (notice) toast(notice);
  if (alert) toasts.show(alert, "danger");

  function load() {
    return get("/settings/account").then((a) => {
      account = a;
      profile = { name: a.name, email_address: a.email_address, password_challenge: "" };
    }).catch(toastError);
  }
  load();

  const emailChanged = $derived(account && profile.email_address.trim().toLowerCase() !== account.email_address);

  async function refreshSession() {
    try { session.apply(await get("/session", { quiet: true })); } catch { /* it stays as it was */ }
  }

  async function saveProfile(e) {
    e.preventDefault();
    savingProfile = true; profileErrors = {};
    try {
      const body = { name: profile.name };
      if (emailChanged) { body.email_address = profile.email_address; body.password_challenge = profile.password_challenge; }
      const res = await patch("/settings/account", { user: body });
      account = res; profile = { name: res.name, email_address: res.email_address, password_challenge: "" };
      toast(res.summary);
      refreshSession();
    } catch (err) { profileErrors = err.errors; if (!Object.keys(profileErrors).length) toastError(err); else focusInvalid(); }
    savingProfile = false;
  }

  async function savePassword(e) {
    e.preventDefault();
    savingPw = true; pwErrors = {};
    try {
      const res = await patch("/settings/account/password", { user: pw });
      pw = { password_challenge: "", password: "", password_confirmation: "" };
      toast(res.summary);
    } catch (err) { pwErrors = err.errors; if (!Object.keys(pwErrors).length) toastError(err); else focusInvalid(); }
    savingPw = false;
  }

  async function uploadAvatar(e) {
    const file = e.currentTarget.files?.[0];
    if (!file) return;
    const form = new FormData();
    form.append("avatar", file);
    try { const res = await post("/settings/account/avatar", form); toast(res.summary); account.avatar = true; avatarVersion = Date.now(); }
    catch (err) { toastError(err); }
    e.currentTarget.value = "";
  }
  async function removeAvatar() {
    try { const res = await del("/settings/account/avatar"); toast(res.summary); account.avatar = false; avatarVersion = Date.now(); }
    catch (err) { toastError(err); }
  }

  async function setTextSize(value) {
    try {
      const res = await patch("/settings/account", { user: { text_size: value } });
      account = res; toast(res.summary);
      appearance.personal(res.text_size);
      refreshSession();
    } catch (err) { toastError(err); }
  }

  async function setPasswordless(on) {
    try { const res = await patch("/settings/passwordless", { user: { passwordless: on } }); account.passwordless = res.passwordless; toast(res.summary); }
    catch (err) { toastError(err); account.passwordless = !on; }
  }

  async function disconnect(identity) {
    if (!(await confirm({ title: `Disconnect ${identity.name}?`, message: "You won’t be able to sign in with this account until you connect it again.", confirmLabel: "Disconnect", destructive: true }))) return;
    try { const res = await del(`/settings/identities/${identity.id}`); toast(res.summary); load(); } catch (err) { toastError(err); }
  }

  // Asks for the password, then runs what needs it (new codes, turning off).
  function withPassword(kind) { askPassword = kind; passwordInput = ""; }
  async function confirmPassword(e) {
    e.preventDefault();
    passwordBusy = true;
    try {
      const res = await post("/settings/two_factor/recovery_codes", { password: passwordInput });
      codes = res.recovery_codes; toast(res.summary);
      askPassword = null;
      load(); refreshSession();
    } catch (err) { toastError(err); }
    passwordBusy = false;
  }
  async function turnOff(e) {
    e.preventDefault();
    passwordBusy = true;
    try {
      const res = await fetchDelete("/settings/two_factor", { password: passwordInput });
      toast(res.summary); askPassword = null; load(); refreshSession();
    } catch (err) { toastError(err); }
    passwordBusy = false;
  }
  // DELETE with a body (the password), which del() doesn't send.
  const fetchDelete = (path, body) => api("DELETE", path, body);

  // Your menu: the order and which show; saved at once.
  async function saveMenu(order, shown, move = "") {
    try {
      const res = await patch("/settings/menu", { order, shown, move });
      account.menu = res.menu; toast(res.summary);
      nav.refresh();
    } catch (err) { toastError(err); }
  }
  const menuKeys = () => account.menu.map((m) => m.key);
  const shownKeys = () => account.menu.filter((m) => m.shown).map((m) => m.key);
  function toggleShown(item, on) {
    const shown = shownKeys().filter((k) => k !== item.key);
    if (on) shown.push(item.key);
    saveMenu(menuKeys(), shown);
  }
</script>

<SettingsLayout title="Your account" description="Choices that are yours alone, not the install’s.">
  {#if !account}
    <Skeleton class="h-64 w-full" />
  {:else}
    <Card.Root>
      <Card.Header>
        <Card.Title>Profile</Card.Title>
        <Card.Description>Your name and picture show on your work; you sign in with your address.</Card.Description>
      </Card.Header>
      <Card.Content>
        <form onsubmit={saveProfile}>
          <Field.Group>
            <div class="flex items-center gap-4">
              <Avatar.Root class="size-16 rounded-lg">
                <Avatar.Image src={`/api/users/${account.id}/avatar?v=${avatarVersion}`} alt={account.name} />
                <Avatar.Fallback class="rounded-lg">{initials(account.name)}</Avatar.Fallback>
              </Avatar.Root>
              <div class="flex flex-wrap gap-2">
                <Button variant="outline" size="sm" onclick={() => document.getElementById("avatar-file")?.click()}>{account.avatar ? "Replace picture" : "Add a picture"}</Button>
                {#if account.avatar}<Button variant="ghost" size="sm" onclick={removeAvatar}>Remove</Button>{/if}
                <input id="avatar-file" type="file" accept="image/png, image/jpeg, image/gif, image/webp" class="hidden" onchange={uploadAvatar} />
              </div>
            </div>
            <Field.Field data-invalid={!!profileErrors.name}>
              <Field.Label for="account-name">Name</Field.Label>
              <Input id="account-name" bind:value={profile.name} required autocomplete="name" />
              {#if profileErrors.name}<Field.Error>Name {profileErrors.name[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field data-invalid={!!profileErrors.email_address}>
              <Field.Label for="account-email">Email address</Field.Label>
              <Input id="account-email" type="email" bind:value={profile.email_address} required autocomplete="email" />
              {#if profileErrors.email_address}<Field.Error>Email address {profileErrors.email_address[0]}</Field.Error>{/if}
            </Field.Field>
            {#if emailChanged}
              <Field.Field data-invalid={!!profileErrors.password_challenge}>
                <Field.Label for="account-challenge">Your password</Field.Label>
                <Input id="account-challenge" type="password" bind:value={profile.password_challenge} required autocomplete="current-password" />
                <Field.Description>To change the address you sign in with.</Field.Description>
                {#if profileErrors.password_challenge}<Field.Error>That password isn’t right.</Field.Error>{/if}
              </Field.Field>
            {/if}
            {#if profileErrors.base}<p class="text-destructive text-sm">{profileErrors.base.join(" ")}</p>{/if}
            <div><Button type="submit" disabled={savingProfile}>Save profile</Button></div>
          </Field.Group>
        </form>
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Password</Card.Title>
        <Card.Description>Changing it signs you out of every other browser.</Card.Description>
      </Card.Header>
      <Card.Content>
        <form onsubmit={savePassword}>
          <Field.Group>
            <Field.Field data-invalid={!!pwErrors.password_challenge}>
              <Field.Label for="pw-current">Current password</Field.Label>
              <Input id="pw-current" type="password" bind:value={pw.password_challenge} required autocomplete="current-password" />
              {#if pwErrors.password_challenge}<Field.Error>That password isn’t right.</Field.Error>{/if}
            </Field.Field>
            <div class="grid gap-4 sm:grid-cols-2">
              <Field.Field data-invalid={!!pwErrors.password}>
                <Field.Label for="pw-new">New password</Field.Label>
                <Input id="pw-new" type="password" bind:value={pw.password} required maxlength={72} autocomplete="new-password" />
                {#if pwErrors.password}<Field.Error>Password {pwErrors.password[0]}</Field.Error>{/if}
              </Field.Field>
              <Field.Field data-invalid={!!pwErrors.password_confirmation}>
                <Field.Label for="pw-confirm">Confirm new password</Field.Label>
                <Input id="pw-confirm" type="password" bind:value={pw.password_confirmation} required maxlength={72} autocomplete="new-password" />
                {#if pwErrors.password_confirmation}<Field.Error>That doesn’t match.</Field.Error>{/if}
              </Field.Field>
            </div>
            <div><Button type="submit" variant="outline" disabled={savingPw}>Change password</Button></div>
          </Field.Group>
        </form>
      </Card.Content>
    </Card.Root>

    <Card.Root id="sign-in">
      <Card.Header><Card.Title>Ways to sign in</Card.Title></Card.Header>
      <Card.Content class="flex flex-col gap-4">
        <Field.Field orientation="horizontal">
          <Field.Content>
            <Field.Label for="passwordless">Sign in with an emailed link</Field.Label>
            <Field.Description>“Email me a sign-in link” on the sign-in page sends you a link that works once, for 15 minutes. Your password keeps working too.</Field.Description>
          </Field.Content>
          <Switch id="passwordless" bind:checked={account.passwordless} onCheckedChange={setPasswordless} />
        </Field.Field>
        {#if account.providers.length || account.identities.length}
          <ul class="divide-y rounded-md border">
            {#each [...new Set([...account.providers.map((p) => p.key), ...account.identities.map((i) => i.provider)])] as key (key)}
              {@const identity = account.identities.find((i) => i.provider === key)}
              {@const provider = account.providers.find((p) => p.key === key)}
              <li class="flex items-center gap-3 px-3 py-2 text-sm">
                <div class="min-w-0 flex-1">
                  <p class="font-medium">{identity?.name || provider?.name}</p>
                  <p class="text-muted-foreground truncate text-xs">
                    {#if identity}Connected{identity.email ? ` as ${identity.email}` : ""}{identity.last_used_at ? ` · last used ${timeAgo(identity.last_used_at)}` : ""}{:else}Not connected{/if}
                  </p>
                </div>
                {#if identity}
                  <Button variant="ghost" size="sm" onclick={() => disconnect(identity)}>Disconnect</Button>
                {:else if provider}
                  <form method="post" action={`/auth/${key}`}><Button type="submit" variant="outline" size="sm">Connect</Button></form>
                {/if}
              </li>
            {/each}
          </ul>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root id="two-factor">
      <Card.Header>
        <Card.Title>Two-factor sign-in</Card.Title>
        {#if account.two_factor.on}
          <Card.Action><Badge>On</Badge></Card.Action>
        {/if}
      </Card.Header>
      <Card.Content class="flex flex-col gap-3 text-sm">
        {#if account.two_factor.on}
          <p>On since {date(account.two_factor.since)}. After your password, Runwell asks for a code from your authenticator app.</p>
          <p class="text-muted-foreground">{plural(account.two_factor.recovery_codes_left, "recovery code")} left.</p>
          <div class="flex flex-wrap gap-2">
            <Button variant="outline" size="sm" onclick={() => withPassword("codes")}>New recovery codes</Button>
            {#if !account.two_factor.required}
              <Button variant="ghost" size="sm" class="text-destructive" onclick={() => withPassword("off")}>Turn off</Button>
            {/if}
          </div>
          {#if account.two_factor.required}<p class="text-muted-foreground text-xs">This install requires two-factor sign-in, so it stays on.</p>{/if}
        {:else}
          <p>Off. Turn it on and Runwell asks for a code from an app on your phone after your password, so a stolen password isn’t enough to get in.</p>
          <div><Button href="/settings/two_factor/new" size="sm"><Lock />Set up two-factor sign-in</Button></div>
        {/if}
      </Card.Content>
    </Card.Root>

    <Card.Root>
      <Card.Header>
        <Card.Title>Text size</Card.Title>
        <Card.Description>Yours alone, on every device you sign in on. “Install default” uses the size set in Settings › Appearance (now {account.text_sizes[account.install_text_size].toLowerCase()}).</Card.Description>
      </Card.Header>
      <Card.Content>
        <RadioGroup.Root value={account.text_size || ""} onValueChange={setTextSize} class="grid grid-cols-2 gap-3 sm:grid-cols-5">
          {#each [["", "Install default"], ...Object.entries(account.text_sizes)] as [value, label] (value)}
            <Field.Label for={`ts-${value || "install"}`} class="w-full has-data-[state=checked]:border-primary flex cursor-pointer flex-col items-center gap-2 rounded-md border p-3">
              <RadioGroup.Item value={value} id={`ts-${value || "install"}`} class="sr-only" />
              <span class="font-semibold" style:font-size={{ small: "0.85em", default: "1em", large: "1.15em", larger: "1.3em" }[value] || "1em"}>Aa</span>
              <span class="text-xs">{label}</span>
            </Field.Label>
          {/each}
        </RadioGroup.Root>
      </Card.Content>
    </Card.Root>

    <Card.Root id="menu">
      <Card.Header>
        <Card.Title>Your menu</Card.Title>
        <Card.Description>Which plugins show in the menu on the left, and in what order: yours alone.</Card.Description>
      </Card.Header>
      <Card.Content>
        {#if !account.menu.length}
          <p class="text-muted-foreground text-sm">No plugin with a page is switched on.</p>
        {:else}
          <ul class="divide-y rounded-md border">
            {#each account.menu as item, i (item.key)}
              <li class="flex items-center gap-2 px-3 py-2 text-sm" class:opacity-60={!item.shown}>
                <span class="flex-1 font-medium">{item.label}</span>
                <Button variant="ghost" size="icon-sm" disabled={i === 0} aria-label={`Move ${item.label} up`} onclick={() => saveMenu(menuKeys(), shownKeys(), `${item.key}:up`)}><ArrowUp /></Button>
                <Button variant="ghost" size="icon-sm" disabled={i === account.menu.length - 1} aria-label={`Move ${item.label} down`} onclick={() => saveMenu(menuKeys(), shownKeys(), `${item.key}:down`)}><ArrowDown /></Button>
                <Switch checked={item.shown} aria-label={`Show ${item.label} in your menu`} onCheckedChange={(on) => toggleShown(item, on)} />
              </li>
            {/each}
          </ul>
        {/if}
      </Card.Content>
    </Card.Root>
  {/if}
</SettingsLayout>

<Dialog.Root open={!!askPassword} onOpenChange={(o) => { if (!o) askPassword = null; }}>
  <Dialog.Content class="sm:max-w-md">
    <form onsubmit={askPassword === "off" ? turnOff : confirmPassword} class="flex flex-col gap-4">
      <Dialog.Header>
        <Dialog.Title>{askPassword === "off" ? "Turn off two-factor sign-in" : "New recovery codes"}</Dialog.Title>
        <Dialog.Description>{askPassword === "off" ? "Enter your password to turn it off." : "Your old codes stop working. Enter your password to see new ones."}</Dialog.Description>
      </Dialog.Header>
      <Input type="password" bind:value={passwordInput} required autocomplete="current-password" placeholder="Your password" aria-label="Your password" />
      <Dialog.Footer>
        <Button variant="ghost" onclick={() => (askPassword = null)}>Cancel</Button>
        <Button type="submit" variant={askPassword === "off" ? "destructive" : "default"} disabled={passwordBusy}>{askPassword === "off" ? "Turn off two-factor sign-in" : "Show new codes"}</Button>
      </Dialog.Footer>
    </form>
  </Dialog.Content>
</Dialog.Root>

<RecoveryCodes bind:codes />
