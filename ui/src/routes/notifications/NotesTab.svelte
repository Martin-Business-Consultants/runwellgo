<script>
  // The notes tab on a record's page: the notes its answer carries.
  import NotesSection from "$lib/components/NotesSection.svelte";
  let { record, reload, type = undefined } = $props();
</script>

<NotesSection subjectType={type || record.type} subjectId={record.id} notes={record.notes || []} {reload} />
