// Package chat is team chat inside Runwell, in place of Slack: direct
// messages, group messages and channels (public to everyone on staff, or
// private by invitation) for the people on staff, with rich text and
// @mentions, files and images, reactions, threads, unread counts, search,
// and each person's own settings (a chat name and picture, a status, the
// hours they're available, what notifies them).
//
// It reaches Runwell only through the plugin API: its own tables (chat_*)
// for conversations, members, messages, reactions and profiles; files on
// its own records (Chat::Message, Chat::Profile); "live" to push new
// messages, edits, reactions, typing and presence to people's open tabs;
// "notify" to put a mention or a direct message in the person's bell (and
// their email when they're away), unless they're outside their hours or
// muted it; records:read for who's on staff; and "mentions:relay" to pass
// a message's @mention of a plugin's name (an AI agent) to that plugin,
// whose reply lands in the message's thread (mentions.go). Its screens are its own
// Svelte (ui/src/plugins/chat). With it off, Runwell is as it was.
package chat

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Key is the plugin's key.
const Key = "chat"

// Module is the plugin, for the core to register.
func Module() *plugin.Module {
	return &plugin.Module{
		Manifest: plugin.Manifest{
			Key:      Key,
			Name:     "Chat",
			Version:  "1.1.0",
			Author:   "Runwell",
			Homepage: "https://github.com/Martin-Business-Consultants/runwellgo",
			Description: "Team chat for everyone on staff: direct messages, groups and channels, with mentions, files, " +
				"reactions, threads and search. New messages arrive as they're sent, and mentions reach people in their bell.",
			Permissions: plugin.Grants{RecordsRead: true, Tables: true, Files: true, Live: true, Notify: true, MentionsRelay: true},
			Schema:      schema,
			// Someone removed from Runwell: their channels go to their successor.
			Hooks:  []string{"user.removed"},
			Routes: routes(),
			Nav:    &plugin.NavPage{Label: "Chat", Path: "/chat", Icon: "message-square", Badge: "/badge"},
		},
		Handlers: map[string]plugin.Handler{
			"bootstrap": bootstrap, "badge": badge, "presence": presence,
			"listConversations": listConversations, "createConversation": createConversation, "showConversation": showConversation,
			"updateConversation": updateConversation, "addMembers": addMembers, "removeMember": removeMember,
			"markRead": markRead, "updateMembership": updateMembership, "typing": typing,
			"listMessages": listMessages, "sendMessage": sendMessage, "showThread": showThread,
			"editMessage": editMessage, "deleteMessage": deleteMessage, "react": react,
			"file": serveFile, "search": search,
			"showSettings": showSettings, "updateSettings": updateSettings, "uploadAvatar": uploadAvatar, "removeAvatar": removeAvatar,
			"serveAvatar": serveAvatar,
		},
		OnMentionReply: mentionReply,
		OnEvent:        onEvent,
	}
}

