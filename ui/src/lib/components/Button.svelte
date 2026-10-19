<script>
  // shadcn's Button with the admin's names for its looks: default (outline),
  // primary, danger (red text), destructive, plain (ghost); sm; icon-only; a
  // link when given an href. While `loading`, it keeps its width and spins.
  import { Button } from "$lib/components/ui/button/index.js";
  import { Spinner } from "$lib/components/ui/spinner/index.js";
  import { cn } from "$lib/utils.js";
  import Icon from "./Icon.svelte";

  let {
    variant = "default",
    size = "md",
    icon = null,
    iconOnly = false,
    href = null,
    type = "button",
    loading = false,
    disabled = false,
    block = false,
    title = undefined,
    class: klass = "",
    children,
    ...rest
  } = $props();

  const variants = { default: "outline", primary: "default", danger: "outline", destructive: "destructive", plain: "ghost" };
  const sizes = { md: ["default", "icon"], sm: ["sm", "icon-sm"] };
</script>

<Button
  variant={variants[variant]}
  size={sizes[size][iconOnly ? 1 : 0]}
  href={href || undefined}
  {type}
  disabled={disabled || loading}
  aria-busy={loading || undefined}
  {title}
  class={cn("relative", variant === "danger" && "text-destructive hover:text-destructive", block && "w-full", klass)}
  {...rest}
>
  <span class={cn("inline-flex items-center gap-1.5", loading && "invisible")}>
    {#if icon}<Icon name={icon} />{/if}
    {#if iconOnly}<span class="sr-only">{title}</span>{:else}{@render children?.()}{/if}
  </span>
  {#if loading}<span class="absolute inset-0 flex items-center justify-center"><Spinner /></span>{/if}
</Button>
