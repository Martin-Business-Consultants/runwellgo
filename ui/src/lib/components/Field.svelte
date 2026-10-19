<script>
  // A labelled field (shadcn's Field): the label (with a required mark and
  // a note), the control, help under it, and the API's errors for it. The
  // control is rendered with the id the label points at:
  //   <Field label="Title" required>{#snippet children(id)}<Input {id} … />{/snippet}</Field>
  import * as F from "$lib/components/ui/field/index.js";
  let { label = "", required = false, note = "", help = "", error = null, id = undefined, class: klass = "", children, labelHidden = false } = $props();
  const uid = $props.id();
  const fid = $derived(id || `f-${uid}`);
  const messages = $derived(error == null ? [] : Array.isArray(error) ? error : [error]);
</script>

<F.Field class="min-w-0 gap-1.5 {klass}" data-invalid={messages.length > 0 || undefined}>
  {#if label}
    <F.Label for={fid} class={labelHidden ? "sr-only" : ""}>
      {label}{#if required}<span class="text-destructive" aria-hidden="true">*</span>{/if}
      {#if note}<span class="text-muted-foreground text-xs font-normal">{note}</span>{/if}
    </F.Label>
  {/if}
  {@render children?.(fid, messages.length > 0)}
  {#if help}<F.Description class="text-xs">{help}</F.Description>{/if}
  {#each messages as m}<F.Error class="text-xs">{m}</F.Error>{/each}
</F.Field>
