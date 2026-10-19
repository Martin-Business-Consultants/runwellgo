// The settings screens, by address: staff screens in routes, ones open signed
// out (an emailed link) in publicRoutes. The first pattern that matches wins.
// Each page also joins Settings' own nav (addSettingsPage).
import { addSettingsPage } from "$lib/settings-nav.js";
import SettingsHome from "./SettingsHome.svelte";
import Account from "./Account.svelte";
import TwoFactorNew from "./TwoFactorNew.svelte";
import People from "./People.svelte";
import Person from "./Person.svelte";
import Address from "./Address.svelte";
import Appearance from "./Appearance.svelte";
import Names from "./Names.svelte";
import Fields from "./Fields.svelte";
import Views from "./Views.svelte";
import Email from "./Email.svelte";
import SignInSettings from "./SignInSettings.svelte";
import Webhooks from "./Webhooks.svelte";
import Updates from "./Updates.svelte";
import Exports from "./Exports.svelte";
import ImportBackup from "./Import.svelte";
import AuditLog from "./AuditLog.svelte";
import Help from "./Help.svelte";
import HelpMcp from "./HelpMcp.svelte";
import HelpAi from "./HelpAi.svelte";
import PasswordNew from "./PasswordNew.svelte";
import PasswordEdit from "./PasswordEdit.svelte";
import SessionLinkNew from "./SessionLinkNew.svelte";
import SessionLinkShow from "./SessionLinkShow.svelte";
import InvitationAccept from "./InvitationAccept.svelte";

const ms = "manage_settings";
addSettingsPage({ href: "/settings/account", label: "Your account", group: "You", order: 10 });
addSettingsPage({ href: "/settings/address", label: "Address", group: "Workspace", perm: ms, order: 10 });
addSettingsPage({ href: "/settings/appearance", label: "Appearance", group: "Workspace", perm: ms, order: 50 });
addSettingsPage({ href: "/settings/names", label: "Names", group: "Workspace", perm: ms, order: 20 });
addSettingsPage({ href: "/settings/fields", label: "Fields", group: "Workspace", perm: ms, order: 30 });
addSettingsPage({ href: "/settings/views", label: "Views", group: "Workspace", perm: ms, order: 40 });
addSettingsPage({ href: "/settings/email", label: "Email", group: "Connections", perm: ms, order: 5 });
addSettingsPage({ href: "/settings/sign_in", label: "Sign-in", group: "Team and access", perm: ms, order: 20 });
addSettingsPage({ href: "/settings/updates", label: "Updates", group: "Data", perm: ms, order: 30 });
addSettingsPage({ href: "/settings/exports", label: "Export", group: "Data", perm: ms, order: 10 });
addSettingsPage({ href: "/settings/import", label: "Import", group: "Data", perm: ms, order: 20 });
addSettingsPage({ href: "/settings/audit_log", label: "Audit log", group: "Team and access", perm: ms, order: 30 });
addSettingsPage({ href: "/settings/people", label: "People", group: "Team and access", perm: "manage_people", order: 10 });
addSettingsPage({ href: "/settings/webhooks", label: "Webhooks", group: "Connections", perm: ms, order: 60 });
addSettingsPage({ href: "/settings/help", label: "Overview", group: "Help", order: 10 });
addSettingsPage({ href: "/settings/help/mcp", label: "Connecting an MCP", group: "Help", order: 20 });
addSettingsPage({ href: "/settings/help/ai", label: "Using with AI", group: "Help", order: 30 });

export const routes = [
  ["/settings", SettingsHome],
  ["/settings/account", Account],
  ["/settings/two_factor/new", TwoFactorNew],
  ["/settings/people", People],
  ["/settings/people/:id", Person],
  ["/settings/address", Address],
  ["/settings/appearance", Appearance],
  ["/settings/names", Names],
  ["/settings/fields", Fields],
  ["/settings/views", Views],
  ["/settings/email", Email],
  ["/settings/sign_in", SignInSettings],
  ["/settings/webhooks", Webhooks],
  ["/settings/updates", Updates],
  ["/settings/exports", Exports],
  ["/settings/import", ImportBackup],
  ["/settings/audit_log", AuditLog],
  ["/settings/help", Help],
  ["/settings/help/mcp", HelpMcp],
  ["/settings/help/ai", HelpAi],
  ["/help", Help],
];
export const publicRoutes = [
  ["/passwords/new", PasswordNew],
  ["/passwords/:token", PasswordEdit],
  ["/session/link", SessionLinkNew],
  ["/session/link/:token", SessionLinkShow],
  ["/invitations/:token", InvitationAccept],
];
