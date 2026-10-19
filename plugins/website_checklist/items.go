package websitechecklist

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/url"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// A website's steps (the Rails ItemsController and Items::*): a custom one
// added or removed, a value recorded, confirmed, reopened or marked not
// applicable, and files kept on each.

// siteAndItem are the site (site_id) and its step (the query's idParam)
// a call is about; a response when either isn't there.
func siteAndItem(ctx context.Context, api *plugin.API, req *plugin.Request, idParam string) (*Site, *Item, *plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "site_id"))
	if errors.Is(err, errNotFound) {
		return nil, nil, missing("website"), nil
	} else if err != nil {
		return nil, nil, nil, err
	}
	i, err := findItem(ctx, api, s.ID, queryID(req, idParam))
	if errors.Is(err, errNotFound) {
		return nil, nil, missing("checklist item"), nil
	} else if err != nil {
		return nil, nil, nil, err
	}
	return s, i, nil, nil
}

// errBlankTitle rolls back a step's save whose title was emptied.
var errBlankTitle = errors.New("title can't be blank")

// addItem adds a step of the team's own, under Custom (add_website_item).
func addItem(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := findSite(ctx, api, queryID(req, "site_id"))
	if err != nil {
		return failedSite(err)
	}
	p := wrapped(params(req), "item")
	i := &Item{SiteID: s.ID, Group: "Custom", State: "unconfirmed", Custom: true,
		Title: strings.TrimSpace(textOf(p, "title")), Detail: textOf(p, "detail"),
		Value: strings.TrimSpace(textOf(p, "value")), Note: strings.TrimSpace(textOf(p, "note"))}
	if i.Title == "" {
		return refuse(http.StatusUnprocessableEntity, "Title can't be blank"), nil
	}
	hexKey := make([]byte, 4)
	_, _ = rand.Read(hexKey)
	i.Key = "custom:" + hex.EncodeToString(hexKey)
	err = api.Tx(ctx, func(tx plugin.Querier) error {
		var max int
		if err := tx.QueryRow(ctx, `SELECT COALESCE(max(position), 0) FROM website_checklist_items WHERE site_id = ?`, s.ID).Scan(&max); err != nil {
			return err
		}
		i.Position = max + 1
		now := store.Now().DB()
		res, err := tx.Exec(ctx, `INSERT INTO website_checklist_items (site_id, "key", "group", title, detail, value, note, custom, position, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`,
			s.ID, i.Key, i.Group, i.Title, nullable(i.Detail), nullable(i.Value), nullable(i.Note), i.Position, now, now)
		if err != nil {
			return err
		}
		i.ID, _ = res.LastInsertId()
		if err := touchSite(ctx, tx, s.ID); err != nil {
			return err
		}
		var d diff
		d.add("group", "", i.Group)
		d.add("title", "", i.Title)
		d.add("detail", "", i.Detail)
		d.add("value", "", i.Value)
		d.add("note", "", i.Note)
		d.add("custom", "", "Yes")
		return audit(ctx, tx, req.Actor, auditEntry{SiteID: s.ID, SiteName: s.Name, ItemID: i.ID, RecordType: "Item", RecordID: i.ID, RecordLabel: i.Title, Action: "created", Diff: d})
	})
	if err != nil {
		return nil, err
	}
	res, err := answerSite(ctx, api, req, s.ID, http.StatusCreated, fmt.Sprintf("Added “%s” to %s.", i.Title, s.Name))
	return withItem(res, i.ID), err
}

// withItem names the step a write was about in its answer, for the page to
// show.
func withItem(res *plugin.Response, id int64) *plugin.Response {
	if res == nil || res.Status >= 300 {
		return res
	}
	res.Body = append(bytes.TrimSuffix(res.Body, []byte("}")), []byte(fmt.Sprintf(`,"item_id":%d}`, id))...)
	return res
}

