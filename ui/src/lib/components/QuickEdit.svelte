<script>
  // Quick Edit (WordPress's): a list row's title, slug, status, locale and,
  // for a page, its parent, edited in place without opening the editor.
  // Enter saves, Escape cancels.
  import Field from "./Field.svelte";
  import Input from "./Input.svelte";
  import Select from "./Select.svelte";
  import Combobox from "./Combobox.svelte";
  import Button from "./Button.svelte";
  import { humanize } from "../format.js";
  let { record, parents = null, canPublish = false, errors = {}, saving = false, onsave, oncancel } = $props();
  // svelte-ignore state_referenced_locally
  let draft = $state({ title: record.title, slug: record.slug, status: record.status, locale: record.locale, parent_id: record.parent_id ?? null });
  function submit(e) { e.preventDefault(); onsave($state.snapshot(draft)); }
</script>

<!-- svelte-ignore a11y_no_noninteractive_element_interactions -->
<form class="flex flex-col gap-4" onsubmit={submit} onkeydown={(e) => { if (e.key === "Escape") { e.preventDefault(); oncancel(); } }}>
  <p class="text-sm font-semibold">Quick Edit</p>
  <div class="grid gap-4 md:grid-cols-2 xl:grid-cols-4">
    <Field label="Title" error={errors.title}>{#snippet children(id, invalid)}<Input {id} {invalid} bind:value={draft.title} autofocus />{/snippet}</Field>
    <Field label="Slug" error={errors.slug}>{#snippet children(id, invalid)}<Input {id} {invalid} mono bind:value={draft.slug} />{/snippet}</Field>
    <Field label="Status" error={errors.status}>
      {#snippet children(id)}
        {#if canPublish}<Select {id} options={["draft", "published", "archived"].map((s) => ({ value: s, label: humanize(s) }))} bind:value={draft.status} />
        {:else}<p class="text-muted-foreground py-2 text-sm">{humanize(draft.status)} (your role can't change it)</p>{/if}
      {/snippet}
    </Field>
    <Field label="Locale" error={errors.locale}>{#snippet children(id, invalid)}<Input {id} {invalid} bind:value={draft.locale} class="max-w-24" />{/snippet}</Field>
    {#if parents}
      <Field label="Parent" class="md:col-span-2" error={errors.parent_id}>
        {#snippet children(id)}<Combobox {id} placeholder="No parent (top level)" options={parents} bind:value={draft.parent_id} />{/snippet}
      </Field>
    {/if}
  </div>
  <div class="flex gap-2">
    <Button type="submit" variant="primary" size="sm" loading={saving}>Update</Button>
    <Button size="sm" onclick={oncancel}>Cancel</Button>
  </div>
</form>
