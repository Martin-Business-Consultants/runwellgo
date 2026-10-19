// Package ai is Runwell AI, ported from the Rails plugin
// (github.com/Martin-Business-Consultants/runwell-ai) onto the Go plugin
// API alone: an assistant on every page (Ask, key i) that answers from the
// person's records and does what they ask, as them, through the same agent
// tools as MCP (the "tools" grant), every change shown to them to approve
// first; one-click suggestions on records; an assistant in clients' portals
// when the owner lets them ask (the "portal" grant); and its settings: the
// provider and key (sealed, the "secrets" grant), models, a monthly budget,
// a connection test and the clients kept out of it. It keeps the Rails
// plugin's tables (ai_*), so a Rails install's chats, suggestions and
// settings carry over, and draws its own screens (ui/src/plugins/ai).
//
// The provider is called over raw HTTP (provider.go): Anthropic's Messages
// API, or Chat Completions for OpenAI, Gemini, OpenRouter, OpenCode Zen
// and Ollama. The cloud providers are declared hosts; an owner's own server
// (Ollama, a compatible address) needs the "http:private" grant, which an
// owner ticks themselves.
//
// Agents (agents.go, runs.go) are named assistants an owner defines, which
// people @mention in a note or in chat (the "mentionables" grant): each
// mention is a run in the background that reads as the person who asked,
// proposes changes for them to approve, and replies where it was asked.
package ai

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Key is the plugin's key.
const Key = "ai"

// tokenName is what history says a change came through: "Ted's agent via AI".
const name = "AI"

