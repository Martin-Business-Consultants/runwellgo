package core

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"html"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// A plugin's notifications (the notify grant, plugin.API.Notify): one in
// the person's bell beside the core's mentions, a notifications row whose
// source is a plugin_notifications row holding the plugin's words and the
// path it opens. Unread ones of the same group collapse into one (its
// count goes up, its words are the latest), emailed only the first time.
// They show only while their plugin is on (visibleNotifications), so a
// plugin switched off or removed leaves the bell as it was.

const pluginNotificationSource = "PluginNotification"

// Notify puts n in its person's bell, and emails it when it asks.
func (e pluginEnv) Notify(ctx context.Context, key string, n plugin.Notification) error {
	a := e.a
	var active bool
	err := a.store.QueryRowContext(ctx, `SELECT deactivated_at IS NULL AND agent_owner_id IS NULL FROM users WHERE id = ?`, n.UserID).Scan(&active)
	if isNoRows(err) || (err == nil && !active) {
		return apierr.NotFound("There's no one active on staff with that id.")
	} else if err != nil {
		return err
	}
	creator := n.FromID
	if creator <= 0 {
		creator = n.UserID
	}
	var notificationID int64
	fresh := false
	err = a.write(ctx, nil, func(t *Tx) error {
		now := store.Now().DB()
		if n.Group != "" {
			var sourceID int64
			err := t.QueryRow(`SELECT n.id, n.source_id FROM notifications n JOIN plugin_notifications p ON p.id = n.source_id
				WHERE n.source_type = ? AND n.user_id = ? AND n.read_at IS NULL AND p.plugin_key = ? AND p.group_key = ? ORDER BY n.id DESC LIMIT 1`,
				pluginNotificationSource, n.UserID, key, n.Group).Scan(&notificationID, &sourceID)
			if err == nil {
				if _, err := t.Exec(`UPDATE plugin_notifications SET title = ?, body = ?, path = ?, source = ?, subject = ?, from_id = ?, updated_at = ? WHERE id = ?`,
					n.Title, n.Body, n.Path, n.Source, n.Subject, nullID(n.FromID), now, sourceID); err != nil {
					return err
				}
				_, err = t.Exec(`UPDATE notifications SET unread_count = unread_count + 1, creator_id = ?, updated_at = ? WHERE id = ?`, creator, now, notificationID)
				return err
			} else if !isNoRows(err) {
				return err
			}
		}
		sourceID, err := t.Insert(`INSERT INTO plugin_notifications (plugin_key, user_id, from_id, title, body, path, source, subject, group_key, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, key, n.UserID, nullID(n.FromID), n.Title, n.Body, n.Path, n.Source, n.Subject, n.Group, now, now)
		if err != nil {
			return err
		}
		notificationID, err = t.Insert(`INSERT INTO notifications (user_id, creator_id, source_type, source_id, read_at, unread_count, created_at, updated_at) VALUES (?, ?, ?, ?, NULL, 1, ?, ?)`,
			n.UserID, creator, pluginNotificationSource, sourceID, now, now)
		fresh = true
		return err
	})
	if err != nil {
		return err
	}
	a.navChanged(n.UserID)
	if fresh && n.Email {
		if m, err := a.pluginNotificationMail(ctx, notificationID); err != nil {
			a.log.Warn("plugin notification mail", "plugin", key, "err", err)
		} else if m != nil {
			a.mailLater(*m)
		}
	}
	return nil
}

func nullID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// RichText cleans HTML the core's way (always granted).
func (e pluginEnv) RichText(s string) plugin.RichText {
	clean := sanitizeRichText(s)
	return plugin.RichText{HTML: clean, Text: strings.TrimSpace(plainText(clean)), Mentions: mentionedUserIDs(clean), Mentionables: mentionedPluginNames(clean)}
}

// visibleNotifications is the condition that keeps a plugin's
// notifications out while it's off (n is the notifications table).
func (a *App) visibleNotifications() string {
	keys := []string{}
	for _, p := range a.enabledPlugins() {
		if p.Granted.Notify {
			keys = append(keys, "'"+p.Key+"'") // keys are [a-z0-9_]
		}
	}
	if len(keys) == 0 {
		return "n.source_type <> '" + pluginNotificationSource + "'"
	}
	return "(n.source_type <> '" + pluginNotificationSource + "' OR n.source_id IN (SELECT id FROM plugin_notifications WHERE plugin_key IN (" + strings.Join(keys, ", ") + ")))"
}

// pluginNotice is a plugin notification's own words.
type pluginNotice struct {
	Title, Body, Path, Source, Subject string
}

// pluginNotices are the plugin notifications by id, in one read.
func (a *App) pluginNotices(ctx context.Context, ids []int64) (map[int64]pluginNotice, error) {
	out := map[int64]pluginNotice{}
	if len(ids) == 0 {
		return out, nil
	}
	rows, err := a.store.QueryContext(ctx, `SELECT id, title, COALESCE(body, ''), path, COALESCE(source, ''), COALESCE(subject, '') FROM plugin_notifications WHERE id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p pluginNotice
		if err := rows.Scan(&id, &p.Title, &p.Body, &p.Path, &p.Source, &p.Subject); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// pluginNotificationMail is the email for a plugin's notification: its
// words and a link that opens it (reading it on the way).
func (a *App) pluginNotificationMail(ctx context.Context, id int64) (*mail.Message, error) {
	var to, title, body string
	err := a.store.QueryRowContext(ctx, `SELECT u.email_address, p.title, COALESCE(p.body, '') FROM notifications n JOIN users u ON u.id = n.user_id
		JOIN plugin_notifications p ON p.id = n.source_id WHERE n.id = ? AND n.source_type = ?`, id, pluginNotificationSource).Scan(&to, &title, &body)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	brand := a.BrandName(ctx)
	url := fmt.Sprintf("%s/notifications/%d", a.BaseURL(), id)
	excerpt := railsTruncate(body, 250)
	text := brand + "\n" + strings.Repeat("=", len([]rune(brand))) + "\n\n" + title + "\n\n"
	if excerpt != "" {
		text += "> " + excerpt + "\n\n"
	}
	text += "Open it: " + url + "\n\n--\nYou're getting this because of your notification settings in " + brand + ".\n"
	color := s.BrandColor()
	esc := html.EscapeString
	quote := ""
	if excerpt != "" {
		quote = `<div style="border-left:3px solid ` + color + `;color:#3a3b40;margin:0 0 16px;padding:4px 0 4px 14px;">` + esc(excerpt) + `</div>`
	}
	page := `<!doctype html><html><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"></head>` +
		`<body style="margin:0;padding:0;background:#f4f4f5;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;">` +
		`<div style="max-width:560px;margin:0 auto;padding:32px 24px;">` +
		`<p style="color:#16171a;font-size:15px;font-weight:700;margin:0 0 24px;">` + esc(brand) + `</p>` +
		`<div style="background:#ffffff;border-radius:12px;padding:32px;color:#3a3b40;font-size:16px;line-height:1.5;">` +
		`<h1 style="color:#16171a;font-size:22px;font-weight:700;line-height:1.3;margin:0 0 16px;">` + esc(title) + `</h1>` + quote +
		`<p style="margin:24px 0 8px;"><a href="` + esc(url) + `" style="display:inline-block;padding:12px 24px;border-radius:8px;background:` + color + `;color:#ffffff;font-weight:600;text-decoration:none;">Open it</a></p>` +
		`</div><p style="color:#8a8c93;font-size:13px;line-height:1.5;margin:16px 0 0;">You’re getting this because of your notification settings in ` + esc(brand) + `.</p></div></body></html>`
	return &mail.Message{To: []string{to}, Subject: title, Text: text, HTML: page}, nil
}
