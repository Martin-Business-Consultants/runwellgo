package core

import (
	"context"
	"html"
	"sort"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
)

// accountMail is one of the emails about someone's way in and the
// install's own business (a sign-in link, a password reset, an invitation,
// an export, the test message), laid out the way the Rails mailer layout
// laid them out: the install's name over the message, and at the foot why
// it was sent.
type accountMail struct {
	To        string
	Subject   string
	Preheader string
	Heading   string
	// Paragraphs are plain text; they're escaped for the HTML part.
	Paragraphs []string
	Button     string
	URL        string
	Small      string
	Reason     string
	// Text is the plain part's body (the Rails .text.erb).
	Text string
}

// sendAccountMail sends m in the background (mailLater).
func (a *App) sendAccountMail(ctx context.Context, m accountMail) {
	brand := a.BrandName(ctx)
	color := SchemeColors["fizzy"]
	if s, err := a.settings(ctx); err == nil {
		color = s.BrandColor()
	}
	text := brand + "\n" + strings.Repeat("=", len([]rune(brand))) + "\n\n" + strings.TrimSpace(m.Text) + "\n"
	if m.Reason != "" {
		text += "\n--\n" + m.Reason + "\n"
	}
	var b strings.Builder
	b.WriteString(`<!doctype html><html><body style="margin:0;padding:24px;background:#f5f5f7;font-family:-apple-system,BlinkMacSystemFont,'Segoe UI',Roboto,Helvetica,Arial,sans-serif;color:#1f2330">`)
	if m.Preheader != "" {
		b.WriteString(`<div style="display:none;max-height:0;overflow:hidden">` + html.EscapeString(m.Preheader) + `</div>`)
	}
	b.WriteString(`<div style="max-width:520px;margin:0 auto;background:#fff;border-radius:12px;padding:32px">`)
	b.WriteString(`<p style="margin:0 0 24px;font-weight:700;color:` + color + `">` + html.EscapeString(brand) + `</p>`)
	if m.Heading != "" {
		b.WriteString(`<h1 style="margin:0 0 16px;font-size:22px">` + html.EscapeString(m.Heading) + `</h1>`)
	}
	for _, p := range m.Paragraphs {
		b.WriteString(`<p style="margin:0 0 16px;line-height:1.5">` + html.EscapeString(p) + `</p>`)
	}
	if m.Button != "" && m.URL != "" {
		b.WriteString(`<p style="margin:24px 0"><a href="` + html.EscapeString(m.URL) + `" style="display:inline-block;background:` + color + `;color:#fff;text-decoration:none;font-weight:600;padding:12px 20px;border-radius:8px">` + html.EscapeString(m.Button) + `</a></p>`)
	}
	if m.Small != "" {
		b.WriteString(`<p style="margin:0 0 16px;font-size:13px;color:#6b6f7b">` + html.EscapeString(m.Small) + `</p>`)
	}
	b.WriteString(`</div>`)
	if m.Reason != "" {
		b.WriteString(`<p style="max-width:520px;margin:16px auto 0;font-size:12px;color:#8a8e99;text-align:center">` + html.EscapeString(m.Reason) + `</p>`)
	}
	b.WriteString(`</body></html>`)
	a.mailLater(mail.Message{To: []string{m.To}, Subject: m.Subject, Text: text, HTML: b.String()})
}

// roleWithArticle is "an owner", "a manager".
func roleWithArticle(role string) string {
	if role == "owner" {
		return "an owner"
	}
	return "a " + role
}

// fullInvalid is inv as a 422 whose message reads the way the Rails
// core's alert did ("Email address is already on the team"), the fields'
// errors beside it; nil when inv is empty.
func fullInvalid(inv apierr.Invalid) error {
	if len(inv) == 0 {
		return nil
	}
	keys := make([]string, 0, len(inv))
	for k := range inv {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var msgs []string
	for _, k := range keys {
		for _, m := range inv[k] {
			if k == "base" {
				msgs = append(msgs, strings.TrimSuffix(m, "."))
			} else {
				msgs = append(msgs, humanize(k)+" "+m)
			}
		}
	}
	return &apierr.Error{Status: 422, Code: "invalid", Message: toSentence(msgs, "and") + ".", Errors: inv}
}

// toSentence is "a, b and c".

// orEmpty is l, or an empty list (JSON [], not null).
func orEmpty[T any](l []T) []T {
	if l == nil {
		return []T{}
	}
	return l
}
