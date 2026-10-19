package core

import (
	"context"
	"encoding/hex"
	"maps"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/internal/secret"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Settings › Email: who the install's mail comes from, the outgoing mail
// server (when the environment names none), a test message to prove they
// work together, and where clients email requests and how that mail comes
// in.

func init() { routeSets = append(routeSets, (*App).emailSettingRoutes) }

// SMTPSecurities are how a mail server may be reached.
var SMTPSecurities = [][2]string{{"starttls", "STARTTLS (usually port 587)"}, {"tls", "TLS (usually port 465)"}, {"none", "None"}}

// inboundNames are the services requests' mail may come in through.
var inboundNames = [][2]string{{"postmark", "Postmark"}, {"mailgun", "Mailgun"}, {"sendgrid", "SendGrid"}, {"mandrill", "Mandrill"},
	{"relay", "a relay (Cloudflare Email Routing or your own mail server)"}}

func (a *App) emailSettingRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	a.handle(mux, "GET /api/settings/email", a.showEmailSettings, ms, tool("show_email_settings", "Show who mail comes from and how it is sent"))
	a.handle(mux, "PATCH /api/settings/email", a.updateEmailSettings, ms, tool("update_email_settings", "Set who mail comes from, and where clients email requests",
		note("from_name and from_email: the sender on every email Runwell sends, on a domain the mail server can send for. Blank uses the default sender (MAIL_FROM, or no-reply at the install's host). requests_email: the address clients email requests to, forwarded here by the inbound service (INBOUND_EMAIL_INGRESS); replies to acknowledgements come back to it."),
		body("setting", `{mail_from_name, mail_from_email, requests_email}`, true)))
	a.handle(mux, "PATCH /api/settings/email/smtp", a.updateSMTP, ms)
	a.handle(mux, "PATCH /api/settings/email/inbound", a.updateInbound, ms)
	a.handle(mux, "POST /api/settings/email/test_message", a.sendTestMessage, ms, tool("send_test_email", "Send a test email",
		note("Sends from a job; show_email_settings says whether it went, or the mail server's error."),
		bodyParam("to", "string", "The address to send it to; yours when left out.", false)))
}

func (a *App) emailJSON(ctx context.Context, s *Settings) Obj {
	name, email := a.mailSender(s)
	sender := email
	if name != "" {
		sender = name + " <" + email + ">"
	}
	defName, defEmail := a.mailSender(&Settings{})
	def := defEmail
	if defName != "" {
		def = defName + " <" + defEmail + ">"
	}
	var ingress, inboundPassword string
	_ = a.store.QueryRowContext(ctx, `SELECT COALESCE(inbound_ingress, ''), COALESCE(inbound_password, '') FROM settings WHERE id = ?`, s.ID).Scan(&ingress, &inboundPassword)
	fromEnv := os.Getenv("INBOUND_EMAIL_INGRESS")
	effective := fromEnv
	if effective == "" {
		effective = ingress
	}
	password, _ := a.openSecret(inboundPassword)
	var inboundURL any
	if effective != "" {
		path := map[string]string{"postmark": "postmark/inbound_emails", "sendgrid": "sendgrid/inbound_emails", "relay": "relay/inbound_emails",
			"mailgun": "mailgun/inbound_emails/mime", "mandrill": "mandrill/inbound_emails"}[effective]
		u := a.BaseURL() + "/rails/action_mailbox/" + path
		if effective == "postmark" || effective == "sendgrid" || effective == "relay" {
			u = strings.Replace(u, "://", "://actionmailbox:PASSWORD@", 1)
		}
		inboundURL = u
	}
	hint := "PASSWORD is the password below."
	switch {
	case effective == "mailgun":
		hint = "Mailgun signs each delivery: set MAILGUN_INGRESS_SIGNING_KEY to its webhook signing key."
	case effective == "mandrill":
		hint = "Mandrill signs each delivery: set MANDRILL_INGRESS_API_KEY to its API key."
	case fromEnv != "" || os.Getenv("RAILS_INBOUND_EMAIL_PASSWORD") != "":
		hint = "PASSWORD is RAILS_INBOUND_EMAIL_PASSWORD from the server’s environment."
	}
	smtpPassword := s.SMTPPassword != ""
	method := "smtp"
	if !SMTPFromEnvironment() && s.SMTPAddress == "" {
		method = "none"
	}
	return O("summary", "Mail comes from "+sender, "sender", sender, "from_name", nullable(s.MailFromName), "from_email", nullable(s.MailFromEmail),
		"default_sender", def, "default_name", defName, "default_email", defEmail, "delivery_method", method, "test_email", s.TestEmail,
		"requests_email", nullable(s.RequestsEmail), "inbound_ingress", nullable(effective),
		"inbound_name", nullable(choiceName(inboundNames, effective)), "inbound_from_env", fromEnv != "", "inbound_saved", nullable(ingress),
		"inbound_url", inboundURL, "inbound_hint", hint, "inbound_password", nullable(password),
		"smtp_from_env", SMTPFromEnvironment(), "smtp_env_address", nullable(os.Getenv("SMTP_ADDRESS")),
		"smtp", O("address", nullable(s.SMTPAddress), "port", nullable0(s.SMTPPort), "username", nullable(s.SMTPUsername), "password_saved", smtpPassword, "security", s.SMTPSecurity),
		"smtp_securities", choicesObj(SMTPSecurities))
}

