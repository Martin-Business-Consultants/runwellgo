<script>
  // The documents tab on a record's page: the files its answer carries.
  import DocumentsSection from "$lib/components/DocumentsSection.svelte";
  let { record, reload, type = undefined } = $props();
</script>

<DocumentsSection subjectType={type || record.type} subjectId={record.id} documents={record.documents || []} {reload} />
