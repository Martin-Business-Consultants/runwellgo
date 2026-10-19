// Package mail builds email messages (MIME, quoted-printable) and sends
// them over SMTP.
package mail

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	netmail "net/mail"
	"net/smtp"
	"net/textproto"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Message is one email. From and Reply-To fall back to the settings'.
type Message struct {
	FromName  string   `json:"from_name"`
	FromEmail string   `json:"from_email"`
	ReplyTo   string   `json:"reply_to"`
	To        []string `json:"to"`
	Cc        []string `json:"cc"`
	Bcc       []string `json:"bcc"`
	Subject   string   `json:"subject"`
	Text      string   `json:"text"`
	HTML      string   `json:"html"`
	// Headers are more headers, by name (Auto-Submitted, In-Reply-To); a
	// Message-ID here replaces the one made up.
	Headers map[string]string `json:"headers,omitempty"`
}

// Config is the SMTP server and the default sender (Settings › Email, with
// the password opened).
type Config struct {
	Host, Security, Username, Password string
	Port                               int
	FromName, FromEmail, ReplyTo       string
}

// Configured is whether there's a server and a from address to send with.
func (c Config) Configured() bool { return c.Host != "" && c.FromEmail != "" }

const maxRecipients = 50

// Compose checks m and builds it for sending: real addresses only, no
// header injection, at most 50 recipients. From and Reply-To fall back to
// cfg's. It answers the envelope's sender and recipients and the message.
func Compose(cfg Config, m Message) (from string, rcpt []string, msg []byte, err error) {
	if m.FromName == "" {
		m.FromName = cfg.FromName
	}
	if m.FromEmail == "" {
		m.FromEmail = cfg.FromEmail
	}
	if m.ReplyTo == "" {
		m.ReplyTo = cfg.ReplyTo
	}
	from, err = oneAddress(m.FromEmail)
	if err != nil {
		return "", nil, nil, fmt.Errorf("from: %w", err)
	}
	headers := [][2]string{}
	for _, list := range []struct {
		name  string
		addrs []string
	}{{"To", m.To}, {"Cc", m.Cc}, {"Bcc", m.Bcc}} {
		var shown []string
		for _, raw := range list.addrs {
			if strings.TrimSpace(raw) == "" {
				continue
			}
			addr, err := oneAddress(raw)
			if err != nil {
				return "", nil, nil, fmt.Errorf("%s: %w", strings.ToLower(list.name), err)
			}
			rcpt = append(rcpt, addr)
			shown = append(shown, addr)
		}
		if len(shown) > 0 && list.name != "Bcc" {
			headers = append(headers, [2]string{list.name, strings.Join(shown, ", ")})
		}
	}
	if len(rcpt) == 0 {
		return "", nil, nil, fmt.Errorf("a message needs at least one recipient")
	}
	if len(rcpt) > maxRecipients {
		return "", nil, nil, fmt.Errorf("a message goes to at most %d recipients", maxRecipients)
	}
	if strings.TrimSpace(m.Subject) == "" {
		return "", nil, nil, fmt.Errorf("a message needs a subject")
	}
	if m.Text == "" && m.HTML == "" {
		return "", nil, nil, fmt.Errorf("a message needs a body")
	}
	head := [][2]string{{"From", (&netmail.Address{Name: oneLine(m.FromName), Address: from}).String()}}
	head = append(head, headers...)
	if m.ReplyTo != "" {
		if rt, err := oneAddress(m.ReplyTo); err == nil {
			head = append(head, [2]string{"Reply-To", rt})
		}
	}
	messageID := fmt.Sprintf("<%s@%s>", strings.ToLower(strings.TrimRight(randomID(), "=")), domainOf(from))
	if id := oneLine(m.Headers["Message-ID"]); id != "" {
		messageID = id
	}
	head = append(head,
		[2]string{"Subject", mime.QEncoding.Encode("utf-8", oneLine(m.Subject))},
		[2]string{"Date", time.Now().Format(time.RFC1123Z)},
		[2]string{"Message-ID", messageID},
		[2]string{"MIME-Version", "1.0"},
	)
	names := make([]string, 0, len(m.Headers))
	for k := range m.Headers {
		names = append(names, k)
	}
	sort.Strings(names)
	for _, k := range names {
		name := textproto.CanonicalMIMEHeaderKey(oneLine(k))
		switch name {
		case "", "Message-Id", "From", "To", "Cc", "Bcc", "Reply-To", "Subject", "Date", "Mime-Version", "Content-Type", "Content-Transfer-Encoding":
			continue
		}
		if v := oneLine(m.Headers[k]); v != "" && !strings.ContainsAny(name, ": ") {
			head = append(head, [2]string{name, v})
		}
	}
	return from, rcpt, buildMIME(head, m.Text, m.HTML), nil
}

