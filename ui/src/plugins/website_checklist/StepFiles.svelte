<script>
  // A step's files, beside its note: each to download or remove, then the
  // drop box (drop files on it, or choose them), its progress on the box.
  import { Button } from "$lib/components/ui/button/index.js";
  import { Progress } from "$lib/components/ui/progress/index.js";
  import Paperclip from "@lucide/svelte/icons/paperclip";
  import Trash from "@lucide/svelte/icons/trash-2";
  import { del } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { bytes } from "$lib/format.js";
  import { wc } from "./wc.js";

  let { site, item, onchange } = $props();
  let dragging = $state(false), progress = $state(null), input = $state();

  function upload(files) {
    files = [...(files || [])];
    if (!files.length) return;
    const form = new FormData();
    for (const f of files) form.append("files[]", f);
    const xhr = new XMLHttpRequest();
    xhr.open("POST", "/api" + wc(`/websites/${site.id}/items/${item.id}/files`));
    xhr.setRequestHeader("X-CSRF-Token", session.csrf);
    xhr.setRequestHeader("Accept", "application/json");
    progress = 0;
    xhr.upload.onprogress = (e) => { if (e.lengthComputable) progress = Math.round((e.loaded / e.total) * 100); };
    xhr.onload = () => {
      let body = {};
      try { body = JSON.parse(xhr.responseText || "{}"); } catch { /* not JSON */ }
      progress = null;
      if (input) input.value = "";
      if (xhr.status >= 200 && xhr.status < 300) { toast(body.message); onchange(body); }
      else toastError(new Error(xhr.status === 413 ? "That’s over the 10 MB limit." : body.message || `Upload failed (${xhr.status}).`));
    };
    xhr.onerror = () => { progress = null; toastError(new Error("Upload failed: the connection dropped.")); };
    xhr.send(form);
  }

  async function remove(f) {
    if (!(await confirm({ title: `Delete ${f.filename}?`, message: `It comes off “${item.title}”. Its history keeps that it was here.`, confirmLabel: "Delete", destructive: true }))) return;
    try { const r = await del(wc(`/websites/${site.id}/items/${item.id}/files/${f.id}`)); toast(r.message); onchange(r); } catch (e) { toastError(e); }
  }
</script>

<section class="flex flex-col gap-2" aria-label="Files">
  <span class="text-sm font-medium">Files</span>
  {#if item.files.length}
    <ul class="flex flex-col gap-1 text-sm">
      {#each item.files as f (f.id)}
        <li class="flex min-w-0 items-center gap-2">
          <Paperclip class="text-muted-foreground size-4 shrink-0" />
          <a href={f.path} download class="truncate underline-offset-4 hover:underline">{f.filename}</a>
          <span class="text-muted-foreground shrink-0 text-xs">{bytes(f.byte_size)}</span>
          <Button variant="ghost" size="icon-sm" class="ms-auto" onclick={() => remove(f)}><Trash /><span class="sr-only">Delete {f.filename}</span></Button>
        </li>
      {/each}
    </ul>
  {:else}
    <p class="text-muted-foreground text-xs">None yet: keep what backs this step up here (an export, a screenshot, a sitemap).</p>
  {/if}
  <label
    class="border-input hover:bg-muted/50 flex min-h-18 cursor-pointer flex-col items-center justify-center gap-1 rounded-lg border-2 border-dashed px-4 py-3 text-center text-sm transition-colors {dragging ? 'border-primary bg-muted/50' : ''}"
    ondragover={(e) => { e.preventDefault(); dragging = true; }}
    ondragleave={() => (dragging = false)}
    ondrop={(e) => { e.preventDefault(); dragging = false; upload(e.dataTransfer?.files); }}>
    {#if progress != null}
      <span>Uploading… {progress}%</span>
      <Progress value={progress} max={100} class="w-48" />
    {:else}
      <span class="flex items-center gap-1.5"><Paperclip class="size-4" />Drop files here or <span class="underline underline-offset-4">choose…</span></span>
      <span class="text-muted-foreground text-xs">up to 10 MB at a time</span>
    {/if}
    <input bind:this={input} type="file" multiple class="sr-only" aria-label={`Add files to ${item.title}`} onchange={(e) => upload(e.currentTarget.files)} />
  </label>
</section>
