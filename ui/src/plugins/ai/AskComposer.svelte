<script module>
  // The models and agents to offer, read once per page load.
  let modelsP = null, agentsP = null;
</script>

<script>
  // Where a question is written: a box that grows as you type (Enter sends,
  // Shift+Enter is a new line), pictures by the paperclip or by pasting
  // (shown before they go, when the provider reads them), the model to
  // answer with, @ to hand the question to one of your agents instead, and
  // Send, which is Stop while it's answering.
  import { Button } from "$lib/components/ui/button/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import ArrowUp from "@lucide/svelte/icons/arrow-up";
  import Square from "@lucide/svelte/icons/square";
  import Paperclip from "@lucide/svelte/icons/paperclip";
  import X from "@lucide/svelte/icons/x";
  import ChevronDown from "@lucide/svelte/icons/chevron-down";
  import Check from "@lucide/svelte/icons/check";
  import Bot from "@lucide/svelte/icons/bot";
  import { staff } from "./ai.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";

  let {
    disabled = false, replying = false, busy = false, portal = false, images = false, providerName = "",
    model = "", modelName = "", onmodel = null, onsend, onstop = null, placeholder = "Ask, or ask it to do something…",
  } = $props();

  let text = $state(""), files = $state([]), box = $state(null), picker = $state(null);
  let models = $state(null), agents = $state([]);
  let suggest = $state(null), index = $state(0);

  // svelte-ignore state_referenced_locally
  if (!portal) {
    modelsP ??= staff.get("/models").catch(() => null);
    agentsP ??= staff.get("/agents").then((r) => r.agents || []).catch(() => []);
    modelsP.then((m) => (models = m));
    agentsP.then((a) => (agents = a));
  }

  export function focus() { box?.focus(); }
  export function set(v) { text = v; queueMicrotask(grow); focus(); }

  function grow() {
    if (!box) return;
    box.style.height = "auto";
    box.style.height = Math.min(box.scrollHeight, 200) + "px";
  }

  const matches = $derived(suggest ? agents.filter((a) => !suggest.q || a.handle.startsWith(suggest.q.toLowerCase()) || a.name.toLowerCase().startsWith(suggest.q.toLowerCase())).slice(0, 6) : []);

  function detect() {
    if (!box || portal || !agents.length) { suggest = null; return; }
    const before = text.slice(0, box.selectionStart);
    const m = /(?:^|\s)@([\w.-]{0,30})$/.exec(before);
    if (!m) { suggest = null; return; }
    if (!suggest || suggest.from !== before.length - m[1].length - 1) index = 0;
    suggest = { q: m[1], from: before.length - m[1].length - 1, to: before.length };
  }
  function choose(a) {
    if (!suggest) return;
    text = text.slice(0, suggest.from) + "@" + a.handle + " " + text.slice(suggest.to);
    const at = suggest.from + a.handle.length + 2;
    suggest = null;
    queueMicrotask(() => { box.focus(); box.setSelectionRange(at, at); grow(); });
  }

  function onkeydown(e) {
    if (suggest && matches.length) {
      if (e.key === "ArrowDown") { e.preventDefault(); index = (index + 1) % matches.length; return; }
      if (e.key === "ArrowUp") { e.preventDefault(); index = (index - 1 + matches.length) % matches.length; return; }
      if (e.key === "Enter" || e.key === "Tab") { e.preventDefault(); choose(matches[index]); return; }
      if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); suggest = null; return; }
    }
    if (e.key === "Enter" && !e.shiftKey && !e.isComposing) { e.preventDefault(); send(); }
  }

  // The agent the question is for: the first @handle that names one.
  function agentFor(t) {
    for (const m of t.matchAll(/(?:^|\s)@([\w.-]+)/g)) {
      const a = agents.find((x) => x.handle === m[1].toLowerCase());
      if (a) return a;
    }
    return null;
  }

  async function send() {
    if (replying || busy || disabled) return;
    const t = text.trim();
    if (!t && !files.length) return;
    const pics = files.map((f) => ({ media_type: f.type, data: f.data }));
    const ok = await onsend({ text: t, images: pics, agent: agentFor(t) });
    if (ok !== false) {
      text = "";
      files.forEach((f) => URL.revokeObjectURL(f.url));
      files = [];
      queueMicrotask(grow);
    }
  }

  function addFiles(list) {
    for (const file of list) {
      if (!file.type.startsWith("image/")) { toastError(new Error(`${file.name} isn’t a picture: Ask reads PNG, JPEG, GIF or WebP.`)); continue; }
      if (!images) { toastError(new Error(`${providerName || "This provider"} doesn’t read pictures here: describe it in words instead.`)); return; }
      if (file.size > 3 * 1024 * 1024) { toastError(new Error(`${file.name} is larger than 3 MB: send a smaller picture.`)); continue; }
      if (files.length >= 4) { toastError(new Error("Four pictures at most with one question.")); break; }
      const reader = new FileReader();
      reader.onload = () => {
        const data = String(reader.result).split(",")[1] || "";
        files = [...files, { name: file.name, type: file.type, data, url: URL.createObjectURL(file) }];
      };
      reader.readAsDataURL(file);
    }
    focus();
  }
  function onpaste(e) {
    const pics = [...(e.clipboardData?.files || [])].filter((f) => f.type.startsWith("image/"));
    if (pics.length) { e.preventDefault(); addFiles(pics); }
  }
  function drop(i) {
    URL.revokeObjectURL(files[i].url);
    files = files.filter((_, j) => j !== i);
  }

  const modelLabel = $derived(model === "fast" ? "Fast" : model && model !== "main" ? model : "Main");
  const mainName = $derived(models?.main || ""), fastName = $derived(models?.fast || "");