// webhookRows reads the Webhooks step's rows, as a list or a form's rows
// by index.
func webhookRows(v any) []webhookChange {
	var out []webhookChange
	for _, r := range rows(v) {
		w := webhookChange{ID: intOf(r["id"]), Name: textOf(r, "name"), URL: textOf(r, "url"), Destroy: truthy(r["_destroy"])}
		for _, p := range rows(r["pairs"]) {
			w.Pairs = append(w.Pairs, Pair{Key: textOf(p, "key"), Value: textOf(p, "value")})
		}
		out = append(out, w)
	}
	return out
}

// updateItem records a step (update_website_item): a value saved is
// confirmed as the saver's (Item#record); a note, or a custom step's title,
// alone is only revised.
func updateItem(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, before, res, err := siteAndItem(ctx, api, req, "id")
	if res != nil || err != nil {
		return res, err
	}
	p := wrapped(params(req), "item")
	ch := itemChange{}
	ch.Value, _ = text(p, "value")
	ch.Note, _ = text(p, "note")
	ch.Title, _ = text(p, "title")
	ch.Detail, _ = text(p, "detail")
	hooksRaw, hasHooks := p["webhooks_attributes"]
	recording := ch.Value != nil || hasHooks
	after := *before
	err = api.Tx(ctx, func(tx plugin.Querier) error {
		if hasHooks && after.webhooks() {
			if _, err := saveWebhooks(ctx, tx, req.Actor, s, &after, webhookRows(hooksRaw)); err != nil {
				return err
			}
			ch.Value = nil
		}
		assignRevision(&after, ch, before.Value)
		if after.Title == "" {
			return errBlankTitle
		}
		if recording {
			record(&after, req.Actor)
		}
		return saveItem(ctx, tx, req.Actor, s, before, &after)
	})
	if errors.Is(err, errBlankTitle) {
		return plugin.Invalid(map[string][]string{"title": {"can't be blank"}}), nil
	} else if err != nil {
		return nil, err
	}
	res, err = answerSite(ctx, api, req, s.ID, http.StatusOK, fmt.Sprintf("“%s” saved.", after.Title))
	return withItem(res, after.ID), err
}

// deleteItem removes a custom step (delete_website_item); the standard
// ones are marked not applicable instead.
func deleteItem(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, i, res, err := siteAndItem(ctx, api, req, "id")
	if res != nil || err != nil {
		return res, err
	}
	if !i.Custom {
		return refuse(http.StatusUnprocessableEntity, "Only an item added to the standard checklist can be removed: mark it not applicable instead."), nil
	}
	err = api.Tx(ctx, func(tx plugin.Querier) error {
		for _, q := range []string{
			`DELETE FROM website_checklist_webhooks WHERE item_id = ?`,
			`DELETE FROM website_checklist_recommendations WHERE item_id = ?`,
			`DELETE FROM website_checklist_todo_links WHERE item_id = ?`,
			`DELETE FROM website_checklist_items WHERE id = ?`,
		} {
			if _, err := tx.Exec(ctx, q, i.ID); err != nil {
				return err
			}
		}
		if err := touchSite(ctx, tx, s.ID); err != nil {
			return err
		}
		var d diff
		d.add("group", i.Group, "")
		d.add("title", i.Title, "")
		d.add("detail", i.Detail, "")
		d.add("value", i.Value, "")
		d.add("confirmed_value", i.ConfirmedValue, "")
		d.add("state", stateLabels[i.State], "")
		d.add("note", i.Note, "")
		d.add("custom", "Yes", "")
		return audit(ctx, tx, req.Actor, auditEntry{SiteID: s.ID, SiteName: s.Name, ItemID: i.ID, RecordType: "Item", RecordID: i.ID, RecordLabel: i.Title, Action: "deleted", Diff: d})
	})
	if err != nil {
		return nil, err
	}
	files, _ := api.Attachments(ctx, itemType, i.ID, "files")
	for _, f := range files {
		_ = api.Detach(ctx, itemType, i.ID, f.ID)
	}
	return answerSite(ctx, api, req, s.ID, http.StatusOK, fmt.Sprintf("Removed “%s”.", i.Title))
}

