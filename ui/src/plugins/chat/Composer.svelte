<script>
  // Where a message is written: rich text with @mentions (Enter sends,
  // Shift+Enter is a new line, Up on an empty box edits your last one),
  // and files by the paperclip, by pasting or by dropping them on the
  // conversation (addFiles). Images show as thumbnails before they go.
  // While you type, the others see that you are.
  import { Button } from "$lib/components/ui/button/index.js";
  import Paperclip from "@lucide/svelte/icons/paperclip";
  import SendHorizontal from "@lucide/svelte/icons/send-horizontal";
  import X from "@lucide/svelte/icons/x";
  import FileIcon from "@lucide/svelte/icons/file";
  import RichTextEditor from "$lib/components/RichTextEditor.svelte";
  import { chat, BASE } from "./chat.svelte.js";
  import { post } from "$lib/api.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { bytes } from "$lib/format.js";

  let { conversationId, parentId = null, placeholder = "Write a message", onsent = null, oneditlast = null, label = "Message" } = $props();

  let value = $state("");
  let files = $state([]); // [{file, url}]
  let sending = $state(false);
  let picker = $state(null);
  let box = $state(null);
  let lastTyping = 0;

  const empty = (html) => !html || !html.replace(/<[^>]*>/g, "").replace(/&nbsp;/g, " ").trim() && !/<img|data-mention-id|data-mentionable/.test(html);

  // addFiles takes files dropped, pasted or picked, within the limit.
  export function addFiles(list) {
    for (const file of list) {
      if (file.size > chat.maxUpload) {
        toastError(new Error(`${file.name} is larger than ${bytes(chat.maxUpload)}: share a link to it instead.`));
        continue;
      }
      if (files.length >= 10) {
        toastError(new Error("Ten files at most in one message: send these, then the rest."));
        break;
      }
      files = [...files, { file, url: file.type.startsWith("image/") ? URL.createObjectURL(file) : null }];
    }
    focus();
  }

  export function focus() {
    box?.querySelector("[contenteditable]")?.focus();
  }

  function drop(i) {
    if (files[i].url) URL.revokeObjectURL(files[i].url);
    files = files.filter((_, j) => j !== i);
  }

  async function send() {
    if (sending || (empty(value) && !files.length)) return;
    const total = files.reduce((n, f) => n + f.file.size, 0);
    if (total > 10 * 1024 * 1024) {
      toastError(new Error("These files are more than 10 MB together: send them in a few messages."));
      return;
    }
    sending = true;
    try {
      let r;
      const path = `${BASE}/conversations/${conversationId}/messages`;
      if (files.length) {
        const form = new FormData();
        form.append("body", empty(value) ? "" : value);
        if (parentId) form.append("parent_id", String(parentId));
        for (const f of files) form.append("files", f.file, f.file.name);
        r = await post(path, form);
      } else {
        r = await post(path, { message: { body: value, parent_id: parentId } });
      }
      for (const f of files) if (f.url) URL.revokeObjectURL(f.url);
      value = "";
      files = [];
      lastTyping = 0;
      onsent?.(r.message);
      focus();
    } catch (e) {
      toastError(e);
    } finally {
      sending = false;
    }
  }

  // Others see that you're typing (said again every few seconds).
  $effect(() => {
    if (empty(value)) return;
    const now = Date.now();
    if (now - lastTyping < 3000) return;
    lastTyping = now;
    post(`${BASE}/conversations/${conversationId}/typing`, { parent_id: parentId }, { quiet: true }).catch(() => {});
  });

  function keys(e) {
    if (e.key === "ArrowUp" && !e.shiftKey && !e.altKey && empty(value) && !files.length && oneditlast) {
      e.preventDefault();
      oneditlast();
    }
  }

  function paste(e) {
    const list = [...(e.clipboardData?.files || [])];
    if (!list.length) return;
    e.preventDefault();
    e.stopPropagation();
    addFiles(list);
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div bind:this={box} class="flex flex-col gap-2" onkeydown={keys} onpastecapture={paste}>
  {#if files.length}
    <ul class="flex flex-wrap gap-2" aria-label="Files to send">
      {#each files as f, i (f.file)}
        <li class="bg-muted/40 relative flex items-center gap-2 rounded-lg border p-1.5 pe-8">
          {#if f.url}
            <img src={f.url} alt="" class="size-10 rounded-md object-cover" />
          {:else}
            <span class="bg-primary/10 text-primary flex size-10 items-center justify-center rounded-md"><FileIcon class="size-4" /></span>
          {/if}
          <span class="min-w-0">
            <span class="block max-w-40 truncate text-xs font-medium">{f.file.name}</span>
            <span class="text-muted-foreground block text-xs tabular-nums">{bytes(f.file.size)}</span>
          </span>
          <Button variant="ghost" size="icon" class="absolute end-1 top-1 size-6" onclick={() => drop(i)} aria-label="Remove {f.file.name}"><X class="size-3.5" /></Button>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="relative">
    <RichTextEditor bind:value {placeholder} {label} minHeight="min-h-10 max-h-60 overflow-y-auto pe-20" submitOnEnter onsubmit={send} />
    <div class="absolute end-1.5 bottom-1.5 flex items-center gap-1">
      <input bind:this={picker} type="file" multiple class="hidden" onchange={(e) => { addFiles([...e.currentTarget.files]); e.currentTarget.value = ""; }} aria-hidden="true" tabindex="-1" />
      <Button variant="ghost" size="icon" class="size-8" onclick={() => picker.click()} aria-label="Attach files" title="Attach files"><Paperclip /></Button>
      <Button size="icon" class="size-8" onclick={send} disabled={sending || (empty(value) && !files.length)} aria-label="Send" title="Send (Enter)"><SendHorizontal /></Button>
    </div>
  </div>
</div>
