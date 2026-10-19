package core

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha1" //nolint:gosec // Action Mailbox's checksum and Mandrill's signature, not ours to choose
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Requests by email, as the Rails core's Action Mailbox took them: the
// inbound service the install names (INBOUND_EMAIL_INGRESS, else Settings ›
// Email) posts each message to /rails/action_mailbox/<ingress>/inbound_emails
// with the password it was given; the raw message is kept (an inbound email
// and its raw_email blob) and processed in the background
// (RequestsMailbox): a new request, or a note on the one it replies to.

// InboundIngresses are the services mail can come in through.
var InboundIngresses = []string{"relay", "postmark", "sendgrid", "mailgun", "mandrill"}

// Inbound emails' statuses, as Action Mailbox numbers them.
const (
	inboundPending    = 0
	inboundProcessing = 1
	inboundDelivered  = 2
	inboundFailed     = 3
)

// maxInbound is the most one message may be.
const maxInbound = 64 << 20

func init() { routeSets = append(routeSets, (*App).mailboxRoutes) }

func (a *App) mailboxRoutes(mux *http.ServeMux) {
	const base = "/rails/action_mailbox/"
	mux.HandleFunc("POST "+base+"relay/inbound_emails", a.ingress("relay", a.ingressRelay))
	mux.HandleFunc("POST "+base+"postmark/inbound_emails", a.ingress("postmark", a.ingressPostmark))
	mux.HandleFunc("POST "+base+"sendgrid/inbound_emails", a.ingress("sendgrid", a.ingressSendgrid))
	mux.HandleFunc("POST "+base+"mailgun/inbound_emails/mime", a.ingress("mailgun", a.ingressMailgun))
	mux.HandleFunc("POST "+base+"mandrill/inbound_emails", a.ingress("mandrill", a.ingressMandrill))
	// Mandrill checks the address with a HEAD (or GET) before it makes the webhook.
	mux.HandleFunc("GET "+base+"mandrill/inbound_emails", func(w http.ResponseWriter, r *http.Request) {
		if ingress, _ := a.inboundIngress(r.Context()); ingress != "mandrill" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
}

// inboundIngress is the service mail comes in through and the password it
// signs in with: the environment's (INBOUND_EMAIL_INGRESS,
// RAILS_INBOUND_EMAIL_PASSWORD), else Settings › Email's.
func (a *App) inboundIngress(ctx context.Context) (ingress, password string) {
	var stored, sealed string
	_ = a.store.QueryRowContext(ctx, `SELECT COALESCE(inbound_ingress, ''), COALESCE(inbound_password, '') FROM settings ORDER BY id LIMIT 1`).Scan(&stored, &sealed)
	ingress = strings.TrimSpace(os.Getenv("INBOUND_EMAIL_INGRESS"))
	if ingress == "" {
		ingress = strings.TrimSpace(stored)
	}
	password = os.Getenv("RAILS_INBOUND_EMAIL_PASSWORD")
	if password == "" && sealed != "" {
		if opened, ok := a.secrets.Open(sealed); ok {
			password = opened
		} else if !strings.HasPrefix(sealed, "{") {
			password = sealed
		}
	}
	return ingress, password
}

// ingressError is an ingress's answer when the request can't be taken.
type ingressError struct {
	status int
	msg    string
}

func (e *ingressError) Error() string { return e.msg }

// ingress answers one service's posts: 404 unless it's the install's
// service; then the service's own reading (and checking) of the request,
// which answers the raw messages to keep.
func (a *App) ingress(name string, read func(w http.ResponseWriter, r *http.Request, password string) ([][]byte, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		ingress, password := a.inboundIngress(ctx)
		if ingress != name {
			http.NotFound(w, r)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxInbound)
		raws, err := read(w, r, password)
		if err != nil {
			if ie, ok := errors.AsType[*ingressError](err); ok {
				if ie.status == http.StatusUnauthorized {
					w.Header().Set("WWW-Authenticate", `Basic realm="Action Mailbox"`)
				}
				a.log.Warn("inbound email refused", "ingress", name, "status", ie.status, "reason", ie.msg)
				w.WriteHeader(ie.status)
				return
			}
			a.log.Error("inbound email", "ingress", name, "err", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		for _, raw := range raws {
			if err := a.receiveInbound(ctx, raw); err != nil {
				a.log.Error("inbound email", "ingress", name, "err", err)
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
		}
		if name == "mandrill" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

// mailboxBasicAuth is Action Mailbox's password check: user actionmailbox and the
// ingress password. With no password set, nothing gets in.
func mailboxBasicAuth(r *http.Request, password string) error {
	if password == "" {
		return errors.New("missing required ingress credentials")
	}
	user, pass, ok := r.BasicAuth()
	if !ok || subtle.ConstantTimeCompare([]byte(user), []byte("actionmailbox")) != 1 || subtle.ConstantTimeCompare([]byte(pass), []byte(password)) != 1 {
		return &ingressError{http.StatusUnauthorized, "wrong ingress password"}
	}
	return nil
}

// mailboxOriginalTo puts X-Original-To on top of a raw message, as the
// services' ingresses do with the envelope's recipient.
func mailboxOriginalTo(raw []byte, to string) []byte {
	return append([]byte("X-Original-To: "+to+"\n"), raw...)
}

// mailboxForm reads a form post (urlencoded or multipart).
func mailboxForm(r *http.Request) error {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		return r.ParseMultipartForm(32 << 20) //nolint:gosec // ingress caps the body at maxInbound
	}
	return r.ParseForm()
}

func (a *App) ingressRelay(w http.ResponseWriter, r *http.Request, password string) ([][]byte, error) {
	if err := mailboxBasicAuth(r, password); err != nil {
		return nil, err
	}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt != "message/rfc822" {
		return nil, &ingressError{http.StatusUnsupportedMediaType, "not message/rfc822"}
	}
	raw, err := io.ReadAll(r.Body)
	if err != nil || len(raw) == 0 {
		return nil, &ingressError{http.StatusUnprocessableEntity, "no message"}
	}
	return [][]byte{raw}, nil
}

func (a *App) ingressPostmark(w http.ResponseWriter, r *http.Request, password string) ([][]byte, error) {
	if err := mailboxBasicAuth(r, password); err != nil {
		return nil, err
	}
	params := map[string]string{}
	if mt, _, _ := mime.ParseMediaType(r.Header.Get("Content-Type")); mt == "application/json" {
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			return nil, &ingressError{http.StatusBadRequest, "the body isn't JSON"}
		}
		for k, v := range body {
			if s, ok := v.(string); ok {
				params[k] = s
			}
		}
	} else {
		if err := mailboxForm(r); err != nil {
			return nil, &ingressError{http.StatusBadRequest, "unreadable form"}
		}
		for k := range r.Form {
			params[k] = r.Form.Get(k)
		}
	}
	raw := params["RawEmail"]
	if raw == "" {
		a.log.Error(`When configuring your Postmark inbound webhook, be sure to check the box labeled "Include raw email content in JSON payload".`)
		return nil, &ingressError{http.StatusUnprocessableEntity, "no RawEmail"}
	}
	out := []byte(raw)
	if to, ok := params["OriginalRecipient"]; ok && to != "" {
		out = mailboxOriginalTo(out, to)
	}
	return [][]byte{out}, nil
}

func (a *App) ingressSendgrid(w http.ResponseWriter, r *http.Request, password string) ([][]byte, error) {
	if err := mailboxBasicAuth(r, password); err != nil {
		return nil, err
	}
	if err := mailboxForm(r); err != nil {
		return nil, &ingressError{http.StatusBadRequest, "unreadable form"}
	}
	raw := r.Form.Get("email")
	if raw == "" {
		return nil, &ingressError{http.StatusBadRequest, "param is missing: email"}
	}
	out := []byte(raw)
	if r.Form.Has("envelope") {
		var envelope struct {
			To []string `json:"to"`
		}
		if err := json.Unmarshal([]byte(r.Form.Get("envelope")), &envelope); err != nil {
			return nil, &ingressError{http.StatusUnprocessableEntity, err.Error()}
		}
		for _, to := range envelope.To {
			out = mailboxOriginalTo(out, to)
		}
	}
	return [][]byte{out}, nil
}

// ingressMailgun checks Mailgun's signature (MAILGUN_INGRESS_SIGNING_KEY):
// HMAC-SHA256 of timestamp and token, made in the last two minutes.
func (a *App) ingressMailgun(w http.ResponseWriter, r *http.Request, _ string) ([][]byte, error) {
	key := os.Getenv("MAILGUN_INGRESS_SIGNING_KEY")
	if key == "" {
		return nil, errors.New("missing required Mailgun signing key: set MAILGUN_INGRESS_SIGNING_KEY")
	}
	if err := mailboxForm(r); err != nil {
		return nil, &ingressError{http.StatusBadRequest, "unreadable form"}
	}
	for _, p := range []string{"timestamp", "token", "signature", "body-mime"} {
		if r.Form.Get(p) == "" {
			return nil, &ingressError{http.StatusBadRequest, "param is missing: " + p}
		}
	}
	ts, err := strconv.ParseInt(r.Form.Get("timestamp"), 10, 64)
	if err != nil {
		return nil, &ingressError{http.StatusBadRequest, "bad timestamp"}
	}
	m := hmac.New(sha256.New, []byte(key))
	m.Write([]byte(r.Form.Get("timestamp") + r.Form.Get("token")))
	expected := hex.EncodeToString(m.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(r.Form.Get("signature"))) != 1 || time.Unix(ts, 0).Before(time.Now().Add(-2*time.Minute)) {
		return nil, &ingressError{http.StatusUnauthorized, "bad Mailgun signature"}
	}
	out := []byte(r.Form.Get("body-mime"))
	if to := r.Form.Get("recipient"); r.Form.Has("recipient") {
		out = mailboxOriginalTo(out, to)
	}
	return [][]byte{out}, nil
}

// ingressMandrill checks Mandrill's signature (MANDRILL_INGRESS_API_KEY):
// base64 HMAC-SHA1 of the URL and the sorted form, and takes each inbound
// event's raw message.
func (a *App) ingressMandrill(w http.ResponseWriter, r *http.Request, _ string) ([][]byte, error) {
	key := os.Getenv("MANDRILL_INGRESS_API_KEY")
	if key == "" {
		return nil, errors.New("missing required Mandrill API key: set MANDRILL_INGRESS_API_KEY")
	}
	if err := mailboxForm(r); err != nil {
		return nil, &ingressError{http.StatusBadRequest, "unreadable form"}
	}
	scheme := "http"
	if r.TLS != nil || (a.cfg.TrustProxy && r.Header.Get("X-Forwarded-Proto") == "https") {
		scheme = "https"
	}
	keys := make([]string, 0, len(r.PostForm))
	for k := range r.PostForm {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var msg strings.Builder
	msg.WriteString(scheme + "://" + r.Host + r.URL.RequestURI())
	for _, k := range keys {
		msg.WriteString(k + r.PostForm.Get(k))
	}
	m := hmac.New(sha1.New, []byte(key))
	m.Write([]byte(msg.String()))
	expected := base64.StdEncoding.EncodeToString(m.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(expected), []byte(r.Header.Get("X-Mandrill-Signature"))) != 1 {
		return nil, &ingressError{http.StatusUnauthorized, "bad Mandrill signature"}
	}
	if r.PostForm.Get("mandrill_events") == "" {
		return nil, &ingressError{http.StatusBadRequest, "param is missing: mandrill_events"}
	}
	var events []struct {
		Event string `json:"event"`
		Msg   struct {
			RawMsg string `json:"raw_msg"`
		} `json:"msg"`
	}
	if err := json.Unmarshal([]byte(r.PostForm.Get("mandrill_events")), &events); err != nil {
		return nil, &ingressError{http.StatusUnprocessableEntity, err.Error()}
	}
	var out [][]byte
	for _, e := range events {
		if e.Event == "inbound" {
			out = append(out, []byte(e.Msg.RawMsg))
		}
	}
	return out, nil
}

// ---- keeping and processing ---------------------------------------------------------

// receiveInbound keeps a raw message (an inbound email, its raw_email blob)
// and processes it in the background. The same message twice (its
// Message-ID and checksum) is kept once.
func (a *App) receiveInbound(ctx context.Context, raw []byte) error {
	sum := sha1.Sum(raw) //nolint:gosec // Action Mailbox's checksum
	checksum := hex.EncodeToString(sum[:])
	messageID := ""
	if msg, err := parseMail(raw); err == nil {
		messageID = msg.MessageID()
	}
	if messageID == "" {
		host, _ := os.Hostname()
		messageID = secret.Token(16) + "@" + host + ".mail"
	}
	var id int64
	err := a.write(ctx, systemActor, func(t *Tx) error {
		if n, err := store.Count(ctx, t, `SELECT count(*) FROM action_mailbox_inbound_emails WHERE message_id = ? AND message_checksum = ?`, messageID, checksum); err != nil || n > 0 {
			return err
		}
		now := store.Now().DB()
		var err error
		id, err = t.Insert(`INSERT INTO action_mailbox_inbound_emails (status, message_id, message_checksum, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`,
			inboundPending, messageID, checksum, now, now)
		if err != nil {
			return err
		}
		blob, err := t.putBlob("message.eml", "message/rfc822", bytes.NewReader(raw))
		if err != nil {
			return err
		}
		return t.attach("ActionMailbox::InboundEmail", id, "raw_email", blob)
	})
	if err != nil || id == 0 {
		return err
	}
	a.later("inbound email", func(ctx context.Context) { a.processInbound(ctx, id, raw) })
	return nil
}

// processInbound runs one kept message through the requests mailbox,
// marking it delivered or failed.
func (a *App) processInbound(ctx context.Context, id int64, raw []byte) {
	setStatus := func(status int) {
		_, _ = a.store.ExecContext(ctx, `UPDATE action_mailbox_inbound_emails SET status = ?, updated_at = ? WHERE id = ?`, status, store.Now().DB(), id)
	}
	setStatus(inboundProcessing)
	if err := a.requestsMailbox(ctx, raw); err != nil {
		a.log.Error("inbound email", "id", id, "err", err)
		setStatus(inboundFailed)
		return
	}
	setStatus(inboundDelivered)
}

var (
	mailboxReplyAddress   = regexp.MustCompile(`(?i)\+r(\d+)-([0-9a-f]{12})@`)
	mailboxReplyMessageID = regexp.MustCompile(`(?i)request-(\d+)-([0-9a-f]{12})@`)
	mailboxSubjectPrefix  = regexp.MustCompile(`(?i)\A(\s*(re|fwd?|aw|sv)\s*:\s*)+`)
	mailboxAutoSuppress   = regexp.MustCompile(`(?i)\b(all|oof|autoreply)\b`)
	mailboxDaemon         = regexp.MustCompile(`\A(mailer-daemon|postmaster)@`)
)

// requestsMailbox is RequestsMailbox#process: a client's email becomes a
// request, or, replying to one of ours, a note on it. Automatic replies and
// our own mail are dropped. A known contact gets an acknowledgement.
func (a *App) requestsMailbox(ctx context.Context, raw []byte) error {
	m, err := parseMail(raw)
	if err != nil {
		return err
	}
	senderEmail, senderName := m.From()
	if m.automatic(senderEmail) || a.fromOurselves(ctx, senderEmail) {
		return nil
	}
	if q := a.repliedTo(ctx, m); q != nil {
		return a.addReply(ctx, m, q, senderEmail, senderName)
	}
	return a.receiveRequest(ctx, m, senderEmail, senderName)
}

// automatic is whether a machine sent it: out-of-office and other machine
// replies say so in their headers (RFC 3834 and the usual vendor ones);
// bounces come from the mail system.
func (m *inboundMail) automatic(sender string) bool {
	submitted := strings.ToLower(strings.TrimSpace(m.Header.Get("Auto-Submitted")))
	precedence := strings.ToLower(strings.TrimSpace(m.Header.Get("Precedence")))
	switch {
	case submitted != "" && submitted != "no",
		m.Header.Get("X-Autoreply") != "", m.Header.Get("X-Autorespond") != "",
		precedence == "bulk" || precedence == "junk" || precedence == "list" || precedence == "auto_reply",
		mailboxAutoSuppress.MatchString(m.Header.Get("X-Auto-Response-Suppress")),
		sender == "", mailboxDaemon.MatchString(sender):
		return true
	}
	return false
}

// fromOurselves is our own mail, looping back through the requests address.
func (a *App) fromOurselves(ctx context.Context, sender string) bool {
	s, err := a.settings(ctx)
	if err != nil {
		return false
	}
	_, from := a.mailSender(s)
	return sender == strings.ToLower(from) || (s.RequestsEmail != "" && sender == strings.ToLower(s.RequestsEmail))
}

// repliedTo is the request a reply belongs to: its plus address on any
// recipient, else the acknowledgement's Message-ID in In-Reply-To or
// References.
func (a *App) repliedTo(ctx context.Context, m *inboundMail) *Request {
	h := m.Header
	addresses := strings.Join([]string{h.Get("To"), h.Get("Cc"), h.Get("X-Original-To"), h.Get("Delivered-To")}, " ")
	threads := strings.Join([]string{h.Get("In-Reply-To"), h.Get("References")}, " ")
	for _, match := range [][]string{mailboxReplyAddress.FindStringSubmatch(addresses), mailboxReplyMessageID.FindStringSubmatch(threads)} {
		if match == nil {
			continue
		}
		id, _ := strconv.ParseInt(match[1], 10, 64)
		if q := a.requestByReplyToken(ctx, id, match[2]); q != nil {
			return q
		}
	}
	return nil
}

// mailboxActor is who an email's writes are by: no one on staff, from email.
var mailboxActor = &Actor{}

// receiveRequest makes a new request of an email, its attachments its
// documents, and acknowledges it to a known contact.
func (a *App) receiveRequest(ctx context.Context, m *inboundMail, senderEmail, senderName string) error {
	title := strings.Join(strings.Fields(mailboxSubjectPrefix.ReplaceAllString(m.Subject(), "")), " ")
	if title == "" {
		title = "Email from " + mailboxFirst(senderName, senderEmail)
	}
	if r := []rune(title); len(r) > 200 {
		title = string(r[:197]) + "..."
	}
	received := m.Date()
	var id int64
	err := a.write(ctx, mailboxActor, func(t *Tx) error {
		var err error
		id, err = t.insertRequest(map[string]any{
			"subject": title, "body": nullable(sanitizeRichText(m.Body().HTML())), "sender_name": nullable(senderName), "sender_email": nullable(senderEmail),
			"source": "email", "received_at": received.DB(),
		})
		if err != nil {
			return err
		}
		if _, err := t.attachMailFiles(m, id); err != nil {
			return err
		}
		q, err := t.App.request(ctx, t, id)
		if err != nil {
			return err
		}
		return t.requestEvent(id, "request.received", nil, "email", q.RequesterName())
	})
	if err != nil {
		return err
	}
	q, err := a.request(ctx, a.store, id)
	if err != nil {
		return err
	}
	if q.ContactID != 0 {
		a.mailRequestReceived(ctx, q, m.MessageID())
	}
	return nil
}

// addReply keeps a reply as an email note on its request, its attachments
// as the request's documents.
func (a *App) addReply(ctx context.Context, m *inboundMail, q *Request, senderEmail, senderName string) error {
	who := mailboxFirst(senderName, senderEmail)
	return a.write(ctx, mailboxActor, func(t *Tx) error {
		files, err := t.attachMailFiles(m, q.ID)
		if err != nil {
			return err
		}
		text := sanitizeRichText(m.Body().HTML())
		if text == "" {
			if files == 1 {
				text = "<p>Sent a file.</p>"
			} else {
				text = fmt.Sprintf("<p>Sent %d files.</p>", files)
			}
		}
		now := store.Now().DB()
		note, err := t.Insert(`INSERT INTO notes (subject_type, subject_id, body, kind, occurred_at, author_id, source, created_at, updated_at) VALUES ('Request', ?, ?, 'email', ?, NULL, ?, ?, ?)`,
			q.ID, text, m.Date().DB(), "email from "+who, now, now)
		if err != nil {
			return err
		}
		t.Reindex("Note", note)
		return t.requestEvent(q.ID, "request.replied", nil, "email", who)
	})
}

// attachMailFiles keeps what the sender attached as the request's
// documents, internal until someone shares them. Pictures inline in the
// message (signature logos) are skipped, and so is a file over the limit.
func (t *Tx) attachMailFiles(m *inboundMail, requestID int64) (int, error) {
	n := 0
	for _, f := range m.Attachments() {
		if f.Inline || int64(len(f.Data)) > MaxUpload() {
			continue
		}
		blob, err := t.putBlob(f.Filename, f.ContentType, bytes.NewReader(f.Data))
		if err != nil {
			return n, err
		}
		now := store.Now().DB()
		doc, err := t.Insert(`INSERT INTO documents (documentable_type, documentable_id, uploaded_by_id, client_visible, created_at, updated_at) VALUES ('Request', ?, NULL, 0, ?, ?)`,
			requestID, now, now)
		if err != nil {
			return n, err
		}
		if err := t.attach("Document", doc, "file", blob); err != nil {
			return n, err
		}
		t.Reindex("Document", doc)
		n++
	}
	return n, nil
}

func mailboxFirst(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
