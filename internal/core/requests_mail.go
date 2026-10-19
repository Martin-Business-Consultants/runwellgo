package core

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/mail"
)

// RequestMailer: tells a contact who emailed a request that it came in.
// Replying adds to the same request: the reply goes to its plus address
// (requests+r42-…@) and threads on its Message-ID (mailbox.go).

// requestsReplyAddress is a request's own address on the requests address:
// requests+r42-<token>@example.com.
func (a *App) requestsReplyAddress(requestsEmail string, id int64) string {
	local, domain, _ := strings.Cut(requestsEmail, "@")
	local, _, _ = strings.Cut(local, "+")
	return fmt.Sprintf("%s+r%d-%s@%s", local, id, a.replyToken(id), domain)
}

// mailRequestReceived sends the acknowledgement (RequestMailer#received),
// threaded on the message it answers when there is one.
func (a *App) mailRequestReceived(ctx context.Context, q *Request, inReplyTo string) {
	s, err := a.settings(ctx)
	if err != nil || s.RequestsEmail == "" || q.SenderEmail == "" {
		return
	}
	_, domain, _ := strings.Cut(s.RequestsEmail, "@")
	headers := map[string]string{
		"Auto-Submitted": "auto-replied",
		"Message-ID":     fmt.Sprintf("<request-%d-%s@%s>", q.ID, a.replyToken(q.ID), domain),
	}
	if inReplyTo != "" {
		headers["In-Reply-To"] = "<" + inReplyTo + ">"
		headers["References"] = "<" + inReplyTo + ">"
	}
	thanks := "Thanks"
	if q.ContactName != "" {
		thanks += ", " + strings.Fields(q.ContactName)[0]
	}
	brand := a.BrandName(ctx)
	reason := "You're getting this because you sent this request."
	text := brand + "\n" + strings.Repeat("=", len([]rune(brand))) + "\n\n" +
		thanks + ". We have your request:\n\n> " + q.Subject + "\n\n" +
		"We'll be in touch. To add anything, reply to this email: it goes with the same request.\n\n--\n" + reason + "\n"
	body := `<h1 style="font-size:20px">We have your request</h1>` +
		"<p>" + html.EscapeString(thanks) + ". This came through:</p>" +
		`<blockquote style="border-left:3px solid ` + s.BrandColor() + `;margin:0;padding:0 0 0 12px">` + html.EscapeString(q.Subject) + "</blockquote>" +
		"<p>We’ll be in touch. To add anything, reply to this email: it goes with the same request.</p>" +
		`<p style="color:#666;font-size:12px">You’re getting this because you sent this request.</p>`
	to := q.SenderEmail
	if name := q.RequesterName(); name != "" && name != q.SenderEmail {
		to = fmt.Sprintf("%q <%s>", name, q.SenderEmail)
	}
	a.mailLater(mail.Message{
		To:      []string{to},
		ReplyTo: a.requestsReplyAddress(s.RequestsEmail, q.ID),
		Subject: "Re: " + q.Subject,
		Text:    text,
		HTML:    body,
		Headers: headers,
	})
}
