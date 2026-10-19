<script>
  // Your Ask chats, latest first, grouped by when (today, yesterday, the
  // last week, the last month, older): open one, start a new one, rename
  // or delete one.
  import { Button } from "$lib/components/ui/button/index.js";
  import { Input } from "$lib/components/ui/input/index.js";
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import { Skeleton } from "$lib/components/ui/skeleton/index.js";
  import Plus from "@lucide/svelte/icons/plus";
  import Ellipsis from "@lucide/svelte/icons/ellipsis";
  import MessageSquare from "@lucide/svelte/icons/message-square";
  import { staff, groupChats } from "./ai.svelte.js";
  import { confirm } from "$lib/confirm.svelte.js";
  import { toast, toastError } from "$lib/toast.svelte.js";

  let { current = null, onopen, onnew, version = 0 } = $props();

  let chats = $state(null), renaming = $state(null), name = $state("");

  async function load() {
    try { chats = (await staff.get("/chats/history")).chats; } catch { chats = []; }
  }
  $effect(() => { version; load(); });
  const groups = $derived(groupChats(chats || []));

  function startRename(c) { renaming = c.id; name = c.title; }
  async function rename(c) {
    const title = name.trim();
    renaming = null;
    if (!title || title === c.title) return;
    try { await staff.patch(`/chats/${c.id}`, { chat: { title } }); c.title = title; toast("Renamed."); } catch (e) { toastError(e); }
  }
  async function remove(c) {
    if (!(await confirm({ title: "Delete this chat?", message: "Its messages go for good. What it cost stays in this month’s spend.", confirmLabel: "Delete", destructive: true }))) return;
    try {
      await staff.del(`/chats/${c.id}`);
      chats = chats.filter((x) => x.id !== c.id);
      toast("Chat deleted.");
      if (c.id === current) onnew?.();
    } catch (e) { toastError(e); }
  }
  function renameKey(e, c) {
    if (e.key === "Enter") { e.preventDefault(); rename(c); }
    if (e.key === "Escape") { e.preventDefault(); e.stopPropagation(); renaming = null; }
  }
</script>

<div class="flex min-h-0 flex-1 flex-col">
  <div class="p-2">
    <Button variant="outline" size="sm" class="w-full justify-start" onclick={() => onnew?.()}><Plus />New chat</Button>
  </div>
  <nav class="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 pb-3" aria-label="Your chats">
    {#if !chats}
      <div class="flex flex-col gap-2 p-2"><Skeleton class="h-6 w-full" /><Skeleton class="h-6 w-4/5" /><Skeleton class="h-6 w-3/5" /></div>
    {:else if !chats.length}
      <p class="text-muted-foreground flex items-center gap-2 px-2 py-3 text-sm"><MessageSquare class="size-4" />Your chats show here once you ask something.</p>
    {:else}
      {#each groups as [label, list] (label)}
        <p class="text-muted-foreground px-2 pt-3 pb-1 text-xs font-medium">{label}</p>
        <ul class="flex flex-col">
          {#each list as c (c.id)}
            <li class="group relative flex items-center rounded-md {c.id === current ? 'bg-accent text-accent-foreground' : 'hover:bg-accent/60'}">
              {#if renaming === c.id}
                <Input bind:value={name} class="h-8 text-sm" aria-label="Chat name" onkeydown={(e) => renameKey(e, c)} onblur={() => rename(c)} autofocus />
              {:else}
                <button type="button" class="min-w-0 flex-1 truncate px-2 py-1.5 text-left text-sm" onclick={() => onopen?.(c.id)} aria-current={c.id === current ? "true" : undefined}>{c.title}</button>
                <DropdownMenu.Root>
                  <DropdownMenu.Trigger>
                    {#snippet child({ props })}
                      <Button {...props} variant="ghost" size="icon-sm" class="size-7 shrink-0 opacity-100 focus-visible:opacity-100 sm:opacity-0 sm:group-hover:opacity-100 data-[state=open]:opacity-100" aria-label="More for {c.title}"><Ellipsis /></Button>
                    {/snippet}
                  </DropdownMenu.Trigger>
                  <DropdownMenu.Content align="end">
                    <DropdownMenu.Item onSelect={() => startRename(c)}>Rename</DropdownMenu.Item>
                    <DropdownMenu.Item variant="destructive" onSelect={() => remove(c)}>Delete</DropdownMenu.Item>
                  </DropdownMenu.Content>
                </DropdownMenu.Root>
              {/if}
            </li>
          {/each}
        </ul>
      {/each}
    {/if}
  </nav>
</div>
