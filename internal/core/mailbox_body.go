package core

import (
	"bytes"
	"encoding/base64"
	"html"
	"io"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	netmail "net/mail"
	"regexp"
	"strings"
	"unicode/utf8"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// An inbound email, read with net/mail and mime/multipart: its headers,
// its text and HTML parts, its attachments, and what the client wrote
// without the rest (Request::EmailBody).

// inboundMail is a parsed message.
type inboundMail struct {
	Header netmail.Header
	parts  []mailPart
}

// mailPart is one leaf of the message: a body part or a file.
type mailPart struct {
	ContentType string
	Charset     string
	Filename    string
	Inline      bool // Content-Disposition: inline
	Attachment  bool // Content-Disposition: attachment
	Data        []byte
}

var mailWords = &mime.WordDecoder{CharsetReader: func(charset string, input io.Reader) (io.Reader, error) {
	raw, err := io.ReadAll(input)
	if err != nil {
		return nil, err
	}
	return strings.NewReader(toUTF8(raw, charset)), nil
}}

func parseMail(raw []byte) (*inboundMail, error) {
	msg, err := netmail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	m := &inboundMail{Header: msg.Header}
	m.walk(mailHeader(msg.Header), msg.Body, 0)
	return m, nil
}

// mailHeader lets a message's header and a part's be read alike.
type mailHeader interface{ Get(string) string }

// walk collects the leaves of a part, multipart or not.
func (m *inboundMail) walk(h mailHeader, body io.Reader, depth int) {
	ct := h.Get("Content-Type")
	if ct == "" {
		ct = "text/plain"
	}
	mt, params, err := mime.ParseMediaType(ct)
	if err != nil {
		mt, params = "text/plain", map[string]string{}
	}
	if strings.HasPrefix(mt, "multipart/") && depth < 10 {
		mr := multipart.NewReader(body, params["boundary"])
		for {
			p, err := mr.NextRawPart()
			if err != nil {
				return
			}
			m.walk(p.Header, p, depth+1)
		}
	}
	data, _ := io.ReadAll(io.LimitReader(body, maxInbound))
	switch strings.ToLower(strings.TrimSpace(h.Get("Content-Transfer-Encoding"))) {
	case "base64":
		clean := bytes.Map(func(r rune) rune {
			if r == '\r' || r == '\n' || r == ' ' || r == '\t' {
				return -1
			}
			return r
		}, data)
		if dec, err := base64.StdEncoding.DecodeString(strings.TrimRight(string(clean), "=") + strings.Repeat("=", (4-len(strings.TrimRight(string(clean), "="))%4)%4)); err == nil {
			data = dec
		}
	case "quoted-printable":
		if dec, err := io.ReadAll(quotedprintable.NewReader(bytes.NewReader(data))); err == nil {
			data = dec
		}
	}
	part := mailPart{ContentType: mt, Charset: params["charset"], Data: data}
	if disp, dparams, err := mime.ParseMediaType(h.Get("Content-Disposition")); err == nil {
		part.Inline, part.Attachment = disp == "inline", disp == "attachment"
		part.Filename = dparams["filename"]
	}
	if part.Filename == "" {
		part.Filename = params["name"]
	}
	if part.Filename != "" {
		if dec, err := mailWords.DecodeHeader(part.Filename); err == nil {
			part.Filename = dec
		}
	}
	m.parts = append(m.parts, part)
}

// toUTF8 is text in a charset as UTF-8: UTF-8 and ASCII as they are,
// Latin-1 and Windows-1252 mapped, anything else with what isn't UTF-8
// dropped.
func toUTF8(b []byte, charset string) string {
	switch strings.ToLower(strings.TrimSpace(charset)) {
	case "iso-8859-1", "latin1", "iso8859-1", "windows-1252", "cp1252", "iso-8859-15":
		var sb strings.Builder
		for _, c := range b {
			if r, ok := cp1252[c]; ok {
				sb.WriteRune(r)
			} else {
				sb.WriteRune(rune(c))
			}
		}
		return sb.String()
	}
	return strings.ToValidUTF8(string(b), "")
}

// cp1252 are Windows-1252's characters where Latin-1 has controls.
var cp1252 = map[byte]rune{0x80: '€', 0x82: '‚', 0x83: 'ƒ', 0x84: '„', 0x85: '…', 0x86: '†', 0x87: '‡', 0x88: 'ˆ', 0x89: '‰', 0x8A: 'Š',
	0x8B: '‹', 0x8C: 'Œ', 0x8E: 'Ž', 0x91: '‘', 0x92: '’', 0x93: '“', 0x94: '”', 0x95: '•', 0x96: '–', 0x97: '—', 0x98: '˜', 0x99: '™',
	0x9A: 'š', 0x9B: '›', 0x9C: 'œ', 0x9E: 'ž', 0x9F: 'Ÿ'}

// Subject is the subject, decoded.
func (m *inboundMail) Subject() string {
	s, err := mailWords.DecodeHeader(m.Header.Get("Subject"))
	if err != nil {
		return m.Header.Get("Subject")
	}
	return s
}

// From is the sender's address (lowercased) and name.
func (m *inboundMail) From() (email, name string) {
	raw := m.Header.Get("From")
	if raw == "" {
		return "", ""
	}
	p := netmail.AddressParser{WordDecoder: mailWords}
	if addr, err := p.Parse(raw); err == nil {
		return strings.ToLower(addr.Address), strings.TrimSpace(addr.Name)
	}
	if list, err := p.ParseList(raw); err == nil && len(list) > 0 {
		return strings.ToLower(list[0].Address), strings.TrimSpace(list[0].Name)
	}
	return "", ""
}

// Date is when it was sent, or now.
func (m *inboundMail) Date() store.Time {
	if t, err := m.Header.Date(); err == nil {
		return store.TimeOf(t)
	}
	return store.Now()
}

// MessageID is its Message-ID, without the angle brackets.
func (m *inboundMail) MessageID() string {
	return strings.Trim(strings.TrimSpace(m.Header.Get("Message-ID")), "<>")
}

// Attachments are its files: parts with a name, or said to be attachments.
func (m *inboundMail) Attachments() []mailPart {
	var out []mailPart
	for _, p := range m.parts {
		if p.Filename == "" && !p.Attachment {
			continue
		}
		if p.Filename == "" {
			p.Filename = "attachment"
		}
		out = append(out, p)
	}
	return out
}

// textPart is the first part of a type that isn't a file.
func (m *inboundMail) textPart(mt string) *mailPart {
	for i, p := range m.parts {
		if p.ContentType == mt && p.Filename == "" && !p.Attachment {
			return &m.parts[i]
		}
	}
	return nil
}

// Body is what the client wrote.
func (m *inboundMail) Body() emailBody {
	var raw string
	if p := m.textPart("text/plain"); p != nil {
		raw = toUTF8(p.Data, p.Charset)
	} else if p := m.textPart("text/html"); p != nil {
		raw = mailHTMLText(toUTF8(p.Data, p.Charset))
	}
	return emailBody(mailClean(raw))
}

// emailBody is an email's own words, without the quoted mail below a
// reply, the signature and "Sent from my iPhone".
type emailBody string

// HTML is the words as paragraphs, for the rich text field.
func (b emailBody) HTML() string {
	if b == "" {
		return ""
	}
	var out strings.Builder
	for _, p := range mailParagraphs.Split(string(b), -1) {
		out.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(p), "\n", "<br>") + "</p>")
	}
	return out.String()
}

