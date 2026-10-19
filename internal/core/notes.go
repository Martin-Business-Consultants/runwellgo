package core

import (
	"context"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Notes: what was said or decided, kept on a client, an engagement, a scope
// item, a piece of work or a request. A note's rich text can @mention
// people, who are notified.

// NoteKinds are a note's kinds.
var NoteKinds = []string{"call", "meeting", "email", "internal"}

// NoteSubjects are the records notes and documents are kept on.
var NoteSubjects = []string{"Client", "Engagement", "ScopeItem", "Todo", "Request"}

// eventfulSubjects are the subjects whose history records a note or a
// document added (the Rails models with record_event!).
var eventfulSubjects = []string{"Client", "Engagement", "Todo", "Request"}

func init() {
	routeSets = append(routeSets, (*App).noteRoutes)
	searchable("Note", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		var kind, subjectType string
		var subjectID int64
		err := db.QueryRowContext(ctx, `SELECT kind, body, subject_type, subject_id, created_at FROM notes WHERE id = ?`, id).
			Scan(&kind, &d.Content, &subjectType, &subjectID, &d.CreatedAt)
		if isNoRows(err) {
			return d, false, nil
		} else if err != nil {
			return d, false, err
		}
		d.Title = humanize(kind) + " note"
		d.ClientID, err = subjectClientID(ctx, db, subjectType, subjectID)
		return d, err == nil, err
	})
	// A client's and an engagement's answers carry their 20 latest notes; the
	// others' all of them (the Rails show.json views).
	for _, typ := range NoteSubjects {
		limit := 0
		if typ == "Client" || typ == "Engagement" {
			limit = 20
		}
		addSection(typ, "notes", func(a *App, ctx context.Context, id int64) (any, error) {
			return a.notesJSON(ctx, typ, id, limit)
		})
		// Documents follow notes, as on the Rails pages (documents.go keeps them).
		addSection(typ, "documents", func(a *App, ctx context.Context, id int64) (any, error) {
			return a.documentsJSON(ctx, `d.documentable_type = ? AND d.documentable_id = ?`, typ, id)
		})
		onDelete(typ, func(t *Tx, id int64) error {
			ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM notes WHERE subject_type = ? AND subject_id = ?`, typ, id)
			if err != nil {
				return err
			}
			for _, nid := range ids {
				if err := t.deleteNote(nid); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func (a *App) noteRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/notes", a.createNote, tool("add_note", "Add a note to a record",
		note(`record: "Type:id" for a Client, Engagement, ScopeItem, Todo or Request (e.g. "Client:12"). @mention someone by writing their name; they are notified.`),
		bodyParam("record", "string", `The record it goes on, "Type:id" (e.g. "Client:12").`, true),
		body("note", `{body (required, rich text or plain), kind: call|meeting|email|internal, source, occurred_at}`, true)))
	a.handle(mux, "DELETE /api/notes/{id}", a.destroyNote, tool("delete_note", "Delete a note",
		note("Only its author (or the author's person, for a note their agent wrote) or someone who may delete records.")))
}

// subjectRef is a record named "Type:id", one of types; a 400 for another
// type, a 404 when there's no such record (the Rails QuickAction.locate).
func (a *App) subjectRef(ctx context.Context, value string, types []string) (string, int64, error) {
	typ, rawID, _ := strings.Cut(strings.TrimSpace(value), ":")
	if !slices.Contains(types, typ) {
		return "", 0, apierr.BadRequest(`record must be "Type:id" for a ` + strings.Join(types, ", "))
	}
	id, err := strconv.ParseInt(strings.TrimSpace(rawID), 10, 64)
	if err != nil {
		return "", 0, apierr.NotFound("Not found: it doesn’t exist, or it was removed.")
	}
	n, err := store.Count(ctx, a.store, `SELECT count(*) FROM `+searchTable(typ)+` WHERE id = ?`, id)
	if err != nil {
		return "", 0, err
	}
	if n == 0 {
		return "", 0, apierr.NotFound("Not found: it doesn’t exist, or it was removed.")
	}
	return typ, id, nil
}

// subjectClientID is the client a record belongs to, 0 for none.
func subjectClientID(ctx context.Context, db store.Querier, typ string, id int64) (int64, error) {
	var q string
	switch typ {
	case "Client":
		return id, nil
	case "Engagement", "Request", "Commitment", "Contact":
		q = `SELECT COALESCE(client_id, 0) FROM ` + searchTable(typ) + ` WHERE id = ?`
	case "Todo":
		q = `SELECT e.client_id FROM todos t JOIN engagements e ON e.id = t.engagement_id WHERE t.id = ?`
	case "ScopeItem":
		q = `SELECT e.client_id FROM scope_items s JOIN agreement_versions v ON v.id = s.agreement_version_id JOIN engagements e ON e.id = v.engagement_id WHERE s.id = ?`
	case "AgreementVersion":
		q = `SELECT e.client_id FROM agreement_versions v JOIN engagements e ON e.id = v.engagement_id WHERE v.id = ?`
	default:
		return 0, nil
	}
	var cid int64
	err := db.QueryRowContext(ctx, q, id).Scan(&cid)
	if isNoRows(err) {
		return 0, nil
	}
	return cid, err
}

// noteRow is a note as read for answers.
type noteRow struct {
	ID, SubjectID, AuthorID, AuthorOwnerID int64
	SubjectType, Body, Kind, Source        string
	AuthorName                             string
	OccurredAt                             store.Time
	// WrittenAs is the plugin's mentionable that wrote it ("ai:7"), for a
	// reply to a mention (plugin_mentions.go).
	WrittenAs string
}

// notesJSON are a record's notes, latest first (notes/_note.json); limit 0
// is all of them.
func (a *App) notesJSON(ctx context.Context, typ string, id int64, limit int) ([]Obj, error) {
	q := `SELECT n.id, n.subject_type, n.subject_id, n.body, n.kind, n.source, n.occurred_at, COALESCE(n.author_id, 0),
			COALESCE(NULLIF(u.name, ''), u.email_address, ''), COALESCE(u.agent_owner_id, 0), COALESCE(n.written_as, '')
		FROM notes n LEFT JOIN users u ON u.id = n.author_id
		WHERE n.subject_type = ? AND n.subject_id = ? ORDER BY n.occurred_at DESC, n.id DESC`
	args := []any{typ, id}
	if limit > 0 {
		q += ` LIMIT ?`
		args = append(args, limit)
	}
	return a.queryNotes(ctx, q, args...)
}

func (a *App) queryNotes(ctx context.Context, q string, args ...any) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	notes, err := store.ScanAll(rows, func(r *sqlRows) (noteRow, error) {
		var n noteRow
		err := r.Scan(&n.ID, &n.SubjectType, &n.SubjectID, &n.Body, &n.Kind, &n.Source, &n.OccurredAt, &n.AuthorID, &n.AuthorName, &n.AuthorOwnerID, &n.WrittenAs)
		return n, err
	})
	if err != nil {
		return nil, err
	}
	paths, err := a.recordPaths(ctx, notes)
	if err != nil {
		return nil, err
	}
	out := make([]Obj, len(notes))
	for i, n := range notes {
		out[i] = a.noteJSON(n, paths[searchRef{n.SubjectType, n.SubjectID}])
	}
	return out, nil
}

// recordPaths are where the notes' subjects open.
func (a *App) recordPaths(ctx context.Context, notes []noteRow) (map[searchRef]string, error) {
	refs := make([]searchRef, 0, len(notes))
	for _, n := range notes {
		refs = append(refs, searchRef{n.SubjectType, n.SubjectID})
	}
	infos, err := a.recordInfos(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := map[searchRef]string{}
	for ref, info := range infos {
		out[ref] = info.Path
	}
	return out, nil
}

// noteJSON is a note as answers show it: the agent's plain-text body, and
// the body as HTML for the app.
func (a *App) noteJSON(n noteRow, path string) Obj {
	var author any
	if n.AuthorID != 0 {
		author = O("id", n.AuthorID, "name", n.AuthorName)
	}
	source, owner := n.Source, nullableID(n.AuthorOwnerID)
	if n.WrittenAs != "" {
		author, source, owner = a.writtenAs(n, author)
	}
	return a.agentRef("Note", n.ID, "", humanize(n.Kind)+" note", path).
		Set("kind", n.Kind).Set("source", source).Set("occurred_at", n.OccurredAt).
		Set("author", author).Set("body", agentTextOrNil(richTextHTML(n.Body))).Set("body_html", richTextHTML(n.Body)).
		Set("subject", O("type", n.SubjectType, "id", n.SubjectID)).
		Set("author_agent_owner_id", owner)
}

func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// agentText is a rich text value as plain text for a model to read: line
// breaks where paragraphs, items and breaks end (the Rails agent_text); nil
// for none.
func agentTextOrNil(html string) any {
	if strings.TrimSpace(html) == "" {
		return nil
	}
	s := brOrBlockEnd.ReplaceAllString(html, "\n")
	s = tags.ReplaceAllString(s, "")
	s = strings.NewReplacer("&nbsp;", " ", "&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'").Replace(s)
	s = manyNewlines.ReplaceAllString(s, "\n")
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	return s
}

func (a *App) createNote(c *Ctx) error {
	ctx := c.R.Context()
	var body map[string]any
	if err := c.Body(&body); err != nil {
		return err
	}
	if strings.TrimSpace(str(body["record"])) == "" {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Choose where this goes."}
	}
	typ, subjectID, err := a.subjectRef(ctx, str(body["record"]), NoteSubjects)
	if err != nil {
		return err
	}
	fields, ok := body["note"].(map[string]any)
	if !ok {
		return apierr.BadRequest("param is missing or the value is empty: note")
	}
	html := sanitizeRichText(str(fields["body"]))
	kind := str(fields["kind"])
	if kind == "" {
		kind = "internal"
	}
	source := strings.TrimSpace(str(fields["source"]))
	if source == "" {
		source = c.Actor.Source()
	}
	occurred := store.Now()
	if t, err := store.ParseTime(str(fields["occurred_at"])); err == nil && t.Valid {
		occurred = t
	}
	inv := apierr.Invalid{}
	if plainText(html) == "" && !strings.Contains(html, "<img") {
		inv.Add("body", "can't be blank")
	}
	if !slices.Contains(NoteKinds, kind) {
		inv.Add("kind", "is not included in the list")
	}
	if err := inv.Err(); err != nil {
		return err
	}
	var id int64
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		now := store.Now().DB()
		var author any
		if uid := c.Actor.UserID(); uid != 0 {
			author = uid
		}
		id, err = t.Insert(`INSERT INTO notes (subject_type, subject_id, body, kind, occurred_at, author_id, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			typ, subjectID, html, kind, occurred.DB(), author, source, now, now)
		if err != nil {
			return err
		}
		t.Reindex("Note", id)
		if err := t.recordMentions("Note", id, html); err != nil {
			return err
		}
		t.relayMentions(typ, subjectID, id, html)
		if slices.Contains(eventfulSubjects, typ) {
			return t.Event(typ, subjectID, "note.added", map[string]any{"kind": kind})
		}
		return nil
	})
	if err != nil {
		return err
	}
	list, err := a.queryNotes(ctx, `SELECT n.id, n.subject_type, n.subject_id, n.body, n.kind, n.source, n.occurred_at, COALESCE(n.author_id, 0),
			COALESCE(NULLIF(u.name, ''), u.email_address, ''), COALESCE(u.agent_owner_id, 0), COALESCE(n.written_as, '')
		FROM notes n LEFT JOIN users u ON u.id = n.author_id WHERE n.id = ?`, id)
	if err != nil {
		return err
	}
	c.Summary = "Note added."
	return c.JSON(201, O("summary", c.Summary, "note", list[0]))
}

// noteDeletableBy is whether user may delete a note: its author, the
// author's person (for a note their agent wrote), or anyone who may delete
// records.
func (c *Ctx) noteDeletable(authorID, authorOwnerID int64) bool {
	u := c.Actor.User
	if u == nil {
		return false
	}
	return (authorID != 0 && authorID == u.ID) || (authorOwnerID != 0 && authorOwnerID == u.ID) || c.Can("delete_records")
}

func (a *App) destroyNote(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	var subjectType, kind string
	var subjectID, authorID, ownerID int64
	err = a.store.QueryRowContext(ctx, `SELECT n.subject_type, n.subject_id, n.kind, COALESCE(n.author_id, 0), COALESCE(u.agent_owner_id, 0)
		FROM notes n LEFT JOIN users u ON u.id = n.author_id WHERE n.id = ?`, id).Scan(&subjectType, &subjectID, &kind, &authorID, &ownerID)
	if isNoRows(err) {
		return notFound("note")
	} else if err != nil {
		return err
	}
	if !c.noteDeletable(authorID, ownerID) {
		return &apierr.Error{Status: 403, Code: "forbidden", Message: "Only its author can delete this note."}
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if err := t.deleteNote(id); err != nil {
			return err
		}
		if slices.Contains(eventfulSubjects, subjectType) {
			return t.Event(subjectType, subjectID, "note.deleted", map[string]any{"kind": kind})
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.Summary = "Note deleted."
	return c.JSON(200, O("summary", c.Summary, "deleted", true))
}

// deleteNote removes a note with its mentions and their notifications.
func (t *Tx) deleteNote(id int64) error {
	if err := t.deleteMentions("Note", id); err != nil {
		return err
	}
	if _, err := t.Exec(`DELETE FROM notes WHERE id = ?`, id); err != nil {
		return err
	}
	t.Reindex("Note", id)
	return nil
}