func nullable0(n int) any {
	if n == 0 {
		return nil
	}
	return n
}

func (a *App) showEmailSettings(c *Ctx) error {
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	out := a.emailJSON(ctx, s)
	c.Summary = str(out.Get("summary"))
	return c.JSON(200, out)
}

func (a *App) updateEmailSettings(c *Ctx) error {
	body, err := c.Wrapped("setting")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	set := map[string]any{}
	bad := apierr.Invalid{}
	if has(body, "mail_from_name") {
		set["mail_from_name"] = nullable(strings.TrimSpace(str(body["mail_from_name"])))
	}
	if has(body, "mail_from_email") {
		v := strings.TrimSpace(str(body["mail_from_email"]))
		if v != "" && !validEmail(v) {
			bad.Add("mail_from_email", "should be an email address")
		}
		set["mail_from_email"] = nullable(v)
	}
	if has(body, "requests_email") {
		v := strings.ToLower(strings.TrimSpace(str(body["requests_email"])))
		if v != "" && !validEmail(v) {
			bad.Add("requests_email", "should be an email address")
		}
		set["requests_email"] = nullable(v)
	}
	if err := fullInvalid(bad); err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("settings", s.ID, set) }); err != nil {
		return err
	}
	if s, err = a.settings(ctx); err != nil {
		return err
	}
	out := a.emailJSON(ctx, s)
	if has(body, "requests_email") {
		to := s.RequestsEmail
		if to == "" {
			to = "nowhere yet"
		}
		c.Summary = "Requests by email go to " + to + "."
	} else {
		c.Summary = "Mail now comes from " + str(out.Get("sender")) + "."
	}
	return c.JSON(200, out.Set("summary", c.Summary))
}

func (a *App) updateSMTP(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	body, err := c.Wrapped("smtp")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	set := map[string]any{}
	bad := apierr.Invalid{}
	for _, k := range []string{"smtp_address", "smtp_username"} {
		if has(body, k) {
			set[k] = nullable(strings.TrimSpace(str(body[k])))
		}
	}
	if has(body, "smtp_port") {
		if v := strings.TrimSpace(str(body["smtp_port"])); v == "" || v == "<nil>" {
			set["smtp_port"] = nil
		} else if n, ok := intOf(body["smtp_port"]); !ok || n <= 0 || n >= 65536 {
			bad.Add("smtp_port", "must be greater than 0 and less than 65536")
		} else {
			set["smtp_port"] = n
		}
	}
	if has(body, "smtp_security") {
		v := str(body["smtp_security"])
		if !slices.Contains(choiceKeys(SMTPSecurities), v) {
			bad.Add("smtp_security", "is not included in the list")
		}
		set["smtp_security"] = v
	}
	if pw := str(body["smtp_password"]); pw != "" {
		set["smtp_password"] = a.secrets.Seal(pw)
	}
	if err := fullInvalid(bad); err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("settings", s.ID, set) }); err != nil {
		return err
	}
	if s, err = a.settings(ctx); err != nil {
		return err
	}
	c.Summary = "No mail server is saved."
	if s.SMTPAddress != "" {
		c.Summary = "Mail goes out through " + s.SMTPAddress + ". Send a test email to check it."
	}
	return c.JSON(200, a.emailJSON(ctx, s).Set("summary", c.Summary))
}

