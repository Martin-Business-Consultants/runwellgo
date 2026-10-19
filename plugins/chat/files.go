package chat

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	_ "image/gif" // an image's size, for its preview
	_ "image/jpeg"
	_ "image/png"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"path"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Files on messages: sent with the message in one request (multipart:
// "body", "parent_id" and any number of "files"), stored in Runwell's file
// store attached to the message (Chat::Message), and served only to those
// who can see its conversation: an image inline, anything else as a
// download. A message's files are 10 MB together at most (a request's
// limit), each no more than the install's upload limit.

const requestLimit = 10 << 20

// maxUpload is the largest file a message may carry.
func maxUpload(site plugin.Site) int64 {
	if site.MaxUpload > 0 && site.MaxUpload < requestLimit {
		return site.MaxUpload
	}
	return requestLimit - 64<<10 // room for the rest of the request
}

type upload struct {
	name, contentType string
	data              []byte
}

type sendInput struct {
	body     string
	parentID int64
	files    []upload
}

// readSend reads a message to send: JSON ({"message": {body, parent_id}}),
// or a form with its files.
func readSend(ctx context.Context, api *plugin.API, req *plugin.Request) (sendInput, *plugin.Response) {
	ct, ps, _ := mime.ParseMediaType(req.Header("Content-Type"))
	if ct != "multipart/form-data" {
		m := wrapped(params(req), "message")
		return sendInput{body: str(m["body"]), parentID: intOf(m["parent_id"])}, nil
	}
	site, _ := api.Site(ctx)
	limit := maxUpload(site)
	in := sendInput{}
	r := multipart.NewReader(bytes.NewReader(req.Body), ps["boundary"])
	for {
		part, err := r.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return in, refuse(http.StatusBadRequest, "The message didn't arrive whole; send it again.")
		}
		data, err := io.ReadAll(io.LimitReader(part, limit+1))
		if err != nil {
			return in, refuse(http.StatusBadRequest, "The message didn't arrive whole; send it again.")
		}
		switch part.FormName() {
		case "body":
			in.body = string(data)
		case "parent_id":
			in.parentID = intOf(string(data))
		case "files", "files[]":
			name := path.Base(strings.ReplaceAll(part.FileName(), `\`, "/"))
			if name == "" || name == "." || name == "/" {
				name = "file"
			}
			if int64(len(data)) > limit {
				return in, invalid("files", fmt.Sprintf("%s is larger than %d MB: share a link to it instead", name, limit>>20))
			}
			typ := part.Header.Get("Content-Type")
			if typ == "" || typ == "application/octet-stream" {
				typ = http.DetectContentType(data)
			}
			in.files = append(in.files, upload{name: name, contentType: typ, data: data})
		}
	}
	if len(in.files) > 10 {
		return in, invalid("files", "are ten at most in one message")
	}
	return in, nil
}

// attachAll stores a message's files.
func attachAll(ctx context.Context, api *plugin.API, messageID int64, files []upload) (*plugin.Response, error) {
	for _, f := range files {
		stored, err := api.Attach(ctx, "Chat::Message", messageID, "files", f.name, f.contentType, bytes.NewReader(f.data))
		if err != nil {
			if res := plugin.Answer(err); res != nil {
				return res, nil
			}
			return nil, err
		}
		var w, h int64
		if strings.HasPrefix(f.contentType, "image/") {
			if cfg, _, err := image.DecodeConfig(bytes.NewReader(f.data)); err == nil {
				w, h = int64(cfg.Width), int64(cfg.Height)
			}
		}
		if _, err := api.Exec(ctx, `INSERT INTO chat_files (message_id, file_id, filename, content_type, byte_size, width, height, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			messageID, stored.ID, stored.Filename, stored.ContentType, stored.ByteSize, nullID(w), nullID(h), stamp()); err != nil {
			return nil, err
		}
	}
	return nil, nil
}

// serveFile answers a message's file to someone who can see it.
func serveFile(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	var messageID, fileID int64
	err := api.QueryRow(ctx, `SELECT message_id, file_id FROM chat_files WHERE id = ?`, pathID(req, "id")).Scan(&messageID, &fileID)
	if isNoRows(err) {
		return missing("file"), nil
	} else if err != nil {
		return nil, err
	}
	m, err := loadMessage(ctx, api, messageID)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return missing("file"), nil
	}
	if _, _, res, err := open(ctx, api, m.ConversationID, me.ID); res != nil || err != nil {
		return res, err
	}
	return fileResponse(ctx, api, "Chat::Message", messageID, fileID, truthy(req.Query.Get("download")))
}

func fileResponse(ctx context.Context, api *plugin.API, recordType string, recordID, fileID int64, download bool) (*plugin.Response, error) {
	rc, f, err := api.OpenFile(ctx, recordType, recordID, fileID)
	if err != nil {
		if res := plugin.Answer(err); res != nil {
			return res, nil
		}
		return nil, err
	}
	defer rc.Close()
	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	disposition := "attachment"
	ctype := f.ContentType
	if isImage(ctype) && !download {
		disposition = "inline"
	} else if !isImage(ctype) && ctype != "application/pdf" {
		ctype = "application/octet-stream"
	}
	return &plugin.Response{Status: http.StatusOK, Body: data, Headers: map[string]string{
		"Content-Type":        ctype,
		"Content-Disposition": mime.FormatMediaType(disposition, map[string]string{"filename": f.Filename}),
		"Cache-Control":       "private, max-age=86400",
	}}, nil
}

// uploadAvatar sets the caller's chat picture (an image of 4 MB at most,
// sent as the request's body).
func uploadAvatar(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	typ := strings.TrimSpace(strings.Split(req.Header("Content-Type"), ";")[0])
	if !isImage(typ) {
		typ = http.DetectContentType(req.Body)
	}
	if !isImage(typ) {
		return invalid("avatar", "must be a PNG, JPEG, GIF or WebP image"), nil
	}
	if len(req.Body) == 0 {
		return invalid("avatar", "is empty: pick an image"), nil
	}
	if len(req.Body) > 4<<20 {
		return invalid("avatar", "is larger than 4 MB: pick a smaller image"), nil
	}
	p, err := profileOf(ctx, api, me.ID)
	if err != nil {
		return nil, err
	}
	name := req.Query.Get("filename")
	if name == "" {
		name = "avatar"
	}
	f, err := api.Attach(ctx, "Chat::Profile", me.ID, "avatar", path.Base(name), typ, bytes.NewReader(req.Body))
	if err != nil {
		return failed(err)
	}
	old := p.AvatarFileID
	p.AvatarFileID = f.ID
	if err := saveProfile(ctx, api, p); err != nil {
		return nil, err
	}
	if old > 0 {
		_ = api.Detach(ctx, "Chat::Profile", me.ID, old)
	}
	if err := announce(ctx, api, me.ID); err != nil {
		return nil, err
	}
	return showSettings(ctx, api, req)
}

func removeAvatar(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	p, err := profileOf(ctx, api, me.ID)
	if err != nil {
		return nil, err
	}
	if p.AvatarFileID > 0 {
		_ = api.Detach(ctx, "Chat::Profile", me.ID, p.AvatarFileID)
		p.AvatarFileID = 0
		if err := saveProfile(ctx, api, p); err != nil {
			return nil, err
		}
		if err := announce(ctx, api, me.ID); err != nil {
			return nil, err
		}
	}
	return showSettings(ctx, api, req)
}

// serveAvatar is someone's chat picture.
func serveAvatar(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if _, res := caller(req); res != nil {
		return res, nil
	}
	id := pathID(req, "user_id")
	p, err := profileOf(ctx, api, id)
	if err != nil {
		return nil, err
	}
	if p.AvatarFileID == 0 {
		return missing("picture"), nil
	}
	return fileResponse(ctx, api, "Chat::Profile", id, p.AvatarFileID, false)
}
