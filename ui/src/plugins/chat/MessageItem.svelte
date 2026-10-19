<script>
  // One message: the person and time (left out when it follows their last
  // one), its text, files, reactions and thread. On hover or focus, a bar
  // to react, reply in a thread and (your own) edit or delete. Editing is
  // in place: Enter saves, Escape lets it be.
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import * as Tooltip from "$lib/components/ui/tooltip/index.js";
  import { Button } from "$lib/components/ui/button/index.js";
  import MessageSquareText from "@lucide/svelte/icons/message-square-text";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import Pencil from "@lucide/svelte/icons/pencil";
  import Trash from "@lucide/svelte/icons/trash-2";
  import FileIcon from "@lucide/svelte/icons/file";
  import Download from "@lucide/svelte/icons/download";
  import RichTextEditor from "$lib/components/RichTextEditor.svelte";
  import ChatAvatar from "./ChatAvatar.svelte";
  import EmojiPicker from "./EmojiPicker.svelte";
  import { chat, BASE, clockTime, QUICK_EMOJI } from "./chat.svelte.js";
  import { patch, post, del } from "$lib/api.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { bytes, timeAgo, dateTime } from "$lib/format.js";

  let { m, grouped = false, inThread = false, highlight = false, onthread = null, editing = $bindable(false), onchanged = null } = $props();

  let draft = $state("");
  let saving = $state(false);
  let viewing = $state(null); // the image open full size

  const images = $derived(m.files.filter((f) => f.image));
  const others = $derived(m.files.filter((f) => !f.image));

  let editor = $state(null);
  $effect(() => {
    if (!editing) return;
    draft = m.body;
    // The editor loads a moment later: then the caret goes to the end.
    let tries = 0;
    const place = () => {
      const el = editor?.querySelector("[contenteditable]");
      if (!el) { if (tries++ < 60) requestAnimationFrame(place); return; }
      el.focus();
      const range = document.createRange();
      range.selectNodeContents(el);
      range.collapse(false);
      const sel = getSelection();
      sel.removeAllRanges();
      sel.addRange(range);
    };
    requestAnimationFrame(place);
  });

  // Your own mentions stand out; links open in a new tab; "(edited)"
  // follows the last line.
  function enhance(node) {
    const run = () => {
      node.querySelector("[data-edited-mark]")?.remove();
      if (m.edited) {
        const mark = document.createElement("span");
        mark.dataset.editedMark = "";
        mark.className = "text-muted-foreground ms-1 text-xs";
        mark.textContent = "(edited)";
        if (m.edited_at) mark.title = `Edited ${dateTime(m.edited_at)}`;
        const last = node.lastElementChild;
        (last && /^(P|H\d|LI|BLOCKQUOTE)$/.test(last.tagName) ? last : node).appendChild(mark);
      }
      node.querySelectorAll("[data-mention-id]").forEach((el) => {
        if (chat.me && el.getAttribute("data-mention-id") === String(chat.me.id)) el.setAttribute("data-me", "");
      });
      node.querySelectorAll("a[href^='http']").forEach((a) => { a.target = "_blank"; a.rel = "noopener noreferrer"; });
    };
    run();
    return { update: run };
  }

  async function react(emoji) {
    try { onchanged?.((await post(`${BASE}/messages/${m.id}/reactions`, { emoji })).message); } catch (e) { toastError(e); }
  }

  // stopEditing leaves the editor first: one removed while focused blurs
  // in the middle of the page being drawn.
  function stopEditing() {
    if (editor?.contains(document.activeElement)) document.activeElement.blur();
    editing = false;
  }

  async function save() {
    if (saving) return;
    saving = true;
    try {
      const r = await patch(`${BASE}/messages/${m.id}`, { message: { body: draft } });
      onchanged?.(r.message);
      stopEditing();
    } catch (e) { toastError(e); } finally { saving = false; }
  }

  async function remove() {
    if (!(await confirm({ title: "Delete this message?", message: "It's gone for everyone, with its files. Replies in its thread stay.", confirmLabel: "Delete", destructive: true }))) return;
    try { onchanged?.((await del(`${BASE}/messages/${m.id}`)).message); } catch (e) { toastError(e); }
  }

  function editKeys(e) {
    // Before the editor (which takes Escape), unless @ is picking someone.
    if (e.key === "Escape" && !document.querySelector("[role=listbox]")) { e.preventDefault(); e.stopPropagation(); stopEditing(); }
  }
</script>