func routes() []plugin.Route {
	conversation := `{kind (required): "channel", "dm" (one other person) or "group" (two to eight others); name: a channel's name, lowercase words joined by -; topic; description; private: true for a channel only its members see; member_ids: [user ids from list_chat_conversations' people]}`
	return []plugin.Route{
		{Method: "GET", Path: "/bootstrap", Export: "bootstrap"},
		{Method: "GET", Path: "/badge", Export: "badge"},
		{Method: "POST", Path: "/presence", Export: "presence"},

		{Method: "GET", Path: "/conversations", Export: "listConversations", Tool: &plugin.Tool{
			Name: "list_chat_conversations", Title: "List your chat conversations",
			Description: "Your channels, direct messages and groups, with how many messages you haven't read, and the people on staff (ids to start a conversation with). browse=true lists the public channels you're not in.\n\nOften next: read_chat_messages, send_chat_message.",
			Query:       []plugin.ToolParam{{Name: "browse", Type: "boolean", Description: "List the public channels you could join instead."}},
		}},
		{Method: "POST", Path: "/conversations", Export: "createConversation", Tool: &plugin.Tool{
			Name: "create_chat_conversation", Title: "Start a chat conversation",
			Description: "A channel, or a direct message with one person or a group of a few. A direct message or group that already exists with the same people is answered as it is.\n\nOften next: send_chat_message.",
			Wrap:        "conversation", Body: conversation,
		}},
		{Method: "GET", Path: "/conversations/{id}", Export: "showConversation", Tool: &plugin.Tool{
			Name: "show_chat_conversation", Title: "Show a chat conversation", Description: "Its name, topic, description and members.",
		}},
		{Method: "PATCH", Path: "/conversations/{id}", Export: "updateConversation", Tool: &plugin.Tool{
			Name: "update_chat_conversation", Title: "Rename, describe or archive a chat conversation",
			Description: "name (a channel's or a group's), topic, description; archived: true archives a channel (no more messages; its history stays), false brings it back.",
			Wrap:        "conversation", Body: `{name, topic, description, archived: boolean}`,
		}},
		{Method: "POST", Path: "/conversations/{id}/members", Export: "addMembers", Tool: &plugin.Tool{
			Name: "add_chat_members", Title: "Join a channel, or add people to one",
			Description: "With no user_ids, you join the channel (a public one). With user_ids, you add those people (you must be a member).",
			Params:      []plugin.ToolParam{{Name: "user_ids", Type: "array", Description: "The people to add (user ids)."}},
		}},
		{Method: "DELETE", Path: "/conversations/{id}/members/{user_id}", Export: "removeMember", Tool: &plugin.Tool{
			Name: "remove_chat_member", Title: "Leave a channel, or remove someone from it",
			Description: "Your own id leaves it; someone else's removes them (the channel's creator, an owner or a manager).",
		}},
		{Method: "POST", Path: "/conversations/{id}/read", Export: "markRead"},
		{Method: "PATCH", Path: "/conversations/{id}/membership", Export: "updateMembership"},
		{Method: "POST", Path: "/conversations/{id}/typing", Export: "typing"},

		{Method: "GET", Path: "/conversations/{id}/messages", Export: "listMessages", Tool: &plugin.Tool{
			Name: "read_chat_messages", Title: "Read a chat conversation's messages",
			Description: "The latest messages (50 unless limit says, at most 100), oldest first, with reactions, files and how many replies each thread has. before: a message id, for the ones before it. after: a message id, for the ones since. Reading them doesn't mark them read.\n\nOften next: send_chat_message, read_chat_thread.",
			Query: []plugin.ToolParam{
				{Name: "before", Type: "integer", Description: "Messages before this message id."},
				{Name: "after", Type: "integer", Description: "Messages after this message id."},
				{Name: "limit", Type: "integer", Description: "How many (50 unless set, at most 100)."},
			},
		}},
		{Method: "POST", Path: "/conversations/{id}/messages", Export: "sendMessage", MaxBody: 10 << 20, Tool: &plugin.Tool{
			Name: "send_chat_message", Title: "Send a chat message",
			Description: "Sends a message as you to a conversation you're in (a public channel you're not in, you join). body is text, or HTML (p, strong, em, a, ul, li, blockquote, code); mention someone with <span data-mention-id=\"12\">@Ann</span>, which notifies them. parent_id replies in that message's thread.",
			Wrap:        "message", Body: `{body (required): text or HTML, parent_id: a message id to reply to in its thread}`,
		}},
		{Method: "GET", Path: "/messages/{id}/thread", Export: "showThread", Tool: &plugin.Tool{
			Name: "read_chat_thread", Title: "Read a chat message's thread", Description: "The message and every reply to it, oldest first.",
		}},
		{Method: "PATCH", Path: "/messages/{id}", Export: "editMessage", Tool: &plugin.Tool{
			Name: "edit_chat_message", Title: "Edit one of your chat messages", Description: "Replaces its text; it shows as edited.",
			Wrap: "message", Body: `{body (required): text or HTML}`,
		}},
		{Method: "DELETE", Path: "/messages/{id}", Export: "deleteMessage", Tool: &plugin.Tool{
			Name: "delete_chat_message", Title: "Delete one of your chat messages", Description: "It shows as deleted, with its files gone; its thread stays.",
		}},
		{Method: "POST", Path: "/messages/{id}/reactions", Export: "react", Tool: &plugin.Tool{
			Name: "react_to_chat_message", Title: "React to a chat message", Description: "Adds your reaction (an emoji), or takes it back if it's there.",
			Params: []plugin.ToolParam{{Name: "emoji", Description: "One emoji, like 👍.", Required: true}},
		}},
		{Method: "GET", Path: "/files/{id}", Export: "file"},
		{Method: "GET", Path: "/search", Export: "search", Tool: &plugin.Tool{
			Name: "search_chat_messages", Title: "Search chat messages",
			Description: "Messages with these words, newest first (50 at most), in the conversations you can see.",
			Query:       []plugin.ToolParam{{Name: "q", Description: "Words to look for (two letters or more).", Required: true}},
		}},

		{Method: "GET", Path: "/settings", Export: "showSettings"},
		{Method: "PATCH", Path: "/settings", Export: "updateSettings"},
		{Method: "POST", Path: "/settings/avatar", Export: "uploadAvatar", MaxBody: 5 << 20},
		{Method: "DELETE", Path: "/settings/avatar", Export: "removeAvatar"},
		{Method: "GET", Path: "/avatars/{user_id}", Export: "serveAvatar"},
	}
}