func (a *App) updateInbound(c *Ctx) error {
	if err := refuseAgent(c); err != nil {
		return err
	}
	body, err := c.Wrapped("inbound")
	if err != nil {
		return err
	}
	ingress := str(body["ingress"])
	if !slices.Contains([]string{"relay", "postmark", "sendgrid"}, ingress) {
		ingress = ""
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if ingress == "" {
			return t.update("settings", s.ID, map[string]any{"inbound_ingress": nil})
		}
		var stored string
		if err := t.QueryRow(`SELECT COALESCE(inbound_password, '') FROM settings WHERE id = ?`, s.ID).Scan(&stored); err != nil {
			return err
		}
		set := map[string]any{"inbound_ingress": ingress}
		if _, ok := a.openSecret(stored); !ok {
			set["inbound_password"] = a.secrets.Seal(hex.EncodeToString(secret.RandomBytes(32)))
		}
		return t.update("settings", s.ID, set)
	})
	if err != nil {
		return err
	}
	if s, err = a.settings(ctx); err != nil {
		return err
	}
	c.Summary = "Runwell isn’t receiving requests by email."
	if ingress != "" {
		c.Summary = "Requests’ mail comes in through " + choiceName(inboundNames, ingress) + ". Give it the address and password shown."
	}
	return c.JSON(200, a.emailJSON(ctx, s).Set("summary", c.Summary))
}

// sendTestMessage sends one email the way every other message goes out,
// and keeps how it went (test_email: to, requested_at, then sent_at or the
// mail server's error), which the page shows.
func (a *App) sendTestMessage(c *Ctx) error {
	var body struct {
		To string `json:"to"`
	}
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	to := strings.TrimSpace(body.To)
	if to == "" {
		to = c.Actor.User.Person().Email
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	record := func(ctx context.Context, outcome map[string]any) error {
		return a.write(ctx, nil, func(t *Tx) error {
			var raw store.JSON
			if err := t.QueryRow(`SELECT test_email FROM settings WHERE id = ?`, s.ID).Scan(&raw); err != nil {
				return err
			}
			m := raw.Map()
			maps.Copy(m, outcome)
			return t.update("settings", s.ID, map[string]any{"test_email": string(store.MustJSON(m))})
		})
	}
	if err := record(ctx, map[string]any{"to": to, "requested_at": time.Now().UTC().Format(time.RFC3339), "sent_at": nil, "error": nil}); err != nil {
		return err
	}
	a.later("test email", func(ctx context.Context) {
		host := a.Host()
		text := a.BrandName(ctx) + "\n" + strings.Repeat("=", len([]rune(a.BrandName(ctx)))) + "\n\n" +
			"This is a test message from Runwell at " + host + ". If you are reading it, Runwell can send email.\n\n--\nSent from Settings > Email to check that mail goes out.\n"
		m := mail.Message{To: []string{to}, Subject: "Runwell can send email", Text: text,
			HTML: "<p><strong>Email works</strong></p><p>This is a test message from Runwell at " + host + ". If you’re reading it, Runwell can send email: approvals, sign-in links and invitations will arrive.</p>"}
		outcome := map[string]any{"sent_at": time.Now().UTC().Format(time.RFC3339)}
		if err := a.sendMail(ctx, m); err != nil {
			outcome = map[string]any{"error": err.Error()}
		}
		if err := record(ctx, outcome); err != nil {
			a.log.Warn("test email", "err", err)
		}
	})
	c.Summary = "Sending a test email to " + to + ". This page says how it went."
	return c.JSON(200, O("summary", c.Summary))
}