// changeItem runs a verb on a step and saves it as one change.
func changeItem(ctx context.Context, api *plugin.API, req *plugin.Request, verb func(*Item, map[string]any), message string) (*plugin.Response, error) {
	s, before, res, err := siteAndItem(ctx, api, req, "id")
	if res != nil || err != nil {
		return res, err
	}
	after := *before
	verb(&after, wrapped(params(req), "item"))
	if err := api.Tx(ctx, func(tx plugin.Querier) error { return saveItem(ctx, tx, req.Actor, s, before, &after) }); err != nil {
		return nil, err
	}
	res, err = answerSite(ctx, api, req, s.ID, http.StatusOK, fmt.Sprintf(message, after.Title))
	return withItem(res, after.ID), err
}

// confirm says a step's value is true, as the caller checked it
// (confirm_website_item, Item#confirm): value and note are recorded first
// when given.
func confirm(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return changeItem(ctx, api, req, func(i *Item, p map[string]any) {
		if v, ok := text(p, "value"); ok {
			i.Value = strings.TrimSpace(*v)
		}
		if v, ok := text(p, "note"); ok {
			i.Note = strings.TrimSpace(*v)
		}
		i.ConfirmedValue, i.State, i.ConfirmedByName, i.ConfirmedAt = i.Value, "confirmed", displayName(req.Actor), store.Now()
	}, "Confirmed “%s”.")
}

// reopen puts a step back to do, forgetting who confirmed it
// (reopen_website_item).
func reopen(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return changeItem(ctx, api, req, func(i *Item, _ map[string]any) {
		i.State, i.ConfirmedValue, i.ConfirmedByName, i.ConfirmedAt = "unconfirmed", "", "", store.Time{}
	}, "“%s” is to confirm again.")
}

// notApplicable marks a step the site doesn't need
// (mark_website_item_not_applicable).
func notApplicable(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return changeItem(ctx, api, req, func(i *Item, p map[string]any) {
		if v, ok := text(p, "note"); ok {
			i.Note = strings.TrimSpace(*v)
		}
		i.State, i.ConfirmedByName, i.ConfirmedAt = "not_applicable", displayName(req.Actor), store.Now()
	}, "“%s” marked not applicable.")
}

// ---- files ---------------------------------------------------------------------------------

type upload struct {
	name, contentType string
	data              []byte
}

// uploads are the files a call sends: a form's files (files, files[]), or
// an agent's {filename, content_base64, content_type}, one or a list.
func uploads(req *plugin.Request) ([]upload, error) {
	ctype, ps, _ := mime.ParseMediaType(req.Header("Content-Type"))
	var out []upload
	if strings.HasPrefix(ctype, "multipart/") {
		mr := multipart.NewReader(bytes.NewReader(req.Body), ps["boundary"])
		for {
			part, err := mr.NextPart()
			if err == io.EOF {
				break
			}
			if err != nil {
				return nil, err
			}
			if part.FileName() == "" || (part.FormName() != "files" && part.FormName() != "files[]") {
				continue
			}
			data, err := io.ReadAll(part)
			if err != nil {
				return nil, err
			}
			out = append(out, upload{name: part.FileName(), contentType: part.Header.Get("Content-Type"), data: data})
		}
		return out, nil
	}
	var list []map[string]any
	switch f := params(req)["files"].(type) {
	case map[string]any:
		list = []map[string]any{f}
	case []any:
		list = rows(f)
	}
	for _, f := range list {
		name := strings.TrimSpace(textOf(f, "filename"))
		data, err := base64.StdEncoding.DecodeString(textOf(f, "content_base64"))
		if err != nil || name == "" {
			return nil, errors.New(`files must be {"filename": "…", "content_base64": "…"}`)
		}
		out = append(out, upload{name: name, contentType: textOf(f, "content_type"), data: data})
	}
	return out, nil
}

// fileAudit records a file added to or removed from a step, and marks the
// step and its site changed.
func fileAudit(ctx context.Context, api *plugin.API, actor *plugin.Caller, s *Site, i *Item, action, before, after string) error {
	return api.Tx(ctx, func(tx plugin.Querier) error {
		if _, err := tx.Exec(ctx, `UPDATE website_checklist_items SET updated_at = ? WHERE id = ?`, store.Now().DB(), i.ID); err != nil {
			return err
		}
		if err := touchSite(ctx, tx, s.ID); err != nil {
			return err
		}
		var d diff
		d.add("files", before, after)
		return audit(ctx, tx, actor, auditEntry{SiteID: s.ID, SiteName: s.Name, ItemID: i.ID, RecordType: "Item", RecordID: i.ID, RecordLabel: i.Title, Action: action, Diff: d})
	})
}

