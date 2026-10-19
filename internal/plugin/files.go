package plugin

import (
	"context"
	"fmt"
	"io"
	"strings"
	"unicode"
)

// A plugin granted "files" keeps files in Runwell's file store (Active
// Storage's, so a Rails plugin's files carry over) and attaches them to its
// own records: a record type in its namespace, the CamelCase of its key and
// "::" (WebsiteChecklist::Item for website_checklist), as the Rails plugin
// named them. It can't reach the core's files, or another plugin's.

// File is a stored file.
type File struct {
	ID          int64  `json:"id"`
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
	CreatedAt   string `json:"created_at"`
}

// Namespace is the record types a plugin's files attach to: "WebsiteChecklist::".
func Namespace(key string) string {
	var b strings.Builder
	up := true
	for _, r := range key {
		if r == '_' {
			up = true
			continue
		}
		if up {
			r = unicode.ToUpper(r)
			up = false
		}
		b.WriteRune(r)
	}
	return b.String() + "::"
}

func (a *API) files(recordType string) error {
	if err := a.need("files", a.grants.Files); err != nil {
		return err
	}
	if !strings.HasPrefix(recordType, Namespace(a.key)) || len(recordType) == len(Namespace(a.key)) {
		return fmt.Errorf("a plugin's files attach to its own records, %s<Name>, not %q", Namespace(a.key), recordType)
	}
	return nil
}

// Attach stores r as a file attached to one of the plugin's records under
// name (files): at most RUNWELL_MAX_UPLOAD_MB.
func (a *API) Attach(ctx context.Context, recordType string, recordID int64, name, filename, contentType string, r io.Reader) (*File, error) {
	if err := a.files(recordType); err != nil {
		return nil, err
	}
	return a.h.env.AttachFile(ctx, recordType, recordID, name, filename, contentType, r)
}

// Attachments are the files attached to a record under name, oldest first
// (files).
func (a *API) Attachments(ctx context.Context, recordType string, recordID int64, name string) ([]File, error) {
	if err := a.files(recordType); err != nil {
		return nil, err
	}
	return a.h.env.Attachments(ctx, recordType, recordID, name)
}

// OpenFile is a file attached to one of the plugin's records, to read
// (files). Close it.
func (a *API) OpenFile(ctx context.Context, recordType string, recordID, fileID int64) (io.ReadCloser, *File, error) {
	if err := a.files(recordType); err != nil {
		return nil, nil, err
	}
	return a.h.env.OpenFile(ctx, recordType, recordID, fileID)
}

// Detach removes a file from one of the plugin's records, and the file
// (files).
func (a *API) Detach(ctx context.Context, recordType string, recordID, fileID int64) error {
	if err := a.files(recordType); err != nil {
		return err
	}
	return a.h.env.DetachFile(ctx, recordType, recordID, fileID)
}
