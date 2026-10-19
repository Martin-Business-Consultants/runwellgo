package core

import (
	"context"
	"errors"
	"maps"
	"slices"

	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// What a plugin granted "tools" reaches (plugin.API.Tools / RunTool): the
// agent tools its caller is offered, run as them through the same path as
// their own agent over MCP (runToolAs: a token for the one call), so roles,
// validation and events are the API's and history reads "Ted's agent via
// <plugin>". The plugin package keeps a change to the caller's own
// request; here the call is only ever as that caller. And the install's
// key, for a plugin granted "secrets".

// Tools are the tools actor is offered, as MCP lists them: a person's (or
// their agent's) by role, a portal contact's portal ones. api_request,
// the escape hatch for routes no tool names, isn't one.
func (e pluginEnv) Tools(_ context.Context, actor plugin.Actor) ([]plugin.ToolInfo, error) {
	who, _ := actor.(*Actor)
	if who == nil || (who.User == nil && who.Contact == nil) {
		return nil, errors.New("tools run as someone: there's no one calling")
	}
	out := []plugin.ToolInfo{}
	for _, t := range e.a.toolsFor(who) {
		if t == apiRequestTool {
			continue
		}
		schema := t.inputSchema()
		// The plugin's caller approved the call itself; these steer an agent's.
		if props, ok := schema["properties"].(map[string]any); ok {
			props = maps.Clone(props)
			for _, k := range controlArgs {
				delete(props, k)
			}
			schema["properties"] = props
		}
		out = append(out, plugin.ToolInfo{Name: t.Name, Title: t.Summary, Description: t.description(), InputSchema: schema, ReadOnly: t.read(), Confirm: t.Confirm})
	}
	return out, nil
}

// RunTool runs one of actor's tools as them, through via (the plugin's
// name, as history shows it). A change that reaches a client is
// confirmed: the person approved it before the plugin could ask.
func (e pluginEnv) RunTool(ctx context.Context, actor plugin.Actor, via, name string, args map[string]any) (map[string]any, error) {
	who, _ := actor.(*Actor)
	var userID, contactID int64
	switch {
	case who != nil && who.Contact != nil:
		contactID = who.Contact.ID
	case who != nil && who.User != nil && who.User.Active():
		userID = who.User.Person().ID
	default:
		return nil, errors.New("tools run as someone: there's no one calling")
	}
	i := slices.IndexFunc(e.a.toolsFor(who), func(t *toolSpec) bool { return t.Name == name && t != apiRequestTool })
	if i < 0 {
		return nil, errors.New("there's no tool " + name + " for this person")
	}
	t := e.a.toolsFor(who)[i]
	call := map[string]any{}
	for k, v := range args {
		if !slices.Contains(controlArgs, k) {
			call[k] = v
		}
	}
	if t.Confirm != "" && !t.read() {
		call["confirm"] = true
	}
	return e.a.runToolAs(ctx, userID, contactID, name, call, via)
}

// Seal seals plain with the install's key.
func (e pluginEnv) Seal(plain string) string { return e.a.secrets.Seal(plain) }

// Unseal opens a value sealed here (or encrypted by the Rails install).
func (e pluginEnv) Unseal(sealed string) (string, bool) { return e.a.openSecret(sealed) }
