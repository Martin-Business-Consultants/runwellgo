// The portal's own session: the contact signed in by emailed link (GET
// /api/portal/me), its CSRF token for writes, the client's zone for times,
// and the install's words. Portal screens are open routes that check it
// themselves: signed out, they go to the sign-in page and come back after.
import { ApiError } from "$lib/api.js";
import { go, router } from "$lib/router.svelte.js";

const RETURN = "runwell.portal_return_to";

class Portal {
  me = $state(null);
  ready = $state(false);
  csrf = "";

  async load() {
    try {
      const res = await api("GET", "/portal/me", undefined, { quiet: true });
      this.me = res;
      this.csrf = res.csrf || "";
    } catch {
      this.me = null;
    }
    this.ready = true;
    return this.me;
  }

  // require loads the contact once; signed out, it remembers where they were
  // headed and sends them to sign in.
  async require() {
    if (!this.ready) await this.load();
    if (!this.me) {
      try { localStorage.setItem(RETURN, router.path + (router.query.toString() ? "?" + router.query.toString() : "")); } catch {}
      go("/portal/session/new", undefined, { replace: true });
      return null;
    }
    return this.me;
  }

  // returnTo is where to go once signed in: where they were headed (a portal
  // page), else the portal's home.
  returnTo() {
    let to = "";
    try { to = localStorage.getItem(RETURN) || ""; localStorage.removeItem(RETURN); } catch {}
    return to.startsWith("/portal") ? to : "/portal";
  }

  term(key, count = 1) {
    const t = this.me?.terms?.[key];
    const fallback = { engagement: ["Engagement", "Engagements"], work: ["Work", "Work"], client: ["Client", "Clients"] }[key] || [key, key];
    return (count === 1 ? t?.one : t?.other) || fallback[count === 1 ? 0 : 1];
  }

  // Times in the client's zone.
  dateTime(iso) {
    if (!iso) return "";
    const d = new Date(iso);
    return isNaN(d) ? iso : d.toLocaleString(undefined, { dateStyle: "long", timeStyle: "short", timeZone: this.me?.time_zone_iana || undefined });
  }

  signedOut() {
    this.me = null;
    this.csrf = "";
  }
}

export const portal = new Portal();

// api is $lib/api.js's, for the portal: its own CSRF token, and a 401 sends
// the contact to sign in instead of the staff sign-in.
export async function api(method, path, body, { quiet = false } = {}) {
  const headers = { Accept: "application/json" };
  let payload = body;
  if (body !== undefined) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  if (method !== "GET") headers["X-CSRF-Token"] = portal.csrf;
  const res = await fetch("/api" + path, { method, headers, body: payload, credentials: "same-origin" });
  if (res.status === 204) return null;
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { message: text }; }
  if (res.status === 401 && !quiet) {
    portal.signedOut();
    portal.ready = true;
    portal.require();
  }
  if (!res.ok) throw new ApiError(res.status, data);
  return data || {};
}

export const pget = (path, opts) => api("GET", path, undefined, opts);
export const ppost = (path, body, opts) => api("POST", path, body, opts);
export const pdel = (path, opts) => api("DELETE", path, undefined, opts);
