<script module>
  // Who can be mentioned, loaded once per page load (GET /api/prompts/users):
  // the people, then plugins' names (an AI agent) while they're on.
  let people = null;
  async function loadPeople(get) {
    if (!people) people = get("/prompts/users", { quiet: true })
      .then((r) => [...(r.users || []), ...(r.mentionables || []).map((m) => ({ ...m, mentionable: true, first_name: m.name, search: `${m.name} ${m.handle}` }))])
      .catch(() => { people = null; return []; });
    return people;
  }
</script>

<script>
  // Rich text with @mentions: the Tiptap editor writing HTML, where typing @
  // offers the people on staff (arrows and Enter, or a click, to pick one).
  // A mention is <span data-mention-id="12">@Ann</span>, which the server
  // reads to notify them; a plugin's name (an AI agent) is
  // <span data-mentionable="ai:7">@Researcher</span>, which it hears. ⌘/Ctrl+Enter calls onsubmit; with submitOnEnter,
  // Enter does (Shift+Enter is a new line), as a chat's composer.
  import TiptapEditor from "./TiptapEditor.svelte";
  import PersonAvatar from "./PersonAvatar.svelte";
  import { get } from "$lib/api.js";

  let { value = $bindable(""), placeholder = "", label = "", minHeight = "min-h-24", invalid = false, id = undefined, tools = "compact", onsubmit = null, submitOnEnter = false } = $props();

  let users = $state([]);
  // The open suggestion: what's typed after @, where it sits, where to draw.
  let suggest = $state(null);
  let index = $state(0);
  let view = null;
  let dismissed = -1;

  const matches = $derived.by(() => {
    if (!suggest) return [];
    const q = suggest.query.toLowerCase();
    return users.filter((u) => !q || `${u.search || u.name} ${u.initials}${u.me ? " me" : ""}`.toLowerCase().split(/\s+/).some((w) => w.startsWith(q))).slice(0, 8);
  });

  // Only when open: a blur can dispatch while the page is being drawn (an
  // editor removed while focused), where state mustn't change.
  function close() { if (suggest) suggest = null; }

  function detect(v) {
    view = v;
    const { selection } = v.state;
    if (!selection.empty) return close();
    const head = selection.$from;
    const before = head.parent.textBetween(Math.max(0, head.parentOffset - 40), head.parentOffset, undefined, "￼");
    const m = /(?:^|\s)@([\p{L}\p{N}_.-]{0,30})$/u.exec(before);
    if (!m) { dismissed = -1; return close(); }
    const from = selection.from - m[1].length - 1;
    if (from === dismissed) return close();
    const at = v.coordsAtPos(from);
    if (!suggest || suggest.from !== from) index = 0;
    suggest = { query: m[1], from, to: selection.from, x: at.left, y: at.bottom };
    loadPeople(get).then((list) => { users = list; });
  }

  function choose(user) {
    if (!view || !suggest || !user) return;
    const { schema, tr } = view.state;
    const node = user.mentionable
      ? schema.nodes.mention.create({ id: null, mentionable: user.id, label: user.name })
      : schema.nodes.mention.create({ id: String(user.id), label: user.first_name });
    view.dispatch(tr.replaceWith(suggest.from, suggest.to, [node, schema.text(" ")]));
    view.focus();
    close();
  }

  function onKey(event) {
    if (!suggest || !matches.length) return false;
    if (event.key === "ArrowDown") index = (index + 1) % matches.length;
    else if (event.key === "ArrowUp") index = (index - 1 + matches.length) % matches.length;
    else if (event.key === "Enter" || event.key === "Tab") choose(matches[index]);
    else if (event.key === "Escape") { dismissed = suggest.from; close(); }
    else return false;
    return true;
  }

  // Enter sends (when asked), unless @ is picking someone.
  function onEnter(event) {
    if (onKey(event)) return true;
    if (!submitOnEnter || !onsubmit || event.key !== "Enter" || event.shiftKey || event.isComposing || (suggest && matches.length)) return false;
    onsubmit();
    return true;
  }

  // The mention node and the @ watcher, made from the editor's own Tiptap.
  function extend({ tiptap, Plugin, PluginKey }) {
    const Mention = tiptap.Node.create({
      name: "mention",
      group: "inline",
      inline: true,
      atom: true,
      selectable: false,
      addAttributes: () => ({
        id: { default: null, parseHTML: (el) => el.getAttribute("data-mention-id"), renderHTML: (a) => (a.id ? { "data-mention-id": a.id } : {}) },
        mentionable: { default: null, parseHTML: (el) => el.getAttribute("data-mentionable"), renderHTML: (a) => (a.mentionable ? { "data-mentionable": a.mentionable } : {}) },
        label: { default: "", parseHTML: (el) => (el.textContent || "").replace(/^@/, ""), renderHTML: () => ({}) },
      }),
      parseHTML: () => [{ tag: "span[data-mention-id]" }, { tag: "span[data-mentionable]" }],
      renderHTML: ({ node, HTMLAttributes }) => ["span", tiptap.mergeAttributes({ "data-type": "mention", class: "mention" }, HTMLAttributes), "@" + node.attrs.label],
      renderText: ({ node }) => "@" + node.attrs.label,
    });
    const Suggest = tiptap.Extension.create({
      name: "mentionSuggest",
      addProseMirrorPlugins: () => [new Plugin({
        key: new PluginKey("mentionSuggest"),
        view: () => ({ update: (v) => detect(v), destroy: () => close() }),
        props: { handleKeyDown: (_v, event) => onEnter(event), handleDOMEvents: { blur: () => { setTimeout(close, 150); return false; } } },
      })],
    });
    return [Mention, Suggest];
  }

  function keydown(e) {
    if (e.key === "Enter" && (e.metaKey || e.ctrlKey) && onsubmit) { e.preventDefault(); onsubmit(); }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div onkeydown={keydown}>
  <TiptapEditor bind:value format="html" {tools} {placeholder} {label} {minHeight} {invalid} {id} {extend} />
</div>

{#if suggest && matches.length}
  <div class="bg-popover text-popover-foreground fixed z-50 w-56 rounded-md border p-1 shadow-md" style="left: {suggest.x}px; top: {suggest.y + 4}px" role="listbox" aria-label="People and agents to mention">
    {#each matches as u, i (u.id)}
      <button type="button" role="option" aria-selected={i === index}
        class="flex w-full items-center gap-2 rounded-sm px-2 py-1.5 text-left text-sm {i === index ? 'bg-accent text-accent-foreground' : ''}"
        onmousedown={(e) => { e.preventDefault(); choose(u); }} onmouseenter={() => (index = i)}>
        <PersonAvatar person={u} size="size-5" />
        <span class="truncate">{u.name}</span>
        {#if u.me}<span class="text-muted-foreground ms-auto text-xs">you</span>{/if}
        {#if u.mentionable}<span class="text-muted-foreground ms-auto shrink-0 text-xs">{u.paused ? "paused" : u.kind || "agent"}</span>{/if}
      </button>
    {/each}
  </div>
{/if}