</script>

<div class="relative">
  {#if suggest && matches.length}
    <div class="bg-popover text-popover-foreground absolute bottom-full left-0 z-50 mb-1 w-64 rounded-md border p-1 shadow-md" role="listbox" aria-label="Agents to ask">
      {#each matches as a, i (a.id)}
        <button type="button" role="option" aria-selected={i === index}
          class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm {i === index ? 'bg-accent text-accent-foreground' : ''}"
          onmousedown={(e) => { e.preventDefault(); choose(a); }} onmouseenter={() => (index = i)}>
          <Bot class="text-primary size-4 shrink-0" />
          <span class="min-w-0 flex-1 truncate">{a.name} <span class="text-muted-foreground">@{a.handle}</span></span>
          {#if a.paused}<span class="text-muted-foreground text-xs">Paused</span>{/if}
        </button>
      {/each}
    </div>
  {/if}

  <form class="bg-background focus-within:ring-ring/50 focus-within:border-ring flex flex-col rounded-xl border shadow-xs transition-[box-shadow] focus-within:ring-[3px] motion-reduce:transition-none"
    onsubmit={(e) => { e.preventDefault(); send(); }}>
    {#if files.length}
      <div class="flex flex-wrap gap-2 px-3 pt-3">
        {#each files as f, i (f.url)}
          <div class="relative">
            <img src={f.url} alt={f.name} class="size-14 rounded-md border object-cover" />
            <button type="button" class="bg-background absolute -top-1.5 -right-1.5 rounded-full border p-0.5" onclick={() => drop(i)} aria-label="Remove {f.name}"><X class="size-3" /></button>
          </div>
        {/each}
      </div>
    {/if}
    <textarea bind:this={box} bind:value={text} rows="1" {placeholder} aria-label="Your question" {onkeydown} {onpaste}
      oninput={() => { grow(); detect(); }} onclick={detect} disabled={disabled}
      class="placeholder:text-muted-foreground max-h-[200px] min-h-11 w-full resize-none bg-transparent px-3 pt-3 pb-1 text-sm outline-none disabled:opacity-50"></textarea>
    <div class="flex items-center gap-1 px-2 pb-2">
      {#if !portal}
        <input bind:this={picker} type="file" accept="image/png,image/jpeg,image/gif,image/webp" multiple class="hidden" onchange={(e) => { addFiles([...e.currentTarget.files]); e.currentTarget.value = ""; }} />
        <Button type="button" variant="ghost" size="icon-sm" class="text-muted-foreground" onclick={() => (images ? picker.click() : toastError(new Error(`${providerName || "This provider"} doesn’t read pictures here: describe it in words instead.`)))}
          aria-label="Attach pictures" title={images ? "Attach pictures" : `${providerName || "This provider"} doesn’t read pictures`} disabled={disabled}><Paperclip /></Button>
        {#if onmodel}
          <DropdownMenu.Root>
            <DropdownMenu.Trigger>
              {#snippet child({ props })}
                <Button {...props} type="button" variant="ghost" size="sm" class="text-muted-foreground h-8 max-w-[11rem] gap-1 px-2 text-xs" aria-label="Model: {modelLabel}" title={modelName}>
                  <span class="truncate">{modelLabel}</span><ChevronDown class="size-3.5" />
                </Button>
              {/snippet}
            </DropdownMenu.Trigger>
            <DropdownMenu.Content align="start" class="max-h-80 w-72 overflow-y-auto">
              <DropdownMenu.Label>Answer with</DropdownMenu.Label>
              <DropdownMenu.Item onSelect={() => onmodel("main")}>
                <span class="flex min-w-0 flex-1 flex-col"><span>Main</span><span class="text-muted-foreground truncate text-xs">{mainName} · thorough</span></span>{#if !model || model === "main"}<Check class="size-4" />{/if}
              </DropdownMenu.Item>
              <DropdownMenu.Item onSelect={() => onmodel("fast")}>
                <span class="flex min-w-0 flex-1 flex-col"><span>Fast</span><span class="text-muted-foreground truncate text-xs">{fastName} · quick and cheap</span></span>{#if model === "fast"}<Check class="size-4" />{/if}
              </DropdownMenu.Item>
              {#if models?.options?.length}
                <DropdownMenu.Separator />
                <DropdownMenu.Label class="text-muted-foreground text-xs font-normal">{models.live ? "Your provider’s models" : "Known models"}</DropdownMenu.Label>
                {#each models.options as o (o.id)}
                  <DropdownMenu.Item onSelect={() => onmodel(o.id)}>
                    <span class="flex min-w-0 flex-1 flex-col"><span class="truncate">{o.label}</span>
                      <span class="text-muted-foreground truncate text-xs">{[o.recommended ? "Recommended" : "", o.context, o.price].filter(Boolean).join(" · ")}</span></span>
                    {#if model === o.id}<Check class="size-4" />{/if}
                  </DropdownMenu.Item>
                {/each}
              {/if}
            </DropdownMenu.Content>
          </DropdownMenu.Root>
        {/if}
      {/if}
      <span class="flex-1"></span>
      {#if replying && onstop}
        <Button type="button" size="icon-sm" class="rounded-full" onclick={onstop} aria-label="Stop" title="Stop"><Square class="size-3.5 fill-current" /></Button>
      {:else}
        <Button type="submit" size="icon-sm" class="rounded-full" disabled={busy || disabled || replying || (!text.trim() && !files.length)} aria-label="Send" title="Send (Enter)"><ArrowUp /></Button>
      {/if}
    </div>
  </form>
</div>
