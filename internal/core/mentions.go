package core

import (
	"context"
	"regexp"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Mentions (Fizzy's): rich text that @mentions someone on staff makes a
// mention of them at that record, once per person, and each new mention
// notifies its person (unless they mentioned themselves). The editor writes
// a mention as <span data-mention-id="12">@Ann</span>.
//
// Whoever saves rich text that can mention calls recordMentions after
// saving it, in the same write: notes here; an engagement's description and
// estimate notes, a piece of work's description, a request's body and an
// agreement version's reason in their areas.

// mentionSources are the records whose rich text can mention, with the
// columns that hold it (the Rails mentionable_fields).
var mentionSources = map[string]struct{ table, columns string }{
	"Note":             {"notes", "body"},
	"Engagement":       {"engagements", "description, estimate_notes"},
	"Todo":             {"todos", "description"},
	"Request":          {"requests", "body"},
	"AgreementVersion": {"agreement_versions", "reason"},
}

var (
	brOrBlockEnd = regexp.MustCompile(`(?i)<br\s*/?>|</p>|</li>`)
	manyNewlines = regexp.MustCompile(`\n+`)
)

func init() {
	// A record that goes takes its mentions, and their notifications, with it.
	for typ := range mentionSources {
		if typ == "Note" {
			continue // deleteNote does
		}
		onDelete(typ, func(t *Tx, id int64) error { return t.deleteMentions(typ, id) })
	}
}

// recordMentions makes a mention of each active person (not an agent) that
// html mentions at the record, as the write's actor, and notifies each one
// new to it. Mentions already made stay; nobody is notified twice for the
// same record.
func (t *Tx) recordMentions(sourceType string, id int64, html string) error {
	mentioner := t.Actor.UserID()
	ids := mentionedUserIDs(html)
	if mentioner == 0 || len(ids) == 0 {
		return nil
	}
	people, err := store.Int64s(t.Ctx, t, `SELECT id FROM users WHERE deactivated_at IS NULL AND agent_owner_id IS NULL AND id IN (`+store.Placeholders(len(ids))+`) ORDER BY id`, idArgs(ids)...)
	if err != nil {
		return err
	}
	for _, mentionee := range people {
		n, err := store.Count(t.Ctx, t, `SELECT count(*) FROM mentions WHERE source_type = ? AND source_id = ? AND mentionee_id = ?`, sourceType, id, mentionee)
		if err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		now := store.Now().DB()
		mentionID, err := t.Insert(`INSERT INTO mentions (source_type, source_id, mentioner_id, mentionee_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			sourceType, id, mentioner, mentionee, now, now)
		if err != nil {
			return err
		}
		if err := t.notifyMention(mentionID, mentioner, mentionee); err != nil {
			return err
		}
	}
	return nil
}

// deleteMentions removes a record's mentions and their notifications.
func (t *Tx) deleteMentions(sourceType string, id int64) error {
	if _, err := t.Exec(`DELETE FROM notifications WHERE source_type = 'Mention' AND source_id IN (SELECT id FROM mentions WHERE source_type = ? AND source_id = ?)`, sourceType, id); err != nil {
		return err
	}
	_, err := t.Exec(`DELETE FROM mentions WHERE source_type = ? AND source_id = ?`, sourceType, id)
	return err
}

// mentionableContent is a record's mentionable text, plain (what a
// notification and its email quote).
func mentionableContent(ctx context.Context, db store.Querier, sourceType string, id int64) string {
	src, ok := mentionSources[sourceType]
	if !ok {
		return ""
	}
	cols := strings.Split(src.columns, ", ")
	vals := make([]any, len(cols))
	ptrs := make([]*string, len(cols))
	for i := range cols {
		ptrs[i] = new(string)
		vals[i] = ptrs[i]
		cols[i] = "COALESCE(" + cols[i] + ", '')"
	}
	if err := db.QueryRowContext(ctx, `SELECT `+strings.Join(cols, ", ")+` FROM `+src.table+` WHERE id = ?`, id).Scan(vals...); err != nil {
		return ""
	}
	var parts []string
	for _, p := range ptrs {
		if s := plainText(richTextHTML(*p)); s != "" {
			parts = append(parts, s)
		}
	}
	return strings.Join(parts, " ")
}

// railsTruncate is s cut to n characters with an ellipsis, as Rails' truncate.
func railsTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-3]) + "..."
}
