// Runwell AI (plugins/ai): talking to its routes, whether it's set up, and
// the AI's words as text with links.
import { get, post, patch, del } from "$lib/api.js";
import { pget, ppost } from "../../routes/portal/portal.svelte.js";

export const key = "ai";

// staff and portal are the plugin's routes, for a person on staff and for a
// client's contact in their portal.
export const staff = {
  get: (p) => get("/x/ai" + p, { quiet: true }),
  post: (p, b = {}) => post("/x/ai" + p, b),
  patch: (p, b = {}) => patch("/x/ai" + p, b),
  del: (p) => del("/x/ai" + p),
};
export const portal = {
  get: (p) => pget("/x/ai/portal" + p, { quiet: true }),
  post: (p, b = {}) => ppost("/x/ai/portal" + p, b),
};

// status is whether AI is set up, and usable now: what decides whether the
// Ask button and the suggestions show. Read once, and again after Settings.
class Status {
  ready = $state(false);
  available = $state(false);
  reason = $state("");
  loading = null;

  load(force = false) {
    if (this.loading && !force) return this.loading;
    this.loading = staff.get("/status").then((s) => {
      this.ready = s.ready;
      this.available = s.available;
      this.reason = s.reason || "";
    }).catch(() => { this.ready = false; });
    return this.loading;
  }
}
export const status = new Status();

// recordOnScreen is the record a page is about, "Client:12": the one the
// page says, else read off its address.
export function recordFrom(record, path) {
  if (record) return record;
  const m = path.match(/^\/(clients|engagements|work|requests|scope_items)\/([^/]+)/);
  if (!m || m[2] === "new") return "";
  const type = { clients: "Client", engagements: "Engagement", work: "Todo", requests: "Request", scope_items: "ScopeItem" }[m[1]];
  return `${type}:${decodeURIComponent(m[2])}`;
}

// ---- the AI's words ---------------------------------------------------------------------

const REF = /\b(Client|Engagement|Todo|Request|ScopeItem):([A-Za-z]{1,4}-\d+|\d+)\b/g;
const PATHS = { Client: "/clients/", Engagement: "/engagements/", Todo: "/work/", Request: "/requests/", ScopeItem: "/scope_items/" };
const BULLET = /^\s*(?:[-*•]|\d+\.)\s+/;

