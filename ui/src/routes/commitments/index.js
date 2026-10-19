// Commitments: promises with a date, ours or the client's. The client and
// engagement pages get a Commitments tab from here.
import CommitmentsIndex from "./CommitmentsIndex.svelte";
import CommitmentForm from "./CommitmentForm.svelte";
import RecordCommitments from "./RecordCommitments.svelte";
import { addTab } from "$lib/record-tabs.js";
import { session } from "$lib/session.svelte.js";

addTab("Engagement", {
  key: "commitments",
  label: () => session.term("commitment", 2),
  count: (record) => record.commitments?.filter((c) => c.open).length,
  component: RecordCommitments,
  order: 20,
});
addTab("Client", {
  key: "commitments",
  label: () => session.term("commitment", 2),
  count: (record) => record.open_commitments?.length,
  component: RecordCommitments,
  order: 20,
});

export const routes = [
  ["/commitments", CommitmentsIndex],
  ["/commitments/:id/edit", CommitmentForm],
];
export const publicRoutes = [];
