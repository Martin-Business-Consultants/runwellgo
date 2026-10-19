<script>
  // Who's on a todo, changed in place (Fizzy's assignments): everyone on it
  // as overlapping avatars, the lead first (an empty circle for nobody),
  // with their names when labelled. The menu lists the team; type to narrow
  // it, and each click puts that person on or takes them off, the menu
  // staying open. The first on leads; taking off the lead hands it to
  // whoever came on next.
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import UserPlus from "@lucide/svelte/icons/user-plus";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import { post } from "$lib/api.js";
  import { session } from "$lib/session.svelte.js";
  import { toastError } from "$lib/toast.svelte.js";
  import { ownersOf } from "./work.js";

  let { todo, labelled = false, onchange = undefined } = $props();
  let query = $state("");
  const owners = $derived(ownersOf(todo));
  const label = $derived(owners.length ? owners.map((p, i) => (i === 0 && owners.length > 1 ? `${p.name} (lead)` : p.name)).join(", ") : "Unassigned");
  // Fuzzy: the letters in order, so "sp" finds Sarah Producer.
  const fuzzy = (name, q) => { let i = 0; for (const ch of name.toLowerCase()) if (ch === q[i]) i++; return i === q.length; };
  const team = $derived((session.me?.team || []).filter((p) => !query || fuzzy(p.name, query.toLowerCase().replace(/\s+/g, ""))));

  async function toggle(person) {
    try { onchange?.((await post(`/work/${todo.id}/assignments`, { assignee_id: person.id })).todo); } catch (e) { toastError(e); }
  }
</script>

<DropdownMenu.Root onOpenChange={(o) => { if (!o) query = ""; }}>
  <DropdownMenu.Trigger>
    {#snippet child({ props })}
      <button {...props} type="button" title="{label}: click to change" class="hover:bg-muted inline-flex min-w-0 items-center gap-1.5 rounded-md px-1 py-0.5 text-sm">
        {#if owners.length}
          <span class="flex -space-x-1.5">
            {#each owners.slice(0, 3) as p (p.id)}<span class="ring-background rounded-full ring-2"><PersonAvatar person={p} /></span>{/each}
            {#if owners.length > 3}<span class="bg-muted ring-background flex size-6 items-center justify-center rounded-full text-[10px] ring-2">+{owners.length - 3}</span>{/if}
          </span>
        {:else}
          <span class="text-muted-foreground flex size-6 items-center justify-center rounded-full border border-dashed" aria-hidden="true"><UserPlus class="size-3.5" /></span>
        {/if}
        {#if labelled}<span class="truncate">{owners.length ? owners.map((p) => p.name).join(", ") : "Unassigned"}</span>{/if}
        <span class="sr-only">On it: {label}. Change who’s on {todo.title}</span>
      </button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="start" class="w-60">
    <input bind:value={query} placeholder="Who’s on it…" aria-label="Find a person" autocomplete="off"
      class="placeholder:text-muted-foreground w-full bg-transparent px-2 py-1.5 text-sm outline-none" onkeydown={(e) => { if (!["Escape", "ArrowDown", "ArrowUp", "Tab"].includes(e.key)) e.stopPropagation(); }} />
    <DropdownMenu.Separator />
    {#each team as p (p.id)}
      <DropdownMenu.CheckboxItem checked={owners.some((o) => o.id === p.id)} onCheckedChange={() => toggle(p)}>
        <PersonAvatar person={p} size="size-5" /><span class="truncate">{p.name}</span>
      </DropdownMenu.CheckboxItem>
    {:else}
      <p class="text-muted-foreground px-2 py-1.5 text-sm">Nobody matches.</p>
    {/each}
    <DropdownMenu.Separator />
    <p class="text-muted-foreground px-2 py-1 text-xs">The first on leads.</p>
  </DropdownMenu.Content>
</DropdownMenu.Root>