// Module is the plugin, for the core to register.
func Module() *plugin.Module {
	settings := "manage_settings"
	return &plugin.Module{
		Manifest: plugin.Manifest{
			Key:      Key,
			Name:     name,
			Version:  "1.4.0",
			Author:   "Runwell",
			Homepage: "https://github.com/Martin-Business-Consultants/runwell-ai",
			Description: "An assistant inside Runwell (Ask, on any page) and one-click suggestions on records, through your own AI provider's key, " +
				"with a monthly budget. Works through the same tools as MCP, as the person, and changes nothing until they approve it.",
			Permissions: plugin.Grants{RecordsRead: true, RecordsWrite: true, Tables: true, Files: true, Secrets: true, Tools: true, Portal: true,
				PrivateHTTP: true, Live: true, Notify: true, Mentionables: true, HTTP: hosts},
			Hooks:  []string{"mention.created", "*.approval.requested", "*.approval.withdrawn", "user.removed"},
			Nav:    &plugin.NavPage{Label: "Approvals", Path: "/ai/approvals", Icon: "check-check", Badge: "/approvals/badge"},
			Tasks:  []plugin.TaskSpec{{Name: "sweep", Every: "1m"}},
			Schema: schema,
			Routes: []plugin.Route{
				{Method: "GET", Path: "/status", Export: "status"},
				{Method: "GET", Path: "/chats", Export: "chats"},
				{Method: "GET", Path: "/chats/current", Export: "currentChat"},
				{Method: "POST", Path: "/chats", Export: "newChat"},
				{Method: "GET", Path: "/chats/{id}", Export: "showChat"},
				{Method: "POST", Path: "/chats/{id}/messages", Export: "askChat", MaxBody: 64 << 10},
				{Method: "POST", Path: "/chats/{id}/decisions", Export: "decideChat"},

				{Method: "POST", Path: "/suggestions", Export: "newSuggestion"},
				{Method: "GET", Path: "/suggestions/latest", Export: "latestSuggestion"},
				{Method: "GET", Path: "/suggestions/{id}", Export: "showSuggestion"},
				{Method: "PATCH", Path: "/suggestions/{id}", Export: "useSuggestion"},

				{Method: "POST", Path: "/clients/{client_id}/exclusion", Export: "exclude"},
				{Method: "DELETE", Path: "/clients/{client_id}/exclusion", Export: "include"},

				{Method: "GET", Path: "/settings", Export: "showSettings", Capability: settings, Tool: &plugin.Tool{
					Name: "show_ai_settings", Title: "Show AI settings and usage",
					Description: "Whether the in-app AI is on, its provider and models (never the key), the monthly budget and this month's spend.",
				}},
				{Method: "PATCH", Path: "/settings", Export: "updateSettings", Capability: settings},
				{Method: "POST", Path: "/settings/test", Export: "testSettings", Capability: settings},
				{Method: "POST", Path: "/settings/models", Export: "refreshModels", Capability: settings},

				{Method: "GET", Path: "/agents", Export: "listAgents", Tool: &plugin.Tool{
					Name: "list_ai_agents", Title: "List the AI agents",
					Description: "The agents you can @mention (or ask with run_ai_agent): each one's id, name, @handle and what it's for. Owners also see how each is set up.\n\nOften next: run_ai_agent.",
				}},
				{Method: "POST", Path: "/agents", Export: "createAgent", Capability: settings},
				{Method: "GET", Path: "/agents/{id}", Export: "showAgent"},
				{Method: "PATCH", Path: "/agents/{id}", Export: "updateAgent", Capability: settings},
				{Method: "DELETE", Path: "/agents/{id}", Export: "deleteAgent", Capability: settings},
				{Method: "POST", Path: "/agents/{id}/avatar", Export: "uploadAgentAvatar", Capability: settings, MaxBody: 5 << 20},
				{Method: "DELETE", Path: "/agents/{id}/avatar", Export: "removeAgentAvatar", Capability: settings},
				{Method: "GET", Path: "/agents/{id}/avatar", Export: "serveAgentAvatar"},
				{Method: "POST", Path: "/agents/{id}/runs", Export: "startRun", Tool: &plugin.Tool{
					Name: "run_ai_agent", Title: "Ask an AI agent to work on something",
					Description: "Starts a run of the agent (id from list_ai_agents) on a prompt, optionally about a record. It works in the background as you, reading through your tools; any change it proposes waits for you to approve it. Read its answer with show_ai_run.",
					Params: []plugin.ToolParam{
						{Name: "prompt", Description: "What to do.", Required: true},
						{Name: "record", Description: `The record it's about, "Type:id" (Client:12, Todo:40, Engagement:WO-3).`},
					},
				}},
				{Method: "GET", Path: "/runs", Export: "listRuns"},
				{Method: "GET", Path: "/runs/{id}", Export: "showRun", Tool: &plugin.Tool{
					Name: "show_ai_run", Title: "Show an AI agent's run",
					Description: "A run's status (queued, working, waiting for approval, done, failed, cancelled), what it was asked, its transcript (its answers and the tools it used) and what it cost.",
				}},
				{Method: "POST", Path: "/runs/{id}/cancel", Export: "cancelRun"},
				{Method: "POST", Path: "/runs/{id}/decisions", Export: "decideRun"},

				{Method: "GET", Path: "/models", Export: "listModels"},
				// ---- Ask (the chat UI's routes go here) ----
				{Method: "GET", Path: "/chats/history", Export: "chatHistory"},
				{Method: "PATCH", Path: "/chats/{id}", Export: "updateChat"},
				{Method: "DELETE", Path: "/chats/{id}", Export: "deleteChat"},
				{Method: "POST", Path: "/chats/{id}/send", Export: "sendChat", MaxBody: 10 << 20},
				{Method: "POST", Path: "/chats/{id}/stop", Export: "stopChat"},
				{Method: "POST", Path: "/chats/{id}/regenerate", Export: "regenerateChat"},
				{Method: "POST", Path: "/chats/{id}/edit", Export: "editChat", MaxBody: 64 << 10},
				{Method: "POST", Path: "/chats/{id}/agent_card", Export: "agentCard"},
				{Method: "POST", Path: "/chats/{id}/messages/{message_id}/feedback", Export: "rateMessage"},
				{Method: "GET", Path: "/chats/{id}/messages/{message_id}/images/{n}", Export: "messageImage"},

				{Method: "GET", Path: "/approvals", Export: "listApprovals"},
				{Method: "GET", Path: "/approvals/badge", Export: "approvalsBadge"},
				{Method: "POST", Path: "/approvals/{id}/decision", Export: "decideApproval"},

				{Method: "GET", Path: "/portal/status", Export: "portalStatus", Portal: true},
				{Method: "GET", Path: "/portal/chats/current", Export: "portalCurrent", Portal: true},
				{Method: "POST", Path: "/portal/chats", Export: "portalNew", Portal: true},
				{Method: "GET", Path: "/portal/chats/{id}", Export: "portalShow", Portal: true},
				{Method: "POST", Path: "/portal/chats/{id}/messages", Export: "portalAsk", Portal: true, MaxBody: 64 << 10},
				{Method: "POST", Path: "/portal/chats/{id}/decisions", Export: "portalDecide", Portal: true},
			},
		},
		Handlers: map[string]plugin.Handler{
			"status": status, "chats": listChats, "currentChat": currentChat, "newChat": newChat, "showChat": showChat,
			"askChat": askChat, "decideChat": decideChat,
			"newSuggestion": newSuggestion, "latestSuggestion": latestSuggestion, "showSuggestion": showSuggestion, "useSuggestion": useSuggestion,
			"exclude": exclude, "include": include,
			"showSettings": showSettings, "updateSettings": updateSettings, "testSettings": testSettings, "refreshModels": refreshModels,
			"portalStatus": portalStatus, "portalCurrent": portalCurrent, "portalNew": portalNew, "portalShow": portalShow,
			"portalAsk": portalAsk, "portalDecide": portalDecide,
			"listAgents": listAgents, "createAgent": createAgent, "showAgent": showAgent, "updateAgent": updateAgent, "deleteAgent": deleteAgent,
			"uploadAgentAvatar": uploadAgentAvatar, "removeAgentAvatar": removeAgentAvatar, "serveAgentAvatar": serveAgentAvatar,
			"listModels":  listModels,
			"chatHistory": chatHistory, "updateChat": updateChat, "deleteChat": deleteChat, "sendChat": sendChat, "stopChat": stopChat,
			"regenerateChat": regenerateChat, "editChat": editChat, "agentCard": agentCard, "rateMessage": rateMessage, "messageImage": messageImage,
			"listApprovals": listApprovals, "approvalsBadge": approvalsBadge, "decideApproval": decideApproval,
			"startRun": startRun, "listRuns": listRuns, "showRun": showRun, "cancelRun": cancelRun, "decideRun": decideRun,
		},
		OnEvent:      onEvent,
		Tasks:        map[string]plugin.Task{"sweep": sweep},
		Mentionables: mentionables,
	}
}

