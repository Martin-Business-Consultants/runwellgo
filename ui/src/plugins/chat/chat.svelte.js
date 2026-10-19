// Chat's state for this tab (plugins/chat): who you are and your settings,
// everyone on staff, your conversations with their unread counts, and who's
// typing where, kept up to date by the live stream. It starts on every
// page (ChatWatcher, in the header), so the sidebar's badge, presence and
// alerts work wherever you are; the chat page adds its own listeners.
import { get, post } from "$lib/api.js";
import { subscribe } from "$lib/live.svelte.js";
import { nav } from "$lib/nav.svelte.js";
import { router, go } from "$lib/router.svelte.js";

export const BASE = "/x/chat";

const IDLE = 5 * 60 * 1000; // no input for this long: away
const BEAT = 60 * 1000; // a tab says it's here this often

class Chat {
  ready = $state(false);
  me = $state(null);
  settings = $state(null);
  people = $state([]);
  // People removed from Runwell who still appear (an old direct message).
  former = $state([]);
  conversations = $state([]);
  maxUpload = $state(10 * 1024 * 1024);
  manage = $state(false);
  // typing[conversationId] = {userId: {name, parent_id, until}}
  typing = $state({});
  // The conversation open on the chat page, if any.
  current = $state(null);
  error = $state("");

  #started = false;
  #listeners = new Set();
  #lastInput = Date.now();

  byId = $derived(Object.fromEntries(this.people.map((p) => [p.id, p])));
  formerById = $derived(Object.fromEntries(this.former.map((p) => [p.id, p])));

  person(id) { return this.byId[id] || this.formerById[id] || { id, name: "Someone", avatar_url: `/api/users/${id}/avatar`, presence: "offline" }; }
  conversation(id) { return this.conversations.find((c) => c.id === id) || null; }

  async load() {
    try {
      const b = await get(BASE + "/bootstrap", { quiet: true });
      this.me = b.me;
      this.settings = b.settings;
      this.people = b.people;
      this.former = b.former || [];
      this.conversations = b.conversations;
      this.maxUpload = b.max_upload;
      this.manage = b.manage;
      this.ready = true;
      this.error = "";
    } catch (e) {
      this.error = e.message || "Chat couldn't load.";
    }
  }

