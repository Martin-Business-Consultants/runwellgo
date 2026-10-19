package core

import (
	"context"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Notifications (Fizzy's): what reaches a person. A mention makes one for
// its person (the Notifier), emailed as it's made; the person reads it by
// opening it, or marks it read or unread, or all read at once. The sidebar
// counts the unread.

const (
	maxUnreadNotifications = 500
	maxTrayEntries         = 100
)

func init() {
	routeSets = append(routeSets, (*App).notificationRoutes)
	navCounts["notifications"] = func(a *App, ctx context.Context, actor *Actor) (int, error) {
		if actor == nil || actor.User == nil {
			return 0, nil
		}
		return store.Count(ctx, a.store, `SELECT count(*) FROM notifications n WHERE n.user_id = ? AND n.read_at IS NULL AND `+a.visibleNotifications(), actor.User.ID)
	}
}

func (a *App) notificationRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/notifications", a.listNotifications, tool("list_notifications", "List your notifications"))
	// The footer tray in the browser; list_notifications covers it.
	a.handle(mux, "GET /api/notifications/tray", a.notificationTray)
	// Opens a notification (reading it on the way): the app goes where it
	// points. list_notifications and mark_notification_read cover it.
	a.handle(mux, "GET /api/notifications/{id}", a.showNotification)
	a.handle(mux, "POST /api/notifications/{id}/reading", a.readNotification, tool("mark_notification_read", "Mark a notification read"))
	a.handle(mux, "DELETE /api/notifications/{id}/reading", a.unreadNotification, tool("mark_notification_unread", "Mark a notification unread"))
	a.handle(mux, "POST /api/notifications/bulk_reading", a.readAllNotifications, tool("mark_all_notifications_read", "Mark all notifications read"))
	// The editor's @mention menu; me lists the team.
	a.handle(mux, "GET /api/prompts/users", a.promptUsers)
}

// notifyMention makes the mention's notification for its person, unless
// they mentioned themselves, and emails it once the write commits.
func (t *Tx) notifyMention(mentionID, mentioner, mentionee int64) error {
	if mentioner == mentionee {
		return nil
	}
	now := store.Now().DB()
	id, err := t.Insert(`INSERT INTO notifications (user_id, creator_id, source_type, source_id, read_at, unread_count, created_at, updated_at) VALUES (?, ?, 'Mention', ?, NULL, 1, ?, ?)`,
		mentionee, mentioner, mentionID, now, now)
	if err != nil {
		return err
	}
	a := t.App
	t.AfterCommit(func(ctx context.Context) {
		m, err := a.notificationMail(ctx, id)
		if err != nil {
			a.log.Warn("notification mail", "notification", id, "err", err)
			return
		}
		if m != nil {
			a.mailLater(*m)
		}
	})
	return nil
}

// firstName is a name's first word.
func firstName(name string) string {
	if f := strings.Fields(name); len(f) > 0 {
		return f[0]
	}
	return name
}

// familiarName is "Ann B." for Ann Bell.
func familiarName(name string) string {
	f := strings.Fields(name)
	if len(f) <= 1 {
		return name
	}
	var out strings.Builder
	out.WriteString(f[0] + " ")
	for _, n := range f[1:] {
		out.WriteString(string([]rune(n)[:1]) + ".")
	}
	return out.String()
}

// notificationMail is the email for a notification (NotificationMailer):
// who mentioned the person, their words, and a link to open it.
func (a *App) notificationMail(ctx context.Context, id int64) (*mail.Message, error) {
	var to, mentionerName, sourceType string
	var sourceID int64
	err := a.store.QueryRowContext(ctx, `SELECT u.email_address, COALESCE(NULLIF(c.name, ''), c.email_address), m.source_type, m.source_id
		FROM notifications n JOIN users u ON u.id = n.user_id JOIN mentions m ON m.id = n.source_id AND n.source_type = 'Mention'
		JOIN users c ON c.id = m.mentioner_id WHERE n.id = ?`, id).Scan(&to, &mentionerName, &sourceType, &sourceID)
	if isNoRows(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	brand := a.BrandName(ctx)
	first := firstName(mentionerName)
	content := mentionableContent(ctx, a.store, sourceType, sourceID)
	url := fmt.Sprintf("%s/notifications/%d", a.BaseURL(), id)
	subject := first + " mentioned you"
	text := brand + "\n" + strings.Repeat("=", len([]rune(brand))) + "\n\n" +
		first + " mentioned you:\n\n> " + railsTruncate(content, 250) + "\n\nOpen it: " + url + "\n\n--\nYou're getting this because someone mentioned you.\n"
	color := s.BrandColor()
	radius := map[string]string{"sharp": "3px", "round": "999px"}[s.Radius]
	if radius == "" {
		radius = "8px"
	}
	esc := html.EscapeString
	body := `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>` +
		`<body style="margin:0;padding:0;background:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">` +
		`<div style="display:none;max-height:0;overflow:hidden;">` + esc(railsTruncate(content, 90)) + `</div>` +
		`<div style="max-width:560px;margin:0 auto;padding:32px 24px;">` +
		`<p style="color:#16171a;font-size:15px;font-weight:700;margin:0 0 24px;">` + esc(brand) + `</p>` +
		`<div style="background:#ffffff;border-radius:12px;padding:32px;color:#3a3b40;font-size:16px;line-height:1.5;">` +
		`<h1 style="color:#16171a;font-size:22px;font-weight:700;line-height:1.3;letter-spacing:-0.01em;margin:0 0 16px;">` + esc(subject) + `</h1>` +
		`<div style="border-left:3px solid ` + color + `;color:#3a3b40;margin:0 0 16px;padding:4px 0 4px 14px;">` + esc(railsTruncate(content, 250)) + `</div>` +
		`<table role="presentation" cellpadding="0" cellspacing="0" border="0" style="margin:24px 0 8px;"><tr><td bgcolor="` + color + `" style="border-radius:` + radius + `;background:` + color + `;">` +
		`<a href="` + esc(url) + `" style="display:inline-block;padding:12px 24px;border-radius:` + radius + `;color:#ffffff;font-size:16px;font-weight:600;line-height:1.2;text-decoration:none;">Open it</a></td></tr></table>` +
		`<p style="color:#8a8c93;font-size:13px;line-height:1.5;margin:0 0 16px;word-break:break-all;">Button not working? Paste this into your browser: <a href="` + esc(url) + `" style="color:#8a8c93;">` + esc(url) + `</a></p>` +
		`</div><p style="color:#8a8c93;font-size:13px;line-height:1.5;margin:16px 0 0;">You’re getting this because someone mentioned you.</p></div></body></html>`
	return &mail.Message{To: []string{to}, Subject: subject, Text: text, HTML: body}, nil
}

// notificationRow is a notification as read for answers.
type notificationRow struct {
	ID, CreatorID, SourceID, UnreadCount int64
	CreatorName, TargetType              string
	TargetID                             int64
	SourceType                           string // Mention, or a plugin's (plugin_notify.go)
	ReadAt, CreatedAt                    store.Time
}

const notificationQuery = `SELECT n.id, n.creator_id, COALESCE(NULLIF(c.name, ''), c.email_address, ''), n.source_id, n.unread_count,
		COALESCE(m.source_type, ''), COALESCE(m.source_id, 0), n.read_at, n.created_at, n.source_type
	FROM notifications n LEFT JOIN users c ON c.id = n.creator_id LEFT JOIN mentions m ON m.id = n.source_id AND n.source_type = 'Mention'`

// notifications are the person's notifications matching where, in Fizzy's
// order (unread first, then latest read; newest first within each).
func (a *App) notifications(ctx context.Context, where string, args ...any) ([]notificationRow, error) {
	rows, err := a.store.QueryContext(ctx, notificationQuery+` WHERE `+a.visibleNotifications()+` AND `+where, args...)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (notificationRow, error) {
		var n notificationRow
		err := r.Scan(&n.ID, &n.CreatorID, &n.CreatorName, &n.SourceID, &n.UnreadCount, &n.TargetType, &n.TargetID, &n.ReadAt, &n.CreatedAt, &n.SourceType)
		return n, err
	})
}

// notificationsJSON are notifications as answers show them
// (notifications/index.json), with what the app's cards show: who, their
// words, what it's about and where it opens.
func (a *App) notificationsJSON(ctx context.Context, list []notificationRow) ([]Obj, error) {
	refs := make([]searchRef, 0, len(list))
	for _, n := range list {
		refs = append(refs, searchRef{n.TargetType, n.TargetID})
	}
	infos, err := a.recordInfos(ctx, refs)
	if err != nil {
		return nil, err
	}
	var noticeIDs []int64
	for _, n := range list {
		if n.SourceType == pluginNotificationSource {
			noticeIDs = append(noticeIDs, n.SourceID)
		}
	}
	notices, err := a.pluginNotices(ctx, noticeIDs)
	if err != nil {
		return nil, err
	}
	excerpts := map[searchRef]string{}
	out := make([]Obj, len(list))
	for i, n := range list {
		if p, ok := notices[n.SourceID]; ok && n.SourceType == pluginNotificationSource {
			// A plugin's: its own words, and the path it opens.
			out[i] = O("id", n.ID, "summary", p.Title, "about", nil, "created_at", n.CreatedAt, "read", n.ReadAt.Valid,
				"read_at", n.ReadAt, "unread_count", n.UnreadCount,
				"creator", userRef(n.CreatorID, n.CreatorName), "creator_familiar_name", familiarName(n.CreatorName),
				"title", p.Title, "excerpt", railsTruncate(p.Body, 200), "target", O("ref", p.Source, "name", p.Subject), "path", p.Path)
			continue
		}
		ref := searchRef{n.TargetType, n.TargetID}
		info := infos[ref]
		var about any
		path := "/"
		head := O("ref", n.TargetType, "name", "")
		if info != nil {
			about = a.agentRef(ref.Type, ref.ID, info.Ref, info.Display, info.Path)
			path = info.Path
			head = O("ref", info.HeadRef, "name", info.HeadName)
		}
		if _, ok := excerpts[ref]; !ok {
			excerpts[ref] = mentionableContent(ctx, a.store, ref.Type, ref.ID)
		}
		out[i] = O("id", n.ID, "summary", n.CreatorName+" mentioned you", "about", about, "created_at", n.CreatedAt, "read", n.ReadAt.Valid,
			"read_at", n.ReadAt, "unread_count", n.UnreadCount,
			"creator", userRef(n.CreatorID, n.CreatorName), "creator_familiar_name", familiarName(n.CreatorName),
			"title", firstName(n.CreatorName)+" @mentioned you", "excerpt", railsTruncate(excerpts[ref], 200),
			"target", head, "path", path)
	}
	return out, nil
}

func (a *App) listNotifications(c *Ctx) error {
	ctx := c.R.Context()
	uid := c.Actor.User.ID
	unread, err := a.notifications(ctx, `n.user_id = ? AND n.read_at IS NULL ORDER BY n.updated_at DESC, n.id DESC LIMIT ?`, uid, maxUnreadNotifications)
	if err != nil {
		return err
	}
	read, err := a.notifications(ctx, `n.user_id = ? AND n.read_at IS NOT NULL ORDER BY n.read_at DESC, n.updated_at DESC LIMIT ?`, uid, 100)
	if err != nil {
		return err
	}
	// Agents get the latest 20 read; the app its 100.
	if c.Actor.Agent() && len(read) > 20 {
		read = read[:20]
	}
	u, err := a.notificationsJSON(ctx, unread)
	if err != nil {
		return err
	}
	r, err := a.notificationsJSON(ctx, read)
	if err != nil {
		return err
	}
	c.Summary = fmt.Sprintf("%d unread", len(unread))
	return c.JSON(200, O("summary", c.Summary, "unread", u, "read", r))
}

func (a *App) notificationTray(c *Ctx) error {
	ctx := c.R.Context()
	list, err := a.notifications(ctx, `n.user_id = ? AND n.read_at IS NULL ORDER BY n.updated_at DESC, n.id DESC LIMIT ?`, c.Actor.User.ID, maxTrayEntries)
	if err != nil {
		return err
	}
	out, err := a.notificationsJSON(ctx, list)
	if err != nil {
		return err
	}
	return c.JSON(200, O("notifications", out))
}

// ownNotification is one of the person's notifications; notFound otherwise.
func (a *App) ownNotification(c *Ctx) (notificationRow, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return notificationRow{}, err
	}
	list, err := a.notifications(c.R.Context(), `n.id = ? AND n.user_id = ?`, id, c.Actor.User.ID)
	if err != nil {
		return notificationRow{}, err
	}
	if len(list) == 0 {
		return notificationRow{}, notFound("notification")
	}
	return list[0], nil
}

