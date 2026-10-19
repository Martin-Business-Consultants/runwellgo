package plugin

import (
	"context"
	"errors"
	"fmt"
	"net/http"
)

// A plugin granted "tools" works on Runwell through the same agent tools a
// person's own AI app uses over MCP, as the person who called its route:
// API.Tools is what their role is offered, API.RunTool runs one as them.
// It can never do more than they could, and what it does is in history as
// theirs, made through the plugin ("Ted's agent via AI"). A tool that
// only reads runs whenever the plugin likes, in the background too; one
// that changes anything runs only inside that person's own request to the
// plugin (not an agent's, not in the background): the plugin holds a
// proposed change and runs it when they approve it. PortalTools and
// RunPortalTool are the same for a client's contact on a portal route,
// with what their portal's agent may do.

// ToolInfo is one agent tool, as MCP offers it.
type ToolInfo struct {
	Name        string         `json:"name"`
	Title       string         `json:"title"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"input_schema"`
	// ReadOnly tools only look; the rest change something.
	ReadOnly bool `json:"read_only"`
	// Confirm is what a change reaches (a client), when it does.
	Confirm string `json:"confirm,omitempty"`
}

// ErrNeedsApproval is a change asked for outside the person's own request.
var ErrNeedsApproval = errors.New("a change runs only when the person approves it, in their own request")

// Tools are the agent tools caller (the route's caller) is offered (tools).
func (a *API) Tools(ctx context.Context, caller *Caller) ([]ToolInfo, error) {
	if err := a.asCaller(caller); err != nil {
		return nil, err
	}
	return a.h.env.Tools(ctx, a.actor)
}

// RunTool runs the tool name with args as caller (tools): at once when it
// only reads, and a change only inside caller's own request (see above).
// It answers the tool's answer, as MCP gives it ({status, summary, …}).
func (a *API) RunTool(ctx context.Context, caller *Caller, name string, args map[string]any) (map[string]any, error) {
	if err := a.asCaller(caller); err != nil {
		return nil, err
	}
	return a.run(ctx, name, args, !caller.Agent)
}

// PortalTools are the tools a client's contact's portal agent is offered
// (tools), on a portal route.
func (a *API) PortalTools(ctx context.Context, contact *Contact) ([]ToolInfo, error) {
	if err := a.asContact(contact); err != nil {
		return nil, err
	}
	return a.h.env.Tools(ctx, a.actor)
}

// RunPortalTool runs a portal tool as the contact (tools), a change only
// inside their own request.
func (a *API) RunPortalTool(ctx context.Context, contact *Contact, name string, args map[string]any) (map[string]any, error) {
	if err := a.asContact(contact); err != nil {
		return nil, err
	}
	return a.run(ctx, name, args, !contact.Agent)
}

func (a *API) asCaller(caller *Caller) error {
	if err := a.need("tools", a.grants.Tools); err != nil {
		return err
	}
	if caller == nil || a.caller == nil || a.actor == nil || caller.ID != a.caller.ID || caller.Agent != a.caller.Agent {
		return errors.New("tools run only as the person who called the route")
	}
	return nil
}

func (a *API) asContact(contact *Contact) error {
	if err := a.need("tools", a.grants.Tools); err != nil {
		return err
	}
	if contact == nil || a.contact == nil || a.actor == nil || contact.ID != a.contact.ID || contact.Agent != a.contact.Agent {
		return errors.New("portal tools run only as the contact who called the route")
	}
	return nil
}

// run finds the tool among those offered and runs it; a change only in the
// caller's own live, non-GET request, from a person rather than an agent.
func (a *API) run(ctx context.Context, name string, args map[string]any, person bool) (map[string]any, error) {
	tools, err := a.h.env.Tools(ctx, a.actor)
	if err != nil {
		return nil, err
	}
	var t *ToolInfo
	for i := range tools {
		if tools[i].Name == name {
			t = &tools[i]
		}
	}
	if t == nil {
		return nil, fmt.Errorf("there's no tool %s for this person", name)
	}
	if !t.ReadOnly && (!person || a.live == nil || !a.live.Load() || a.method == http.MethodGet || a.method == "") {
		return nil, ErrNeedsApproval
	}
	via := a.key
	if p := a.h.Get(a.key); p != nil {
		via = p.Name
	}
	return a.h.env.RunTool(ctx, a.actor, via, name, args)
}
