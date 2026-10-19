<script>
  // shadcn's Dialog, or its Sheet (sliding in from the right) with `sheet`:
  // a title, the body, and actions in the footer. Escape and a click
  // outside close it; onclose follows.
  import * as D from "$lib/components/ui/dialog/index.js";
  import * as S from "$lib/components/ui/sheet/index.js";
  let { open = $bindable(false), title = "", size = "md", sheet = false, children, actions = undefined, onclose = undefined } = $props();
  const widths = { sm: "sm:max-w-md", md: "sm:max-w-xl", lg: "sm:max-w-3xl", xl: "sm:max-w-5xl" };
  const change = (v) => { if (!v) onclose?.(); };
</script>

{#if sheet}
  <S.Root bind:open onOpenChange={change}>
    <S.Content class="w-full {widths[size] || widths.md}">
      <S.Header><S.Title>{title}</S.Title></S.Header>
      <div class="flex min-h-0 flex-1 flex-col gap-4 overflow-y-auto px-4">{@render children()}</div>
      {#if actions}<S.Footer class="flex-row flex-wrap justify-end">{@render actions()}</S.Footer>{/if}
    </S.Content>
  </S.Root>
{:else}
  <D.Root bind:open onOpenChange={change}>
    <D.Content class="max-h-[calc(100dvh-2rem)] overflow-y-auto {widths[size]}">
      <D.Header><D.Title>{title}</D.Title></D.Header>
      <div class="flex min-w-0 flex-col gap-4">{@render children()}</div>
      {#if actions}<D.Footer>{@render actions()}</D.Footer>{/if}
    </D.Content>
  </D.Root>
{/if}
