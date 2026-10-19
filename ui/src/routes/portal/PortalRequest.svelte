<script>
  // One request, as the client sees it: what they asked, then the notes on
  // it, oldest first (theirs and the ones the team shared with them), with
  // a box to add one; beside it, where it stands and its files, which they
  // can add to. Never the team's internal notes or files.
  import * as Card from "$lib/components/ui/card/index.js";
  import { Badge } from "$lib/components/ui/badge/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import FileText from "@lucide/svelte/icons/file-text";
  import Upload from "@lucide/svelte/icons/upload";
  import RichText from "$lib/components/RichText.svelte";
  import TiptapEditor from "$lib/components/TiptapEditor.svelte";
  import EmptyState from "$lib/components/EmptyState.svelte";
  import PortalLayout from "./PortalLayout.svelte";
  import { toast, toastError, errorMessage } from "$lib/toast.svelte.js";
  import { bytes } from "$lib/format.js";
  import { portal, pget, ppost } from "./portal.svelte.js";

  let { params } = $props();
  let r = $state(null);
  let missing = $state(false);
  let body = $state(""), busy = $state(false), error = $state("");
  let uploads = $state([]); // {name, progress, error}
  let dragging = $state(false);
  let input = $state();

  async function load() {
    if (!(await portal.require())) return;
    try { r = (await pget(`/portal/requests/${encodeURIComponent(params.id)}`)).request; } catch (err) { if (err.status === 404) missing = true; else toastError(err); }
  }
  load();

  const label = (state) => (state === "open" ? "Open" : state === "promoted" ? "Taken on" : "Closed");
  const empty = (html) => !html || (!html.replace(/<[^>]*>/g, "").trim() && !/<img/.test(html));
  const who = (a) => (a?.you ? "You" : a?.name || (a?.from_client ? "Your team" : "Us"));

  async function addNote(e) {
    e?.preventDefault();
    if (busy) return;
    if (empty(body)) { error = "Write the note first."; return; }
    busy = true; error = "";
    try {
      const res = await ppost(`/portal/requests/${r.id}/notes`, { note: { body } });
      r.notes = res.notes;
      body = "";
      toast(res.summary);
    } catch (err) { error = errorMessage(err); }
    busy = false;
  }

  // One file, one request, so each shows its own progress.
  function send(file) {
    uploads.push({ name: file.name, progress: 0, error: "" });
    const item = uploads[uploads.length - 1];
    return new Promise((resolve) => {
      const form = new FormData();
      form.append("files", file);
      const xhr = new XMLHttpRequest();
      xhr.open("POST", `/api/portal/requests/${r.id}/documents`);
      xhr.setRequestHeader("X-CSRF-Token", portal.csrf);
      xhr.setRequestHeader("Accept", "application/json");
      xhr.upload.onprogress = (e) => { if (e.lengthComputable) item.progress = Math.round((e.loaded / e.total) * 100); };
      xhr.onload = () => {
        let res = {};
        try { res = JSON.parse(xhr.responseText || "{}"); } catch { /* not JSON */ }
        if (xhr.status >= 200 && xhr.status < 300) { item.progress = 100; resolve(res); }
        else { item.error = res.message || res.summary || `Upload failed (${xhr.status}).`; resolve(null); }
      };
      xhr.onerror = () => { item.error = "Upload failed: the connection dropped."; resolve(null); };
      xhr.send(form);
    });
  }

  async function upload(files) {
    files = [...(files || [])];
    if (!files.length) return;
    const results = await Promise.all(files.map(send));
    const done = results.filter(Boolean);
    if (done.length) {
      toast(`${done.length === 1 ? "1 file" : `${done.length} files`} added.`);
      r.documents = done.at(-1).documents;
    }
    for (const u of uploads.filter((u) => u.error)) toastError(new Error(`${u.name}: ${u.error}`));
    uploads = [];
    if (input) input.value = "";
  }

  function drop(e) {
    e.preventDefault();
    dragging = false;
    upload(e.dataTransfer?.files);
  }
</script>

