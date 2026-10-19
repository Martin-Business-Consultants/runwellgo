<script>
  // ? opens this: every key the app answers (lib/keys.svelte.js), grouped
  // by what they do.
  import * as Dialog from "$lib/components/ui/dialog/index.js";
  import { Kbd } from "$lib/components/ui/kbd/index.js";
  import { keys, GOTO } from "../keys.svelte.js";
  import { session } from "../session.svelte.js";

  const word = (label) => ({ Clients: session.term("client", 2), Engagements: session.term("engagement", 2), Commitments: session.term("commitment", 2), Work: session.term("work", 2) })[label] || label;
  const groups = [
    ["Go to", GOTO.map(([k, , label]) => [["g", k], word(label)])],
    ["On a list", [[["j"], "Next item"], [["k"], "Previous item"], [["g", "g"], "First item"], [["G"], "Last item"], [["Enter"], "Open it"], [["o"], "Open it"], [["O"], "Open it in a new tab"], [["x"], "Pick it, for the bulk bar"], [["Esc"], "Let go of it"]]],
    ["On a screen", [[["h"], "Previous tab"], [["l"], "Next tab"], [["["], "Previous page"], [["]"], "Next page"], [["/"], "Search this list"], [["n"], "New"], [["e"], "Edit"]]],
    ["Anywhere", [[["s"], "Search everything"], [["⌘", "K"], "Search everything"], [["a"], "Add something"], [["i"], "Ask AI (when it's on)"], [["?"], "These keys"]]],
  ];
</script>

<Dialog.Root bind:open={keys.help}>
  <Dialog.Content class="sm:max-w-2xl">
    <Dialog.Header>
      <Dialog.Title>Keyboard</Dialog.Title>
      <Dialog.Description>Move around and act without the mouse. Keys wait while you're typing in a field.</Dialog.Description>
    </Dialog.Header>
    <div class="grid gap-x-8 gap-y-5 sm:grid-cols-2">
      {#each groups as [title, list] (title)}
        <section>
          <h3 class="text-muted-foreground mb-2 text-xs font-semibold">{title}</h3>
          <dl class="flex flex-col gap-1.5 text-sm">
            {#each list as [combo, what] (what + combo.join(""))}
              <div class="flex items-center justify-between gap-4">
                <dt>{what}</dt>
                <dd class="flex shrink-0 items-center gap-1">{#each combo as k, i (i)}{#if i && combo[0] === "g"}<span class="text-muted-foreground text-xs">then</span>{/if}<Kbd>{k}</Kbd>{/each}</dd>
              </div>
            {/each}
          </dl>
        </section>
      {/each}
    </div>
  </Dialog.Content>
</Dialog.Root>