var (
	mailParagraphs = regexp.MustCompile(`\n{2,}`)
	// Where the quoted mail (or the signature) starts.
	mailQuoteStarts = []*regexp.Regexp{
		regexp.MustCompile(`(?ims)^On\b.{0,200}\bwrote:\s*$`),        // Gmail, Apple Mail (may wrap onto two lines)
		regexp.MustCompile(`(?im)^-{2,}\s*Original Message\s*-{2,}`), // Outlook, older clients
		regexp.MustCompile(`(?m)^_{10,}\s*$`),                        // Outlook's rule above the quoted message
		regexp.MustCompile(`(?im)^From:\s.+\n(Sent|Date):\s`),        // Outlook's header block
		regexp.MustCompile(`(?im)^-{2,}\s*Forwarded message\s*-{2,}`),
		regexp.MustCompile(`(?im)^Sent from my \w+`),
		regexp.MustCompile(`(?im)^Get Outlook for `),
		regexp.MustCompile(`(?m)^-- ?$`), // signature delimiter
	}
	mailTrailing = regexp.MustCompile(`(?m)[ \t]+$`)
	mailBlank    = regexp.MustCompile(`\n{3,}`)
)

// mailClean cuts the quote and signature, drops > lines, and tidies.
func mailClean(text string) string {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.ReplaceAll(text, " ", " ")
	cut := -1
	for _, re := range mailQuoteStarts {
		if loc := re.FindStringIndex(text); loc != nil && (cut < 0 || loc[0] < cut) {
			cut = loc[0]
		}
	}
	if cut >= 0 {
		text = text[:cut]
	}
	var kept []string
	for line := range strings.SplitAfterSeq(text, "\n") {
		if !strings.HasPrefix(line, ">") {
			kept = append(kept, line)
		}
	}
	text = mailTrailing.ReplaceAllString(strings.Join(kept, ""), "")
	text = mailBlank.ReplaceAllString(text, "\n\n")
	if !utf8.ValidString(text) {
		text = strings.ToValidUTF8(text, "")
	}
	return strings.TrimSpace(text)
}