// addFiles keeps files on a step (add_website_item_file).
func addFiles(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, i, res, err := siteAndItem(ctx, api, req, "item_id")
	if res != nil || err != nil {
		return res, err
	}
	files, err := uploads(req)
	if err != nil {
		return refuse(http.StatusUnprocessableEntity, err.Error()), nil
	}
	if len(files) == 0 {
		return refuse(http.StatusUnprocessableEntity, "Choose a file to add."), nil
	}
	for _, f := range files {
		ctype := f.contentType
		if ctype == "" || ctype == "application/octet-stream" {
			if t := mime.TypeByExtension(strings.ToLower(extOf(f.name))); t != "" {
				ctype = t
			} else {
				ctype = http.DetectContentType(f.data)
			}
		}
		if _, err := api.Attach(ctx, itemType, i.ID, "files", f.name, ctype, bytes.NewReader(f.data)); err != nil {
			if r := plugin.Answer(err); r != nil {
				return r, nil
			}
			return nil, err
		}
		if err := fileAudit(ctx, api, req.Actor, s, i, "file added", "", f.name); err != nil {
			return nil, err
		}
	}
	what := "the file"
	if len(files) > 1 {
		what = fmt.Sprintf("%d files", len(files))
	}
	res, err = answerSite(ctx, api, req, s.ID, http.StatusCreated, fmt.Sprintf("Added %s to “%s”.", what, i.Title))
	return withItem(res, i.ID), err
}

func extOf(name string) string {
	if n := strings.LastIndexByte(name, '.'); n >= 0 {
		return name[n:]
	}
	return ""
}

// fileOf is a step's file (id), nil when it isn't one of its.
func fileOf(ctx context.Context, api *plugin.API, itemID, id int64) (*plugin.File, error) {
	files, err := api.Attachments(ctx, itemType, itemID, "files")
	if err != nil {
		return nil, err
	}
	for i := range files {
		if files[i].ID == id {
			return &files[i], nil
		}
	}
	return nil, nil
}

// removeFile takes a file off a step (remove_website_item_file).
func removeFile(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, i, res, err := siteAndItem(ctx, api, req, "item_id")
	if res != nil || err != nil {
		return res, err
	}
	f, err := fileOf(ctx, api, i.ID, queryID(req, "id"))
	if err != nil {
		return nil, err
	}
	if f == nil {
		return missing("file"), nil
	}
	if err := api.Detach(ctx, itemType, i.ID, f.ID); err != nil {
		return nil, err
	}
	if err := fileAudit(ctx, api, req.Actor, s, i, "file removed", f.Filename, ""); err != nil {
		return nil, err
	}
	res, err = answerSite(ctx, api, req, s.ID, http.StatusOK, fmt.Sprintf("Removed %s from “%s”.", f.Filename, i.Title))
	return withItem(res, i.ID), err
}

// download is a step's file, to save.
func download(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	_, i, res, err := siteAndItem(ctx, api, req, "item_id")
	if res != nil || err != nil {
		return res, err
	}
	r, f, err := api.OpenFile(ctx, itemType, i.ID, queryID(req, "id"))
	if err != nil {
		if res := plugin.Answer(err); res != nil {
			return missing("file"), nil
		}
		return missing("file"), nil
	}
	defer r.Close()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	ctype := f.ContentType
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	return &plugin.Response{Status: http.StatusOK, Body: data, Headers: map[string]string{
		"Content-Type":           ctype,
		"Content-Disposition":    fmt.Sprintf(`attachment; filename="%s"; filename*=UTF-8''%s`, strings.ReplaceAll(f.Filename, `"`, ""), url.PathEscape(f.Filename)),
		"X-Content-Type-Options": "nosniff",
	}}, nil
}
