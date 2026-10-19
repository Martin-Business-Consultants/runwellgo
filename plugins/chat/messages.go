package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Messages: rich text (cleaned the core's way) with @mentions, files,
// reactions, and threads (replies to a message, beside the conversation).
// Your own you may edit (shown as edited) and delete (shown as deleted,
// its files gone). Everything that changes is pushed to the members' tabs.

type message struct {
	ID             int64
	ConversationID int64
	UserID         int64
	ParentID       int64
	Body, Text     string
	Via            string
	EditedAt       string
	DeletedAt      string
	ReplyCount     int
	LastReplyAt    string
	CreatedAt      string
	// Agent is the plugin's mentionable that wrote it for UserID, as JSON.
	Agent string
}

const messageColumns = `id, conversation_id, user_id, COALESCE(parent_id, 0), COALESCE(body, ''), COALESCE(body_text, ''), COALESCE(via, ''),
	COALESCE(edited_at, ''), COALESCE(deleted_at, ''), reply_count, COALESCE(last_reply_at, ''), created_at, COALESCE(agent, '')`

func scanMessages(ctx context.Context, q plugin.Querier, query string, args ...any) ([]*message, error) {
	rows, err := q.Query(ctx, `SELECT `+messageColumns+` FROM chat_messages `+query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*message
	for rows.Next() {
		m := &message{}
		if err := rows.Scan(&m.ID, &m.ConversationID, &m.UserID, &m.ParentID, &m.Body, &m.Text, &m.Via, &m.EditedAt, &m.DeletedAt, &m.ReplyCount, &m.LastReplyAt, &m.CreatedAt, &m.Agent); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func loadMessage(ctx context.Context, q plugin.Querier, id int64) (*message, error) {
	list, err := scanMessages(ctx, q, `WHERE id = ?`, id)
	if err != nil || len(list) == 0 {
		return nil, err
	}
	return list[0], nil
}

type reaction struct {
	Emoji   string
	UserIDs []int64
}

type chatFile struct {
	ID, FileID     int64
	Filename, Type string
	Size           int64
	Width, Height  int64
	MessageID      int64
}

// extras are each message's reactions, files and the first few people who
// replied in its thread, in three reads.
type extras struct {
	reactions map[int64][]reaction
	files     map[int64][]chatFile
	repliers  map[int64][]int64
}

func loadExtras(ctx context.Context, q plugin.Querier, list []*message) (*extras, error) {
	x := &extras{reactions: map[int64][]reaction{}, files: map[int64][]chatFile{}, repliers: map[int64][]int64{}}
	if len(list) == 0 {
		return x, nil
	}
	mids := make([]int64, len(list))
	var parents []int64
	for i, m := range list {
		mids[i] = m.ID
		if m.ReplyCount > 0 {
			parents = append(parents, m.ID)
		}
	}
	rows, err := q.Query(ctx, `SELECT message_id, emoji, user_id FROM chat_reactions WHERE message_id IN (`+placeholders(len(mids))+`) ORDER BY id`, anys(mids)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var mid, uid int64
		var emoji string
		if err := rows.Scan(&mid, &emoji, &uid); err != nil {
			rows.Close()
			return nil, err
		}
		rs := x.reactions[mid]
		i := slices.IndexFunc(rs, func(r reaction) bool { return r.Emoji == emoji })
		if i < 0 {
			rs = append(rs, reaction{Emoji: emoji})
			i = len(rs) - 1
		}
		rs[i].UserIDs = append(rs[i].UserIDs, uid)
		x.reactions[mid] = rs
	}
	rows.Close()
	rows, err = q.Query(ctx, `SELECT id, message_id, file_id, filename, COALESCE(content_type, ''), byte_size, COALESCE(width, 0), COALESCE(height, 0)
		FROM chat_files WHERE message_id IN (`+placeholders(len(mids))+`) ORDER BY id`, anys(mids)...)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var f chatFile
		if err := rows.Scan(&f.ID, &f.MessageID, &f.FileID, &f.Filename, &f.Type, &f.Size, &f.Width, &f.Height); err != nil {
			rows.Close()
			return nil, err
		}
		x.files[f.MessageID] = append(x.files[f.MessageID], f)
	}
	rows.Close()
	if len(parents) > 0 {
		// An agent's reply isn't its person's: they're shown by the thread.
		rows, err = q.Query(ctx, `SELECT parent_id, user_id FROM chat_messages WHERE parent_id IN (`+placeholders(len(parents))+`) AND deleted_at IS NULL AND agent IS NULL ORDER BY id`, anys(parents)...)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var pid, uid int64
			if err := rows.Scan(&pid, &uid); err != nil {
				rows.Close()
				return nil, err
			}
			if !contains(x.repliers[pid], uid) && len(x.repliers[pid]) < 4 {
				x.repliers[pid] = append(x.repliers[pid], uid)
			}
		}
		rows.Close()
	}
	return x, nil
}

func isImage(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
		return true
	}
	return false
}

// messageJSON is a message as the app and agents see it.
func (d *directory) messageJSON(m *message, x *extras, me int64) map[string]any {
	reactions := []map[string]any{}
	for _, r := range x.reactions[m.ID] {
		names := []string{}
		for _, id := range r.UserIDs {
			names = append(names, d.name(id))
		}
		reactions = append(reactions, map[string]any{"emoji": r.Emoji, "count": len(r.UserIDs), "user_ids": r.UserIDs, "names": names, "mine": contains(r.UserIDs, me)})
	}
	files := []map[string]any{}
	for _, f := range x.files[m.ID] {
		files = append(files, map[string]any{"id": f.ID, "filename": f.Filename, "content_type": f.Type, "byte_size": f.Size,
			"width": nullID(f.Width), "height": nullID(f.Height), "image": isImage(f.Type), "url": fmt.Sprintf("/api/x/chat/files/%d", f.ID)})
	}
	repliers := []map[string]any{}
	for _, id := range x.repliers[m.ID] {
		repliers = append(repliers, d.ref(id))
	}
	body, text := m.Body, m.Text
	if m.DeletedAt != "" {
		body, text = "", ""
	}
	var agent any
	if m.Agent != "" {
		var a map[string]any
		if json.Unmarshal([]byte(m.Agent), &a) == nil {
			agent = a
		}
	}
	return map[string]any{"id": m.ID, "conversation_id": m.ConversationID, "parent_id": nullID(m.ParentID), "user": d.ref(m.UserID),
		"body": body, "text": text, "via": nullStr(m.Via), "mine": m.UserID == me && m.Agent == "", "agent": agent,
		"edited": m.EditedAt != "", "edited_at": nullStr(m.EditedAt), "deleted": m.DeletedAt != "",
		"reply_count": m.ReplyCount, "last_reply_at": nullStr(m.LastReplyAt), "repliers": repliers,
		"reactions": reactions, "files": files, "created_at": m.CreatedAt}
}

func (d *directory) messagesJSON(ctx context.Context, q plugin.Querier, list []*message, me int64) ([]map[string]any, error) {
	x, err := loadExtras(ctx, q, list)
	if err != nil {
		return nil, err
	}
	out := make([]map[string]any, len(list))
	for i, m := range list {
		out[i] = d.messageJSON(m, x, me)
	}
	return out, nil
}

func listMessages(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	c, ms, res, err := open(ctx, api, pathID(req, "id"), me.ID)
	if res != nil || err != nil {
		return res, err
	}
	limit := int(queryID(req, "limit"))
	if limit <= 0 {
		limit = 50
	}
	limit = min(limit, 100)
	before, after := queryID(req, "before"), queryID(req, "after")
	var list []*message
	if after > 0 {
		list, err = scanMessages(ctx, api, `WHERE conversation_id = ? AND parent_id IS NULL AND id > ? ORDER BY id LIMIT ?`, c.ID, after, limit+1)
	} else {
		where := `WHERE conversation_id = ? AND parent_id IS NULL`
		args := []any{c.ID}
		if before > 0 {
			where += ` AND id < ?`
			args = append(args, before)
		}
		list, err = scanMessages(ctx, api, where+` ORDER BY id DESC LIMIT ?`, append(args, limit+1)...)
		slices.Reverse(list)
	}
	if err != nil {
		return nil, err
	}
	more := len(list) > limit
	if more {
		if after > 0 {
			list = list[:limit]
		} else {
			list = list[1:]
		}
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	out, err := d.messagesJSON(ctx, api, list, me.ID)
	if err != nil {
		return nil, err
	}
	counts, mentions, err := unread(ctx, api, me.ID)
	if err != nil {
		return nil, err
	}
	cj := d.convJSON(c, ms, me.ID, counts[c.ID], mentions[c.ID])
	summary := fmt.Sprintf("%s in %s", plural(len(out), "message"), cj["name"])
	return ok(map[string]any{"summary": summary, "conversation": cj, "messages": out, "has_more": more})
}

func showThread(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	parent, err := loadMessage(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if parent == nil || parent.ParentID != 0 {
		return missing("message"), nil
	}
	c, ms, res, err := open(ctx, api, parent.ConversationID, me.ID)
	if res != nil || err != nil {
		return res, err
	}
	replies, err := scanMessages(ctx, api, `WHERE parent_id = ? ORDER BY id LIMIT 1000`, parent.ID)
	if err != nil {
		return nil, err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	all, err := d.messagesJSON(ctx, api, append([]*message{parent}, replies...), me.ID)
	if err != nil {
		return nil, err
	}
	cj := d.convJSON(c, ms, me.ID, 0, 0)
	return ok(map[string]any{"summary": fmt.Sprintf("%d %s in %s", len(replies), map[bool]string{true: "reply", false: "replies"}[len(replies) == 1], cj["name"]), "conversation": cj, "message": all[0], "replies": all[1:]})
}

var beforePunct = regexp.MustCompile(`\s+([,.;:!?)])`)

// richText is a message's HTML cleaned, its text read as a sentence (no
// space left before a comma after a mention).
func richText(api *plugin.API, body string) plugin.RichText {
	rt := api.RichText(body)
	rt.Text = strings.TrimSpace(beforePunct.ReplaceAllString(rt.Text, "$1"))
	return rt
}

// maxBody is the most a message says (its HTML).
const maxBody = 40000

func sendMessage(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	in, res := readSend(ctx, api, req)
	if res != nil {
		return res, nil
	}
	c, ms, res, err := open(ctx, api, pathID(req, "id"), me.ID)
	if res != nil || err != nil {
		return res, err
	}
	if c.ArchivedAt != "" {
		return refuse(http.StatusConflict, "This channel is archived: bring it back to send messages to it."), nil
	}
	rt := richText(api, in.body)
	if rt.Text == "" && len(in.files) == 0 && !strings.Contains(rt.HTML, "<img") {
		return invalid("body", "can't be blank: write something, or attach a file"), nil
	}
	if len(rt.HTML) > maxBody {
		return invalid("body", "is too long: split it into a few messages"), nil
	}
	var parent *message
	if in.parentID > 0 {
		if parent, err = loadMessage(ctx, api, in.parentID); err != nil {
			return nil, err
		}
		if parent == nil || parent.ConversationID != c.ID || parent.ParentID != 0 {
			return invalid("parent_id", "isn't a message in this conversation to reply to"), nil
		}
	}
	via := ""
	if me.Agent {
		via = me.Via
		if via == "" {
			via = "an agent"
		}
	}
	now := stamp()
	var id int64
	err = api.Tx(ctx, func(q plugin.Querier) error {
		// Writing in a public channel you're not in joins it.
		if find(ms, me.ID) == nil {
			if err := addTo(ctx, q, c.ID, []int64{me.ID}, now); err != nil {
				return err
			}
		}
		r, err := q.Exec(ctx, `INSERT INTO chat_messages (conversation_id, user_id, parent_id, body, body_text, via, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, me.ID, nullID(in.parentID), rt.HTML, rt.Text, nullStr(via), now, now)
		if err != nil {
			return err
		}
		id, _ = r.LastInsertId()
		if parent != nil {
			if _, err := q.Exec(ctx, `UPDATE chat_messages SET reply_count = reply_count + 1, last_reply_at = ?, updated_at = ? WHERE id = ?`, now, now, parent.ID); err != nil {
				return err
			}
		} else if _, err := q.Exec(ctx, `UPDATE chat_members SET last_read_id = ? WHERE conversation_id = ? AND user_id = ?`, id, c.ID, me.ID); err != nil {
			return err
		}
		_, err = q.Exec(ctx, `UPDATE chat_conversations SET last_message_at = ?, updated_at = ? WHERE id = ?`, now, now, c.ID)
		return err
	})
	if err != nil {
		return nil, err
	}
	if res, err := attachAll(ctx, api, id, in.files); res != nil || err != nil {
		// The message goes with its files or not at all.
		_, _ = api.Exec(ctx, `DELETE FROM chat_messages WHERE id = ?`, id)
		if parent != nil {
			_, _ = api.Exec(ctx, `UPDATE chat_messages SET reply_count = max(reply_count - 1, 0) WHERE id = ?`, parent.ID)
		}
		return res, err
	}
	all, err := membersOf(ctx, api, []int64{c.ID})
	if err != nil {
		return nil, err
	}
	ms = all[c.ID]
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	m, err := loadMessage(ctx, api, id)
	if err != nil {
		return nil, err
	}
	out, err := d.messagesJSON(ctx, api, []*message{m}, me.ID)
	if err != nil {
		return nil, err
	}
	if err := deliver(ctx, api, d, c, ms, m, parent, rt.Mentions, me); err != nil {
		return nil, err
	}
	relay(ctx, api, d, c, ms, m, parent, rt, me)
	return plugin.JSON(http.StatusCreated, map[string]any{"summary": "Sent to " + d.title(c, ms, me.ID) + ".", "message": out[0]}), nil
}

// changed pushes a message as it is now (an edit, a reaction, a reply
// count) to the conversation's members.
func changed(ctx context.Context, api *plugin.API, id int64) error {
	m, err := loadMessage(ctx, api, id)
	if err != nil || m == nil {
		return err
	}
	all, err := membersOf(ctx, api, []int64{m.ConversationID})
	if err != nil {
		return err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return err
	}
	x, err := loadExtras(ctx, api, []*message{m})
	if err != nil {
		return err
	}
	// "mine" differs per person, so each hears it as theirs.
	for _, mem := range all[m.ConversationID] {
		if err := publishMessage(ctx, api, "message_updated", []int64{mem.UserID}, d.messageJSON(m, x, mem.UserID), false); err != nil {
			return err
		}
	}
	return nil
}

// publishMessage pushes a message, or (too long to push) word that it
// changed, for the tab to read.
func publishMessage(ctx context.Context, api *plugin.API, topic string, to []int64, msg map[string]any, notify bool) error {
	err := api.Publish(ctx, topic, to, map[string]any{"message": msg, "notify": notify})
	if err != nil && strings.Contains(err.Error(), "64 KB") {
		return api.Publish(ctx, topic, to, map[string]any{"stale": true, "notify": notify,
			"message": map[string]any{"id": msg["id"], "conversation_id": msg["conversation_id"], "parent_id": msg["parent_id"]}})
	}
	return err
}

// own is one of the caller's messages (or, for deleting, any for an
// owner), or an answer saying why not.
func own(ctx context.Context, api *plugin.API, me *plugin.Caller, id int64, deleting bool) (*message, *plugin.Response, error) {
	m, err := loadMessage(ctx, api, id)
	if err != nil {
		return nil, nil, err
	}
	if m == nil {
		return nil, missing("message"), nil
	}
	if _, _, res, err := open(ctx, api, m.ConversationID, me.ID); res != nil || err != nil {
		return nil, res, err
	}
	if m.DeletedAt != "" {
		return nil, refuse(http.StatusConflict, "That message was deleted."), nil
	}
	if m.Agent != "" && !deleting {
		return nil, refuse(http.StatusForbidden, "That reply was written by an agent: delete it instead."), nil
	}
	if m.UserID != me.ID && !(deleting && me.Role == "owner") {
		return nil, refuse(http.StatusForbidden, "You can only change your own messages."), nil
	}
	return m, nil, nil
}

func editMessage(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	m, res, err := own(ctx, api, me, pathID(req, "id"), false)
	if res != nil || err != nil {
		return res, err
	}
	rt := richText(api, str(wrapped(params(req), "message")["body"]))
	if rt.Text == "" && !strings.Contains(rt.HTML, "<img") {
		var files int
		if err := api.QueryRow(ctx, `SELECT count(*) FROM chat_files WHERE message_id = ?`, m.ID).Scan(&files); err != nil {
			return nil, err
		}
		if files == 0 {
			return invalid("body", "can't be blank: delete the message instead"), nil
		}
	}
	if len(rt.HTML) > maxBody {
		return invalid("body", "is too long: split it into a few messages"), nil
	}
	if rt.HTML == m.Body {
		return answerMessage(ctx, api, m.ID, me.ID, "Nothing changed.")
	}
	now := stamp()
	if _, err := api.Exec(ctx, `UPDATE chat_messages SET body = ?, body_text = ?, edited_at = ?, updated_at = ? WHERE id = ?`, rt.HTML, rt.Text, now, now, m.ID); err != nil {
		return nil, err
	}
	if err := changed(ctx, api, m.ID); err != nil {
		return nil, err
	}
	return answerMessage(ctx, api, m.ID, me.ID, "Edited.")
}

func deleteMessage(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	m, res, err := own(ctx, api, me, pathID(req, "id"), true)
	if res != nil || err != nil {
		return res, err
	}
	rows, err := api.Query(ctx, `SELECT file_id FROM chat_files WHERE message_id = ?`, m.ID)
	if err != nil {
		return nil, err
	}
	var fileIDs []int64
	for rows.Next() {
		var f int64
		if err := rows.Scan(&f); err != nil {
			rows.Close()
			return nil, err
		}
		fileIDs = append(fileIDs, f)
	}
	rows.Close()
	for _, f := range fileIDs {
		if err := api.Detach(ctx, "Chat::Message", m.ID, f); err != nil && plugin.Answer(err) == nil {
			return nil, err
		}
	}
	now := stamp()
	err = api.Tx(ctx, func(q plugin.Querier) error {
		if _, err := q.Exec(ctx, `UPDATE chat_messages SET body = '', body_text = '', deleted_at = ?, updated_at = ? WHERE id = ?`, now, now, m.ID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM chat_files WHERE message_id = ?`, m.ID); err != nil {
			return err
		}
		if _, err := q.Exec(ctx, `DELETE FROM chat_reactions WHERE message_id = ?`, m.ID); err != nil {
			return err
		}
		if m.ParentID != 0 {
			_, err := q.Exec(ctx, `UPDATE chat_messages SET reply_count = max(reply_count - 1, 0), updated_at = ? WHERE id = ?`, now, m.ParentID)
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := changed(ctx, api, m.ID); err != nil {
		return nil, err
	}
	if m.ParentID != 0 {
		if err := changed(ctx, api, m.ParentID); err != nil {
			return nil, err
		}
	}
	return answerMessage(ctx, api, m.ID, me.ID, "Deleted.")
}

func react(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	m, err := loadMessage(ctx, api, pathID(req, "id"))
	if err != nil {
		return nil, err
	}
	if m == nil {
		return missing("message"), nil
	}
	if _, _, res, err := open(ctx, api, m.ConversationID, me.ID); res != nil || err != nil {
		return res, err
	}
	if m.DeletedAt != "" {
		return refuse(http.StatusConflict, "That message was deleted."), nil
	}
	emoji := strings.TrimSpace(str(params(req)["emoji"]))
	if emoji == "" || utf8.RuneCountInString(emoji) > 12 || strings.ContainsAny(emoji, " <>\"'&") {
		return invalid("emoji", "is one emoji, like 👍"), nil
	}
	r, err := api.Exec(ctx, `DELETE FROM chat_reactions WHERE message_id = ? AND user_id = ? AND emoji = ?`, m.ID, me.ID, emoji)
	if err != nil {
		return nil, err
	}
	summary := "Took back " + emoji + "."
	if n, _ := r.RowsAffected(); n == 0 {
		var kinds int
		if err := api.QueryRow(ctx, `SELECT count(DISTINCT emoji) FROM chat_reactions WHERE message_id = ?`, m.ID).Scan(&kinds); err != nil {
			return nil, err
		}
		if kinds >= 40 {
			return refuse(http.StatusUnprocessableEntity, "That message has all the reactions it can hold."), nil
		}
		if _, err := api.Exec(ctx, `INSERT INTO chat_reactions (message_id, user_id, emoji, created_at) VALUES (?, ?, ?, ?)`, m.ID, me.ID, emoji, stamp()); err != nil {
			return nil, err
		}
		summary = "Reacted " + emoji + "."
	}
	if err := changed(ctx, api, m.ID); err != nil {
		return nil, err
	}
	return answerMessage(ctx, api, m.ID, me.ID, summary)
}

func answerMessage(ctx context.Context, api *plugin.API, id, me int64, summary string) (*plugin.Response, error) {
	m, err := loadMessage(ctx, api, id)
	if err != nil {
		return nil, err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	out, err := d.messagesJSON(ctx, api, []*message{m}, me)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"summary": summary, "message": out[0]})
}

// search finds messages with all the words, newest first, in the
// conversations the caller can see.
func search(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	q := strings.TrimSpace(req.Query.Get("q"))
	if utf8.RuneCountInString(q) < 2 {
		return invalid("q", "needs two letters or more"), nil
	}
	words := strings.Fields(q)
	if len(words) > 8 {
		words = words[:8]
	}
	where := []string{`m.deleted_at IS NULL`, `(m.conversation_id IN (SELECT conversation_id FROM chat_members WHERE user_id = ?) OR m.conversation_id IN (SELECT id FROM chat_conversations WHERE kind = 'channel' AND private = 0))`}
	args := []any{me.ID}
	for _, w := range words {
		where = append(where, `m.body_text LIKE ? ESCAPE '\'`)
		args = append(args, "%"+strings.NewReplacer(`\`, `\\`, "%", `\%`, "_", `\_`).Replace(w)+"%")
	}
	if cid := queryID(req, "conversation_id"); cid > 0 {
		where = append(where, `m.conversation_id = ?`)
		args = append(args, cid)
	}
	rows, err := api.Query(ctx, `SELECT m.id FROM chat_messages m WHERE `+strings.Join(where, " AND ")+` ORDER BY m.id DESC LIMIT 50`, args...)
	if err != nil {
		return nil, err
	}
	var found []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		found = append(found, id)
	}
	rows.Close()
	list := []*message{}
	if len(found) > 0 {
		if list, err = scanMessages(ctx, api, `WHERE id IN (`+placeholders(len(found))+`) ORDER BY id DESC`, anys(found)...); err != nil {
			return nil, err
		}
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	out, err := d.messagesJSON(ctx, api, list, me.ID)
	if err != nil {
		return nil, err
	}
	// Where each was said.
	cids := []int64{}
	for _, m := range list {
		if !contains(cids, m.ConversationID) {
			cids = append(cids, m.ConversationID)
		}
	}
	ms, err := membersOf(ctx, api, cids)
	if err != nil {
		return nil, err
	}
	names := map[int64]map[string]any{}
	for _, cid := range cids {
		c, err := loadConv(ctx, api, cid)
		if err != nil {
			return nil, err
		}
		if c != nil {
			names[cid] = map[string]any{"id": c.ID, "kind": c.Kind, "name": d.title(c, ms[cid], me.ID)}
		}
	}
	for i, m := range list {
		out[i]["conversation"] = names[m.ConversationID]
	}
	return ok(map[string]any{"summary": fmt.Sprintf("%s found for %q", plural(len(out), "message"), q), "messages": out})
}
