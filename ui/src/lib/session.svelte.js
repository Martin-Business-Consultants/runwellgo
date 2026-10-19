// Who's signed in on staff: their role, what it lets them do, the team, and
// the install's words for its things (Settings › Names).
class Session {
  me = $state(null);
  csrf = $state("");
  brand = $state("Runwell");
  version = $state("");
  needsSetup = $state(false);
  ready = $state(false);

  // can("send_agreements"): the person's role holds the permission.
  can(permission) {
    return !!this.me && this.me.permissions.includes(permission);
  }

  // term("client") → "Client", term("client", 2) → "Clients": this
  // install's word for a core thing.
  term(key, count = 1) {
    const t = this.me?.terms?.[key];
    const fallback = { client: ["Client", "Clients"], engagement: ["Engagement", "Engagements"], scope_item: ["Scope item", "Scope items"], work: ["Work", "Work"], commitment: ["Commitment", "Commitments"] }[key] || [key, key];
    return (count === 1 ? t?.one : t?.other) || fallback[count === 1 ? 0 : 1];
  }

  // label("work_order") → "Work order": an engagement label's word.
  label(key, count = 1) {
    const l = this.me?.terms?.labels?.[key];
    return (count === 1 ? l?.one : l?.other) || key;
  }

  async load() {
    const { get } = await import("./api.js");
    try {
      this.apply(await get("/session", { quiet: true }));
      if (this.me) await this.signedIn;
    } catch {
      this.me = null;
    }
    this.ready = true;
  }

  // What has to happen once someone is signed in, before the first screen
  // draws (plugins register their tabs and settings pages): areas add an
  // async hook(me) to sessionHooks.
  signedIn = Promise.resolve();

  // apply takes a /api/session answer (also what signing in answers).
  apply(s) {
    this.brand = s.brand || "Runwell";
    this.version = s.version || "";
    this.needsSetup = !!s.needs_setup;
    this.me = s.me || null;
    this.csrf = s.csrf || "";
    if (this.me) this.signedIn = Promise.all(sessionHooks.map((h) => h(this.me).catch(() => {})));
  }

  signedOut() {
    this.me = null;
    this.csrf = "";
  }
}

export const sessionHooks = [];
export const session = new Session();
