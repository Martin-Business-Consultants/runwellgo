<script>
  // A date field: a button showing "Oct 20, 2026" that opens shadcn's
  // Calendar. value is YYYY-MM-DD ("" for none), what the API takes. t is
  // today, + and - a day, Delete clears, from the button.
  import * as Popover from "$lib/components/ui/popover/index.js";
  import { Calendar } from "$lib/components/ui/calendar/index.js";
  import { buttonVariants } from "$lib/components/ui/button/index.js";
  import { parseDate, today, getLocalTimeZone } from "@internationalized/date";
  import CalendarIcon from "@lucide/svelte/icons/calendar";
  import { cn } from "$lib/utils.js";
  import { date } from "../format.js";

  let { value = $bindable(""), id = undefined, placeholder = "Pick a date", clearable = true, class: klass = "", invalid = false, onchange = undefined } = $props();
  let open = $state(false);
  const parsed = $derived.by(() => { try { return value ? parseDate(String(value).slice(0, 10)) : undefined; } catch { return undefined; } });

  function set(d) {
    value = d ? d.toString() : "";
    onchange?.(value);
  }
  function keys(e) {
    const base = parsed || today(getLocalTimeZone());
    if (e.key === "t") set(today(getLocalTimeZone()));
    else if (e.key === "+" || e.key === "=") set(base.add({ days: 1 }));
    else if (e.key === "-") set(base.subtract({ days: 1 }));
    else if ((e.key === "Delete" || e.key === "Backspace") && clearable) set(null);
    else return;
    e.preventDefault();
  }
</script>

<Popover.Root bind:open>
  <Popover.Trigger {id} onkeydown={keys} aria-invalid={invalid}
    class={cn(buttonVariants({ variant: "outline" }), "w-full justify-start font-normal", !value && "text-muted-foreground", klass)}>
    <CalendarIcon />{value ? date(value) : placeholder}
  </Popover.Trigger>
  <Popover.Content class="w-auto overflow-hidden p-0" align="start">
    <Calendar type="single" value={parsed} captionLayout="dropdown" onValueChange={(d) => { set(d); open = false; }} />
    {#if clearable && value}
      <div class="border-t p-2"><button type="button" class="text-muted-foreground hover:text-foreground w-full text-sm" onclick={() => { set(null); open = false; }}>Clear</button></div>
    {/if}
  </Popover.Content>
</Popover.Root>
