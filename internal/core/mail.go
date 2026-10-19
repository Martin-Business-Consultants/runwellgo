package core

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
)

// Mail goes out through the SMTP server in the environment (SMTP_ADDRESS
// and kin win, as in the Rails core), else the one in Settings › Email. The
// sender is Settings › Email's name and address, else MAIL_FROM, else
// "Runwell <no-reply@host>". Mail is always sent in the background
// (App.later), so nothing waits on SMTP.

// Mailer delivers a built message; SMTP by default, a capture in tests.
type Mailer interface {
	Send(ctx context.Context, cfg mail.Config, from string, to []string, msg []byte) error
}

// SMTPFromEnvironment is whether SMTP_ADDRESS names the mail server.
func SMTPFromEnvironment() bool { return os.Getenv("SMTP_ADDRESS") != "" }

func (a *App) mailConfig(ctx context.Context) (mail.Config, error) {
	s, err := a.settings(ctx)
	if err != nil {
		return mail.Config{}, err
	}
	var cfg mail.Config
	if SMTPFromEnvironment() {
		port, _ := strconv.Atoi(os.Getenv("SMTP_PORT"))
		if port == 0 {
			port = 587
		}
		security := "starttls"
		switch {
		case os.Getenv("SMTP_TLS") == "true":
			security = "tls"
		case os.Getenv("SMTP_STARTTLS") == "false":
			security = "none"
		}
		cfg = mail.Config{Host: os.Getenv("SMTP_ADDRESS"), Port: port, Security: security,
			Username: os.Getenv("SMTP_USERNAME"), Password: os.Getenv("SMTP_PASSWORD")}
	} else {
		password, _ := a.secrets.Open(s.SMTPPassword)
		port := s.SMTPPort
		if port == 0 {
			port = 587
			if s.SMTPSecurity == "tls" {
				port = 465
			}
		}
		cfg = mail.Config{Host: s.SMTPAddress, Port: port, Security: s.SMTPSecurity, Username: s.SMTPUsername, Password: password}
	}
	cfg.FromName, cfg.FromEmail = a.mailSender(s)
	return cfg, nil
}

// mailSender is who mail comes from: Settings › Email's name and address,
// each falling back to its half of MAIL_FROM, else Runwell at no-reply@host.
func (a *App) mailSender(s *Settings) (name, email string) {
	defName, defEmail := "Runwell", "no-reply@"+a.Host()
	if env := os.Getenv("MAIL_FROM"); env != "" {
		if i := strings.Index(env, "<"); i >= 0 {
			defName = strings.Trim(strings.TrimSpace(env[:i]), `"`)
			defEmail = strings.Trim(env[i:], "<> ")
		} else {
			defName, defEmail = "", strings.TrimSpace(env)
		}
	}
	name, email = s.MailFromName, s.MailFromEmail
	if name == "" {
		name = defName
	}
	if email == "" {
		email = defEmail
	}
	return name, email
}

// BrandName is who the install is, in words: the sender's name, else
// Runwell.
func (a *App) BrandName(ctx context.Context) string {
	s, err := a.settings(ctx)
	if err != nil {
		return "Runwell"
	}
	if name, _ := a.mailSender(s); name != "" {
		return name
	}
	return "Runwell"
}

// sendMail checks and sends one message now. Callers in a request use
// mailLater.
func (a *App) sendMail(ctx context.Context, m mail.Message) error {
	cfg, err := a.mailConfig(ctx)
	if err != nil {
		return err
	}
	if cfg.Host == "" {
		return errors.New("email isn't set up: add a mail server in Settings › Email (or SMTP_ADDRESS)")
	}
	from, rcpt, msg, err := mail.Compose(cfg, m)
	if err != nil {
		return err
	}
	return a.mailer().Send(ctx, cfg, from, rcpt, msg)
}

// mailLater sends m in the background, logging a failure.
func (a *App) mailLater(m mail.Message) {
	a.later("mail", func(ctx context.Context) {
		if err := a.sendMail(ctx, m); err != nil {
			a.log.Warn("mail", "subject", m.Subject, "err", err)
		}
	})
}

func (a *App) mailer() Mailer {
	if a.cfg.Mailer != nil {
		return a.cfg.Mailer
	}
	return mail.SMTP{}
}