// setRead marks a notification read or unread.
func (a *App) setRead(c *Ctx, id int64, read bool) error {
	return a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		var err error
		if read {
			_, err = t.Exec(`UPDATE notifications SET read_at = ?, unread_count = 0, updated_at = ? WHERE id = ?`, now, now, id)
		} else {
			_, err = t.Exec(`UPDATE notifications SET read_at = NULL, unread_count = 1, updated_at = ? WHERE id = ?`, now, id)
		}
		return err
	})
}

// answerNotification answers one notification as it is now.
func (a *App) answerNotification(c *Ctx, id int64) error {
	ctx := c.R.Context()
	list, err := a.notifications(ctx, `n.id = ?`, id)
	if err != nil {
		return err
	}
	out, err := a.notificationsJSON(ctx, list)
	if err != nil {
		return err
	}
	return c.JSON(200, O("summary", c.Summary, "notification", out[0], "path", out[0].Get("path")))
}

// showNotification reads a notification on the way to what it's about:
// the answer's path is where to go.
func (a *App) showNotification(c *Ctx) error {
	n, err := a.ownNotification(c)
	if err != nil {
		return err
	}
	if err := a.setRead(c, n.ID, true); err != nil {
		return err
	}
	c.Summary = "Read."
	return a.answerNotification(c, n.ID)
}

