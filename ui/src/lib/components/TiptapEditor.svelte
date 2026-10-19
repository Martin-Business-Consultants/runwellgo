<script>
  // The rich-text editor: Tiptap (its default text editor example,
  // StarterKit and task lists), its toolbar in shadcn's toggles. The value
  // is the editor's own document (format "json": Tiptap JSON, what a
  // richtext field stores), Markdown or HTML. tools="email" keeps to what email shows
  // well; mergeTags marks {{ tags }} as chips, and insert(text) puts text at
  // the cursor (a tag clicked in a sidebar; dragged ones drop as text).
  // extend(modules) adds extensions built from Tiptap's own modules (loaded
  // with the editor): {tiptap, Plugin, PluginKey}, as @mentions do.
  import { onMount, onDestroy, untrack } from "svelte";
  import * as Popover from "$lib/components/ui/popover/index.js";
  import { toggleVariants } from "$lib/components/ui/toggle/index.js";
  import { Separator } from "$lib/components/ui/separator/index.js";
  import Bold from "@lucide/svelte/icons/bold";
  import Italic from "@lucide/svelte/icons/italic";
  import Strikethrough from "@lucide/svelte/icons/strikethrough";
  import Code from "@lucide/svelte/icons/code";
  import RemoveFormatting from "@lucide/svelte/icons/remove-formatting";
  import Pilcrow from "@lucide/svelte/icons/pilcrow";
  import Heading1 from "@lucide/svelte/icons/heading-1";
  import Heading2 from "@lucide/svelte/icons/heading-2";
  import Heading3 from "@lucide/svelte/icons/heading-3";
  import Heading4 from "@lucide/svelte/icons/heading-4";
  import List from "@lucide/svelte/icons/list";
  import ListOrdered from "@lucide/svelte/icons/list-ordered";
  import ListTodo from "@lucide/svelte/icons/list-todo";
  import SquareCode from "@lucide/svelte/icons/square-code";
  import TextQuote from "@lucide/svelte/icons/text-quote";
  import Minus from "@lucide/svelte/icons/minus";
  import CornerDownLeft from "@lucide/svelte/icons/corner-down-left";
  import Undo from "@lucide/svelte/icons/undo-2";
  import Redo from "@lucide/svelte/icons/redo-2";
  import LinkIcon from "@lucide/svelte/icons/link";
  import Input from "./Input.svelte";
  import Button from "./Button.svelte";

  let { value = $bindable(""), format = "markdown", label = "", tools = "full", mergeTags = false, active: live = true, id = undefined, invalid = false, placeholder = "", minHeight = "min-h-40", extend = null } = $props();
  let element = $state();
  let editor = $state(null);
  // Bumped on every transaction, so the toolbar's active states follow.
  let tick = $state(0);
  // What the editor last gave out, to tell its own updates from a value
  // set from outside (a load, a source tab).
  let emitted = null;

  // Tiptap loads with the first editor shown, not with the admin.
  onMount(async () => {
    const [tiptap, { richTextExtensions }, { Placeholder }, { Markdown }, { Plugin, PluginKey }, { Decoration, DecorationSet }] = await Promise.all([
      import("@tiptap/core"), import("../richtext-extensions.js"), import("@tiptap/extensions"), import("@tiptap/markdown"),
      import("@tiptap/pm/state"), import("@tiptap/pm/view"),
    ]);
    if (destroyed) return;
    const { Editor, Extension } = tiptap;
    // {{ tags }} drawn as chips: decorations, so the text stays the text.
    const Tags = Extension.create({
      name: "mergeTagChips",
      addProseMirrorPlugins: () => [new Plugin({ props: { decorations(state) {
        const found = [];
        state.doc.descendants((node, pos) => {
          if (!node.isText) return;
          for (const m of node.text.matchAll(/\{\{\s*[\w.]+\s*\}\}/g)) found.push(Decoration.inline(pos + m.index, pos + m.index + m[0].length, { class: "merge-tag" }));
        });
        return DecorationSet.create(state.doc, found);
      } } })],
    });
    editor = new Editor({
      element,
      extensions: [
        ...richTextExtensions(tools),
        Placeholder.configure({ placeholder }),
        ...(format === "markdown" ? [Markdown] : []),
        ...(mergeTags ? [Tags] : []),
        ...(extend ? extend({ tiptap, Plugin, PluginKey }) : []),
      ],
      content: value || (format === "json" ? null : ""),
      contentType: format,
      editorProps: {
        attributes: { class: `prose-admin tiptap ${minHeight} px-3 py-2 outline-none`, ...(id ? { id } : {}), "aria-multiline": "true", role: "textbox", "aria-label": label || placeholder || "Text" },
        // A merge tag dragged from a sidebar goes in where it's dropped.
        handleDrop(view, event) {
          const text = event.dataTransfer?.getData(TAG_TYPE);
          if (!text) return false;
          const at = view.posAtCoords({ left: event.clientX, top: event.clientY });
          if (!at) return false;
          event.preventDefault();
          view.dispatch(view.state.tr.insertText(text, at.pos));
          view.focus();
          return true;
        },
      },
      // A moment later: an editor removed while focused blurs (a
      // transaction) while the page is being drawn, when state can't change.
      onTransaction: () => { queueMicrotask(() => { if (!destroyed) tick++; }); },
      onUpdate: () => { emitted = out(); value = emitted; },
    });
    emitted = out();
  });
  let destroyed = false;
  const TAG_TYPE = "application/x-lp-merge-tag";
  const out = () => format === "json" ? (editor.isEmpty ? null : editor.getJSON())
    : format === "markdown" ? editor.getMarkdown() : editor.isEmpty ? "" : editor.getHTML();
  // A document is compared by what it says, not which object holds it.
  const same = (a, b) => (format === "json" ? JSON.stringify(a ?? null) === JSON.stringify(b ?? null) : (a || "") === (b || ""));
  export function insert(text) { editor?.chain().focus().insertContent(text).run(); }
  onDestroy(() => { destroyed = true; editor?.destroy(); });

  $effect(() => {
    const v = format === "json" ? (value ?? null) : value || "";
    if (editor && live && !same(v, emitted)) {
      // Untracked: the transaction bumps tick, which this mustn't follow.
      untrack(() => editor.commands.setContent(v, { contentType: format, emitUpdate: false }));
      emitted = out();
    }
  });

  const active = (name, attrs) => (tick, editor?.isActive(name, attrs ?? {}) ?? false);
  const can = (fn) => (tick, editor ? fn(editor.can().chain()).run() : false);
  const run = (fn) => fn(editor.chain().focus()).run();

  // svelte-ignore state_referenced_locally
  const email = tools === "email";
  // compact: what a note, a description or a comment needs (bold, italic,
  // a link, a heading, lists, a quote), so the toolbar stays one short row.
  // svelte-ignore state_referenced_locally
  const compact = tools === "compact";
  const marks = compact ? [
    ["bold", "Bold", Bold, (c) => c.toggleBold()],
    ["italic", "Italic", Italic, (c) => c.toggleItalic()],
  ] : [
    ["bold", "Bold", Bold, (c) => c.toggleBold()],
    ["italic", "Italic", Italic, (c) => c.toggleItalic()],
    ["strike", "Strikethrough", Strikethrough, (c) => c.toggleStrike()],
    ...(email ? [] : [["code", "Code", Code, (c) => c.toggleCode()]]),
  ];
  const allBlocks = [
    ["paragraph", null, "Paragraph", Pilcrow, (c) => c.setParagraph()],
    ["heading", { level: 1 }, "Heading 1", Heading1, (c) => c.toggleHeading({ level: 1 })],
    ["heading", { level: 2 }, "Heading 2", Heading2, (c) => c.toggleHeading({ level: 2 })],
    ["heading", { level: 3 }, compact ? "Heading" : "Heading 3", Heading3, (c) => c.toggleHeading({ level: 3 })],
    ["heading", { level: 4 }, "Heading 4", Heading4, (c) => c.toggleHeading({ level: 4 })],
    ["bulletList", null, "Bullet list", List, (c) => c.toggleBulletList()],
    ["orderedList", null, "Ordered list", ListOrdered, (c) => c.toggleOrderedList()],
    ["taskList", null, "Task list", ListTodo, (c) => c.toggleTaskList()],
    ["codeBlock", null, "Code block", SquareCode, (c) => c.toggleCodeBlock()],
    ["blockquote", null, "Quote", TextQuote, (c) => c.toggleBlockquote()],
  ];
  const compactBlocks = ["heading-3", "bulletList", "orderedList", "taskList", "blockquote"];
  const blocks = compact ? allBlocks.filter(([name, attrs]) => compactBlocks.includes(name === "heading" ? "heading-" + attrs.level : name))
    : email ? allBlocks.filter(([name, attrs]) => !["taskList", "codeBlock"].includes(name) && !(name === "heading" && attrs.level > 3)) : allBlocks;
  const btn = toggleVariants({ size: "sm" }) + " h-7 min-w-7 px-1.5";
  // The toolbar's buttons leave the focus (and the selection) in the text.
  const keep = (e) => e.preventDefault();

  let linkOpen = $state(false), href = $state("");
  function openLink(open) {
    linkOpen = open;
    if (open) href = editor?.getAttributes("link").href || "";
  }
  function applyLink() {
    run((c) => (href.trim() ? c.extendMarkRange("link").setLink({ href: href.trim() }) : c.extendMarkRange("link").unsetLink()));
    linkOpen = false;
  }