  // start subscribes this tab to chat's events, once.
  start() {
    if (this.#started || typeof window === "undefined") return;
    this.#started = true;
    subscribe("chat", (topic, data) => this.#on(topic, data));
    // Here first, so everyone (this tab too) reads it as online.
    this.beat().finally(() => this.load());
    // Presence: this tab says whether it's in use, now and every minute.
    const input = () => {
      const was = Date.now() - this.#lastInput < IDLE;
      this.#lastInput = Date.now();
      if (!was) this.beat();
    };
    for (const ev of ["pointerdown", "keydown", "focus"]) addEventListener(ev, input, { passive: true });
    document.addEventListener("visibilitychange", () => this.beat());
    setInterval(() => this.beat(), BEAT);
    // Everyone's presence moves when tabs close: read it again now and then.
    setInterval(() => { if (document.visibilityState === "visible") this.load(); }, 5 * BEAT);
  }

  beat() {
    const active = document.visibilityState === "visible" && Date.now() - this.#lastInput < IDLE;
    return post(BASE + "/presence", { active }, { quiet: true }).catch(() => {});
  }

  // listen hands the chat page every event; it answers the stop.
  listen(fn) {
    this.#listeners.add(fn);
    return () => this.#listeners.delete(fn);
  }

  #emit(topic, data) {
    for (const fn of this.#listeners) {
      try { fn(topic, data); } catch (e) { console.error(e); }
    }
  }

  // upsert puts a conversation in the list as the server sent it, keeping
  // the counts this tab has (an update says nothing of them).
  upsert(c, counts = false) {
    const i = this.conversations.findIndex((x) => x.id === c.id);
    if (i < 0) this.conversations = [c, ...this.conversations];
    else this.conversations[i] = counts ? c : { ...c, unread: this.conversations[i].unread, mentions: this.conversations[i].mentions, last_read_id: Math.max(this.conversations[i].last_read_id || 0, c.last_read_id || 0) };
  }

  read(id, lastReadId) {
    const c = this.conversation(id);
    if (!c) return;
    c.unread = 0;
    c.mentions = 0;
    c.last_read_id = Math.max(c.last_read_id || 0, lastReadId || 0);
  }

  #on(topic, data) {
    switch (topic) {
      case "message": {
        const m = data.message;
        const c = this.conversation(m.conversation_id);
        if (!c) { this.load(); break; }
        if (!m.parent_id) {
          c.last_message_at = m.created_at;
          if (!m.mine) {
            c.unread = (c.unread || 0) + 1;
            if (this.me && (m.body || "").includes(`data-mention-id="${this.me.id}"`)) c.mentions = (c.mentions || 0) + 1;
          }
          this.conversations = [c, ...this.conversations.filter((x) => x.id !== c.id)];
        }
        // Whoever wrote has stopped typing.
        this.#stopTyping(m.conversation_id, m.user.id);
        if (data.notify && !m.mine) this.alert(m, c);
        nav.refresh();
        break;
      }
      case "typing": {
        const t = { ...(this.typing[data.conversation_id] || {}) };
        t[data.user.id] = { name: data.user.name, parent_id: data.parent_id, until: Date.now() + 6000 };
        this.typing = { ...this.typing, [data.conversation_id]: t };
        setTimeout(() => this.#expireTyping(), 6500);
        break;
      }
      case "read":
        this.read(data.conversation_id, data.last_read_id);
        nav.refresh();
        break;
      case "conversation":
        this.upsert(data);
        break;
      case "conversation_left":
        this.conversations = this.conversations.filter((c) => c.id !== data.id);
        nav.refresh();
        break;
      case "person": {
        const i = this.people.findIndex((p) => p.id === data.id);
        if (i >= 0) this.people[i] = data; else this.people = [...this.people, data];
        if (this.me?.id === data.id) this.me = data;
        break;
      }
      case "resync":
        this.load();
        break;
    }
    this.#emit(topic, data);
  }

  #stopTyping(convId, userId) {
    const t = this.typing[convId];
    if (!t?.[userId]) return;
    const rest = { ...t };
    delete rest[userId];
    this.typing = { ...this.typing, [convId]: rest };
  }

  #expireTyping() {
    const now = Date.now();
    let changed = false;
    const next = {};
    for (const [cid, t] of Object.entries(this.typing)) {
      const keep = Object.fromEntries(Object.entries(t).filter(([, v]) => v.until > now));
      if (Object.keys(keep).length !== Object.keys(t).length) changed = true;
      next[cid] = keep;
    }
    if (changed) this.typing = next;
  }

  // typers is who's typing in a conversation (or a thread), by name.
  typers(convId, parentId = null) {
    const t = this.typing[convId] || {};
    return Object.entries(t).filter(([, v]) => (v.parent_id || null) === (parentId || null) && v.until > Date.now()).map(([, v]) => v.name);
  }

  // looking is whether this tab is showing that conversation to someone.
  looking(convId) {
    return document.visibilityState === "visible" && document.hasFocus() && router.path === "/chat" && this.current === convId;
  }

  // alert chimes and shows a browser notification for a message meant for
  // you, when you're not looking at its conversation, as your settings say.
  // One tab does it, not every tab you have open.
  alert(m, c) {
    if (this.looking(m.conversation_id) || !claim(`chat-alert-${m.id}`)) return;
    const s = this.settings || {};
    if (s.sound) chime();
    if (s.browser && typeof Notification !== "undefined" && Notification.permission === "granted") {
      const where = c?.kind === "dm" ? m.user.name : `${m.user.name} in ${c?.name || "chat"}`;
      const n = new Notification(where, { body: m.text || "Sent a file.", tag: `chat-${m.conversation_id}`, icon: m.user.avatar_url });
      n.onclick = () => {
        window.focus();
        go("/chat", { c: m.conversation_id, thread: m.parent_id || undefined });
        n.close();
      };
    }
  }
}