func (a *App) readNotification(c *Ctx) error {
	n, err := a.ownNotification(c)
	if err != nil {
		return err
	}
	if err := a.setRead(c, n.ID, true); err != nil {
		return err
	}
	c.Summary = "Marked read."
	return a.answerNotification(c, n.ID)
}

func (a *App) unreadNotification(c *Ctx) error {
	n, err := a.ownNotification(c)
	if err != nil {
		return err
	}
	if err := a.setRead(c, n.ID, false); err != nil {
		return err
	}
	c.Summary = "Marked unread."
	return a.answerNotification(c, n.ID)
}

func (a *App) readAllNotifications(c *Ctx) error {
	uid := c.Actor.User.ID
	var n int64
	err := a.write(c.R.Context(), c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		res, err := t.Exec(`UPDATE notifications SET read_at = ?, unread_count = 0, updated_at = ? WHERE user_id = ? AND read_at IS NULL`, now, now, uid)
		if err == nil {
			n, err = res.RowsAffected()
		}
		return err
	})
	if err != nil {
		return err
	}
	c.Summary = pluralize(int(n), "notification") + " marked read."
	return c.JSON(200, O("summary", c.Summary, "read", n))
}

// promptUsers are who can be @mentioned: the active people on staff (not
// agents), by name.
func (a *App) promptUsers(c *Ctx) error {
	rows, err := a.store.QueryContext(c.R.Context(), `SELECT id, name, email_address FROM users WHERE deactivated_at IS NULL AND agent_owner_id IS NULL ORDER BY name`)
	if err != nil {
		return err
	}
	me := c.Actor.User.Person().ID
	users, err := store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		var id int64
		var name, email string
		if err := r.Scan(&id, &name, &email); err != nil {
			return nil, err
		}
		display := name
		if display == "" {
			display = email
		}
		return O("id", id, "name", display, "first_name", firstName(display), "initials", nameInitials(display), "me", id == me), nil
	})
	if err != nil {
		return err
	}
	if users == nil {
		users = []Obj{}
	}
	// Plugins' names (an AI agent) beside the people, while they're on.
	return c.JSON(200, O("users", users, "mentionables", a.mentionablesFor(c.R.Context(), c.Actor.User.Person().Role)))
}
