<script>
  // Who's on the picked piece of work (the core's owner picker, saved
  // through My day): everyone on it as overlapping avatars with their names,
  // the lead first. The menu lists the team; type to narrow it, and each
  // click puts that person on or takes them off. The first on leads; taking
  // off the lead hands it to whoever came on next.
  import * as DropdownMenu from "$lib/components/ui/dropdown-menu/index.js";
  import UserPlus from "@lucide/svelte/icons/user-plus";
  import PersonAvatar from "$lib/components/PersonAvatar.svelte";
  import { session } from "$lib/session.svelte.js";
  import { call } from "./day.js";

  let { todo, onchange } = $props();
  let query = $state("");
  const owners = $derived([todo.owner, ...(todo.other_owners || [])].filter(Boolean));
  const names = $derived(owners.length ? owners.map((p) => p.name).join(", ") : "Unassigned");
  const fuzzy = (name, q) => { let i = 0; for (const ch of name.toLowerCase()) if (ch === q[i]) i++; return i === q.length; };
  const team = $derived((session.me?.team || []).filter((p) => !query || fuzzy(p.name, query.toLowerCase().replace(/\s+/g, ""))));

  async function toggle(person) {
    const on = owners.some((o) => o.id === person.id);
    const ids = on ? owners.filter((o) => o.id !== person.id).map((o) => o.id) : [...owners.map((o) => o.id), person.id];
    if (await call("PATCH", `/todos/${todo.id}/people`, { owner_id: ids[0] || 0, other_owner_ids: ids.slice(1) })) onchange?.();
  }
</script>

<DropdownMenu.Root onOpenChange={(o) => { if (!o) query = ""; }}>
  <DropdownMenu.Trigger>
    {#snippet child({ props })}
      <button {...props} type="button" title="{names}: click to change"
        class="border-input hover:bg-muted inline-flex h-8 min-w-0 items-center gap-1.5 rounded-full border px-2 text-sm">
        {#if owners.length}
          <span class="flex -space-x-1.5">
            {#each owners.slice(0, 3) as p (p.id)}<span class="ring-background rounded-full ring-2"><PersonAvatar person={p} size="size-5" /></span>{/each}
          </span>
        {:else}
          <UserPlus class="text-muted-foreground size-4" />
        {/if}
        <span class="max-w-56 truncate">{names}</span>
        <span class="sr-only">: change who's on {todo.title}</span>
      </button>
    {/snippet}
  </DropdownMenu.Trigger>
  <DropdownMenu.Content align="start" class="w-60">
    <input bind:value={query} placeholder="Who’s on it…" aria-label="Find a person" autocomplete="off"
      class="placeholder:text-muted-foreground w-full bg-transparent px-2 py-1.5 text-sm outline-none" onkeydown={(e) => { if (!["Escape", "ArrowDown", "ArrowUp", "Tab"].includes(e.key)) e.stopPropagation(); }} />
    <DropdownMenu.Separator />
    {#each team as p (p.id)}
      <DropdownMenu.CheckboxItem checked={owners.some((o) => o.id === p.id)} onCheckedChange={() => toggle(p)} closeOnSelect={false}>
        <PersonAvatar person={p} size="size-5" /><span class="truncate">{p.name}</span>
      </DropdownMenu.CheckboxItem>
    {:else}
      <p class="text-muted-foreground px-2 py-1.5 text-sm">Nobody matches.</p>
    {/each}
    <DropdownMenu.Separator />
    <p class="text-muted-foreground px-2 py-1 text-xs">The first on leads.</p>
  </DropdownMenu.Content>
</DropdownMenu.Root>
