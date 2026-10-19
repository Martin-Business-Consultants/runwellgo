<script>
  // Settings › Chat, for each person: how chat shows you (a name, a
  // picture, a status), the hours you're available (outside them you show
  // as away and nothing notifies you; messages wait, unread), and what
  // notifies you: in the bell (and by email when you're away from
  // Runwell), with a sound, or as a browser notification.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Field from "$lib/components/ui/field/index.js";
  import * as RadioGroup from "$lib/components/ui/radio-group/index.js";
  import * as Select from "$lib/components/ui/select/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import ChatAvatar from "./ChatAvatar.svelte";
  import EmojiPicker from "./EmojiPicker.svelte";
  import { chat, BASE, chime } from "./chat.svelte.js";
  import { get, patch, del, post, ApiError } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { focusInvalid } from "$lib/forms.js";

  chat.start();
  let s = $state(null);
  let errors = $state({});
  let saving = $state(false);
  let permission = $state(typeof Notification === "undefined" ? "unsupported" : Notification.permission);
  let avatarV = $state(0);

  const DAYS = [[1, "Mon"], [2, "Tue"], [3, "Wed"], [4, "Thu"], [5, "Fri"], [6, "Sat"], [0, "Sun"]];
  const here = Intl.DateTimeFormat().resolvedOptions().timeZone;
  const zones = (() => {
    try { return Intl.supportedValuesOf("timeZone"); } catch { return [here]; }
  })();

  async function load() {
    try {
      s = (await get(`${BASE}/settings`)).settings;
      if (!s.hours.zone) s.hours.zone = here;
    } catch (e) { toastError(e); }
  }
  load();

  async function save(e) {
    e?.preventDefault();
    saving = true;
    errors = {};
    try {
      s = (await patch(`${BASE}/settings`, { settings: s })).settings;
      chat.settings = s;
      toast("Saved your chat settings.");
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) { errors = err.errors; focusInvalid(); }
      else toastError(err);
    } finally { saving = false; }
  }

  function toggleDay(d, on) {
    s.hours.days = on ? [...new Set([...s.hours.days, d])] : s.hours.days.filter((x) => x !== d);
  }

  async function browser(on) {
    if (on && permission !== "granted" && typeof Notification !== "undefined") {
      permission = await Notification.requestPermission();
      if (permission !== "granted") {
        s.browser = false;
        toastError(new Error("Your browser said no. Allow notifications for this site in its settings, then switch this on again."));
        return;
      }
    }
    s.browser = on;
  }

  async function upload(e) {
    const file = e.currentTarget.files?.[0];
    e.currentTarget.value = "";
    if (!file) return;
    if (file.size > 4 * 1024 * 1024) { toastError(new Error("That picture is larger than 4 MB: pick a smaller one.")); return; }
    try {
      s = (await post(`${BASE}/settings/avatar?filename=${encodeURIComponent(file.name)}`, file)).settings;
      avatarV++;
      chat.load();
    } catch (err) { toastError(err); }
  }

  async function removeAvatar() {
    try { s = (await del(`${BASE}/settings/avatar`)).settings; avatarV++; chat.load(); } catch (err) { toastError(err); }
  }

  const preview = $derived(s && chat.me ? { ...chat.me, avatar_url: s.avatar_url + (avatarV ? `${s.avatar_url.includes("?") ? "&" : "?"}r=${avatarV}` : ""), name: s.display_name || chat.me.full_name } : null);
</script>