<PortalLayout title={r ? r.subject : ""} back={{ href: "/portal/requests", label: "Requests" }}>
  {#if missing}
    <p class="text-muted-foreground text-sm">Couldn’t find that request. Your requests are listed on the <a href="/portal/requests" class="underline underline-offset-4">Requests</a> page.</p>
  {:else if !r}
    <Skeleton class="h-64" />
  {:else}
    <div class="grid gap-4 lg:grid-cols-3">
      <div class="flex min-w-0 flex-col gap-4 lg:col-span-2">
        <Card.Root>
          <Card.Header>
            <Card.Title>What you asked</Card.Title>
            <Card.Description>{[r.sender_name, portal.dateTime(r.received_at)].filter(Boolean).join(" · ")}</Card.Description>
          </Card.Header>
          <Card.Content>
            {#if r.body_html}<RichText html={r.body_html} />{:else}<p class="text-muted-foreground text-sm">No details, just the subject.</p>{/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header>
            <Card.Title>Notes{#if r.notes.length} <span class="text-muted-foreground ml-1 font-normal tabular-nums">{r.notes.length}</span>{/if}</Card.Title>
            <Card.Description>Between you and us about this request, oldest first.</Card.Description>
          </Card.Header>
          {#if r.notes.length}
            <Card.Content>
              <ul class="flex flex-col divide-y">
                {#each r.notes as n (n.id)}
                  <li class="flex flex-col gap-1 py-3 first:pt-0 last:pb-0">
                    <div class="text-muted-foreground text-xs"><span class="text-foreground font-medium">{who(n.author)}</span> · {portal.dateTime(n.occurred_at)}</div>
                    <RichText html={n.body_html} class="text-sm" />
                  </li>
                {/each}
              </ul>
            </Card.Content>
          {:else}
            <EmptyState class="mx-6" variant="inline" icon="document" title="No notes yet." description="Add anything we should know below. Our replies show here too." />
          {/if}
          <Card.Content>
            <form onsubmit={addNote} class="flex flex-col gap-2">
              <TiptapEditor bind:value={body} format="html" tools="email" label="Add a note" placeholder="Add a note for us…" minHeight="min-h-24" invalid={!!error} />
              {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
              <div><Button type="submit" disabled={busy}>Add note</Button></div>
            </form>
          </Card.Content>
        </Card.Root>
      </div>

      <div class="flex min-w-0 flex-col gap-4">
        <Card.Root>
          <Card.Header><Card.Title>Details</Card.Title></Card.Header>
          <Card.Content>
            <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <dt class="text-muted-foreground">Status</dt>
              <dd><Badge variant={r.state === "promoted" ? "default" : r.state === "open" ? "outline" : "secondary"}>{label(r.state)}</Badge></dd>
              <dt class="text-muted-foreground">Received</dt>
              <dd class="tabular-nums">{portal.dateTime(r.received_at)}</dd>
            </dl>
            {#if r.state === "closed" && r.status.startsWith("closed: ")}<p class="text-muted-foreground mt-3 text-sm">{r.status.slice(8)}</p>{/if}
          </Card.Content>
        </Card.Root>

        <Card.Root>
          <Card.Header>
            <Card.Title>Files{#if r.documents.length} <span class="text-muted-foreground ml-1 font-normal tabular-nums">{r.documents.length}</span>{/if}</Card.Title>
          </Card.Header>
          <Card.Content class="flex flex-col gap-3">
            <label
              class="hover:bg-muted/50 flex w-full cursor-pointer flex-col items-center gap-1 rounded-xl border border-dashed px-4 py-6 text-center text-sm transition-colors focus-within:ring-ring/50 focus-within:ring-[3px] {dragging ? 'border-primary bg-muted/50' : ''}"
              ondragover={(e) => { e.preventDefault(); dragging = true; }}
              ondragleave={() => (dragging = false)}
              ondrop={drop}>
              <Upload class="text-muted-foreground size-5" />
              <span class="font-medium">Drop files here, or click to choose</span>
              <span class="text-muted-foreground text-xs">We’ll see them on this request.</span>
              <input bind:this={input} type="file" multiple class="sr-only" onchange={(e) => upload(e.currentTarget.files)} />
            </label>
            {#if uploads.length}
              <ul class="flex flex-col gap-2">
                {#each uploads as u, i (i)}
                  <li class="flex flex-col gap-1 text-sm">
                    <span class="flex justify-between gap-2"><span class="truncate">{u.name}</span><span class="text-muted-foreground tabular-nums">{u.error ? "Failed" : `${u.progress}%`}</span></span>
                    <Progress value={u.progress} max={100} />
                  </li>
                {/each}
              </ul>
            {/if}
            {#if r.documents.length}
              <ul class="flex flex-col gap-1 text-sm">
                {#each r.documents as d (d.id)}
                  <li>
                    <a href={d.url} download data-external class="hover:bg-muted -mx-2 flex items-center gap-2 rounded-md px-2 py-1">
                      <FileText class="text-muted-foreground size-4 shrink-0" />
                      <span class="min-w-0 flex-1">
                        <span class="block truncate">{d.filename}</span>
                        <span class="text-muted-foreground block text-xs">{[bytes(d.byte_size), d.uploaded_by && who(d.uploaded_by), portal.dateTime(d.created_at)].filter(Boolean).join(" · ")}</span>
                      </span>
                    </a>
                  </li>
                {/each}
              </ul>
            {/if}
          </Card.Content>
        </Card.Root>
      </div>
    </div>
  {/if}
</PortalLayout>