func oneAddress(s string) (string, error) {
	addr, err := netmail.ParseAddress(strings.TrimSpace(s))
	if err != nil || strings.ContainsAny(addr.Address, "\r\n") {
		return "", fmt.Errorf("%q isn't an email address", s)
	}
	return addr.Address, nil
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

func domainOf(addr string) string {
	if i := strings.LastIndex(addr, "@"); i >= 0 {
		return addr[i+1:]
	}
	return "localhost"
}

func buildMIME(head [][2]string, text, html string) []byte {
	var b bytes.Buffer
	for _, h := range head {
		fmt.Fprintf(&b, "%s: %s\r\n", h[0], h[1])
	}
	part := func(contentType, body string) {
		fmt.Fprintf(&b, "Content-Type: %s; charset=utf-8\r\nContent-Transfer-Encoding: quoted-printable\r\n\r\n", contentType)
		// The writer ends lines (\n or \r\n) with CRLF itself. Writes to a
		// bytes.Buffer don't fail.
		w := quotedprintable.NewWriter(&b)
		_, _ = w.Write([]byte(body))
		_ = w.Close()
		b.WriteString("\r\n")
	}
	switch {
	case html == "":
		part("text/plain", text)
	case text == "":
		part("text/html", html)
	default:
		boundary := "lp-" + strings.ToLower(strings.TrimRight(randomID(), "="))
		fmt.Fprintf(&b, "Content-Type: multipart/alternative; boundary=%q\r\n\r\n", boundary)
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		part("text/plain", text)
		fmt.Fprintf(&b, "--%s\r\n", boundary)
		part("text/html", html)
		fmt.Fprintf(&b, "--%s--\r\n", boundary)
	}
	return b.Bytes()
}

// randomID is 28 random lowercase letters and digits.
func randomID() string {
	const alphabet = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, 28)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	for i, c := range b {
		b[i] = alphabet[int(c)%len(alphabet)]
	}
	return string(b)
}

// SMTP sends through cfg's server: TLS from the start, STARTTLS, or
// neither, as cfg.Security says.
type SMTP struct{}

// Send delivers msg to the addresses in to, with from as the envelope sender.
func (SMTP) Send(ctx context.Context, cfg Config, from string, to []string, msg []byte) error {
	addr := net.JoinHostPort(cfg.Host, strconv.Itoa(cfg.Port))
	d := net.Dialer{Timeout: 15 * time.Second}
	if dl, ok := ctx.Deadline(); ok {
		d.Deadline = dl
	} else {
		d.Deadline = time.Now().Add(45 * time.Second)
	}
	var conn net.Conn
	var err error
	if cfg.Security == "tls" {
		conn, err = (&tls.Dialer{NetDialer: &d, Config: &tls.Config{ServerName: cfg.Host}}).DialContext(ctx, "tcp", addr)
	} else {
		conn, err = d.DialContext(ctx, "tcp", addr)
	}
	if err != nil {
		return fmt.Errorf("couldn't reach %s: %w", addr, err)
	}
	if err := conn.SetDeadline(d.Deadline); err != nil {
		conn.Close()
		return err
	}
	c, err := smtp.NewClient(conn, cfg.Host)
	if err != nil {
		conn.Close()
		return err
	}
	defer c.Close()
	if cfg.Security == "starttls" {
		if ok, _ := c.Extension("STARTTLS"); !ok {
			return fmt.Errorf("%s doesn't offer STARTTLS; choose TLS or none", cfg.Host)
		}
		if err := c.StartTLS(&tls.Config{ServerName: cfg.Host}); err != nil {
			return err
		}
	}
	if cfg.Username != "" {
		if err := c.Auth(smtp.PlainAuth("", cfg.Username, cfg.Password, cfg.Host)); err != nil {
			return fmt.Errorf("the SMTP server refused the login: %w", err)
		}
	}
	if err := c.Mail(from); err != nil {
		return err
	}
	for _, r := range to {
		if err := c.Rcpt(r); err != nil {
			return fmt.Errorf("the SMTP server refused %s: %w", r, err)
		}
	}
	w, err := c.Data()
	if err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return c.Quit()
}
