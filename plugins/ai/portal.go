package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// The portal assistant (Settings › AI › Let clients ask): a client's
// contact's own chat in their portal, answered through their portal's own
// tools (portal_action) as them, so it sees exactly what their portal
// shows. Reading runs at once; sending a request for the team waits for the
// contact to approve it; deciding on an agreement is never offered.

// portalAllowed are the portal tools the assistant may use.
var portalAllowed = []string{"portal_me", "portal_engagements", "portal_engagement", "portal_work", "portal_requests", "portal_send_request"}

// portalAvailable is why the contact can't ask now ("" when they can):
// the owner lets clients ask, AI is on and within budget, and their client
// isn't kept out.
func portalAvailable(ctx context.Context, api *plugin.API, contact *plugin.Contact) (string, error) {
	s, err := loadSetting(ctx, api)
	if err != nil {
		return "", err
	}
	if !s.Portal {
		return "Not available here.", nil
	}
	if reason, err := available(ctx, api, s); err != nil || reason != "" {
		return "Not available right now.", err
	}
	ex, err := excludedClients(ctx, api)
	if err != nil {
		return "", err
	}
	if ex[contact.ClientID] {
		return "Not available here.", nil
	}
	return "", nil
}

func portalStatus(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	reason, err := portalAvailable(ctx, api, req.Contact)
	if err != nil {
		return nil, err
	}
	return plugin.JSON(200, map[string]any{"available": reason == ""}), nil
}

// portalGate answers 404 while the contact can't ask (require_portal_ai).
func portalGate(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	reason, err := portalAvailable(ctx, api, req.Contact)
	if err != nil {
		return nil, err
	}
	if reason != "" {
		return refuse(http.StatusNotFound, reason), nil
	}
	return nil, nil
}

func portalCurrent(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if res, err := portalGate(ctx, api, req); res != nil || err != nil {
		return res, err
	}
	since := store.TimeOf(time.Now().Add(-12 * time.Hour)).DB()
	c, err := scanChat(api.QueryRow(ctx, `SELECT `+chatColumns+` FROM ai_chats WHERE contact_id = ? AND purpose = 'ask' AND updated_at >= ? ORDER BY updated_at DESC LIMIT 1`, req.Contact.ID, since))
	if errors.Is(err, sql.ErrNoRows) {
		id, err := insertChat(ctx, api, 0, req.Contact.ID, "", 0, "ask", "")
		if err != nil {
			return nil, err
		}
		c, err = loadChat(ctx, api, id)
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}
	return portalAnswer(ctx, api, c, req.Contact)
}

func portalNew(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if res, err := portalGate(ctx, api, req); res != nil || err != nil {
		return res, err
	}
	id, err := insertChat(ctx, api, 0, req.Contact.ID, "", 0, "ask", "")
	if err != nil {
		return nil, err
	}
	c, err := loadChat(ctx, api, id)
	if err != nil {
		return nil, err
	}
	return portalAnswer(ctx, api, c, req.Contact)
}

func portalChat(ctx context.Context, api *plugin.API, req *plugin.Request) (*chat, *plugin.Response, error) {
	if res, err := portalGate(ctx, api, req); res != nil || err != nil {
		return nil, res, err
	}
	c, err := loadChat(ctx, api, pathID(req, "id"))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && c.ContactID != req.Contact.ID) {
		return nil, missing("chat"), nil
	}
	return c, nil, err
}

func portalShow(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := portalChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	return portalAnswer(ctx, api, c, req.Contact)
}

func portalAsk(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := portalChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	contact := req.Contact
	text := strings.TrimSpace(str(wrapped(params(req), "message")["content"]))
	if err := askSoon(ctx, api, c, text, func(ctx context.Context, api *plugin.API) { reply(ctx, api, c.ID, nil, contact) }); err != nil {
		return failed(err)
	}
	if c, err = loadChat(ctx, api, c.ID); err != nil {
		return nil, err
	}
	return portalAnswer(ctx, api, c, contact)
}

func portalDecide(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	c, res, err := portalChat(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	contact := req.Contact
	run := func(ctx context.Context, name string, args map[string]any) (map[string]any, error) {
		if !contains(portalAllowed, name) {
			return nil, errors.New("Not available here.")
		}
		return api.RunPortalTool(ctx, contact, name, args)
	}
	again := func(ctx context.Context, api *plugin.API) { reply(ctx, api, c.ID, nil, contact) }
	if res, err := decide(ctx, api, c, params(req), run, again); res != nil || err != nil {
		return res, err
	}
	if c, err = loadChat(ctx, api, c.ID); err != nil {
		return nil, err
	}
	return portalAnswer(ctx, api, c, contact)
}

func portalAnswer(ctx context.Context, api *plugin.API, c *chat, contact *plugin.Contact) (*plugin.Response, error) {
	tools, _ := api.PortalTools(ctx, contact)
	return panelAnswer(ctx, api, c, "", tools)
}

// ---- the portal's one tool ----------------------------------------------------------------

type portalRunner struct {
	api     *plugin.API
	contact *plugin.Contact
}

// system is Ai::Instructions.for_portal; today goes with each question.
func (r *portalRunner) system(ctx context.Context) string {
	site, _ := r.api.Site(ctx)
	return fmt.Sprintf(`You are the assistant in %s's portal with %s. You help %s, using only what their portal shows them (portal_action).

Each question starts with today's date, in brackets.

- Answer from the portal: their %s, agreements, %s shared with them, and requests. Never guess; if it isn't there, say so.
- You can't promise dates, prices or anything new for %s. When they want something new or changed, offer to send it as a request (portal_send_request) for the team; they approve it before it goes.
- You can't approve or decline an agreement for them; point them to the agreement's page.
- Be brief, warm and plain.`, r.contact.ClientName, site.Name, r.contact.Name, strings.ToLower(term(site, "engagement", true)), strings.ToLower(term(site, "work", false)), site.Name)
}

func (r *portalRunner) tools() []toolDef {
	enum := make([]any, len(portalAllowed))
	for i, n := range portalAllowed {
		enum[i] = n
	}
	return []toolDef{{Name: "portal_action",
		Description: "Use the client portal: " + strings.Join(portalAllowed, ", ") + ". portal_engagement takes { ref }. portal_send_request takes { request: { subject, body } } and waits for the client to approve it.",
		Schema: object(map[string]any{
			"name":      map[string]any{"type": "string", "enum": enum},
			"arguments": map[string]any{"type": "object", "additionalProperties": true},
		}, "name")}}
}

func (r *portalRunner) call(ctx context.Context, name string, input json.RawMessage) (string, bool, bool) {
	var in struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if name != "portal_action" || json.Unmarshal(input, &in) != nil || !contains(portalAllowed, in.Name) {
		return trim(map[string]any{"error": "Not available here."}), true, false
	}
	tools, err := r.api.PortalTools(ctx, r.contact)
	if err != nil {
		return trim(map[string]any{"error": err.Error()}), true, false
	}
	for _, t := range tools {
		if t.Name != in.Name {
			continue
		}
		if !t.ReadOnly {
			return "", false, true // the contact approves it first
		}
		args := in.Arguments
		if args == nil {
			args = map[string]any{}
		}
		out, err := r.api.RunPortalTool(ctx, r.contact, t.Name, args)
		if err != nil {
			return trim(map[string]any{"error": err.Error()}), true, false
		}
		return trim(out), out["status"] == "error", false
	}
	return trim(map[string]any{"error": "Not available here."}), true, false
}
