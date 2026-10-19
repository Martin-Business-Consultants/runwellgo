<script>
  // A record's documents (the Rails documents/_section): drop files on the
  // box (or click it to choose), each uploading with its progress; then the
  // files, latest first. An image opens in a preview, a PDF in a new tab,
  // anything else downloads. The switch shows a file to the client in their
  // portal.
  import * as Card from "$lib/components/ui/card/index.js";
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as Tooltip from "$lib/components/ui/tooltip/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import { Switch } from "$lib/components/ui/switch/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import Upload from "@lucide/svelte/icons/upload";
  import Download from "@lucide/svelte/icons/download";
  import ExternalLink from "@lucide/svelte/icons/external-link";
  import Trash from "@lucide/svelte/icons/trash-2";
  import PersonAvatar from "./PersonAvatar.svelte";
  import { patch, del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { bytes, timeAgo } from "$lib/format.js";

  let { subjectType, subjectId, documents = [], reload } = $props();

  let uploads = $state([]); // {name, progress, error}
  let dragging = $state(false);
  let input = $state();
  let preview = $state(null);

  const fileURL = (d, inline = false) => `/api/documents/${d.id}/file${inline ? "?disposition=inline" : ""}`;
  const kind = (d) => (d.content_type?.startsWith("image/") && d.content_type !== "image/svg+xml" ? "image" : d.content_type === "application/pdf" ? "tab" : "download");
  const ext = (name) => (name.includes(".") ? name.split(".").pop().toLowerCase().slice(0, 4) : "file");

  // One file, one request, so each shows its own progress.
  function send(file) {
    const entry = { name: file.name, progress: 0, error: "" };
    uploads.push(entry);
    const item = uploads[uploads.length - 1];
    return new Promise((resolve) => {
      const form = new FormData();
      form.append("record", `${subjectType}:${subjectId}`);
      form.append("files", file);
      const xhr = new XMLHttpRequest();
      xhr.open("POST", "/api/documents");
      xhr.setRequestHeader("X-CSRF-Token", session.csrf);
      xhr.setRequestHeader("Accept", "application/json");
      xhr.upload.onprogress = (e) => { if (e.lengthComputable) item.progress = Math.round((e.loaded / e.total) * 100); };
      xhr.onload = () => {
        let body = {};
        try { body = JSON.parse(xhr.responseText || "{}"); } catch { /* not JSON */ }
        if (xhr.status >= 200 && xhr.status < 300) { item.progress = 100; resolve(true); }
        else { item.error = body.message || body.summary || `Upload failed (${xhr.status}).`; resolve(false); }
      };
      xhr.onerror = () => { item.error = "Upload failed: the connection dropped."; resolve(false); };
      xhr.send(form);
    });
  }

  async function upload(files) {
    files = [...(files || [])];
    if (!files.length) return;
    const results = await Promise.all(files.map(send));
    const ok = results.filter(Boolean).length;
    if (ok) toast(`${ok === 1 ? "1 file" : `${ok} files`} added.`);
    for (const e of uploads.filter((u) => u.error)) toastError(new Error(`${e.name}: ${e.error}`));
    uploads = [];
    if (input) input.value = "";
    if (ok) await reload?.();
  }

  function drop(e) {
    e.preventDefault();
    dragging = false;
    upload(e.dataTransfer?.files);
  }

  async function setVisible(d, visible) {
    try {
      const r = await patch(`/documents/${d.id}`, { document: { client_visible: visible } });
      toast(r.summary);
      await reload?.();
    } catch (e) { toastError(e); }
  }

  // Whoever uploaded it, or anyone who may delete records.
  const deletable = (d) => !!session.me && (d.uploaded_by?.id === session.me.id || session.can("delete_records"));

  async function remove(d) {
    if (!(await confirm({ title: `Delete ${d.filename}?`, message: "The file is removed for everyone, including the client’s portal.", confirmLabel: "Delete", destructive: true }))) return;
    try { const r = await del(`/documents/${d.id}`); toast(r.summary); await reload?.(); } catch (e) { toastError(e); }
  }
</script>

<Card.Root>
  <Card.Content class="flex flex-col gap-4">
    <label
      class="border-input hover:bg-muted/50 flex cursor-pointer flex-col items-center gap-1 rounded-lg border-2 border-dashed px-4 py-6 text-center text-sm transition-colors {dragging ? 'border-primary bg-muted/50' : ''}"
      ondragover={(e) => { e.preventDefault(); dragging = true; }}
      ondragleave={() => (dragging = false)}
      ondrop={drop}>
      <Upload class="text-muted-foreground size-5" />
      <span class="font-medium">Drop files here, or click to choose</span>
      <span class="text-muted-foreground text-xs">They start internal: only your team sees them.</span>
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

    {#if !documents.length}
      <p class="text-muted-foreground text-sm">No documents yet.</p>
    {:else}
      <ul class="flex flex-col divide-y">
        {#each documents as d (d.id)}
          <li class="flex items-center gap-3 py-2 first:pt-0 last:pb-0">
            {#snippet file()}
              <span class="bg-muted text-muted-foreground flex size-9 shrink-0 items-center justify-center rounded-md text-[10px] font-semibold uppercase">{ext(d.filename)}</span>
              <span class="min-w-0 flex-1 text-start">
                <span class="block truncate text-sm font-medium">{d.filename}</span>
                <span class="text-muted-foreground flex items-center gap-1 truncate text-xs">
                  {bytes(d.byte_size)}{#if d.uploaded_by} · <PersonAvatar person={d.uploaded_by} size="size-4" />{/if} · {timeAgo(d.created_at)}
                </span>
              </span>
            {/snippet}
            {#if kind(d) === "image"}
              <button type="button" class="flex min-w-0 flex-1 items-center gap-3" title="Preview {d.filename}" onclick={() => (preview = d)}>{@render file()}</button>
            {:else if kind(d) === "tab"}
              <a href={fileURL(d, true)} target="_blank" rel="noopener" class="flex min-w-0 flex-1 items-center gap-3" title="Open {d.filename} in a new tab">{@render file()}</a>
            {:else}
              <a href={fileURL(d)} download={d.filename} class="flex min-w-0 flex-1 items-center gap-3" title="Download {d.filename}">{@render file()}</a>
            {/if}
            <Tooltip.Provider>
              <Tooltip.Root>
                <Tooltip.Trigger>
                  {#snippet child({ props })}
                    <span {...props}><Switch checked={d.client_visible} onCheckedChange={(v) => setVisible(d, v)} aria-label="Visible to the client" /></span>
                  {/snippet}
                </Tooltip.Trigger>
                <Tooltip.Content>{d.client_visible ? "The client can see it" : "Visible to the client"}</Tooltip.Content>
              </Tooltip.Root>
            </Tooltip.Provider>
            <Button variant="ghost" size="icon" class="size-8" href={fileURL(d)} download={d.filename} aria-label="Download {d.filename}" title="Download"><Download /></Button>
            {#if deletable(d)}
              <Button variant="ghost" size="icon" class="size-8" aria-label="Delete {d.filename}" title="Delete" onclick={() => remove(d)}><Trash /></Button>
            {/if}
          </li>
        {/each}
      </ul>
      <p class="text-muted-foreground text-xs">The switch shows a file to the client in their portal.</p>
    {/if}
  </Card.Content>
</Card.Root>

<Dialog.Root open={!!preview} onOpenChange={(o) => { if (!o) preview = null; }}>
  <Dialog.Content class="sm:max-w-3xl">
    {#if preview}
      <Dialog.Header><Dialog.Title class="truncate">{preview.filename}</Dialog.Title></Dialog.Header>
      <img src={fileURL(preview, true)} alt={preview.filename} class="max-h-[70vh] w-full rounded-md object-contain" loading="lazy" />
      <Dialog.Footer>
        <Button variant="outline" href={fileURL(preview, true)} target="_blank" rel="noopener"><ExternalLink />Open</Button>
        <Button variant="outline" href={fileURL(preview)} download={preview.filename}><Download />Download</Button>
      </Dialog.Footer>
    {/if}
  </Dialog.Content>
</Dialog.Root>
