// Runwell AI (the ai plugin's own screens): Ask on every page (the header
// slot, key i), suggestions where the Rails plugin's slots put them, the
// portal's Ask, and Settings › AI. Shown only while the plugin is on.
import { addSlot } from "$lib/plugin-slots.js";
import { addSettingsPage } from "$lib/settings-nav.js";
import { isOn } from "../../routes/plugins/extensions.svelte.js";
import AskButton from "./AskButton.svelte";
import ClientAside from "./ClientAside.svelte";
import RequestAside from "./RequestAside.svelte";
import ScopeItemAside from "./ScopeItemAside.svelte";
import DraftSuggestions from "./DraftSuggestions.svelte";
import CommitmentRow from "./CommitmentRow.svelte";
import NoteRow from "./NoteRow.svelte";
import DayCard from "./DayCard.svelte";
import PortalAsk from "./PortalAsk.svelte";
import AiSettings from "./AiSettings.svelte";
import AskPage from "./AskPage.svelte";
import AgentsSettings from "./AgentsSettings.svelte";
import RunsPage from "./RunsPage.svelte";
import RunPage from "./RunPage.svelte";
import ApprovalsPage from "./ApprovalsPage.svelte";

export const key = "ai";
export const routes = [
  ["/settings/ai", AiSettings],
  ["/settings/ai/agents", AgentsSettings],
  ["/settings/ai/runs", RunsPage],
  ["/ai", AskPage],
  ["/ai/runs/:id", RunPage],
  ["/ai/approvals", ApprovalsPage],
];

export function setup() {
  addSlot("header", { key, component: AskButton });
  // DayCard (Plan my day, on Home) is set aside for now.
  addSlot("client_aside", { key, component: ClientAside });
  addSlot("request_aside", { key, component: RequestAside });
  addSlot("scope_item_aside", { key, component: ScopeItemAside });
  addSlot("draft_version", { key, component: DraftSuggestions });
  addSlot("commitment_row", { key, component: CommitmentRow });
  addSlot("note_row", { key, component: NoteRow });
  addSlot("portal_header", { key, component: PortalAsk });
  addSettingsPage({ href: "/settings/ai", label: "AI", group: "Install", perm: "manage_settings", order: 44, when: () => isOn(key) });
  addSettingsPage({ href: "/settings/ai/agents", label: "AI agents", group: "Install", perm: "manage_settings", order: 45, when: () => isOn(key) });
  addSettingsPage({ href: "/settings/ai/runs", label: "Agent runs", group: "Install", order: 46, when: () => isOn(key) });
}
