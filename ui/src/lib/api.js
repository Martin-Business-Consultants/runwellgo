// The app talks to the same /api as the CLI and agents, signed in by the
// session cookie, with the session's CSRF token on every write.
import { session } from "./session.svelte.js";

export class ApiError extends Error {
  constructor(status, body) {
    super((body && (body.message || body.summary || body.error)) || `HTTP ${status}`);
    this.status = status;
    this.body = body || {};
  }
  // Field errors, {field: ["message", …]}.
  get errors() { return this.body.errors || {}; }
}

// Reads in flight, so a view transition can wait for a screen's data
// before it shows the screen (router.svelte.js).
let reading = 0;
const settledWaiters = new Set();
export function readsSettled() {
  if (!reading) return Promise.resolve();
  return new Promise((resolve) => settledWaiters.add(resolve));
}

export async function api(method, path, body, opts = {}) {
  if (method !== "GET") return request(method, path, body, opts);
  reading++;
  try { return await request(method, path, body, opts); } finally {
    if (--reading === 0) { for (const r of settledWaiters) r(); settledWaiters.clear(); }
  }
}

async function request(method, path, body, { quiet = false } = {}) {
  const headers = { Accept: "application/json" };
  let payload = body;
  if (body !== undefined && !(body instanceof FormData) && !(body instanceof Blob)) {
    headers["Content-Type"] = "application/json";
    payload = JSON.stringify(body);
  }
  if (method !== "GET") headers["X-CSRF-Token"] = session.csrf;
  const res = await fetch("/api" + path, { method, headers, body: payload, credentials: "same-origin" });
  if (res.status === 204) return null;
  const text = await res.text();
  let data = null;
  try { data = text ? JSON.parse(text) : null; } catch { data = { message: text }; }
  if (res.status === 401 && !quiet) session.signedOut();
  if (!res.ok) throw new ApiError(res.status, data);
  return data || {};
}

export const get = (path, opts) => api("GET", path, undefined, opts);
export const post = (path, body, opts) => api("POST", path, body, opts);
export const patch = (path, body, opts) => api("PATCH", path, body, opts);
export const put = (path, body, opts) => api("PUT", path, body, opts);
export const del = (path, opts) => api("DELETE", path, undefined, opts);