// ---- answers -------------------------------------------------------------------------------

func refuse(status int, message string) *plugin.Response {
	code := map[int]string{http.StatusNotFound: "not_found", http.StatusForbidden: "forbidden", http.StatusConflict: "conflict",
		http.StatusUnauthorized: "unauthorized", http.StatusRequestEntityTooLarge: "too_large"}[status]
	if code == "" {
		code = "invalid"
	}
	return plugin.JSON(status, map[string]any{"error": code, "message": message})
}

func missing(what string) *plugin.Response {
	return refuse(http.StatusNotFound, "Couldn't find that "+what+", or you can't see it.")
}

func invalid(field, message string) *plugin.Response {
	return plugin.Invalid(map[string][]string{field: {message}})
}

// failed answers an error the caller should see, or the plugin failing.
func failed(err error) (*plugin.Response, error) {
	if res := plugin.Answer(err); res != nil {
		return res, nil
	}
	return nil, err
}

func ok(v map[string]any) (*plugin.Response, error) { return plugin.JSON(http.StatusOK, v), nil }

// ---- requests ------------------------------------------------------------------------------

func pathID(req *plugin.Request, name string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(req.Params[name]), 10, 64)
	return n
}

func queryID(req *plugin.Request, name string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(req.Query.Get(name)), 10, 64)
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
	return m
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

func ids(v any) []int64 {
	var out []int64
	switch l := v.(type) {
	case []any:
		for _, x := range l {
			if id := intOf(x); id > 0 {
				out = append(out, id)
			}
		}
	case string:
		for _, s := range strings.Split(l, ",") {
			if id := intOf(s); id > 0 {
				out = append(out, id)
			}
		}
	}
	return out
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

// caller is the person calling, or an answer refusing.
func caller(req *plugin.Request) (*plugin.Caller, *plugin.Response) {
	if req.Actor == nil {
		return nil, refuse(http.StatusUnauthorized, "Sign in first.")
	}
	return req.Actor, nil
}

// stamp is now, as the chat tables keep times (UTC, sortable).
func stamp() string { return time.Now().UTC().Format("2006-01-02T15:04:05.000Z") }

func placeholders(n int) string {
	if n <= 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func anys(list []int64) []any {
	out := make([]any, len(list))
	for i, v := range list {
		out[i] = v
	}
	return out
}

func contains(list []int64, v int64) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
