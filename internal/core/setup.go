package core

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// The first-run checklist on home: the few things a new install needs
// before Runwell is useful, each checked off by what exists rather than by
// ticking. Shown to the owner until the required steps are done or they
// hide it (settings.setup_dismissed_at).

func init() { routeSets = append(routeSets, (*App).setupRoutes) }

func (a *App) setupRoutes(mux *http.ServeMux) {
	// Hiding a checklist on home: nothing for an agent to do, so no tool.
	a.handle(mux, "DELETE /api/setup", a.hideSetup, need("manage_settings"))
}

// setupStep is one thing to do.
type setupStep struct {
	Key, Title, Text, Path string
	Done, Optional         bool
}

// setupSteps are the checklist, each with whether it's done.
func (a *App) setupSteps(ctx context.Context) ([]setupStep, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	exists := func(q string, args ...any) bool {
		n, _ := store.Count(ctx, a.store, q, args...)
		return n > 0
	}
	clients := strings.ToLower(s.Term("client", 2))
	ingress, _ := a.inboundIngress(ctx)

	firstClient := "/clients/new"
	var cid int64
	if a.store.QueryRowContext(ctx, `SELECT id FROM clients ORDER BY created_at LIMIT 1`).Scan(&cid) == nil {
		firstClient = fmt.Sprintf("/clients/%d", cid)
	}
	firstDraft := "/engagements?state=draft"
	var ref string
	if a.store.QueryRowContext(ctx, `SELECT ref FROM engagements WHERE closed_at IS NULL
		AND id NOT IN (SELECT engagement_id FROM agreement_versions WHERE sent_at IS NOT NULL) ORDER BY created_at LIMIT 1`).Scan(&ref) == nil {
		firstDraft = "/engagements/" + ref
	}
	people, _ := store.Count(ctx, a.store, `SELECT count(*) FROM users WHERE deactivated_at IS NULL AND agent_owner_id IS NULL`)
	sent := s.TestEmail["sent_at"]

	return []setupStep{
		{"address", "Set Runwell’s address", "Where your team and " + clients + " open it. Every link Runwell emails points there.", "/settings/address",
			s.AppHost != "" || a.cfg.BaseURL != "" || os.Getenv("APP_HOST") != "", false},
		{"time_zone", "Choose your time zone", "Times on your pages and in emails show in it.", "/settings/address#time_zone",
			s.TimeZone != "" || os.Getenv("TIME_ZONE") != "", false},
		{"mail", "Make sure email goes out", "Approval links, sign-ins and invitations are emails: give Runwell a mail server (or switch on a mail plugin) and send a test.", "/settings/email",
			sent != nil && sent != "", false},
		{"client", "Add your first " + strings.ToLower(s.Term("client", 1)), "Who you work for.", "/clients/new",
			exists(`SELECT count(*) FROM clients`), false},
		{"approver", "Add a contact who can approve", "Agreements go to them to approve or ask for changes.", firstClient,
			exists(`SELECT count(*) FROM contacts WHERE archived_at IS NULL AND can_approve = 1`), false},
		{"team", "Invite your team", "Everyone else joins by an emailed link.", "/settings/people?tab=invite",
			people > 1 || exists(`SELECT count(*) FROM invitations WHERE accepted_at IS NULL`), true},
		{"names", "Check the words Runwell uses", "Call clients, " + strings.ToLower(s.Term("engagement", 2)) + " and work whatever your business calls them.", "/settings/names",
			len(s.Terminology) > 0, true},
		{"engagement", "Draft your first " + strings.ToLower(s.Term("engagement", 1)), "What you agreed to do, as scope items.", "/engagements/new",
			exists(`SELECT count(*) FROM engagements`), false},
		{"send", "Send an agreement", "The client approves it from a link, and approving creates the work.", firstDraft,
			exists(`SELECT count(*) FROM agreement_versions WHERE sent_at IS NOT NULL`), false},
		{"requests_by_email", "Take requests by email", s.Term("client", 2) + " email an address and each message becomes a request to triage.", "/settings/email#requests_by_email",
			s.RequestsEmail != "" && ingress != "", true},
	}, nil
}

// setupJSON is the checklist for home, or nil when it isn't shown: only to
// someone who manages settings, until it's hidden or the required steps
// are done.
func (a *App) setupJSON(ctx context.Context, c *Ctx) (any, error) {
	if !c.Can("manage_settings") {
		return nil, nil
	}
	s, err := a.settings(ctx)
	if err != nil || s.SetupDismissedAt.Valid {
		return nil, err
	}
	steps, err := a.setupSteps(ctx)
	if err != nil {
		return nil, err
	}
	list := make([]Obj, len(steps))
	done, required := 0, true
	for i, st := range steps {
		list[i] = O("key", st.Key, "title", st.Title, "text", st.Text, "done", st.Done, "optional", st.Optional, "path", st.Path)
		if st.Done {
			done++
		}
		if !st.Optional && !st.Done {
			required = false
		}
	}
	if required {
		return nil, nil
	}
	return O("done_count", done, "steps", list), nil
}

func (a *App) hideSetup(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		_, err := t.Exec(`UPDATE settings SET setup_dismissed_at = ?, updated_at = ? WHERE id = ?`, store.Now().DB(), store.Now().DB(), s.ID)
		return err
	})
	if err != nil {
		return err
	}
	c.Summary = "Setup checklist hidden. How Runwell works is always in Settings › Help."
	return c.JSON(200, O("summary", c.Summary, "path", "/"))
}
