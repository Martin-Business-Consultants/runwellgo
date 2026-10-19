package core

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Plugins' mentionables (the mentionables grant, plugin/mentions.go):
// names that aren't people (an AI agent) in every @ picker while their
// plugin is on, written <span data-mentionable="ai:7">@Researcher</span>.
// A note that mentions one is kept in plugin_mentions and heard by its
// plugin as mention.created, with the note's author as the call's caller;
// its reply is a note on the same record written as the mentionable
// (notes.written_as), its author the person who mentioned it, so it reads
// "Researcher · via AI for Ted" and they may delete it. A plugin granted
// mentions:relay passes on mentions in its own content the same way, and
// takes the replies itself. With the plugin off, the names leave the
// pickers, nothing is heard, and its notes read as the person's with the
// byline they were written with.

var mentionableRef = regexp.MustCompile(`data-mentionable="([a-z][a-z0-9_]*:[A-Za-z0-9_.-]+)"`)

// mentionedPluginNames are the mentionables rich text mentions, each once.
func mentionedPluginNames(html string) []string {
	var out []string
	for _, m := range mentionableRef.FindAllStringSubmatch(html, -1) {
		if !slices.Contains(out, m[1]) {
			out = append(out, m[1])
		}
	}
	return out
}

// relayMentions hands a new note's mentions of plugins' mentionables to
// their plugins, once the note is saved.
func (t *Tx) relayMentions(subjectType string, subjectID, noteID int64, html string) {
	names := mentionedPluginNames(html)
	u := t.Actor.userOrNil()
	if len(names) == 0 || u == nil {
		return
	}
	a, actor := t.App, t.Actor
	t.AfterCommit(func(ctx context.Context) {
		for _, name := range names {
			m, owner := a.plugins.Mentionable(ctx, name)
			if m == nil || !m.MentionedBy(u.Person().Role) {
				continue
			}
			ev, caller, err := a.recordMention(ctx, owner, "note", actor, plugin.Mention{Mentionable: name, HTML: html}, subjectType, subjectID, noteID)
			if err != nil {
				a.log.Warn("plugin mention", "plugin", owner, "err", err)
				continue
			}
			a.plugins.DeliverMention(ctx, owner, ev, caller, actor)
		}
	})
}

func (a *Actor) userOrNil() *User {
	if a == nil || a.User == nil || !a.User.Active() {
		return nil
	}
	return a.User
}

// RecordMention keeps a mention relayed from source's own content.
func (e pluginEnv) RecordMention(ctx context.Context, owner, source string, actor plugin.Actor, m plugin.Mention) (plugin.MentionEvent, *plugin.Caller, error) {
	who, _ := actor.(*Actor)
	return e.a.recordMention(ctx, owner, source, who, m, "", 0, 0)
}

