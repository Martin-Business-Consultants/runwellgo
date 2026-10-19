package chat

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Conversations: channels (named; public to everyone on staff, or private
// to those invited), direct messages between two people (or a note to
// yourself) and groups of a few. Each person's membership holds where they
// read up to and whether they muted it.

type conv struct {
	ID            int64
	Kind          string // channel, dm or group
	Name          string
	Topic         string
	Description   string
	Private       bool
	CreatedByID   int64
	ArchivedAt    string
	LastMessageAt string
	CreatedAt     string
}

type member struct {
	UserID     int64
	LastReadID int64
	Muted      bool
}

const convColumns = `id, kind, COALESCE(name, ''), COALESCE(topic, ''), COALESCE(description, ''), private, COALESCE(created_by_id, 0), COALESCE(archived_at, ''), COALESCE(last_message_at, ''), created_at`

func scanConv(s interface{ Scan(...any) error }) (*conv, error) {
	c := &conv{}
	err := s.Scan(&c.ID, &c.Kind, &c.Name, &c.Topic, &c.Description, &c.Private, &c.CreatedByID, &c.ArchivedAt, &c.LastMessageAt, &c.CreatedAt)
	return c, err
}

func loadConv(ctx context.Context, q plugin.Querier, id int64) (*conv, error) {
	c, err := scanConv(q.QueryRow(ctx, `SELECT `+convColumns+` FROM chat_conversations WHERE id = ?`, id))
	if isNoRows(err) {
		return nil, nil
	}
	return c, err
}

// membersOf are the members of each conversation named.
func membersOf(ctx context.Context, q plugin.Querier, convIDs []int64) (map[int64][]member, error) {
	out := map[int64][]member{}
	if len(convIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT conversation_id, user_id, last_read_id, muted FROM chat_members WHERE conversation_id IN (`+placeholders(len(convIDs))+`) ORDER BY id`, anys(convIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int64
		var m member
		if err := rows.Scan(&cid, &m.UserID, &m.LastReadID, &m.Muted); err != nil {
			return nil, err
		}
		out[cid] = append(out[cid], m)
	}
	return out, rows.Err()
}

func memberIDs(list []member) []int64 {
	out := make([]int64, len(list))
	for i, m := range list {
		out[i] = m.UserID
	}
	return out
}

func find(list []member, userID int64) *member {
	for i := range list {
		if list[i].UserID == userID {
			return &list[i]
		}
	}
	return nil
}

// visible is whether someone may see a conversation: its members, and
// everyone for a public channel.
func (c *conv) visible(members []member, userID int64) bool {
	return find(members, userID) != nil || (c.Kind == "channel" && !c.Private)
}

// open is a conversation the caller may see, with its members, or an
// answer saying it isn't there.
func open(ctx context.Context, api *plugin.API, id, userID int64) (*conv, []member, *plugin.Response, error) {
	c, err := loadConv(ctx, api, id)
	if err != nil {
		return nil, nil, nil, err
	}
	if c == nil {
		return nil, nil, missing("conversation"), nil
	}
	ms, err := membersOf(ctx, api, []int64{id})
	if err != nil {
		return nil, nil, nil, err
	}
	if !c.visible(ms[id], userID) {
		return nil, nil, missing("conversation"), nil
	}
	return c, ms[id], nil, nil
}

// title is what the conversation is called for someone: #name for a
// channel, the others' names for a direct message or group.
func (d *directory) title(c *conv, members []member, me int64) string {
	switch {
	case c.Kind == "channel":
		return "#" + c.Name
	case c.Name != "":
		return c.Name
	}
	var names []string
	for _, m := range members {
		if m.UserID != me {
			names = append(names, d.name(m.UserID))
		}
	}
	if len(names) == 0 {
		return d.name(me) + " (you)"
	}
	return strings.Join(names, ", ")
}

// unread counts each conversation's messages someone hasn't read (not
// their own, not replies in threads), and those that mention them.
func unread(ctx context.Context, q plugin.Querier, userID int64) (counts, mentions map[int64]int, err error) {
	counts, mentions = map[int64]int{}, map[int64]int{}
	rows, err := q.Query(ctx, `SELECT msg.conversation_id, count(*), sum(CASE WHEN msg.body LIKE ? THEN 1 ELSE 0 END)
		FROM chat_messages msg JOIN chat_members m ON m.conversation_id = msg.conversation_id AND m.user_id = ?
		WHERE msg.parent_id IS NULL AND msg.deleted_at IS NULL AND msg.user_id <> ? AND msg.id > m.last_read_id GROUP BY msg.conversation_id`,
		fmt.Sprintf(`%%data-mention-id="%d"%%`, userID), userID, userID)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var n, m int
		if err := rows.Scan(&id, &n, &m); err != nil {
			return nil, nil, err
		}
		counts[id], mentions[id] = n, m
	}
	return counts, mentions, rows.Err()
}

// convJSON is a conversation as someone sees it.
func (d *directory) convJSON(c *conv, members []member, me int64, unreadN, mentionN int) map[string]any {
	mine := find(members, me)
	var lastRead int64
	muted := false
	if mine != nil {
		lastRead, muted = mine.LastReadID, mine.Muted
	}
	ids := memberIDs(members)
	var other any
	if c.Kind == "dm" {
		other = me
		for _, id := range ids {
			if id != me {
				other = id
			}
		}
	}
	return map[string]any{"id": c.ID, "kind": c.Kind, "name": d.title(c, members, me), "channel_name": c.Name,
		"topic": c.Topic, "description": c.Description, "private": c.Private, "archived": c.ArchivedAt != "",
		"member_ids": ids, "member_count": len(ids), "member": mine != nil, "dm_user_id": other,
		"unread": unreadN, "mentions": mentionN, "muted": muted, "last_read_id": lastRead,
		"created_by_id": c.CreatedByID, "last_message_at": nullStr(c.LastMessageAt), "created_at": c.CreatedAt,
		"path": fmt.Sprintf("/chat?c=%d", c.ID)}
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// mine are the conversations someone is in, as they see them, latest first.
func mine(ctx context.Context, api *plugin.API, d *directory, me int64) ([]map[string]any, error) {
	rows, err := api.Query(ctx, `SELECT `+convColumns+` FROM chat_conversations WHERE id IN (SELECT conversation_id FROM chat_members WHERE user_id = ?)
		ORDER BY COALESCE(last_message_at, created_at) DESC`, me)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var list []*conv
	for rows.Next() {
		c, err := scanConv(rows)
		if err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	cids := make([]int64, len(list))
	for i, c := range list {
		cids[i] = c.ID
	}
	ms, err := membersOf(ctx, api, cids)
	if err != nil {
		return nil, err
	}
	counts, mentions, err := unread(ctx, api, me)
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for _, c := range list {
		out = append(out, d.convJSON(c, ms[c.ID], me, counts[c.ID], mentions[c.ID]))
	}
	return out, nil
}

// bootstrap is what the chat screen starts from: who you are and your
// settings, everyone on staff, your conversations, and the largest file
// you may send.
func bootstrap(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	convs, err := mine(ctx, api, d, me.ID)
	if err != nil {
		return nil, err
	}
	site, _ := api.Site(ctx)
	return ok(map[string]any{"me": d.personJSON(me.ID), "settings": settingsJSON(d.profiles[me.ID], site.TimeZone),
		"people": d.peopleJSON(), "former": d.formerJSON(), "conversations": convs, "max_upload": maxUpload(site), "manage": me.Role == "owner" || me.Role == "manager"})
}

// badge is the number beside Chat in the sidebar: unread messages in the
// conversations someone hasn't muted.
func badge(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	var n int
	err := api.QueryRow(ctx, `SELECT count(*) FROM chat_messages msg JOIN chat_members m ON m.conversation_id = msg.conversation_id AND m.user_id = ?
		JOIN chat_conversations c ON c.id = msg.conversation_id
		WHERE msg.parent_id IS NULL AND msg.deleted_at IS NULL AND msg.user_id <> ? AND msg.id > m.last_read_id AND m.muted = 0 AND c.archived_at IS NULL`, me.ID, me.ID).Scan(&n)
	if err != nil {
		return nil, err
	}
	return ok(map[string]any{"count": n})
}

func listConversations(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	if truthy(req.Query.Get("browse")) {
		rows, err := api.Query(ctx, `SELECT `+convColumns+` FROM chat_conversations WHERE kind = 'channel' AND private = 0
			AND id NOT IN (SELECT conversation_id FROM chat_members WHERE user_id = ?) ORDER BY archived_at IS NOT NULL, name`, me.ID)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var list []*conv
		for rows.Next() {
			c, err := scanConv(rows)
			if err != nil {
				return nil, err
			}
			list = append(list, c)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		cids := []int64{}
		for _, c := range list {
			cids = append(cids, c.ID)
		}
		ms, err := membersOf(ctx, api, cids)
		if err != nil {
			return nil, err
		}
		out := []map[string]any{}
		for _, c := range list {
			out = append(out, d.convJSON(c, ms[c.ID], me.ID, 0, 0))
		}
		return ok(map[string]any{"summary": plural(len(out), "public channel") + " you're not in", "channels": out})
	}
	convs, err := mine(ctx, api, d, me.ID)
	if err != nil {
		return nil, err
	}
	total := 0
	for _, c := range convs {
		n, _ := c["unread"].(int)
		total += n
	}
	people := []map[string]any{}
	for _, p := range d.peopleJSON() {
		people = append(people, map[string]any{"id": p["id"], "name": p["name"], "presence": p["presence"]})
	}
	return ok(map[string]any{"summary": fmt.Sprintf("%s, %s unread", plural(len(convs), "conversation"), plural(total, "message")), "conversations": convs, "people": people})
}

func showConversation(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	c, ms, res, err := open(ctx, api, pathID(req, "id"), me.ID)
	if res != nil || err != nil {
		return res, err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	return answerConv(ctx, api, d, c, ms, me.ID)
}

func answerConv(ctx context.Context, api *plugin.API, d *directory, c *conv, ms []member, me int64) (*plugin.Response, error) {
	counts, mentions, err := unread(ctx, api, me)
	if err != nil {
		return nil, err
	}
	o := d.convJSON(c, ms, me, counts[c.ID], mentions[c.ID])
	people := []map[string]any{}
	for _, id := range memberIDs(ms) {
		people = append(people, d.personJSON(id))
	}
	o["members"] = people
	o["summary"] = fmt.Sprintf("%s: %s", o["name"], plural(len(ms), "member"))
	return ok(map[string]any{"summary": o["summary"], "conversation": o})
}

var channelName = regexp.MustCompile(`[^a-z0-9_-]+`)

// cleanName is a channel's name as chat keeps it: lowercase words joined
// by -, at most 80 characters.
func cleanName(s string) string {
	s = strings.ToLower(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(s), "#")))
	s = channelName.ReplaceAllString(strings.ReplaceAll(s, " ", "-"), "")
	s = strings.Trim(regexp.MustCompile(`-{2,}`).ReplaceAllString(s, "-"), "-")
	if len(s) > 80 {
		s = s[:80]
	}
	return s
}

func dmKey(kind string, userIDs []int64) string {
	ids := slices.Clone(userIDs)
	slices.Sort(ids)
	parts := make([]string, len(ids))
	for i, id := range ids {
		parts[i] = strconv.FormatInt(id, 10)
	}
	return kind + ":" + strings.Join(parts, ",")
}

func createConversation(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	body := wrapped(params(req), "conversation")
	kind := str(body["kind"])
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	others := []int64{}
	for _, id := range ids(body["member_ids"]) {
		if id == me.ID || contains(others, id) {
			continue
		}
		if _, ok := d.people[id]; !ok {
			return invalid("member_ids", fmt.Sprintf("has someone who isn't on staff (%d)", id)), nil
		}
		others = append(others, id)
	}
	everyone := append([]int64{me.ID}, others...)
	now := stamp()
	var id int64
	created := false
	switch kind {
	case "dm", "group":
		if kind == "dm" && len(others) > 1 {
			kind = "group"
		}
		if kind == "group" && len(others) < 2 {
			if len(others) == 0 {
				return invalid("member_ids", "needs two or more people for a group"), nil
			}
			kind = "dm"
		}
		if kind == "group" && len(others) > 8 {
			return invalid("member_ids", "is at most eight others: make a channel for more"), nil
		}
		key := dmKey(kind, everyone)
		err = api.Tx(ctx, func(q plugin.Querier) error {
			err := q.QueryRow(ctx, `SELECT id FROM chat_conversations WHERE dm_key = ?`, key).Scan(&id)
			if err == nil {
				return nil
			} else if !isNoRows(err) {
				return err
			}
			r, err := q.Exec(ctx, `INSERT INTO chat_conversations (kind, name, dm_key, private, created_by_id, created_at, updated_at) VALUES (?, ?, ?, 1, ?, ?, ?)`,
				kind, strings.TrimSpace(str(body["name"])), key, me.ID, now, now)
			if err != nil {
				return err
			}
			id, _ = r.LastInsertId()
			created = true
			return addTo(ctx, q, id, everyone, now)
		})
	case "channel":
		name := cleanName(str(body["name"]))
		if name == "" {
			return invalid("name", "can't be blank: use lowercase words joined by -, like design-team"), nil
		}
		var taken int
		if err := api.QueryRow(ctx, `SELECT count(*) FROM chat_conversations WHERE kind = 'channel' AND name = ?`, name).Scan(&taken); err != nil {
			return nil, err
		}
		if taken > 0 {
			return invalid("name", "is taken by another channel: pick another, or join that one"), nil
		}
		err = api.Tx(ctx, func(q plugin.Querier) error {
			r, err := q.Exec(ctx, `INSERT INTO chat_conversations (kind, name, topic, description, private, created_by_id, created_at, updated_at) VALUES ('channel', ?, ?, ?, ?, ?, ?, ?)`,
				name, strings.TrimSpace(str(body["topic"])), strings.TrimSpace(str(body["description"])), truthy(body["private"]), me.ID, now, now)
			if err != nil {
				return err
			}
			id, _ = r.LastInsertId()
			created = true
			return addTo(ctx, q, id, everyone, now)
		})
	default:
		return invalid("kind", `is "channel", "dm" or "group"`), nil
	}
	if err != nil {
		return nil, err
	}
	c, ms, res, err := open(ctx, api, id, me.ID)
	if res != nil || err != nil {
		return res, err
	}
	if created {
		if err := tellMembers(ctx, api, d, c, ms, nil); err != nil {
			return nil, err
		}
	}
	return answerConv(ctx, api, d, c, ms, me.ID)
}

// addTo makes people members (those already in stay as they are).
func addTo(ctx context.Context, q plugin.Querier, convID int64, userIDs []int64, now string) error {
	for _, uid := range userIDs {
		// Someone joining starts at the latest message: what came before isn't unread.
		if _, err := q.Exec(ctx, `INSERT INTO chat_members (conversation_id, user_id, last_read_id, created_at)
			VALUES (?, ?, (SELECT COALESCE(max(id), 0) FROM chat_messages WHERE conversation_id = ?), ?) ON CONFLICT (conversation_id, user_id) DO NOTHING`,
			convID, uid, convID, now); err != nil {
			return err
		}
	}
	return nil
}

// tellMembers pushes a conversation as it is now to its members (and to
// those who just left it), each as they see it.
func tellMembers(ctx context.Context, api *plugin.API, d *directory, c *conv, ms []member, left []int64) error {
	for _, m := range ms {
		if err := api.Publish(ctx, "conversation", []int64{m.UserID}, d.convJSON(c, ms, m.UserID, 0, 0)); err != nil {
			return err
		}
	}
	if len(left) > 0 {
		return api.Publish(ctx, "conversation_left", left, map[string]any{"id": c.ID})
	}
	return nil
}

// manages is whether someone may archive a channel or remove its
// members: whoever made it, an owner or a manager.
func manages(me *plugin.Caller, c *conv) bool {
	return c.CreatedByID == me.ID || me.Role == "owner" || me.Role == "manager"
}

func updateConversation(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	c, ms, res, err := open(ctx, api, pathID(req, "id"), me.ID)
	if res != nil || err != nil {
		return res, err
	}
	body := wrapped(params(req), "conversation")
	if find(ms, me.ID) == nil {
		return refuse(http.StatusForbidden, "Join the channel first to change it."), nil
	}
	sets, args := []string{}, []any{}
	if v, ok := body["name"]; ok {
		switch c.Kind {
		case "channel":
			name := cleanName(str(v))
			if name == "" {
				return invalid("name", "can't be blank: use lowercase words joined by -, like design-team"), nil
			}
			var taken int
			if err := api.QueryRow(ctx, `SELECT count(*) FROM chat_conversations WHERE kind = 'channel' AND name = ? AND id <> ?`, name, c.ID).Scan(&taken); err != nil {
				return nil, err
			}
			if taken > 0 {
				return invalid("name", "is taken by another channel: pick another"), nil
			}
			sets, args = append(sets, "name = ?"), append(args, name)
		case "group":
			sets, args = append(sets, "name = ?"), append(args, strings.TrimSpace(str(v)))
		default:
			return invalid("name", "can't be set on a direct message"), nil
		}
	}
	for _, k := range []string{"topic", "description"} {
		if v, ok := body[k]; ok {
			s := strings.TrimSpace(str(v))
			if len(s) > 1000 {
				return invalid(k, "is too long (1,000 characters at most)"), nil
			}
			sets, args = append(sets, k+" = ?"), append(args, s)
		}
	}
	if v, ok := body["archived"]; ok {
		if c.Kind != "channel" {
			return invalid("archived", "only channels are archived"), nil
		}
		if !manages(me, c) {
			return refuse(http.StatusForbidden, "Only whoever made the channel, an owner or a manager can archive it."), nil
		}
		if truthy(v) {
			sets, args = append(sets, "archived_at = ?"), append(args, stamp())
		} else {
			sets = append(sets, "archived_at = NULL")
		}
	}
	if len(sets) == 0 {
		return invalid("conversation", "says nothing to change: name, topic, description or archived"), nil
	}
	args = append(args, stamp(), c.ID)
	if _, err := api.Exec(ctx, `UPDATE chat_conversations SET `+strings.Join(sets, ", ")+`, updated_at = ? WHERE id = ?`, args...); err != nil {
		return nil, err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	if c, err = loadConv(ctx, api, c.ID); err != nil {
		return nil, err
	}
	if err := tellMembers(ctx, api, d, c, ms, nil); err != nil {
		return nil, err
	}
	return answerConv(ctx, api, d, c, ms, me.ID)
}

func addMembers(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	c, ms, res, err := open(ctx, api, pathID(req, "id"), me.ID)
	if res != nil || err != nil {
		return res, err
	}
	if c.Kind == "dm" || c.Kind == "group" {
		return refuse(http.StatusUnprocessableEntity, "A direct message keeps its people: start a new one with everyone you want in it."), nil
	}
	if c.ArchivedAt != "" {
		return refuse(http.StatusConflict, "This channel is archived: bring it back before adding anyone."), nil
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	add := ids(params(req)["user_ids"])
	if len(add) == 0 {
		add = []int64{me.ID}
	} else if find(ms, me.ID) == nil {
		return refuse(http.StatusForbidden, "Join the channel first to add people to it."), nil
	}
	for _, id := range add {
		if _, ok := d.people[id]; !ok {
			return invalid("user_ids", fmt.Sprintf("has someone who isn't on staff (%d)", id)), nil
		}
	}
	if err := api.Tx(ctx, func(q plugin.Querier) error { return addTo(ctx, q, c.ID, add, stamp()) }); err != nil {
		return nil, err
	}
	all, err := membersOf(ctx, api, []int64{c.ID})
	if err != nil {
		return nil, err
	}
	if err := tellMembers(ctx, api, d, c, all[c.ID], nil); err != nil {
		return nil, err
	}
	return answerConv(ctx, api, d, c, all[c.ID], me.ID)
}

func removeMember(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	c, ms, res, err := open(ctx, api, pathID(req, "id"), me.ID)
	if res != nil || err != nil {
		return res, err
	}
	who := pathID(req, "user_id")
	switch {
	case c.Kind == "dm":
		return refuse(http.StatusUnprocessableEntity, "You can't leave a direct message; mute it instead."), nil
	case find(ms, who) == nil:
		return missing("member"), nil
	case who != me.ID && (c.Kind != "channel" || !manages(me, c)):
		return refuse(http.StatusForbidden, "Only whoever made the channel, an owner or a manager can remove people from it."), nil
	}
	if _, err := api.Exec(ctx, `DELETE FROM chat_members WHERE conversation_id = ? AND user_id = ?`, c.ID, who); err != nil {
		return nil, err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	rest := slices.DeleteFunc(slices.Clone(ms), func(m member) bool { return m.UserID == who })
	if err := tellMembers(ctx, api, d, c, rest, []int64{who}); err != nil {
		return nil, err
	}
	if who == me.ID {
		return ok(map[string]any{"summary": "You left " + d.title(c, ms, me.ID) + ".", "left": true})
	}
	return answerConv(ctx, api, d, c, rest, me.ID)
}

// markRead moves where someone has read up to (never back), and tells
// their other tabs.
func markRead(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	id := pathID(req, "id")
	upTo := intOf(params(req)["message_id"])
	if upTo <= 0 {
		if err := api.QueryRow(ctx, `SELECT COALESCE(max(id), 0) FROM chat_messages WHERE conversation_id = ? AND parent_id IS NULL`, id).Scan(&upTo); err != nil {
			return nil, err
		}
	}
	r, err := api.Exec(ctx, `UPDATE chat_members SET last_read_id = ? WHERE conversation_id = ? AND user_id = ? AND last_read_id < ?`, upTo, id, me.ID, upTo)
	if err != nil {
		return nil, err
	}
	if n, _ := r.RowsAffected(); n > 0 {
		if err := api.Publish(ctx, "read", []int64{me.ID}, map[string]any{"conversation_id": id, "last_read_id": upTo}); err != nil {
			return nil, err
		}
	}
	return ok(map[string]any{"conversation_id": id, "last_read_id": upTo})
}

// updateMembership mutes or unmutes a conversation for the caller.
func updateMembership(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	id := pathID(req, "id")
	body := wrapped(params(req), "membership")
	r, err := api.Exec(ctx, `UPDATE chat_members SET muted = ? WHERE conversation_id = ? AND user_id = ?`, truthy(body["muted"]), id, me.ID)
	if err != nil {
		return nil, err
	}
	if n, _ := r.RowsAffected(); n == 0 {
		return missing("conversation"), nil
	}
	c, ms, res, err := open(ctx, api, id, me.ID)
	if res != nil || err != nil {
		return res, err
	}
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return nil, err
	}
	if err := api.Publish(ctx, "conversation", []int64{me.ID}, d.convJSON(c, ms, me.ID, 0, 0)); err != nil {
		return nil, err
	}
	return answerConv(ctx, api, d, c, ms, me.ID)
}

// typing tells the others someone is writing (their tab says so every few
// seconds while they type).
func typing(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	c, ms, res, err := open(ctx, api, pathID(req, "id"), me.ID)
	if res != nil || err != nil {
		return res, err
	}
	var others []int64
	for _, m := range ms {
		if m.UserID != me.ID {
			others = append(others, m.UserID)
		}
	}
	p := params(req)
	prof, err := profileOf(ctx, api, me.ID)
	if err != nil {
		return nil, err
	}
	name := prof.DisplayName
	if name == "" {
		name = me.Name
	}
	if err := api.Publish(ctx, "typing", others, map[string]any{"conversation_id": c.ID, "parent_id": nullID(intOf(p["parent_id"])),
		"user": map[string]any{"id": me.ID, "name": name}}); err != nil {
		return nil, err
	}
	return ok(map[string]any{"ok": true})
}
