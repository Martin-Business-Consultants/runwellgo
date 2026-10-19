package mail

import (
	"bytes"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"slices"
	"strings"
	"testing"
)

var cfg = Config{Host: "smtp.acme.test", Port: 587, FromName: "Acme", FromEmail: "hello@acme.test", ReplyTo: "help@acme.test"}

// parse reads a composed message back, as a mail client would.
func parse(t *testing.T, msg []byte) *netmail.Message {
	t.Helper()
	m, err := netmail.ReadMessage(bytes.NewReader(msg))
	if err != nil {
		t.Fatalf("not a message: %v\n%s", err, msg)
	}
	return m
}

func TestConfigured(t *testing.T) {
	t.Parallel()
	if !cfg.Configured() || (Config{Host: "x"}).Configured() || (Config{FromEmail: "a@b.test"}).Configured() {
		t.Error("Configured needs both a host and a from address")
	}
}

func TestCompose(t *testing.T) {
	t.Parallel()
	from, rcpt, msg, err := Compose(cfg, Message{
		To: []string{"Ada <ada@example.test>", " "}, Cc: []string{"team@acme.test"}, Bcc: []string{"audit@acme.test"},
		Subject: "Ünïcode — hi", Text: "Line one\nLine two",
	})
	if err != nil {
		t.Fatal(err)
	}
	if from != "hello@acme.test" || !slices.Equal(rcpt, []string{"ada@example.test", "team@acme.test", "audit@acme.test"}) {
		t.Errorf("envelope: %s %v", from, rcpt)
	}
	m := parse(t, msg)
	subject, err := new(mime.WordDecoder).DecodeHeader(m.Header.Get("Subject"))
	if err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"From": `"Acme" <hello@acme.test>`, "To": "ada@example.test", "Cc": "team@acme.test", "Bcc": "",
		"Reply-To": "help@acme.test", "MIME-Version": "1.0", "Content-Type": "text/plain; charset=utf-8",
	} {
		if got := m.Header.Get(name); got != want {
			t.Errorf("%s: %q, want %q", name, got, want)
		}
	}
	if subject != "Ünïcode — hi" {
		t.Errorf("Subject decodes to %q", subject)
	}
	if id := m.Header.Get("Message-ID"); !strings.HasSuffix(id, "@acme.test>") {
		t.Errorf("Message-ID %q", id)
	}
	body, err := io.ReadAll(quotedprintable.NewReader(m.Body))
	if err != nil {
		t.Fatal(err)
	}
	if string(body) != "Line one\r\nLine two\r\n" {
		t.Errorf("body %q", body)
	}
}

func TestComposeKeepsLineEndings(t *testing.T) {
	t.Parallel()
	// A browser's textarea sends CRLF; it mustn't come out double-spaced.
	for _, text := range []string{"a\nb", "a\r\nb"} {
		_, _, msg, err := Compose(cfg, Message{To: []string{"a@b.test"}, Subject: "s", Text: text})
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(quotedprintable.NewReader(parse(t, msg).Body))
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "a\r\nb\r\n" {
			t.Errorf("Text %q is sent as %q", text, body)
		}
	}
}

func TestComposeAlternative(t *testing.T) {
	t.Parallel()
	_, _, msg, err := Compose(cfg, Message{To: []string{"a@b.test"}, Subject: "s", Text: "plain", HTML: "<p>html</p>"})
	if err != nil {
		t.Fatal(err)
	}
	m := parse(t, msg)
	mediaType, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type %q: %v", m.Header.Get("Content-Type"), err)
	}
	r := multipart.NewReader(m.Body, params["boundary"])
	var types []string
	for {
		p, err := r.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		types = append(types, p.Header.Get("Content-Type"))
	}
	if !slices.Equal(types, []string{"text/plain; charset=utf-8", "text/html; charset=utf-8"}) {
		t.Errorf("parts %v", types)
	}
}

func TestComposeRefuses(t *testing.T) {
	t.Parallel()
	many := make([]string, maxRecipients+1)
	for i := range many {
		many[i] = "a@b.test"
	}
	tests := []struct {
		name string
		m    Message
		cfg  Config
		want string
	}{
		{"no sender", Message{To: []string{"a@b.test"}, Subject: "s", Text: "t"}, Config{}, "from:"},
		{"no recipient", Message{To: []string{" "}, Subject: "s", Text: "t"}, cfg, "at least one recipient"},
		{"not an address", Message{To: []string{"nobody"}, Subject: "s", Text: "t"}, cfg, `to: "nobody" isn't an email address`},
		{"a header in an address", Message{Cc: []string{"a@b.test\r\nBcc: evil@x.test"}, Subject: "s", Text: "t"}, cfg, "cc:"},
		{"too many", Message{To: many, Subject: "s", Text: "t"}, cfg, "at most 50 recipients"},
		{"no subject", Message{To: []string{"a@b.test"}, Subject: " ", Text: "t"}, cfg, "needs a subject"},
		{"no body", Message{To: []string{"a@b.test"}, Subject: "s"}, cfg, "needs a body"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, _, _, err := Compose(tt.cfg, tt.m); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err %v, want one saying %q", err, tt.want)
			}
		})
	}
}

func TestComposeDropsABadReplyTo(t *testing.T) {
	t.Parallel()
	_, _, msg, err := Compose(Config{FromEmail: "a@b.test"}, Message{To: []string{"c@d.test"}, ReplyTo: "not an address", Subject: "s", Text: "t"})
	if err != nil {
		t.Fatal(err)
	}
	if rt := parse(t, msg).Header.Get("Reply-To"); rt != "" {
		t.Errorf("Reply-To %q", rt)
	}
}

// FuzzComposeHeaders: whatever a form puts in the sender's name and the
// subject, the message has exactly the headers Compose writes, none
// smuggled in, and is still from the configured address.
func FuzzComposeHeaders(f *testing.F) {
	f.Add("Ada", "Hello")
	f.Add("x\r\nBcc: evil@x.test", "hi\r\nBcc: evil@x.test")
	f.Add("\"quoted\" <name>", "=?utf-8?q?already?=")
	f.Add("Ünï", "\n\n\nbody")
	f.Fuzz(func(t *testing.T, name, subject string) {
		_, _, msg, err := Compose(cfg, Message{FromName: name, To: []string{"a@b.test"}, Subject: subject, Text: "t"})
		if err != nil {
			return // a blank subject
		}
		m := parse(t, msg)
		want := []string{"Content-Transfer-Encoding", "Content-Type", "Date", "From", "Message-Id", "Mime-Version", "Reply-To", "Subject", "To"}
		var got []string
		for k := range m.Header {
			got = append(got, k)
		}
		slices.Sort(got)
		if !slices.Equal(got, want) {
			t.Fatalf("headers %v, want %v\n%s", got, want, msg)
		}
		if from := m.Header.Get("From"); !strings.HasSuffix(from, "<hello@acme.test>") {
			t.Fatalf("From %q", from)
		}
	})
}
