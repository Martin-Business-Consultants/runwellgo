<script>
  // Settings › Import: replace everything here with a backup from Runwell on
  // Rails (its Settings › Export › Back up for Runwell Go). The backup is
  // uploaded and checked, then Runwell restarts and puts it in place; the
  // install it replaces is kept in the data directory. Everyone signs in
  // again, with the passwords they had there.
  import SettingsLayout from "$lib/layout/SettingsLayout.svelte";
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Alert from "$lib/components/ui/alert/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import Upload from "@lucide/svelte/icons/upload";
  import TriangleAlert from "@lucide/svelte/icons/triangle-alert";
  import { get, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { bytes, dateTime, plural } from "$lib/format.js";

  let info = $state(null), file = $state(null), progress = $state(0), stage = $state("pick"), result = $state(null), error = $state("");
  // A data export's password for everyone in it: it has none of its own.
  let password = $state(""), needsPassword = $state(false);
  let input = $state();
  get("/settings/import").then((d) => (info = d)).catch(toastError);

  // XHR rather than fetch: a backup can be large, and this shows how far it got.
  function upload() {
    return new Promise((resolve, reject) => {
      const body = new FormData();
      if (password) body.append("password", password);
      body.append("file", file);
      const xhr = new XMLHttpRequest();
      xhr.open("POST", "/api/settings/import");
      xhr.setRequestHeader("X-CSRF-Token", session.csrf);
      xhr.setRequestHeader("Accept", "application/json");
      xhr.upload.onprogress = (e) => { if (e.lengthComputable) progress = Math.round((e.loaded / e.total) * 100); };
      xhr.onload = () => {
        let data = {};
        try { data = JSON.parse(xhr.responseText); } catch { /* not JSON */ }
        if (xhr.status < 300) return resolve(data);
        const err = new Error(data.message || `The import failed (${xhr.status}).`);
        err.code = data.error;
        reject(err);
      };
      xhr.onerror = () => reject(new Error("The upload was cut off."));
      xhr.send(body);
    });
  }

  async function start() {
    if (!file) return;
    if (password && password.length < 8) { error = "The password needs at least 8 characters."; return; }
    if (!(await confirm({ title: "Replace everything here?", message: `Everything in this Runwell (${session.term("client", 2).toLowerCase()}, ${session.term("engagement", 2).toLowerCase()}, people, settings and files) is replaced by the import. What's here now is kept in the data directory, not deleted. Everyone, you included, signs in again: with their own password from a backup, or the one you chose for a data export.`, confirmLabel: "Import and replace", destructive: true }))) return;
    stage = "uploading"; error = ""; progress = 0;
    try {
      result = await upload();
      stage = result.restarting ? "restarting" : "staged";
      if (result.restarting) waitForRestart();
    } catch (e) {
      stage = "pick";
      if (e.code === "needs_password") { needsPassword = true; error = "This is a data export: it has no passwords. Choose one for everyone in it, then import again."; }
      else error = e.message;
    }
  }

  // Once Runwell answers again (on the imported database), go to sign in.
  async function waitForRestart() {
    await new Promise((r) => setTimeout(r, 2500));
    for (let i = 0; i < 120; i++) {
      try {
        const r = await fetch("/up", { cache: "no-store" });
        if (r.ok) { session.signedOut(); location.href = "/"; return; }
      } catch { /* still restarting */ }
      await new Promise((r) => setTimeout(r, 1000));
    }
    error = "Runwell hasn't come back after two minutes. Check its log, or start it again.";
  }

  async function discard() {
    try { info = await del("/settings/import"); toast("The staged backup is discarded."); } catch (e) { toastError(e); }
  }
</script>

<SettingsLayout title="Import" description="Replace everything here with a backup from Runwell on Rails, to try it out here or move to Runwell Go.">
  <Card.Root>
    <Card.Header>
      <Card.Title>Import a backup</Card.Title>
      <Card.Description>
        In Runwell on Rails, open Settings › Export and press <strong>Make a backup</strong> (under “Back up for Runwell Go”, from 2.30), then download it when it’s emailed,
        and upload that .zip here. A data export (<strong>Make an export</strong>) works too, for trying a copy out: it has no passwords, so everyone in it signs in with one you choose below.
      </Card.Description>
    </Card.Header>
    <Card.Content class="flex flex-col gap-4">
      <Alert.Root>
        <TriangleAlert />
        <Alert.Title>It replaces everything here</Alert.Title>
        <Alert.Description>
          People, {session.term("client", 2).toLowerCase()}, {session.term("engagement", 2).toLowerCase()}, settings and files all come from the backup. What’s here now is kept in the data directory
          (replaced-&lt;time&gt;), not deleted. The backup’s address comes with it, so emails sent from here link there until you change Settings › Address.
        </Alert.Description>
      </Alert.Root>

      {#if info?.pending && stage === "pick"}
        <Alert.Root>
          <Alert.Title>A backup is waiting to be put in place</Alert.Title>
          <Alert.Description class="flex flex-wrap items-center justify-between gap-2">
            <span>From {info.pending.manifest.host}, made {dateTime(info.pending.manifest.created_at)}. It goes in when Runwell next starts.</span>
            <Button variant="outline" size="sm" onclick={discard}>Discard it</Button>
          </Alert.Description>
        </Alert.Root>
      {/if}

      {#if stage === "pick"}
        <button type="button" onclick={() => input.click()}
          ondragover={(e) => e.preventDefault()} ondrop={(e) => { e.preventDefault(); file = e.dataTransfer.files[0] || null; }}
          class="border-muted-foreground/30 hover:bg-muted/50 flex flex-col items-center gap-2 rounded-lg border border-dashed px-6 py-10 text-center">
          <Upload class="text-muted-foreground size-6" />
          {#if file}
            <span class="text-sm font-medium">{file.name}</span>
            <span class="text-muted-foreground text-xs">{bytes(file.size)} · click to choose another</span>
          {:else}
            <span class="text-sm font-medium">Drop the backup’s .zip here, or click to choose it</span>
            {#if info}<span class="text-muted-foreground text-xs">Up to {bytes(info.max_bytes)}; a bigger one goes through <code>runwell import</code>.</span>{/if}
          {/if}
        </button>
        <input bind:this={input} type="file" accept=".zip,application/zip" class="hidden" onchange={(e) => (file = e.target.files[0] || null)} />
        <div class="flex max-w-sm flex-col gap-1.5">
          <label for="import-password" class="text-sm font-medium">Password for everyone {needsPassword ? "" : "(a data export only)"}</label>
          <Input id="import-password" type="password" bind:value={password} minlength={8} autocomplete="new-password" aria-invalid={needsPassword && password.length < 8} />
          <p class="text-muted-foreground text-xs">A data export leaves passwords out, so everyone in it, you included, signs in with this. A backup keeps their own; leave it blank.</p>
        </div>
        {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
        <div><Button variant="destructive" disabled={!file} onclick={start}>Import and replace everything</Button></div>
      {:else if stage === "uploading"}
        <div class="flex flex-col gap-2">
          <p class="text-sm">{progress < 100 ? `Uploading ${file.name}…` : "Checking the backup…"}</p>
          <Progress value={progress} />
        </div>
      {:else}
        {@const r = result.report}
        <div class="flex flex-col gap-1 text-sm">
          <p class="font-medium">The backup checked out.</p>
          {#if r.kind === "data"}
            <p class="text-muted-foreground">A data export of {r.manifest.host}, made {r.manifest.created_at} by {r.manifest.by}: {plural(r.people, "person", "people")} who sign in with the password you chose, {r.clients} {session.term("client", r.clients).toLowerCase()}, {plural(r.files, "file")}.</p>
            {#if r.skipped?.length}<p class="text-muted-foreground">Left out, as tables this Runwell doesn’t have (a Rails plugin’s): {r.skipped.join(", ")}.</p>{/if}
          {:else}
            <p class="text-muted-foreground">From {r.manifest.host}, made {dateTime(r.manifest.created_at)} by {r.manifest.by} (Runwell {r.manifest.runwell_version}): {plural(r.people, "person", "people")} who can sign in, {r.clients} {session.term("client", r.clients).toLowerCase()}, {plural(r.files, "file")}.</p>
          {/if}
        </div>
        {#if stage === "restarting"}
          <p class="text-sm">Runwell is restarting to put it in place. This page goes to sign in once it’s back.</p>
          <Progress value={null} class="animate-pulse" />
        {:else}
          <Alert.Root><Alert.Description>Runwell can’t restart itself here. Restart it (stop and start <code>runwell serve</code>) and the backup goes in place as it starts.</Alert.Description></Alert.Root>
        {/if}
        {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
      {/if}
    </Card.Content>
  </Card.Root>

  <Card.Root>
    <Card.Header>
      <Card.Title>From a terminal</Card.Title>
      <Card.Description>With Runwell stopped, the same import, for a backup of any size:</Card.Description>
    </Card.Header>
    <Card.Content><pre class="bg-muted overflow-x-auto rounded-md p-3 text-xs">runwell import runwell-backup.zip --data ./data --force</pre></Card.Content>
  </Card.Root>
</SettingsLayout>
