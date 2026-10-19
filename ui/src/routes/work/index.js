// Work: what is happening against what was agreed (todos, at /work). The
// engagement page gets a Work tab from here, which honours ?item=<scope
// item id> (or "other") to show just that item's work.
import WorkIndex from "./WorkIndex.svelte";
import TodoShow from "./TodoShow.svelte";
import TodoForm from "./TodoForm.svelte";
import EngagementWork from "./EngagementWork.svelte";
import { addTab } from "$lib/record-tabs.js";
import { session } from "$lib/session.svelte.js";

addTab("Engagement", {
  key: "work",
  label: () => session.term("work"),
  count: (record) => record.work?.filter((t) => t.status !== "done").length,
  component: EngagementWork,
  order: 10,
});

export const routes = [
  ["/work", WorkIndex],
  ["/work/new", TodoForm],
  ["/work/:id/edit", TodoForm],
  ["/work/:id", TodoShow],
];
export const publicRoutes = [];