// inline splits a line into pieces: text, **bold**, `code` and record refs
// (as links).
export function inline(line) {
  const out = [];
  const re = /\*\*(.+?)\*\*|`([^`]+)`|\b(Client|Engagement|Todo|Request|ScopeItem):([A-Za-z]{1,4}-\d+|\d+)\b/g;
  let last = 0, m;
  while ((m = re.exec(line))) {
    if (m.index > last) out.push({ t: "text", v: line.slice(last, m.index) });
    if (m[1] !== undefined) out.push({ t: "b", v: m[1] });
    else if (m[2] !== undefined) out.push({ t: "code", v: m[2] });
    else out.push({ t: "ref", v: `${m[3]}:${m[4]}`, href: PATHS[m[3]] + m[4] });
    last = re.lastIndex;
  }
  if (last < line.length) out.push({ t: "text", v: line.slice(last) });
  return out;
}

// blocks are paragraphs (lines kept) and bullet lists, as ai_text drew them.
export function blocks(text) {
  return (text || "").trim().split(/\n{2,}/).filter(Boolean).map((block) => {
    const lines = block.split("\n").map((l) => l.trimEnd());
    if (lines.every((l) => BULLET.test(l))) return { list: true, lines: lines.map((l) => inline(l.replace(BULLET, ""))) };
    return { list: false, lines: lines.map(inline) };
  });
}

export { REF };

// ---- Ask's markdown ------------------------------------------------------------------------

// md reads the AI's markdown into blocks the chat draws itself (never HTML
// from the model): headings, paragraphs, lists, quotes, code blocks and
// tables, each line's pieces from mdInline.
export function md(text) {
  const lines = (text || "").replace(/\r\n/g, "\n").split("\n");
  const out = [];
  let i = 0;
  const isTableSep = (l) => /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/.test(l);
  const cells = (l) => l.trim().replace(/^\|/, "").replace(/\|$/, "").split("|").map((c) => mdInline(c.trim()));
  while (i < lines.length) {
    const line = lines[i];
    const fence = /^\s*(```|~~~)\s*([\w+#.-]*)\s*$/.exec(line);
    if (fence) {
      const code = [];
      i++;
      while (i < lines.length && !lines[i].trim().startsWith(fence[1])) code.push(lines[i++]);
      i++;
      out.push({ t: "code", lang: fence[2] || "", code: code.join("\n") });
      continue;
    }
    if (!line.trim()) { i++; continue; }
    const h = /^(#{1,4})\s+(.*)$/.exec(line);
    if (h) { out.push({ t: "h", level: h[1].length, v: mdInline(h[2]) }); i++; continue; }
    if (line.includes("|") && i + 1 < lines.length && isTableSep(lines[i + 1])) {
      const head = cells(line);
      i += 2;
      const rows = [];
      while (i < lines.length && lines[i].includes("|") && lines[i].trim()) rows.push(cells(lines[i++]));
      out.push({ t: "table", head, rows });
      continue;
    }
    if (/^\s*>/.test(line)) {
      const q = [];
      while (i < lines.length && /^\s*>/.test(lines[i])) q.push(lines[i++].replace(/^\s*>\s?/, ""));
      out.push({ t: "quote", lines: q.map(mdInline) });
      continue;
    }
    if (BULLET.test(line)) {
      const ordered = /^\s*\d+\./.test(line);
      const items = [];
      while (i < lines.length && (BULLET.test(lines[i]) || (/^\s{2,}\S/.test(lines[i]) && items.length))) {
        if (BULLET.test(lines[i])) items.push(lines[i].replace(BULLET, ""));
        else items[items.length - 1] += " " + lines[i].trim();
        i++;
      }
      out.push({ t: ordered ? "ol" : "ul", items: items.map(mdInline) });
      continue;
    }
    const para = [];
    while (i < lines.length && lines[i].trim() && !/^\s*(```|~~~)/.test(lines[i]) && !/^#{1,4}\s/.test(lines[i]) && !BULLET.test(lines[i]) && !/^\s*>/.test(lines[i]) &&
      !(lines[i].includes("|") && i + 1 < lines.length && isTableSep(lines[i + 1]))) para.push(lines[i++]);
    out.push({ t: "p", lines: para.map(mdInline) });
  }
  return out;
}

// mdInline is a line's pieces: text, bold, italic, code, links (web
// addresses and the app's own paths only) and record refs.
export function mdInline(line) {
  const out = [];
  const re = /\*\*(.+?)\*\*|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)|(?<![\w*])\*(?!\s)([^*]+?)\*(?!\w)|(https?:\/\/[^\s<>)]+[^\s<>).,;:!?])|\b(Client|Engagement|Todo|Request|ScopeItem):([A-Za-z]{1,4}-\d+|\d+)\b/g;
  let last = 0, m;
  while ((m = re.exec(line))) {
    if (m.index > last) out.push({ t: "text", v: line.slice(last, m.index) });
    if (m[1] !== undefined) out.push({ t: "b", v: m[1] });
    else if (m[2] !== undefined) out.push({ t: "code", v: m[2] });
    else if (m[3] !== undefined) {
      const url = m[4];
      if (/^(https?:\/\/|\/(?!\/))/.test(url)) out.push({ t: "link", v: m[3], href: url, external: url.startsWith("http") });
      else out.push({ t: "text", v: m[3] });
    } else if (m[5] !== undefined) out.push({ t: "i", v: m[5] });
    else if (m[6] !== undefined) out.push({ t: "link", v: m[6], href: m[6], external: true });
    else out.push({ t: "ref", v: `${m[7]}:${m[8]}`, href: PATHS[m[7]] + m[8] });
    last = re.lastIndex;
  }
  if (last < line.length) out.push({ t: "text", v: line.slice(last) });
  return out;
}

// groupChats puts the history in the groups the list shows, latest first.
export function groupChats(chats, now = new Date()) {
  const day = new Date(now.getFullYear(), now.getMonth(), now.getDate()).getTime();
  const groups = [["Today", []], ["Yesterday", []], ["Previous 7 days", []], ["Previous 30 days", []], ["Older", []]];
  for (const c of chats) {
    const t = new Date(c.updated_at).getTime();
    const g = t >= day ? 0 : t >= day - 864e5 ? 1 : t >= day - 7 * 864e5 ? 2 : t >= day - 30 * 864e5 ? 3 : 4;
    groups[g][1].push(c);
  }
  return groups.filter(([, list]) => list.length);
}

// copyText puts text on the clipboard, answering whether it could.
export async function copyText(text) {
  try { await navigator.clipboard.writeText(text); return true; } catch { return false; }
}
