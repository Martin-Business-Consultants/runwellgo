package core

import "net/http"

// Settings › Help, open to all staff: the overview, connecting an AI
// assistant (MCP, the CLI) and using Runwell through one. The pages are
// the app's; this is what they say that only the server knows.

func init() {
	routeSets = append(routeSets, func(a *App, mux *http.ServeMux) {
		a.handle(mux, "GET /api/settings/help", a.showHelp)
	})
}

func (a *App) showHelp(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	agent := "your agent"
	var name string
	if err := a.store.QueryRowContext(ctx, `SELECT name FROM users WHERE agent_owner_id = ?`, c.Actor.User.Person().ID).Scan(&name); err == nil && name != "" {
		agent = name
	}
	base := a.BaseURL()
	return c.JSON(200, O("version", a.cfg.Version, "agent_name", agent, "mcp_url", base+"/mcp", "portal_mcp_url", base+"/portal/mcp",
		"cli_install_url", base+"/install/cli", "rate_limit", 120, "client_agent_approvals", s.ClientAgentApprovals))
}
