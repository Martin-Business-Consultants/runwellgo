package chat

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// deliver pushes a new message to everyone in its conversation, and
// notifies those it's for: a direct message or group, a mention, a reply
// in a thread they're in, or anything at all for someone who asked for
// all. Not someone who muted the conversation (unless they're mentioned),
// asked for nothing, or is outside their hours: for them it just waits,
// unread. A notification is in their bell (emailed when they've no tab
// open), and their tab may also chime or show a browser notification
// ("notify" on the event).
func deliver(ctx context.Context, api *plugin.API, d *directory, c *conv, ms []member, m *message, parent *message, mentions []int64, me *plugin.Caller) error {
	involved := map[int64]bool{}
	if parent != nil {
		involved[parent.UserID] = true
		rows, err := api.Query(ctx, `SELECT DISTINCT user_id FROM chat_messages WHERE parent_id = ? AND deleted_at IS NULL`, parent.ID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var id int64
			if err := rows.Scan(&id); err != nil {
				return err
			}
			involved[id] = true
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	// Who it reaches: the members, and anyone mentioned in a public channel.
	reach := memberIDs(ms)
	if c.Kind == "channel" && !c.Private {
		for _, id := range mentions {
			if _, ok := d.people[id]; ok && !contains(reach, id) {
				reach = append(reach, id)
			}
		}
	}
	x, err := loadExtras(ctx, api, []*message{m})
	if err != nil {
		return err
	}
	direct := c.Kind != "channel"
	for _, uid := range reach {
		notify := false
		if uid != me.ID {
			mem := find(ms, uid)
			pr := d.profiles[uid]
			mentioned := contains(mentions, uid)
			switch pr.Notify {
			case "all":
				notify = true
			case "mentions":
				notify = mentioned || direct || involved[uid]
			}
			if mem != nil && mem.Muted && !mentioned {
				notify = false
			}
			if !d.available(uid) {
				notify = false
			}
			if notify {
				if err := api.Notify(ctx, notification(d, c, ms, m, parent, uid, mentioned)); err != nil {
					api.Log("chat notification: " + err.Error())
				}
			}
		}
		if mem := find(ms, uid); mem == nil && uid != me.ID {
			// Mentioned, not a member: the bell is enough.
			continue
		}
		if err := publishMessage(ctx, api, "message", []int64{uid}, d.messageJSON(m, x, uid), notify); err != nil {
			return err
		}
	}
	if parent != nil {
		return changed(ctx, api, parent.ID)
	}
	return nil
}

// notification is the bell's words for a message, for one person.
func notification(d *directory, c *conv, ms []member, m *message, parent *message, to int64, mentioned bool) plugin.Notification {
	who := d.firstName(m.UserID)
	if a := agentOf(m); a != nil {
		who = str(a["name"])
	}
	where := d.title(c, ms, to)
	var title string
	switch {
	case mentioned && c.Kind == "channel":
		title = fmt.Sprintf("%s mentioned you in %s", who, where)
	case parent != nil:
		title = fmt.Sprintf("%s replied in a thread in %s", who, where)
	case c.Kind == "dm":
		title = who + " sent you a message"
	case c.Kind == "group":
		title = fmt.Sprintf("%s wrote to %s", who, where)
	default:
		title = fmt.Sprintf("%s wrote in %s", who, where)
	}
	body := m.Text
	if body == "" {
		body = "Sent a file."
	}
	if utf8.RuneCountInString(body) > 300 {
		body = string([]rune(body)[:299]) + "…"
	}
	path := fmt.Sprintf("/chat?c=%d", c.ID)
	if parent != nil {
		path += fmt.Sprintf("&thread=%d", parent.ID)
	}
	return plugin.Notification{UserID: to, FromID: m.UserID, Title: title, Body: strings.TrimSpace(body), Path: path,
		Source: "Chat", Subject: where, Group: fmt.Sprintf("conversation:%d", c.ID), Email: !d.online[to]}
}