// ---- answers -------------------------------------------------------------------------------

func refuse(status int, message string) *plugin.Response {
	code := map[int]string{http.StatusNotFound: "not_found", http.StatusForbidden: "forbidden", http.StatusConflict: "conflict"}[status]
	if code == "" {
		code = "invalid"
	}
	return plugin.JSON(status, map[string]any{"error": code, "message": message})
}

func missing(what string) *plugin.Response {
	return refuse(http.StatusNotFound, "Couldn't find that "+what+".")
}

// failed answers an error the caller should see, or the plugin failing.
func failed(err error) (*plugin.Response, error) {
	if u, ok := errors.AsType[*unavailable](err); ok {
		return refuse(http.StatusUnprocessableEntity, u.msg), nil
	}
	if res := plugin.Answer(err); res != nil {
		return res, nil
	}
	return nil, err
}

// unavailable is a request AI won't take now, in Rails' words.
type unavailable struct{ msg string }

func (u *unavailable) Error() string { return u.msg }

func notNow(format string, args ...any) error { return &unavailable{fmt.Sprintf(format, args...)} }

// person is the caller when they're a person in the app: the Ask panel and
// suggestions are theirs, and agents reach Runwell over MCP instead.
func person(req *plugin.Request) (*plugin.Caller, *plugin.Response) {
	if req.Actor == nil {
		return nil, refuse(http.StatusUnauthorized, "Sign in first.")
	}
	if req.Actor.Agent {
		return nil, refuse(http.StatusForbidden, "The in-app AI is for people in the app; agents use the tools directly over MCP.")
	}
	return req.Actor, nil
}

// ---- requests ------------------------------------------------------------------------------

func pathID(req *plugin.Request, name string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(req.Params[name]), 10, 64)
	return n
}

func params(req *plugin.Request) map[string]any {
	m := map[string]any{}
	if len(bytes.TrimSpace(req.Body)) > 0 {
		dec := json.NewDecoder(bytes.NewReader(req.Body))
		dec.UseNumber()
		_ = dec.Decode(&m)
	}
	return m
}

func wrapped(m map[string]any, key string) map[string]any {
	if w, ok := m[key].(map[string]any); ok {
		return w
	}
	return map[string]any{}
}

func str(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	}
	return fmt.Sprint(v)
}

func intOf(v any) int64 {
	switch n := v.(type) {
	case json.Number:
		i, err := n.Int64()
		if err != nil {
			f, _ := n.Float64()
			return int64(f)
		}
		return i
	case float64:
		return int64(n)
	case int64:
		return n
	case int:
		return int64(n)
	case string:
		i, _ := strconv.ParseInt(strings.TrimSpace(n), 10, 64)
		return i
	}
	return 0
}

// eachRow calls fn for every row, closes rows, and returns the first of
// fn's errors or the iteration's.
func eachRow(rows *sql.Rows, fn func(*sql.Rows) error) error {
	defer rows.Close()
	for rows.Next() {
		if err := fn(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func truthy(v any) bool {
	switch b := v.(type) {
	case bool:
		return b
	case string:
		return b == "1" || b == "true" || b == "on"
	case json.Number:
		return b.String() == "1"
	}
	return false
}

// ctxErr says the plugin's call ran out of time, in words.
func ctxErr(ctx context.Context, err error) error {
	if ctx.Err() != nil {
		return &providerError{Message: "It took too long to answer; try again, or ask for less."}
	}
	return err
}
