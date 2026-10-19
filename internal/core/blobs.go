package core

import (
	"context"
	"crypto/md5" //nolint:gosec // Active Storage's checksum, not security
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"math/big"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Files are kept the way the Rails core's Active Storage kept them, so its
// data directory's files open here as they are: a blob row
// (active_storage_blobs) per file, its bytes at DATA_DIR/<k[0:2]>/<k[2:4]>/<key>
// (the Disk service), and an attachment row (active_storage_attachments)
// tying it to a record by name: a Document's "file", a User's "avatar",
// the Setting's "logo" and "favicon".

// Blob is one stored file.
type Blob struct {
	ID          int64      `json:"id"`
	Key         string     `json:"key"`
	Filename    string     `json:"filename"`
	ContentType string     `json:"content_type"`
	ByteSize    int64      `json:"byte_size"`
	Checksum    string     `json:"checksum"`
	CreatedAt   store.Time `json:"created_at"`
}

// MaxUpload is the largest file one upload may be: RUNWELL_MAX_UPLOAD_MB
// (100 MB unless set), as in the Rails core.
func MaxUpload() int64 {
	if mb, err := strconv.Atoi(os.Getenv("RUNWELL_MAX_UPLOAD_MB")); err == nil && mb > 0 {
		return int64(mb) << 20
	}
	return 100 << 20
}

const blobKeyAlphabet = "0123456789abcdefghijklmnopqrstuvwxyz"

// newBlobKey is a key like Active Storage's: 28 base-36 characters.
func newBlobKey() string {
	b := make([]byte, 28)
	for i := range b {
		n, _ := rand.Int(rand.Reader, big.NewInt(int64(len(blobKeyAlphabet))))
		b[i] = blobKeyAlphabet[n.Int64()]
	}
	return string(b)
}

// blobPath is where a blob's bytes are on disk.
func (a *App) blobPath(key string) string {
	if len(key) < 4 {
		return filepath.Join(a.cfg.DataDir, "invalid")
	}
	return filepath.Join(a.cfg.DataDir, key[0:2], key[2:4], filepath.Base(key))
}

// putBlob stores r (at most MaxUpload bytes) as a new blob, in the write.
// The file is written before the row; a write that rolls back leaves an
// orphan file, never a row without one.
func (t *Tx) putBlob(filename, contentType string, r io.Reader) (*Blob, error) {
	key := newBlobKey()
	path := t.App.blobPath(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o640)
	if err != nil {
		return nil, err
	}
	sum := md5.New() //nolint:gosec // Active Storage's checksum
	n, err := io.Copy(io.MultiWriter(f, sum), io.LimitReader(r, MaxUpload()+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil && n > MaxUpload() {
		err = errTooLarge
	}
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	if contentType == "" || contentType == "application/octet-stream" {
		if byExt := mime.TypeByExtension(strings.ToLower(filepath.Ext(filename))); byExt != "" {
			contentType = byExt
		} else if contentType == "" {
			contentType = "application/octet-stream"
		}
	}
	b := &Blob{Key: key, Filename: filepath.Base(filename), ContentType: contentType, ByteSize: n,
		Checksum: base64.StdEncoding.EncodeToString(sum.Sum(nil)), CreatedAt: store.Now()}
	meta, _ := json.Marshal(map[string]any{"identified": true})
	b.ID, err = t.Insert(`INSERT INTO active_storage_blobs (key, filename, content_type, metadata, service_name, byte_size, checksum, created_at) VALUES (?, ?, ?, ?, 'local', ?, ?, ?)`,
		b.Key, b.Filename, b.ContentType, string(meta), b.ByteSize, b.Checksum, b.CreatedAt.DB())
	if err != nil {
		_ = os.Remove(path)
		return nil, err
	}
	return b, nil
}

var errTooLarge = errors.New("the file is over the upload limit")

// attach ties a blob to a record by name, replacing what was attached
// under that name (the old blob is purged once the write commits).
func (t *Tx) attach(recordType string, recordID int64, name string, b *Blob) error {
	old, err := t.App.attachedBlob(t.Ctx, t, recordType, recordID, name)
	if err != nil {
		return err
	}
	if old != nil {
		if err := t.detach(recordType, recordID, name); err != nil {
			return err
		}
	}
	_, err = t.Exec(`INSERT INTO active_storage_attachments (name, record_type, record_id, blob_id, created_at) VALUES (?, ?, ?, ?, ?)`,
		name, recordType, recordID, b.ID, store.Now().DB())
	return err
}

// detach removes what's attached under name, and the blob with its file
// once the write commits.
func (t *Tx) detach(recordType string, recordID int64, name string) error {
	b, err := t.App.attachedBlob(t.Ctx, t, recordType, recordID, name)
	if err != nil || b == nil {
		return err
	}
	if _, err := t.Exec(`DELETE FROM active_storage_attachments WHERE record_type = ? AND record_id = ? AND name = ?`, recordType, recordID, name); err != nil {
		return err
	}
	if _, err := t.Exec(`DELETE FROM active_storage_variant_records WHERE blob_id = ?`, b.ID); err != nil {
		return err
	}
	if _, err := t.Exec(`DELETE FROM active_storage_blobs WHERE id = ?`, b.ID); err != nil {
		return err
	}
	path := t.App.blobPath(b.Key)
	t.AfterCommit(func(context.Context) { _ = os.Remove(path) })
	return nil
}

// attachedBlob is the blob attached to a record under name; nil for none.
func (a *App) attachedBlob(ctx context.Context, db store.Querier, recordType string, recordID int64, name string) (*Blob, error) {
	var b Blob
	var ct *string
	err := db.QueryRowContext(ctx, `SELECT b.id, b.key, b.filename, b.content_type, b.byte_size, COALESCE(b.checksum, ''), b.created_at
		FROM active_storage_attachments x JOIN active_storage_blobs b ON b.id = x.blob_id
		WHERE x.record_type = ? AND x.record_id = ? AND x.name = ? ORDER BY x.id DESC LIMIT 1`, recordType, recordID, name).
		Scan(&b.ID, &b.Key, &b.Filename, &ct, &b.ByteSize, &b.Checksum, &b.CreatedAt)
	if isNoRows(err) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	if ct != nil {
		b.ContentType = *ct
	}
	return &b, nil
}

// serveBlob answers a blob's bytes: inline for images and PDFs, as a
// download otherwise (download true forces it). The bytes are served with
// a sandboxing CSP, so an uploaded HTML file can't run as Runwell.
func (a *App) serveBlob(w http.ResponseWriter, r *http.Request, b *Blob, download bool) {
	f, err := os.Open(a.blobPath(b.Key))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	inline := !download && (strings.HasPrefix(b.ContentType, "image/") && b.ContentType != "image/svg+xml" || b.ContentType == "application/pdf")
	disposition := "attachment"
	if inline {
		disposition = "inline"
	}
	h := w.Header()
	h.Set("Content-Type", b.ContentType)
	h.Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": b.Filename}))
	h.Set("Content-Security-Policy", "sandbox; default-src 'none'; img-src 'self'; style-src 'unsafe-inline'")
	h.Set("Cache-Control", "private, max-age=300")
	http.ServeContent(w, r, "", b.CreatedAt.Time, f)
}