// recordMention keeps a mention and makes what its plugin hears of it,
// with the person who made it as the caller.
func (a *App) recordMention(ctx context.Context, owner, source string, actor *Actor, m plugin.Mention, subjectType string, subjectID, noteID int64) (plugin.MentionEvent, *plugin.Caller, error) {
	u := actor.userOrNil()
	if u == nil {
		return plugin.MentionEvent{}, nil, &apierr.Error{Status: 403, Code: "forbidden", Message: "Only someone on staff can mention it."}
	}
	_, localID, _ := strings.Cut(m.Mentionable, ":")
	clean := sanitizeRichText(m.HTML)
	if m.Text == "" {
		m.Text = strings.TrimSpace(plainText(clean))
	}
	ev := plugin.MentionEvent{Mentionable: localID, Source: source, Author: plugin.Person{ID: u.Person().ID, Name: u.Person().DisplayName()},
		Text: m.Text, HTML: clean, Path: m.Path, Subject: m.Subject, Context: m.Context, At: store.Now().Time.UTC().Format("2006-01-02T15:04:05Z")}
	if subjectType != "" {
		ev.Record, ev.NoteID = fmt.Sprintf("%s:%d", subjectType, subjectID), noteID
		if infos, err := a.recordInfos(ctx, []searchRef{{subjectType, subjectID}}); err == nil {
			if info := infos[searchRef{subjectType, subjectID}]; info != nil {
				ev.Path, ev.Subject = info.Path, info.Display
			}
		}
	}
	res, err := a.store.ExecContext(ctx, `INSERT INTO plugin_mentions (plugin_key, mentionable_id, source, subject_type, subject_id, note_id, ref, author_id, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		owner, localID, source, nullStr(subjectType), nullableID(subjectID), nullableID(noteID), nullStr(m.Ref), u.Person().ID, store.Now().DB())
	if err != nil {
		return ev, nil, err
	}
	ev.ID, _ = res.LastInsertId()
	return ev, a.pluginCallerFor(u, actor.AppLabel()), nil
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// pluginCallerFor is a person as a plugin sees its caller, outside a
// request (a mention's).
func (a *App) pluginCallerFor(u *User, via string) *plugin.Caller {
	p := u.Person()
	caps := []string{}
	for _, perm := range a.permissions() {
		if a.can(p.Role, perm.Key) {
			caps = append(caps, perm.Key)
		}
	}
	c := &plugin.Caller{ID: p.ID, Name: p.DisplayName(), Role: p.Role, Agent: u.IsAgent(), Capabilities: caps, Via: via}
	if u.IsAgent() {
		c.AgentID = u.ID
	}
	return c
}

// ReplyToMention answers a mention of one of key's mentionables where it
// was made: a note on the record (a new one, or the one it replaces), or
// the relaying plugin's own reply.
func (e pluginEnv) ReplyToMention(ctx context.Context, key string, r plugin.Reply) (string, error) {
	a := e.a
	var mentionable, source, subjectType, ref string
	var subjectID, authorID int64
	err := a.store.QueryRowContext(ctx, `SELECT mentionable_id, source, COALESCE(subject_type, ''), COALESCE(subject_id, 0), COALESCE(ref, ''), author_id
		FROM plugin_mentions WHERE id = ? AND plugin_key = ?`, r.Mention, key).Scan(&mentionable, &source, &subjectType, &subjectID, &ref, &authorID)
	if isNoRows(err) {
		return "", apierr.NotFound("There's no mention of this plugin's with that id.")
	} else if err != nil {
		return "", err
	}
	full := key + ":" + mentionable
	as, _ := a.plugins.Mentionable(ctx, full)
	if as == nil {
		return "", apierr.Conflict("It isn't offered any more, so it can't reply.")
	}
	as.ID = full
	author, err := a.loadUser(ctx, authorID)
	if err != nil || author == nil {
		return "", apierr.NotFound("The person who mentioned it isn't here any more.")
	}
	via := key
	if p := a.plugins.Get(key); p != nil {
		via = p.Name
	}
	html := richTextHTML(sanitizeRichText(r.Body))
	if plainText(html) == "" {
		return "", apierr.BadRequest("A reply says something.")
	}
	if source != "note" {
		return a.plugins.ReplyTo(ctx, source, plugin.MentionReply{Ref: ref, As: *as, Via: via, For: plugin.Person{ID: author.ID, Name: author.DisplayName()},
			HTML: html, Text: strings.TrimSpace(plainText(html)), Replace: r.Replace})
	}
	byline := fmt.Sprintf("%s · via %s for %s", as.Name, via, firstName(author.DisplayName()))
	var noteID int64
	err = a.write(ctx, &Actor{User: author}, func(t *Tx) error {
		now := store.Now().DB()
		if r.Replace != "" {
			id, _ := strconv.ParseInt(r.Replace, 10, 64)
			res, err := t.Exec(`UPDATE notes SET body = ?, source = ?, updated_at = ? WHERE id = ? AND written_as = ? AND subject_type = ? AND subject_id = ?`,
				html, byline, now, id, full, subjectType, subjectID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 1 {
				noteID = id
				t.Reindex("Note", id)
				return nil
			}
		}
		noteID, err = t.Insert(`INSERT INTO notes (subject_type, subject_id, body, kind, occurred_at, author_id, source, written_as, created_at, updated_at) VALUES (?, ?, ?, 'internal', ?, ?, ?, ?, ?, ?)`,
			subjectType, subjectID, html, now, author.ID, byline, full, now, now)
		if err != nil {
			return err
		}
		t.Reindex("Note", noteID)
		if slices.Contains(eventfulSubjects, subjectType) {
			return t.Event(subjectType, subjectID, "note.added", map[string]any{"kind": "internal", "plugin": key, "mentionable": full})
		}
		return nil
	})
	if err != nil {
		return "", err
	}
	return strconv.FormatInt(noteID, 10), nil
}

// writtenAs is who wrote a note a plugin's mentionable wrote: it, while
// its plugin offers it (else the person, with the byline it was written
// with), the byline's "via" part, and the person whose it is.
func (a *App) writtenAs(n noteRow, person any) (any, string, any) {
	m, key := a.plugins.Mentionable(context.Background(), n.WrittenAs)
	if m == nil {
		return person, n.Source, nil
	}
	via := key
	if p := a.plugins.Get(key); p != nil {
		via = p.Name
	}
	return O("id", n.WrittenAs, "name", m.Name, "handle", m.Handle, "avatar_url", nullStr(m.Avatar), "kind", m.Kind),
		"via " + via + " for " + firstName(n.AuthorName), nullableID(n.AuthorID)
}

// mentionablesFor are the plugins' names role may mention, for the @
// pickers.
func (a *App) mentionablesFor(ctx context.Context, role string) []Obj {
	out := []Obj{}
	for _, m := range a.plugins.Mentionables(ctx) {
		if !m.MentionedBy(role) {
			continue
		}
		out = append(out, O("id", m.ID, "handle", m.Handle, "name", m.Name, "description", nullStr(m.Description),
			"avatar_url", nullStr(m.Avatar), "kind", m.Kind, "paused", m.Paused, "initials", nameInitials(m.Name)))
	}
	return out
}
