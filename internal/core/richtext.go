package core

import (
	"bytes"
	"html"
	"regexp"
	"strconv"
	"strings"

	xhtml "golang.org/x/net/html"
	"golang.org/x/net/html/atom"
)

// Rich text is HTML in a plain text column, as the Rails core kept it (Lexxy
// wrote it; the app's Tiptap editor writes it now). Every rich text field is
// cleaned on the way in (sanitizeRichText) and a plain-text value (an older
// one, or an agent's) stays readable: richTextHTML turns it into paragraphs.
//
// A mention is <span data-mention-id="12">@Ted</span> (the editor's), or the
// Rails core's Action Text attachment; mentionedUserIDs finds them.

// richTextTags are the elements rich text keeps; everything else keeps only
// its text.
var richTextTags = map[atom.Atom]bool{
	atom.P: true, atom.Br: true, atom.Div: true, atom.Span: true, atom.Strong: true, atom.B: true, atom.Em: true, atom.I: true,
	atom.U: true, atom.S: true, atom.Del: true, atom.Code: true, atom.Pre: true, atom.Blockquote: true, atom.Ul: true, atom.Ol: true,
	atom.Li: true, atom.H1: true, atom.H2: true, atom.H3: true, atom.H4: true, atom.H5: true, atom.H6: true, atom.A: true, atom.Hr: true,
	atom.Mark: true, atom.Sub: true, atom.Sup: true, atom.Table: true, atom.Thead: true, atom.Tbody: true, atom.Tr: true, atom.Th: true, atom.Td: true,
	atom.Input: true, atom.Label: true, atom.Figure: true, atom.Figcaption: true, atom.Img: true,
}

// richTextAttrs are the attributes kept, per element ("" for any).
var richTextAttrs = map[string]map[string]bool{
	"":      {"data-type": true, "data-checked": true, "data-mention-id": true, "data-mentionable": true, "data-language": true, "class": true},
	"a":     {"href": true, "title": true},
	"img":   {"src": true, "alt": true, "width": true, "height": true},
	"input": {"type": true, "checked": true, "disabled": true},
	"ol":    {"start": true},
	"td":    {"colspan": true, "rowspan": true},
	"th":    {"colspan": true, "rowspan": true},
}

var safeURL = regexp.MustCompile(`(?i)^(https?:|mailto:|tel:|/|#)`)

// sanitizeRichText cleans HTML for keeping and showing: the tags and
// attributes rich text uses, links only to http(s), mail and phone. Plain
// text comes back as it is (richTextHTML shows it).
func sanitizeRichText(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || !strings.Contains(s, "<") {
		return s
	}
	nodes, err := xhtml.ParseFragment(strings.NewReader(s), &xhtml.Node{Type: xhtml.ElementNode, Data: "div", DataAtom: atom.Div})
	if err != nil {
		return html.EscapeString(plainText(s))
	}
	var buf bytes.Buffer
	for _, n := range nodes {
		writeClean(&buf, n)
	}
	return strings.TrimSpace(buf.String())
}

func writeClean(buf *bytes.Buffer, n *xhtml.Node) {
	switch n.Type {
	case xhtml.TextNode:
		buf.WriteString(html.EscapeString(n.Data))
		return
	case xhtml.ElementNode:
	default:
		return
	}
	switch n.DataAtom {
	case atom.Script, atom.Style, atom.Iframe, atom.Object, atom.Embed, atom.Template, atom.Noscript:
		return
	}
	// The Rails core's mention: an Action Text attachment whose content is
	// the person's chip; keep their name.
	if n.Data == "action-text-attachment" {
		for _, at := range n.Attr {
			if at.Key == "content" {
				buf.WriteString(`<span data-type="mention">` + html.EscapeString(plainText(at.Val)) + `</span>`)
				return
			}
		}
	}
	keep := richTextTags[n.DataAtom]
	if keep {
		buf.WriteByte('<')
		buf.WriteString(n.Data)
		for _, at := range n.Attr {
			if !richTextAttrs[""][at.Key] && !richTextAttrs[n.Data][at.Key] {
				continue
			}
			if (at.Key == "href" || at.Key == "src") && !safeURL.MatchString(strings.TrimSpace(at.Val)) {
				continue
			}
			buf.WriteString(" " + at.Key + `="` + html.EscapeString(at.Val) + `"`)
		}
		if n.DataAtom == atom.A {
			buf.WriteString(` rel="noopener noreferrer"`)
		}
		buf.WriteByte('>')
		switch n.DataAtom {
		case atom.Br, atom.Hr, atom.Img, atom.Input:
			return
		}
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		writeClean(buf, c)
	}
	if keep {
		buf.WriteString("</" + n.Data + ">")
	}
}

// richTextHTML is a rich text value to show: its HTML cleaned, or plain
// text as paragraphs.
func richTextHTML(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.Contains(s, "<") {
		return sanitizeRichText(s)
	}
	var out []string
	for _, p := range regexp.MustCompile(`\n\s*\n`).Split(s, -1) {
		out = append(out, "<p>"+strings.ReplaceAll(html.EscapeString(strings.TrimSpace(p)), "\n", "<br>")+"</p>")
	}
	return strings.Join(out, "")
}

var mentionID = regexp.MustCompile(`data-mention-id="(\d+)"`)

// mentionedUserIDs are the people a rich text value mentions, each once.
func mentionedUserIDs(s string) []int64 {
	var ids []int64
	seen := map[int64]bool{}
	for _, m := range mentionID.FindAllStringSubmatch(s, -1) {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err == nil && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids
}
