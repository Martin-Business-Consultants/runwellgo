package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// A message that @mentions a plugin's name (an AI agent, offered through
// the core's mentionables) is passed to that plugin as its writer
// (mentions:relay), with the thread or the conversation so far; its reply
// comes back through mentionReply, into the message's thread, written as
// the agent on behalf of the person who mentioned it. Other plugins can
// hook chat.message.mentioned.

// contextLines is how much of the conversation goes with a mention.
const contextLines = 20

// relay passes on a new message's mentions of plugins' names.
func relay(ctx context.Context, api *plugin.API, d *directory, c *conv, ms []member, m *message, parent *message, rt plugin.RichText, me *plugin.Caller) {
	if len(rt.Mentionables) == 0 {
		return
	}
	root := m
	if parent != nil {
		root = parent
	}
	where := d.title(c, ms, me.ID)
	said := conversationSoFar(ctx, api, d, c, root, m)
	for _, name := range rt.Mentionables {
		ok, err := api.RelayMention(ctx, plugin.Mention{Mentionable: name, Text: rt.Text, HTML: rt.HTML, Subject: where, Context: said,
			Path: fmt.Sprintf("/chat?c=%d&thread=%d", c.ID, root.ID), Ref: strconv.FormatInt(root.ID, 10)})
		if err != nil {
			api.Log("chat mention: " + err.Error())
		}
		if ok {
			_ = api.Emit(ctx, "chat.message.mentioned", map[string]any{"conversation_id": c.ID, "conversation": where, "message_id": m.ID,
				"thread_id": root.ID, "author": map[string]any{"id": me.ID, "name": me.Name}, "text": rt.Text, "mentionable": name})
		}
	}
}

// conversationSoFar is what was said before m: its thread, or the
// conversation's last messages, as "Name: text" lines.
func conversationSoFar(ctx context.Context, api *plugin.API, d *directory, c *conv, root, m *message) string {
	var list []*message
	var err error
	if root.ID != m.ID {
		list, err = scanMessages(ctx, api, `WHERE (id = ? OR parent_id = ?) AND id < ? AND deleted_at IS NULL ORDER BY id DESC LIMIT ?`, root.ID, root.ID, m.ID, contextLines)
	} else {
		list, err = scanMessages(ctx, api, `WHERE conversation_id = ? AND parent_id IS NULL AND id < ? AND deleted_at IS NULL ORDER BY id DESC LIMIT ?`, c.ID, m.ID, contextLines)
	}
	if err != nil {
		return ""
	}
	lines := []string{}
	for i := len(list) - 1; i >= 0; i-- {
		who := d.name(list[i].UserID)
		if a := agentOf(list[i]); a != nil {
			who = str(a["name"])
		}
		lines = append(lines, who+": "+list[i].Text)
	}
	return strings.Join(lines, "\n")
}

func agentOf(m *message) map[string]any {
	if m.Agent == "" {
		return nil
	}
	var a map[string]any
	if json.Unmarshal([]byte(m.Agent), &a) != nil {
		return nil
	}
	return a
}

// mentionReply puts a plugin's reply in the thread of the message that
// mentioned it, or changes its earlier reply (Replace).
func mentionReply(ctx context.Context, api *plugin.API, r plugin.MentionReply) (string, error) {
	rootID, _ := strconv.ParseInt(r.Ref, 10, 64)
	root, err := loadMessage(ctx, api, rootID)
	if err != nil {
		return "", err
	}
	if root == nil {
		return "", errors.New("the message it was mentioned in is gone")
	}
	c, err := loadConv(ctx, api, root.ConversationID)
	if err != nil || c == nil {
		return "", errors.New("the conversation it was mentioned in is gone")
	}
	html := r.HTML
	if len(html) > maxBody {
		html = html[:maxBody]
	}
	now := stamp()
	if id, _ := strconv.ParseInt(r.Replace, 10, 64); id > 0 {
		res, err := api.Exec(ctx, `UPDATE chat_messages SET body = ?, body_text = ?, updated_at = ? WHERE id = ? AND parent_id = ? AND agent IS NOT NULL AND deleted_at IS NULL`,
			html, r.Text, now, id, root.ID)
		if err != nil {
			return "", err
		}
		if n, _ := res.RowsAffected(); n == 1 {
			return r.Replace, changed(ctx, api, id)
		}
	}
	agent, _ := json.Marshal(map[string]any{"id": r.As.ID, "name": r.As.Name, "handle": r.As.Handle, "avatar_url": nullStr(r.As.Avatar), "via": r.Via, "for": r.For.Name})
	var id int64
	err = api.Tx(ctx, func(q plugin.Querier) error {
		res, err := q.Exec(ctx, `INSERT INTO chat_messages (conversation_id, user_id, parent_id, body, body_text, via, agent, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, r.For.ID, root.ID, html, r.Text, r.Via, string(agent), now, now)
		if err != nil {
			return err
		}
		id, _ = res.LastInsertId()
		if _, err := q.Exec(ctx, `UPDATE chat_messages SET reply_count = reply_count + 1, last_reply_at = ?, updated_at = ? WHERE id = ?`, now, now, root.ID); err != nil {
			return err
		}
		_, err = q.Exec(ctx, `UPDATE chat_conversations SET last_message_at = ?, updated_at = ? WHERE id = ?`, now, now, c.ID)
		return err
	})
	if err != nil {
		return "", err
	}
	all, err := membersOf(ctx, api, []int64{c.ID})
	if err != nil {
		return "", err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return "", err
	}
	m, err := loadMessage(ctx, api, id)
	if err != nil || m == nil {
		return "", err
	}
	if err := deliver(ctx, api, d, c, all[c.ID], m, root, nil, &plugin.Caller{ID: r.For.ID, Name: r.For.Name}); err != nil {
		return "", err
	}
	return strconv.FormatInt(id, 10), nil
}