// mailHTMLText is an HTML email's text, its quote and signature blocks
// removed, paragraphs and breaks kept as blank lines and newlines.
func mailHTMLText(src string) string {
	doc, err := xhtml.Parse(strings.NewReader(src))
	if err != nil {
		return plainText(src)
	}
	var drop []*xhtml.Node
	var find func(n *xhtml.Node)
	find = func(n *xhtml.Node) {
		if n.Type == xhtml.ElementNode && mailQuoteNode(n) {
			drop = append(drop, n)
			// What follows Outlook's #appendonsend, or a rule, is the
			// quoted message.
			if mailAttr(n, "id") == "appendonsend" {
				for s := n.NextSibling; s != nil; s = s.NextSibling {
					if s.Type == xhtml.ElementNode {
						drop = append(drop, s)
					}
				}
			}
			return
		}
		if n.Type == xhtml.ElementNode && n.DataAtom == atom.Hr {
			for s := n.NextSibling; s != nil; s = s.NextSibling {
				if s.Type == xhtml.ElementNode {
					drop = append(drop, s)
				}
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			find(c)
		}
	}
	find(doc)
	for _, n := range drop {
		if n.Parent != nil {
			n.Parent.RemoveChild(n)
		}
	}
	var b strings.Builder
	var write func(n *xhtml.Node)
	write = func(n *xhtml.Node) {
		switch n.Type {
		case xhtml.TextNode:
			b.WriteString(n.Data)
			return
		case xhtml.ElementNode:
			if n.DataAtom == atom.Br {
				b.WriteString("\n")
				return
			}
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			write(c)
		}
		if n.Type == xhtml.ElementNode {
			switch n.DataAtom {
			case atom.P, atom.Div, atom.Li, atom.Tr, atom.H1, atom.H2, atom.H3, atom.H4, atom.H5, atom.H6:
				b.WriteString("\n\n")
			}
		}
	}
	write(doc)
	return b.String()
}

// mailQuoteNode is whether an element is a quote, a signature, or not
// text at all.
func mailQuoteNode(n *xhtml.Node) bool {
	switch n.DataAtom {
	case atom.Blockquote, atom.Style, atom.Script, atom.Head, atom.Title:
		return true
	}
	classes := strings.FieldsSeq(mailAttr(n, "class"))
	for c := range classes {
		switch c {
		case "gmail_quote", "gmail_signature", "yahoo_quoted", "moz-cite-prefix", "moz-signature":
			return true
		}
	}
	switch mailAttr(n, "id") {
	case "appendonsend", "divRplyFwdMsg", "Signature":
		return true
	}
	return n.DataAtom == atom.Div && mailAttr(n, "name") == "messageReplySection"
}

func mailAttr(n *xhtml.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}
