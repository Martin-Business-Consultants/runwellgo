// This browser's preferences (a list's hidden columns, rows a page,
// density), kept in localStorage.
const read = (key, fallback) => {
  try { const v = localStorage.getItem("runwell:" + key); return v === null ? fallback : JSON.parse(v); } catch { return fallback; }
};

export const prefs = {
  get: read,
  set(key, value) {
    try { localStorage.setItem("runwell:" + key, JSON.stringify(value)); } catch { /* private window */ }
  },
  remove(key) {
    try { localStorage.removeItem("runwell:" + key); } catch { /* private window */ }
  },
};

// A list's view: its hidden columns, rows a page and density.
export function listView(name, defaults = {}) {
  const key = "list:" + name;
  const get = () => ({ hidden: [], per: 25, density: "compact", ...defaults, ...(read(key, {}) || {}) });
  return {
    get hidden() { return get().hidden; },
    get per() { return get().per; },
    get density() { return get().density; },
    update(change) { prefs.set(key, { ...get(), ...change }); },
  };
}