<SettingsLayout title="Chat" description="How chat shows you, when you're available, and what notifies you.">
  {#if !s}
    <Skeleton class="h-64" />
  {:else}
    <form onsubmit={save} class="flex flex-col gap-6">
      <Card.Root>
        <Card.Header>
          <Card.Title>How you show</Card.Title>
          <Card.Description>Your name and picture in chat; your Runwell ones when you leave these empty.</Card.Description>
        </Card.Header>
        <Card.Content>
          <Field.Group class="gap-5">
            <div class="flex flex-wrap items-center gap-4">
              {#if preview}<ChatAvatar person={preview} size="size-16" />{/if}
              <div class="flex flex-wrap gap-2">
                <Button type="button" variant="outline" size="sm" onclick={() => document.getElementById("chat-avatar-file")?.click()}>{s.has_avatar ? "Replace picture" : "Add a chat picture"}</Button>
                {#if s.has_avatar}<Button type="button" variant="ghost" size="sm" onclick={removeAvatar}>Use my Runwell picture</Button>{/if}
                <input id="chat-avatar-file" type="file" accept="image/png, image/jpeg, image/gif, image/webp" class="hidden" onchange={upload} />
              </div>
            </div>
            <Field.Field data-invalid={!!errors.display_name}>
              <Field.Label for="chat-display-name">Name in chat</Field.Label>
              <Input id="chat-display-name" bind:value={s.display_name} placeholder={chat.me?.full_name} maxlength={80} />
              {#if errors.display_name}<Field.Error>Name {errors.display_name[0]}</Field.Error>{/if}
            </Field.Field>
            <Field.Field data-invalid={!!errors.status_text}>
              <Field.Label for="chat-status">Status</Field.Label>
              <div class="flex items-center gap-2">
                <EmojiPicker onpick={(e) => (s.status_emoji = e)} current={s.status_emoji} label={s.status_emoji ? `Status emoji ${s.status_emoji}: change it` : "Pick a status emoji"} class="size-9 shrink-0 border" align="start" />
                <Input id="chat-status" bind:value={s.status_text} placeholder="In a meeting, on leave until Monday…" maxlength={100} />
                {#if s.status_text || s.status_emoji}<Button type="button" variant="ghost" size="sm" onclick={() => { s.status_text = ""; s.status_emoji = ""; }}>Clear</Button>{/if}
              </div>
              {#if errors.status_text}<Field.Error>Status {errors.status_text[0]}</Field.Error>{/if}
            </Field.Field>
          </Field.Group>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>Available hours</Card.Title>
          <Card.Description>Outside them you show as away, and nothing notifies you: messages wait for you, unread.</Card.Description>
        </Card.Header>
        <Card.Content>
          <Field.Group class="gap-5">
            <Field.Field orientation="horizontal">
              <Field.Content>
                <Field.Label for="chat-hours-on">Keep set hours</Field.Label>
                <Field.Description>Off: you're available whenever you have Runwell open.</Field.Description>
              </Field.Content>
              <Switch id="chat-hours-on" bind:checked={s.hours.on} />
            </Field.Field>
            {#if s.hours.on}
              <Field.Set data-invalid={!!errors["hours.days"]}>
                <Field.Legend variant="label">Days</Field.Legend>
                <div class="flex flex-wrap gap-1.5">
                  {#each DAYS as [d, label] (d)}
                    <Button type="button" size="sm" variant={s.hours.days.includes(d) ? "default" : "outline"} aria-pressed={s.hours.days.includes(d)} onclick={() => toggleDay(d, !s.hours.days.includes(d))} class="w-12">{label}</Button>
                  {/each}
                </div>
                {#if errors["hours.days"]}<Field.Error>Days {errors["hours.days"][0]}</Field.Error>{/if}
              </Field.Set>
              <div class="grid gap-4 sm:grid-cols-3">
                <Field.Field data-invalid={!!errors["hours.start"]}>
                  <Field.Label for="chat-hours-start">From</Field.Label>
                  <Input id="chat-hours-start" type="time" bind:value={s.hours.start} required />
                  {#if errors["hours.start"]}<Field.Error>From {errors["hours.start"][0]}</Field.Error>{/if}
                </Field.Field>
                <Field.Field data-invalid={!!errors["hours.end"]}>
                  <Field.Label for="chat-hours-end">Until</Field.Label>
                  <Input id="chat-hours-end" type="time" bind:value={s.hours.end} required />
                  {#if errors["hours.end"]}<Field.Error>Until {errors["hours.end"][0]}</Field.Error>{/if}
                </Field.Field>
                <Field.Field data-invalid={!!errors["hours.zone"]}>
                  <Field.Label for="chat-hours-zone">Time zone</Field.Label>
                  <Select.Root type="single" bind:value={s.hours.zone}>
                    <Select.Trigger id="chat-hours-zone" class="w-full"><span class="truncate">{s.hours.zone || "Pick a zone"}</span></Select.Trigger>
                    <Select.Content class="max-h-72">
                      {#each zones as z (z)}<Select.Item value={z} label={z}>{z}</Select.Item>{/each}
                    </Select.Content>
                  </Select.Root>
                  {#if errors["hours.zone"]}<Field.Error>Time zone {errors["hours.zone"][0]}</Field.Error>{/if}
                </Field.Field>
              </div>
            {/if}
          </Field.Group>
        </Card.Content>
      </Card.Root>

      <Card.Root>
        <Card.Header>
          <Card.Title>Notifications</Card.Title>
          <Card.Description>In your bell, and by email when you don't have Runwell open. Mute a conversation from its details.</Card.Description>
        </Card.Header>
        <Card.Content>
          <Field.Group class="gap-5">
            <Field.Set>
              <Field.Legend variant="label">Notify me about</Field.Legend>
              <RadioGroup.Root bind:value={s.notify} class="gap-3">
                {#each [["all", "Every message", "Everything in the conversations you're in."], ["mentions", "Mentions and direct messages", "When someone @mentions you, writes to you directly or in a group, or replies in a thread you're in."], ["none", "Nothing", "You'll see unread counts, nothing more."]] as [v, label, help] (v)}
                  <Field.Field orientation="horizontal">
                    <RadioGroup.Item value={v} id="chat-notify-{v}" />
                    <Field.Content>
                      <Field.Label for="chat-notify-{v}">{label}</Field.Label>
                      <Field.Description>{help}</Field.Description>
                    </Field.Content>
                  </Field.Field>
                {/each}
              </RadioGroup.Root>
            </Field.Set>
            <Field.Field orientation="horizontal">
              <Field.Content>
                <Field.Label for="chat-sound">Play a sound</Field.Label>
                <Field.Description>When a message for you arrives and you're not looking at its conversation. <button type="button" class="text-primary underline underline-offset-2" onclick={chime}>Hear it</button></Field.Description>
              </Field.Content>
              <Switch id="chat-sound" bind:checked={s.sound} />
            </Field.Field>
            <Field.Field orientation="horizontal">
              <Field.Content>
                <Field.Label for="chat-browser">Browser notifications</Field.Label>
                <Field.Description>
                  {#if permission === "unsupported"}This browser doesn't show notifications.
                  {:else if permission === "denied"}Your browser blocks them for this site: allow notifications in its site settings, then switch this on.
                  {:else}A notification from your browser, even when Runwell is in another tab.{/if}
                </Field.Description>
              </Field.Content>
              <Switch id="chat-browser" checked={s.browser} onCheckedChange={browser} disabled={permission === "unsupported"} />
            </Field.Field>
          </Field.Group>
        </Card.Content>
      </Card.Root>

      <div><Button type="submit" disabled={saving}>Save chat settings</Button></div>
    </form>
  {/if}
</SettingsLayout>
