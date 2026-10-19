<script>
  // A list table (shadcn's Table) in a bordered box: headers, rows that
  // light up under the pointer, a tick column with select-all when rows can
  // be acted on in bulk, and an empty row that says why it's empty.
  //   <DataTable {columns} rows={pages} key="id" selectable bind:selected>
  //     {#snippet row(page, show)}<Cell>…</Cell>{#if show("path")}<Cell>…</Cell>{/if}{/snippet}
  //   </DataTable>
  // Columns with a key can be hidden (hidden: [keys]; the row snippet asks
  // show(key)); density is compact or comfortable; the row whose key is
  // `editing` is drawn by the editor snippet across the table (Quick Edit).
  import * as Table from "$lib/components/ui/table/index.js";
  import { Checkbox } from "$lib/components/ui/checkbox/index.js";
  import { setContext } from "svelte";
  let {
    columns = [],
    rows = [],
    key = "id",
    selectable = false,
    selected = $bindable([]),
    row,
    empty = undefined,
    emptyText = "Nothing here yet.",
    onrowclick = undefined,
    hidden = [],
    density = "compact",
    editing = null,
    editor = undefined,
    class: klass = "",
  } = $props();
  const show = (key) => !hidden.includes(key);
  const shown = $derived(columns.filter((c) => !c.key || show(c.key)));
  setContext("table-density", () => density);

  const ids = $derived(rows.map((r) => r[key]));
  const all = $derived(ids.length > 0 && ids.every((id) => selected.includes(id)));
  const some = $derived(!all && ids.some((id) => selected.includes(id)));
  function toggleAll() { selected = all ? [] : [...ids]; }
  function toggle(id) { selected = selected.includes(id) ? selected.filter((x) => x !== id) : [...selected, id]; }
  const span = $derived(shown.length + (selectable ? 1 : 0));
</script>

<div class="min-w-0 overflow-hidden rounded-lg border {klass}">
  <Table.Root>
    <Table.Header>
      <Table.Row>
        {#if selectable}
          <Table.Head class="w-10">{#if rows.length}<Checkbox checked={all} indeterminate={some} onCheckedChange={toggleAll} aria-label="Select every row" />{/if}</Table.Head>
        {/if}
        {#each shown as c}
          <Table.Head class="{c.num ? 'text-right' : ''} {c.class || ''}">{c.label}</Table.Head>
        {/each}
      </Table.Row>
    </Table.Header>
    <Table.Body>
      {#each rows as r (r[key])}
        {#if editor && editing != null && r[key] === editing}
          <Table.Row class="bg-muted/40 hover:bg-muted/40">
            <Table.Cell colspan={span} class="whitespace-normal p-4">{@render editor(r)}</Table.Cell>
          </Table.Row>
        {:else}
        <Table.Row
          class="group {onrowclick ? 'cursor-pointer' : ''}"
          data-state={selected.includes(r[key]) ? "selected" : undefined}
          onclick={onrowclick ? (e) => { if (!e.target.closest('a,button,input,label,[role=checkbox]')) onrowclick(r); } : undefined}
        >
          {#if selectable}
            <Table.Cell class="w-10"><Checkbox checked={selected.includes(r[key])} onCheckedChange={() => toggle(r[key])} aria-label="Select" /></Table.Cell>
          {/if}
          {@render row(r, show)}
        </Table.Row>
        {/if}
      {:else}
        <Table.Row class="hover:bg-transparent">
          <Table.Cell colspan={span} class="text-muted-foreground h-32 whitespace-normal text-center">
            {#if empty}{@render empty()}{:else}{emptyText}{/if}
          </Table.Cell>
        </Table.Row>
      {/each}
    </Table.Body>
  </Table.Root>
</div>