<div class="group/msg hover:bg-muted/40 focus-within:bg-muted/40 relative flex gap-3 px-4 {grouped ? 'py-0.5' : 'pt-2 pb-0.5'} {highlight ? 'bg-amber-50 dark:bg-amber-950/30' : ''}" data-message-id={m.id}>
  <div class="w-9 shrink-0">
    {#if grouped}
      <time datetime={m.created_at} title={dateTime(m.created_at)} class="text-muted-foreground invisible -ms-1 block pt-1 text-end text-[10px] leading-4 whitespace-nowrap tabular-nums group-hover/msg:visible group-focus-within/msg:visible">{clockTime(m.created_at)}</time>
    {:else}
      <ChatAvatar person={m.agent || chat.byId[m.user.id] || m.user} size="size-9" />
    {/if}
  </div>
  <div class="min-w-0 flex-1">
    {#if !grouped}
      <div class="flex items-baseline gap-2">
        {#if m.agent}
          <span class="text-sm font-semibold">{m.agent.name}</span>
          <span class="text-muted-foreground text-xs">via {m.agent.via} for {m.agent.for}</span>
        {:else}
          <span class="text-sm font-semibold">{m.user.name}</span>
        {/if}
        {#if !m.agent && chat.byId[m.user.id]?.status_emoji}<span class="text-sm" title={chat.byId[m.user.id].status_text}>{chat.byId[m.user.id].status_emoji}</span>{/if}
        <time datetime={m.created_at} title={dateTime(m.created_at)} class="text-muted-foreground text-xs tabular-nums">{clockTime(m.created_at)}</time>
        {#if m.via && !m.agent}<span class="text-muted-foreground text-xs">via {m.via}</span>{/if}
      </div>
    {/if}

    {#if m.deleted}
      <p class="text-muted-foreground text-sm italic">This message was deleted.</p>
    {:else if editing}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="my-1" data-editing bind:this={editor} onkeydowncapture={editKeys}>
        <RichTextEditor bind:value={draft} label="Edit message" minHeight="min-h-10" submitOnEnter onsubmit={save} />
        <div class="mt-1.5 flex items-center gap-2">
          <Button size="sm" onclick={save} disabled={saving}>Save</Button>
          <Button size="sm" variant="ghost" onclick={stopEditing}>Cancel</Button>
          <span class="text-muted-foreground text-xs">Enter to save, Escape to cancel</span>
        </div>
      </div>
    {:else}
      {#if m.body}
        <div use:enhance={[m.body, m.edited, chat.me?.id]} class="prose-admin text-sm break-words [&_[data-mention-id]]:bg-primary/10 [&_[data-mention-id]]:text-primary [&_[data-mention-id]]:rounded-sm [&_[data-mention-id]]:px-0.5 [&_[data-mention-id]]:font-medium [&_[data-mentionable]]:bg-primary/10 [&_[data-mentionable]]:text-primary [&_[data-mentionable]]:rounded-sm [&_[data-mentionable]]:px-0.5 [&_[data-mentionable]]:font-medium [&_a]:text-primary [&_a]:underline [&_[data-me]]:bg-amber-100 [&_[data-me]]:text-amber-900 dark:[&_[data-me]]:bg-amber-900/40 dark:[&_[data-me]]:text-amber-100 [&>*+*]:mt-1.5">
          {@html m.body}
        </div>
      {:else if m.edited}
        <span class="text-muted-foreground text-xs">(edited)</span>
      {/if}

      {#if images.length}
        <div class="mt-1.5 flex flex-wrap gap-2">
          {#each images as f (f.id)}
            <button type="button" class="focus-visible:ring-ring/50 overflow-hidden rounded-lg border outline-none focus-visible:ring-[3px]" onclick={() => (viewing = f)} aria-label="Open {f.filename} full size">
              <img src={f.url} alt={f.filename} width={f.width || undefined} height={f.height || undefined} loading="lazy"
                class="block h-auto max-h-60 w-auto max-w-[min(20rem,100%)] object-contain" />
            </button>
          {/each}
        </div>
      {/if}
      {#if others.length}
        <div class="mt-1.5 flex flex-wrap gap-2">
          {#each others as f (f.id)}
            <a href="{f.url}?download=1" download={f.filename} data-external class="hover:bg-muted/60 focus-visible:ring-ring/50 flex w-72 max-w-full items-center gap-3 rounded-lg border px-3 py-2 outline-none focus-visible:ring-[3px]">
              <span class="bg-primary/10 text-primary flex size-9 shrink-0 items-center justify-center rounded-md"><FileIcon class="size-4" /></span>
              <span class="min-w-0 flex-1">
                <span class="block truncate text-sm font-medium">{f.filename}</span>
                <span class="text-muted-foreground block text-xs tabular-nums">{bytes(f.byte_size)}</span>
              </span>
              <Download class="text-muted-foreground size-4 shrink-0" aria-label="Download" />
            </a>
          {/each}
        </div>
      {/if}

      {#if m.reactions.length}
        <div class="mt-1 flex flex-wrap items-center gap-1">
          {#each m.reactions as r (r.emoji)}
            <Tooltip.Provider>
              <Tooltip.Root>
                <Tooltip.Trigger>
                  {#snippet child({ props })}
                    <button {...props} type="button" aria-pressed={r.mine} onclick={() => react(r.emoji)}
                      aria-label="{r.emoji} {r.count}, from {r.names.join(', ')}{r.mine ? ' (take yours back)' : ''}"
                      class="focus-visible:ring-ring/50 inline-flex h-6 items-center gap-1 rounded-full border px-2 text-xs tabular-nums outline-none focus-visible:ring-[3px] {r.mine ? 'border-primary/50 bg-primary/10 text-primary' : 'hover:bg-muted'}">
                      <span class="text-sm leading-none">{r.emoji}</span>{r.count}
                    </button>
                  {/snippet}
                </Tooltip.Trigger>
                <Tooltip.Content>{r.names.join(", ")}</Tooltip.Content>
              </Tooltip.Root>
            </Tooltip.Provider>
          {/each}
          <EmojiPicker onpick={react} class="size-6" align="start" />
        </div>
      {/if}

      {#if !inThread && m.reply_count > 0}
        <button type="button" onclick={() => onthread?.(m)} class="hover:bg-background hover:border-border focus-visible:ring-ring/50 mt-1 -ms-1 flex items-center gap-2 rounded-md border border-transparent px-1 py-0.5 text-xs outline-none focus-visible:ring-[3px]">
          <span class="flex -space-x-1">
            {#each m.repliers as p (p.id)}<ChatAvatar person={chat.byId[p.id] || p} size="size-5" />{/each}
          </span>
          <span class="text-primary font-semibold">{m.reply_count === 1 ? "1 reply" : `${m.reply_count} replies`}</span>
          {#if m.last_reply_at}<span class="text-muted-foreground">Last reply {timeAgo(m.last_reply_at)}</span>{/if}
        </button>
      {/if}
    {/if}
  </div>

  {#if !m.deleted && !editing}
    <div class="bg-background invisible absolute -top-3 end-4 z-10 flex items-center gap-0.5 rounded-md border p-0.5 shadow-sm group-hover/msg:visible group-focus-within/msg:visible has-[[data-state=open]]:visible">
      {#each QUICK_EMOJI.slice(0, 3) as e (e)}
        <button type="button" class="hover:bg-muted focus-visible:ring-ring/50 flex size-7 items-center justify-center rounded-md text-base outline-none focus-visible:ring-[3px]" onclick={() => react(e)} aria-label="React {e}" title="React {e}">{e}</button>
      {/each}
      <EmojiPicker onpick={react} />
      {#if !inThread && !m.parent_id}
        <Button variant="ghost" size="icon" class="size-7" onclick={() => onthread?.(m)} aria-label="Reply in thread" title="Reply in thread"><MessageSquareText /></Button>
      {/if}
      {#if m.mine || chat.me?.role === "owner"}
        <DropdownMenu.Root>
          <DropdownMenu.Trigger>
            {#snippet child({ props })}
              <Button {...props} variant="ghost" size="icon" class="size-7" aria-label="More actions" title="More actions"><Ellipsis /></Button>
            {/snippet}
          </DropdownMenu.Trigger>
          <DropdownMenu.Content align="end">
            {#if m.mine}<DropdownMenu.Item onSelect={() => (editing = true)}><Pencil />Edit message</DropdownMenu.Item>{/if}
            <DropdownMenu.Item variant="destructive" onSelect={remove}><Trash />Delete message</DropdownMenu.Item>
          </DropdownMenu.Content>
        </DropdownMenu.Root>
      {/if}
    </div>
  {/if}
</div>

<Dialog.Root open={!!viewing} onOpenChange={(o) => { if (!o) viewing = null; }}>
  <Dialog.Content class="max-h-[95dvh] w-auto max-w-[95vw] p-2 sm:max-w-[95vw]">
    <Dialog.Title class="sr-only">{viewing?.filename}</Dialog.Title>
    {#if viewing}
      <img src={viewing.url} alt={viewing.filename} class="max-h-[85dvh] max-w-full rounded-md object-contain" />
      <div class="flex items-center justify-between gap-2 px-1 pb-1">
        <span class="text-muted-foreground truncate text-sm">{viewing.filename} · {bytes(viewing.byte_size)}</span>
        <Button size="sm" variant="outline" href="{viewing.url}?download=1" download={viewing.filename} data-external><Download />Download</Button>
      </div>
    {/if}
  </Dialog.Content>
</Dialog.Root>
