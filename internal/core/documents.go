package core

import (
	"context"
	"errors"
	"fmt"
	"mime/multipart"
	"net/http"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Documents: files kept on a client, an engagement, a scope item, a piece
// of work or a request. Staff see them all; the client's portal shows only
// the ones marked client_visible. The file is an Active Storage blob
// attached as the document's "file".

func init() {
	routeSets = append(routeSets, (*App).documentRoutes)
	searchable("Document", func(ctx context.Context, db store.Querier, id int64) (SearchDoc, bool, error) {
		var d SearchDoc
		var typ string
		var subjectID int64
		err := db.QueryRowContext(ctx, `SELECT documentable_type, documentable_id, created_at FROM documents WHERE id = ?`, id).Scan(&typ, &subjectID, &d.CreatedAt)
		if isNoRows(err) {
			return d, false, nil
		} else if err != nil {
			return d, false, err
		}
		_ = db.QueryRowContext(ctx, `SELECT b.filename FROM active_storage_attachments x JOIN active_storage_blobs b ON b.id = x.blob_id
			WHERE x.record_type = 'Document' AND x.record_id = ? AND x.name = 'file' ORDER BY x.id DESC LIMIT 1`, id).Scan(&d.Title)
		d.ClientID, err = subjectClientID(ctx, db, typ, subjectID)
		return d, err == nil, err
	})
	for _, typ := range NoteSubjects {
		onDelete(typ, func(t *Tx, id int64) error {
			ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM documents WHERE documentable_type = ? AND documentable_id = ?`, typ, id)
			if err != nil {
				return err
			}
			for _, did := range ids {
				if err := t.deleteDocument(did); err != nil {
					return err
				}
			}
			return nil
		})
	}
}

func (a *App) documentRoutes(mux *http.ServeMux) {
	a.handle(mux, "POST /api/documents", a.createDocuments, tool("add_document", "Add a document to a record",
		note(`record: "Type:id" for a Client, Engagement, ScopeItem, Todo or Request. Files start internal; set_document_visibility shows one to the client.`),
		formField("record", `The record it goes on, "Type:id" (e.g. "Client:12").`),
		file("files", "The file.")))
	a.handle(mux, "PATCH /api/documents/{id}", a.updateDocument, tool("set_document_visibility", "Show or hide a document in the client’s portal",
		body("document", `{client_visible: boolean (required)}`, true),
		confirm("Making it visible shows the file to the client in their portal.")))
	a.handle(mux, "DELETE /api/documents/{id}", a.destroyDocument, tool("delete_document", "Delete a document"))
	// The file itself, for staff: a download, or ?disposition=inline to view it.
	a.handle(mux, "GET /api/documents/{id}/file", a.documentFile)
}

// documentRow is a document as read for answers.
type documentRow struct {
	ID, SubjectID, UploaderID, ByteSize int64
	SubjectType, Filename, ContentType  string
	UploaderName                        string
	ClientVisible                       bool
	CreatedAt                           store.Time
}

// documentsJSON are the documents matching where, latest first
// (documents/_document.json).
func (a *App) documentsJSON(ctx context.Context, where string, args ...any) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT d.id, d.documentable_type, d.documentable_id, COALESCE(d.uploaded_by_id, 0), COALESCE(NULLIF(u.name, ''), u.email_address, ''),
			d.client_visible, d.created_at, COALESCE(b.filename, ''), COALESCE(b.content_type, ''), COALESCE(b.byte_size, 0)
		FROM documents d LEFT JOIN users u ON u.id = d.uploaded_by_id
		LEFT JOIN active_storage_attachments x ON x.record_type = 'Document' AND x.record_id = d.id AND x.name = 'file'
		LEFT JOIN active_storage_blobs b ON b.id = x.blob_id
		WHERE `+where+` ORDER BY d.created_at DESC, d.id DESC`, args...)
	if err != nil {
		return nil, err
	}
	docs, err := store.ScanAll(rows, func(r *sqlRows) (documentRow, error) {
		var d documentRow
		err := r.Scan(&d.ID, &d.SubjectType, &d.SubjectID, &d.UploaderID, &d.UploaderName, &d.ClientVisible, &d.CreatedAt, &d.Filename, &d.ContentType, &d.ByteSize)
		return d, err
	})
	if err != nil {
		return nil, err
	}
	refs := make([]searchRef, len(docs))
	for i, d := range docs {
		refs[i] = searchRef{d.SubjectType, d.SubjectID}
	}
	infos, err := a.recordInfos(ctx, refs)
	if err != nil {
		return nil, err
	}
	out := make([]Obj, len(docs))
	for i, d := range docs {
		path := ""
		if info := infos[refs[i]]; info != nil {
			path = info.Path
		}
		var uploader any
		if d.UploaderID != 0 {
			uploader = O("id", d.UploaderID, "name", d.UploaderName)
		}
		out[i] = a.agentRef("Document", d.ID, "", d.Filename, path).
			Set("filename", d.Filename).Set("byte_size", d.ByteSize).Set("content_type", d.ContentType).
			Set("client_visible", d.ClientVisible).Set("created_at", d.CreatedAt).Set("uploaded_by", uploader).
			Set("download_url", fmt.Sprintf("%s/api/documents/%d/file", a.BaseURL(), d.ID)).
			Set("subject", O("type", d.SubjectType, "id", d.SubjectID))
	}
	return out, nil
}

// humanSize is a byte count as Rails' number_to_human_size says it.
func humanSize(n int64) string {
	units := []string{"Bytes", "KB", "MB", "GB", "TB"}
	f, i := float64(n), 0
	for f >= 1024 && i < len(units)-1 {
		f /= 1024
		i++
	}
	if i == 0 {
		return fmt.Sprintf("%d Bytes", n)
	}
	// Three significant digits, without trailing zeros: 100 MB, 1.5 MB.
	format := "%.2f"
	if f >= 100 {
		format = "%.0f"
	} else if f >= 10 {
		format = "%.1f"
	}
	s := fmt.Sprintf(format, f)
	if strings.Contains(s, ".") {
		s = strings.TrimSuffix(strings.TrimRight(s, "0"), ".")
	}
	return s + " " + units[i]
}

func (a *App) createDocuments(c *Ctx) error {
	ctx := c.R.Context()
	c.R.Body = http.MaxBytesReader(c.W, c.R.Body, MaxUpload()+(10<<20))
	if err := c.R.ParseMultipartForm(32 << 20); err != nil {
		if _, ok := errors.AsType[*http.MaxBytesError](err); ok {
			return &apierr.Error{Status: 422, Code: "refused", Message: "The upload is over the " + humanSize(MaxUpload()) + " limit."}
		}
		return apierr.BadRequest("Send the files as multipart/form-data.")
	}
	defer func() { _ = c.R.MultipartForm.RemoveAll() }() // best-effort: temp files
	record := c.R.FormValue("record")
	if strings.TrimSpace(record) == "" {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Choose where this goes."}
	}
	typ, subjectID, err := a.subjectRef(ctx, record, NoteSubjects)
	if err != nil {
		return err
	}
	var files []*multipart.FileHeader
	for _, key := range []string{"files", "files[]", "document[files]", "document[files][]", "file"} {
		files = append(files, c.R.MultipartForm.File[key]...)
	}
	files = slices.DeleteFunc(files, func(f *multipart.FileHeader) bool { return f.Filename == "" })
	if len(files) == 0 {
		return &apierr.Error{Status: 422, Code: "refused", Message: "Choose a file to upload."}
	}
	for _, f := range files {
		if f.Size > MaxUpload() {
			return &apierr.Error{Status: 422, Code: "invalid", Message: f.Filename + " is over the " + humanSize(MaxUpload()) + " limit."}
		}
	}
	var ids []int64
	var names []any
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		var uploader any
		if uid := c.Actor.UserID(); uid != 0 {
			uploader = uid
		}
		for _, fh := range files {
			f, err := fh.Open()
			if err != nil {
				return err
			}
			blob, err := t.putBlob(fh.Filename, fh.Header.Get("Content-Type"), f)
			f.Close()
			if errors.Is(err, errTooLarge) {
				return &apierr.Error{Status: 422, Code: "invalid", Message: fh.Filename + " is over the " + humanSize(MaxUpload()) + " limit."}
			} else if err != nil {
				return err
			}
			now := store.Now().DB()
			id, err := t.Insert(`INSERT INTO documents (documentable_type, documentable_id, uploaded_by_id, client_visible, created_at, updated_at) VALUES (?, ?, ?, 0, ?, ?)`,
				typ, subjectID, uploader, now, now)
			if err != nil {
				return err
			}
			if err := t.attach("Document", id, "file", blob); err != nil {
				return err
			}
			t.Reindex("Document", id)
			ids = append(ids, id)
			names = append(names, blob.Filename)
		}
		if slices.Contains(eventfulSubjects, typ) {
			return t.Event(typ, subjectID, "document.added", map[string]any{"files": names})
		}
		return nil
	})
	if err != nil {
		return err
	}
	docs, err := a.documentsJSON(ctx, `d.id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...)
	if err != nil {
		return err
	}
	c.Summary = pluralize(len(ids), "file") + " added."
	return c.JSON(201, O("summary", c.Summary, "documents", docs))
}

// document is one document; notFound when there's none.
func (a *App) document(ctx context.Context, id int64) (Obj, error) {
	list, err := a.documentsJSON(ctx, `d.id = ?`, id)
	if err != nil {
		return nil, err
	}
	if len(list) == 0 {
		return nil, notFound("document")
	}
	return list[0], nil
}

func (a *App) updateDocument(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	body, err := c.Wrapped("document")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	if _, err := a.document(ctx, id); err != nil {
		return err
	}
	visible := boolOf(body["client_visible"])
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		return t.update("documents", id, map[string]any{"client_visible": visible})
	})
	if err != nil {
		return err
	}
	d, err := a.document(ctx, id)
	if err != nil {
		return err
	}
	name := str(d.Get("filename"))
	if visible {
		c.Summary = "The client can see " + name + "."
	} else {
		c.Summary = name + " is internal."
	}
	return c.JSON(200, O("summary", c.Summary, "document", d))
}

// documentDeletable is whether the actor may delete a document: whoever
// uploaded it, or anyone who may delete records.
func (c *Ctx) documentDeletable(uploaderID int64) bool {
	return (c.Actor.User != nil && uploaderID != 0 && uploaderID == c.Actor.User.ID) || c.Can("delete_records")
}

func (a *App) destroyDocument(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	d, err := a.document(ctx, id)
	if err != nil {
		return err
	}
	var uploader int64
	if u, ok := d.Get("uploaded_by").(Obj); ok {
		uploader, _ = u.Get("id").(int64)
	}
	if !c.documentDeletable(uploader) {
		return forbidden()
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.deleteDocument(id) }); err != nil {
		return err
	}
	c.Summary = str(d.Get("filename")) + " removed."
	return c.JSON(200, O("summary", c.Summary, "deleted", true))
}

// deleteDocument removes a document and its file.
func (t *Tx) deleteDocument(id int64) error {
	if err := t.detach("Document", id, "file"); err != nil {
		return err
	}
	if _, err := t.Exec(`DELETE FROM documents WHERE id = ?`, id); err != nil {
		return err
	}
	t.Reindex("Document", id)
	return nil
}

func (a *App) documentFile(c *Ctx) error {
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	if n, err := store.Count(ctx, a.store, `SELECT count(*) FROM documents WHERE id = ?`, id); err != nil {
		return err
	} else if n == 0 {
		return notFound("document")
	}
	b, err := a.attachedBlob(ctx, a.store, "Document", id, "file")
	if err != nil {
		return err
	}
	if b == nil {
		return notFound("file")
	}
	a.serveBlob(c.W, c.R, b, c.Query("disposition") != "inline")
	return nil
}
