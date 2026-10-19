// What the header shows for the screen on view: its breadcrumbs (up a
// level, then where you are) and the actions beside them. A screen sets it
// once it knows: page.set("WO-12 Website", [{label: "Engagements", href: "/engagements"}], "Engagement:4").
class Page {
  title = $state("");
  crumbs = $state([]);
  // The record on screen, "Client:12", for the header's plugin quick actions.
  record = $state("");

  set(title, crumbs = [], record = "") {
    this.title = title;
    this.crumbs = crumbs;
    this.record = record;
    document.title = title ? `${title} · Runwell` : "Runwell";
  }
}

export const page = new Page();
