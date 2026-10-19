<script>
  // Settings › Updates: the version this install runs, the newest release,
  // the button that updates to it, and how past updates went.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import RefreshCw from "@lucide/svelte/icons/refresh-cw";
  import Trash from "@lucide/svelte/icons/trash-2";
  import SecretBox from "$lib/components/SecretBox.svelte";
  import { get, post, del } from "$lib/api.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { timeAgo, date, plural, humanize } from "$lib/format.js";

  let data = $state(null), timer = null;

  function load() {
    return get("/settings/updates").then((d) => {
      data = d;
      clearTimeout(timer);
      if (d.checking || d.updating) timer = setTimeout(load, d.updating ? 4000 : 2000);
    }).catch(() => {
      // Restarting on the new version: look again shortly.
      clearTimeout(timer);
      if (data?.updating) timer = setTimeout(load, 4000);
    });
  }
  load();
  $effect(() => () => clearTimeout(timer));

  async function check() {
    try { toast((await post("/settings/updates/check")).summary); load(); } catch (err) { toastError(err); }
  }
  async function update() {
    if (!(await confirm({ title: `Update to ${data.latest_release.version}?`, message: `Runwell backs up its database, updates and restarts. It’s unavailable for a minute or two; nobody is signed out. If anything fails, ${data.version} keeps running and this page says why.`, confirmLabel: "Update now" }))) return;
    try { toast((await post("/settings/updates")).summary); load(); } catch (err) { toastError(err); }
  }
  async function clearFailed() {
    if (!(await confirm({ title: `Clear ${plural(data.failed_count, "failed update")}?`, message: "They leave Past updates, with what went wrong. Updates that worked stay.", confirmLabel: "Clear", destructive: true }))) return;
    try { toast((await del("/settings/updates/failures")).summary); load(); } catch (err) { toastError(err); }
  }
</script>

<SettingsLayout title="Updates">
  {#snippet actions()}
    {#if data?.failed_count}<Button variant="ghost" size="sm" class="text-destructive" onclick={clearFailed}><Trash />Clear failed updates</Button>{/if}
  {/snippet}
  {#if !data}
    <Skeleton class="h-64 w-full" />
  {:else}
    <Card.Root>
      <Card.Header><Card.Title>This install</Card.Title></Card.Header>
      <Card.Content class="flex flex-col gap-4 text-sm">
        <dl class="grid gap-x-6 gap-y-2 sm:grid-cols-[10rem_1fr]">
          <dt class="text-muted-foreground">Running</dt><dd>Runwell {data.version}</dd>
          <dt class="text-muted-foreground">Newest release</dt>
          <dd>
            {#if data.latest_release}
              <a href={data.latest_release.url} target="_blank" rel="noopener" class="underline underline-offset-4">Runwell {data.latest_release.version}</a>
              <span class="text-muted-foreground">{data.latest_release.newer ? "(newer)" : "(up to date)"}</span>
            {:else}<span class="text-muted-foreground">Not checked yet</span>{/if}
          </dd>
          <dt class="text-muted-foreground">Checked</dt>
          <dd>{#if data.checked_at}{timeAgo(data.checked_at)}{data.checking_nightly ? ", and every night" : ""}{:else}<span class="text-muted-foreground">Never</span>{/if}</dd>
          <dt class="text-muted-foreground">Updates by</dt><dd>{data.updates_by_description}</dd>
        </dl>
        {#if data.checking}<p class="text-muted-foreground">Checking GitHub for a newer release…</p>
        {:else if data.check_error}<p class="text-destructive">Couldn’t check: {data.check_error}</p>{/if}
        {#if data.failed_release}<p class="text-destructive">Runwell {data.failed_release.version} didn’t come up, so this install stayed on {data.version}.</p>{/if}
        <div><Button variant="outline" size="sm" onclick={check}><RefreshCw />Check now</Button></div>
      </Card.Content>
    </Card.Root>

    {#if data.updating}
      <Card.Root>
        <Card.Header><Card.Title>Updating to {data.updating.to_version}</Card.Title></Card.Header>
        <Card.Content class="text-sm">
          Started {timeAgo(data.updating.started_at)} by {data.updating.requested_by}. The install backs up its database, swaps in {data.updating.to_version} and restarts; until then {data.version} keeps running. This page checks every few seconds.
        </Card.Content>
      </Card.Root>
    {:else if data.latest_release?.newer}
      <Card.Root>
        <Card.Header>
          <Card.Title>Runwell {data.latest_release.version}</Card.Title>
          {#if data.latest_release.published_at}<Card.Description>Released {date(data.latest_release.published_at)}.</Card.Description>{/if}
        </Card.Header>
        <Card.Content class="flex flex-col gap-4 text-sm">
          {#if data.latest_release.notes}<div class="whitespace-pre-line">{data.latest_release.notes}</div>{/if}
          {#if data.available}
            <p>Runwell backs up its database, updates and restarts. It’s unavailable for a minute or two; nobody is signed out. If anything fails, {data.version} keeps running and this page says why.</p>
            <div><Button onclick={update}>Update to {data.latest_release.version}</Button></div>
          {:else}
            <p>Updating from here isn’t set up on this install. On the server:</p>
            <SecretBox value={data.manual_command} />
          {/if}
        </Card.Content>
      </Card.Root>
    {/if}

    {#if data.updates.length}
      <Card.Root>
        <Card.Header><Card.Title>Past updates</Card.Title></Card.Header>
        <Card.Content>
          <ul class="divide-y rounded-md border">
            {#each data.updates as u (u.id)}
              <li class="flex flex-col gap-1 px-3 py-2 text-sm">
                <p><strong>{u.from_version} → {u.to_version}</strong> <Badge variant={u.status === "failed" ? "destructive" : u.status === "succeeded" ? "default" : "outline"}>{humanize(u.status)}</Badge> <span class="text-muted-foreground">· {u.requested_by} · {timeAgo(u.started_at)}</span>
                  {#if u.log_url} · <a href={u.log_url} target="_blank" rel="noopener" class="underline underline-offset-4">Log</a>{/if}</p>
                {#if u.message}<pre class="bg-muted overflow-x-auto rounded p-2 text-xs whitespace-pre-wrap">{u.message}</pre>{/if}
              </li>
            {/each}
          </ul>
        </Card.Content>
      </Card.Root>
    {/if}
  {/if}
</SettingsLayout>