// claim is true for the first tab to ask about a key (localStorage is
// shared by a person's tabs), so only one alerts.
function claim(key) {
  try {
    if (localStorage.getItem(key)) return false;
    localStorage.setItem(key, "1");
    setTimeout(() => { try { localStorage.removeItem(key); } catch { /* gone */ } }, 60000);
    return true;
  } catch {
    return true;
  }
}

// chime is a short soft two-note sound, made rather than downloaded.
let audio = null;
export function chime() {
  try {
    audio ||= new (window.AudioContext || window.webkitAudioContext)();
    const now = audio.currentTime;
    [[880, 0], [1320, 0.09]].forEach(([freq, at]) => {
      const o = audio.createOscillator();
      const g = audio.createGain();
      o.type = "sine";
      o.frequency.value = freq;
      g.gain.setValueAtTime(0.0001, now + at);
      g.gain.exponentialRampToValueAtTime(0.12, now + at + 0.02);
      g.gain.exponentialRampToValueAtTime(0.0001, now + at + 0.25);
      o.connect(g).connect(audio.destination);
      o.start(now + at);
      o.stop(now + at + 0.3);
    });
  } catch { /* no sound here */ }
}

export const chat = new Chat();

// ---- words and shapes ------------------------------------------------------

// dayLabel is a day divider's words: Today, Yesterday, or the date.
export function dayLabel(iso) {
  const d = new Date(iso);
  const today = new Date();
  const yesterday = new Date();
  yesterday.setDate(today.getDate() - 1);
  if (d.toDateString() === today.toDateString()) return "Today";
  if (d.toDateString() === yesterday.toDateString()) return "Yesterday";
  return d.toLocaleDateString(undefined, { weekday: "long", month: "long", day: "numeric", ...(d.getFullYear() !== today.getFullYear() ? { year: "numeric" } : {}) });
}

export const clockTime = (iso) => new Date(iso).toLocaleTimeString(undefined, { hour: "numeric", minute: "2-digit" });

export const sameDay = (a, b) => new Date(a).toDateString() === new Date(b).toDateString();

// grouped is whether a message joins the one before it (same person, a
// few minutes apart, same day, neither deleted).
export const grouped = (prev, m) => prev && prev.user.id === m.user.id && (prev.agent?.id || "") === (m.agent?.id || "") && !prev.deleted && sameDay(prev.created_at, m.created_at) &&
  new Date(m.created_at) - new Date(prev.created_at) < 5 * 60 * 1000;

export function typingWords(names) {
  if (!names.length) return "";
  if (names.length === 1) return `${names[0]} is typing…`;
  if (names.length === 2) return `${names[0]} and ${names[1]} are typing…`;
  return "Several people are typing…";
}

export const PRESENCE = { online: "Online", away: "Away", offline: "Offline" };

export const QUICK_EMOJI = ["👍", "❤️", "😂", "🎉", "👀", "✅"];
export const EMOJI = [
  "👍", "👎", "❤️", "😂", "🎉", "👀", "✅", "🙏", "🔥", "💯", "😀", "😃", "😄", "😁", "😅", "🤣", "😊", "😇", "🙂", "😉",
  "😍", "🥰", "😘", "😋", "😎", "🤓", "🤔", "🤨", "😐", "😶", "🙄", "😏", "😬", "😌", "😴", "😷", "🤒", "🤯", "🥳", "😢",
  "😭", "😱", "😡", "🤬", "🥺", "🤗", "🤝", "👏", "🙌", "👋", "✌️", "🤞", "💪", "👌", "☝️", "👉", "✍️", "🧠", "💡", "📌",
  "📎", "📅", "📈", "📉", "📝", "📣", "🚀", "⭐", "🌟", "⚡", "☕", "🍕", "🍰", "🎂", "🏆", "🎯", "❌", "⚠️", "❓", "❗",
  "💬", "🔔", "🔒", "🔑", "🛠️", "🐛", "🌴", "🏖️", "🤒", "🏠", "🚗", "✈️", "🎧", "📞", "💻", "🗓️",
];
