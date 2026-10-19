// The client portal's screens, by address. All are open routes: each checks
// the portal's own session (GET /api/portal/me) and sends a contact who
// isn't signed in to the emailed-link sign-in. The first pattern that
// matches wins.
import PortalEngagements from "./PortalEngagements.svelte";
import PortalEngagement from "./PortalEngagement.svelte";
import PortalWork from "./PortalWork.svelte";
import PortalRequests from "./PortalRequests.svelte";
import PortalRequestNew from "./PortalRequestNew.svelte";
import PortalRequest from "./PortalRequest.svelte";
import PortalConnectedApps from "./PortalConnectedApps.svelte";
import PortalSignIn from "./PortalSignIn.svelte";
import PortalSessionLink from "./PortalSessionLink.svelte";
import PortalOauthAuthorize from "./PortalOauthAuthorize.svelte";
import PortalRedirect from "./PortalRedirect.svelte";

export const routes = [];
export const publicRoutes = [
  ["/portal", PortalEngagements],
  ["/portal/engagements", PortalEngagements],
  ["/portal/engagements/:ref", PortalEngagement],
  ["/portal/work", PortalWork],
  ["/portal/todos", PortalRedirect],
  ["/portal/me", PortalRedirect],
  ["/portal/requests", PortalRequests],
  ["/portal/requests/new", PortalRequestNew],
  ["/portal/requests/:id", PortalRequest],
  ["/portal/connected_apps", PortalConnectedApps],
  ["/portal/session/new", PortalSignIn],
  ["/portal/session/:token", PortalSessionLink],
  ["/portal/oauth/authorization", PortalOauthAuthorize],
];