</script>

<div class="border-input dark:bg-input/30 focus-within:border-ring focus-within:ring-ring/50 rounded-md border shadow-xs focus-within:ring-3 {invalid ? 'border-destructive' : ''}">
  {#if editor}
    <div class="flex flex-wrap items-center gap-0.5 border-b p-1" role="toolbar" aria-label="Formatting">
      {#each marks as [name, label, Glyph, fn]}
        <button type="button" onmousedown={keep} class={btn} title={label} aria-label={label} aria-pressed={active(name)} disabled={!can(fn)} onclick={() => run(fn)}><Glyph /></button>
      {/each}
      <Popover.Root open={linkOpen} onOpenChange={openLink}>
        <Popover.Trigger class={btn} title="Link" aria-label="Link" aria-pressed={active("link")}><LinkIcon /></Popover.Trigger>
        <Popover.Content class="w-80" align="start">
          <form class="flex gap-2" onsubmit={(e) => { e.preventDefault(); applyLink(); }}>
            <Input bind:value={href} placeholder="https://… or /a-page" aria-label="Link address" />
            <Button type="submit" size="sm">{href.trim() ? "Apply" : "Remove"}</Button>
          </form>
        </Popover.Content>
      </Popover.Root>
      {#if !compact}
        <button type="button" onmousedown={keep} class={btn} title="Clear formatting" aria-label="Clear formatting" onclick={() => run((c) => c.unsetAllMarks().clearNodes())}><RemoveFormatting /></button>
      {/if}
      <Separator orientation="vertical" class="mx-1 data-vertical:h-5 data-vertical:self-center" />
      {#each blocks as [name, attrs, label, Glyph, fn]}
        <button type="button" onmousedown={keep} class={btn} title={label} aria-label={label} aria-pressed={active(name, attrs)} onclick={() => run(fn)}><Glyph /></button>
      {/each}
      {#if !compact}
      <Separator orientation="vertical" class="mx-1 data-vertical:h-5 data-vertical:self-center" />
      <button type="button" onmousedown={keep} class={btn} title="Horizontal rule" aria-label="Horizontal rule" onclick={() => run((c) => c.setHorizontalRule())}><Minus /></button>
      <button type="button" onmousedown={keep} class={btn} title="Line break" aria-label="Line break" onclick={() => run((c) => c.setHardBreak())}><CornerDownLeft /></button>
      <span class="ml-auto flex gap-0.5">
        <button type="button" onmousedown={keep} class={btn} title="Undo" aria-label="Undo" disabled={!can((c) => c.undo())} onclick={() => run((c) => c.undo())}><Undo /></button>
        <button type="button" onmousedown={keep} class={btn} title="Redo" aria-label="Redo" disabled={!can((c) => c.redo())} onclick={() => run((c) => c.redo())}><Redo /></button>
      </span>
      {/if}
    </div>
  {/if}
  <div bind:this={element}></div>
</div>
