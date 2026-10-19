package core

import (
	"context"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Settings is the install's one settings row: what it calls things, who its
// mail comes from, how it looks. One install is one business (or household,
// or whatever it runs).
type Settings struct {
	ID                   int64
	Terminology          map[string]any
	IndexView            string // cards, table
	Theme                string // system, light, dark
	Radius               string // sharp, fizzy, round
	Scheme               string // fizzy, forest, plum, ocean, graphite
	Font                 string
	TextSize             string
	SetupDismissedAt     store.Time
	MailFromName         string
	MailFromEmail        string
	AppHost              string
	AppProtocol          string
	TimeZone             string
	RequestsEmail        string
	ClientAgentApprovals bool
	Prices               bool
	RequireTwoFactor     bool
	ProvidersOnly        bool
	CustomCSS            string
	CustomCSSEverywhere  bool
	TestEmail            map[string]any
	SMTPAddress          string
	SMTPPort             int
	SMTPUsername         string
	SMTPPassword         string // sealed
	SMTPSecurity         string
}

// settings is the settings row, made on first use.
func (a *App) settings(ctx context.Context) (*Settings, error) {
	var s Settings
	var terms, test store.JSON
	var smtpPort store.NullInt
	scan := func() error {
		return a.store.QueryRowContext(ctx, `SELECT id, terminology, index_view, theme, radius, scheme, font, text_size, setup_dismissed_at,
			COALESCE(mail_from_name, ''), COALESCE(mail_from_email, ''), COALESCE(app_host, ''), COALESCE(app_protocol, ''), COALESCE(time_zone, ''),
			COALESCE(requests_email, ''), client_agent_approvals, prices, require_two_factor, providers_only, COALESCE(custom_css, ''), custom_css_everywhere,
			test_email, COALESCE(smtp_address, ''), smtp_port, COALESCE(smtp_username, ''), COALESCE(smtp_password, ''), smtp_security
			FROM settings ORDER BY id LIMIT 1`).Scan(&s.ID, &terms, &s.IndexView, &s.Theme, &s.Radius, &s.Scheme, &s.Font, &s.TextSize, &s.SetupDismissedAt,
			&s.MailFromName, &s.MailFromEmail, &s.AppHost, &s.AppProtocol, &s.TimeZone,
			&s.RequestsEmail, &s.ClientAgentApprovals, &s.Prices, &s.RequireTwoFactor, &s.ProvidersOnly, &s.CustomCSS, &s.CustomCSSEverywhere,
			&test, &s.SMTPAddress, &smtpPort, &s.SMTPUsername, &s.SMTPPassword, &s.SMTPSecurity)
	}
	err := scan()
	if isNoRows(err) {
		now := store.Now().DB()
		if _, err = a.store.ExecContext(ctx, `INSERT INTO settings (created_at, updated_at) VALUES (?, ?)`, now, now); err != nil {
			return nil, err
		}
		err = scan()
	}
	if err != nil {
		return nil, err
	}
	s.Terminology, s.TestEmail = terms.Map(), test.Map()
	s.SMTPPort = int(smtpPort.Int64)
	return &s, nil
}

// TermKeys are the core's things Settings › Names may rename.
var TermKeys = []string{"client", "engagement", "scope_item", "work", "commitment"}

// defaultTerms are the words for them, singular and plural.
var defaultTerms = map[string][2]string{
	"client":     {"Client", "Clients"},
	"engagement": {"Engagement", "Engagements"},
	"scope_item": {"Scope item", "Scope items"},
	"work":       {"Work", "Work"},
	"commitment": {"Commitment", "Commitments"},
}

// Labels are an engagement's kinds, each with its words and ref prefix.
var Labels = []string{"project", "work_order", "service"}

var defaultLabels = map[string][3]string{
	"project":    {"Project", "Projects", "P"},
	"work_order": {"Work order", "Work orders", "WO"},
	"service":    {"Service", "Services", "S"},
}

func form(count int) string {
	if count == 1 {
		return "one"
	}
	return "other"
}

func nested(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[k]
	}
	return strings.TrimSpace(str(cur))
}

// Term is the install's word for a core thing (client, engagement, …).
func (s *Settings) Term(key string, count int) string {
	if v := nested(s.Terminology, key, form(count)); v != "" {
		return v
	}
	d := defaultTerms[key]
	if count == 1 {
		return d[0]
	}
	return d[1]
}

// LabelName is the install's word for an engagement label.
func (s *Settings) LabelName(label string, count int) string {
	if v := nested(s.Terminology, "labels", label, form(count)); v != "" {
		return v
	}
	d := defaultLabels[label]
	if count == 1 {
		return d[0]
	}
	return d[1]
}

// LabelPrefix is the ref prefix for a label (WO, P, S).
func (s *Settings) LabelPrefix(label string) string {
	if v := nested(s.Terminology, "labels", label, "prefix"); v != "" {
		return strings.ToUpper(v)
	}
	return defaultLabels[label][2]
}

// LabelEnabled is whether a label is offered (a team turns off work orders).
func (s *Settings) LabelEnabled(label string) bool {
	labels, _ := s.Terminology["labels"].(map[string]any)
	l, _ := labels[label].(map[string]any)
	enabled, set := l["enabled"]
	return !set || enabled != false
}

// TermsJSON is every word, as the app and agents read them.
func (s *Settings) TermsJSON() Obj {
	terms := Obj{}
	for _, k := range TermKeys {
		terms = terms.Set(k, O("one", s.Term(k, 1), "other", s.Term(k, 2)))
	}
	labels := Obj{}
	for _, l := range Labels {
		labels = labels.Set(l, O("one", s.LabelName(l, 1), "other", s.LabelName(l, 2), "prefix", s.LabelPrefix(l), "enabled", s.LabelEnabled(l)))
	}
	return terms.Set("labels", labels)
}

// SchemeColors are each scheme's accent as hex, for email.
var SchemeColors = map[string]string{"fizzy": "#5d63cd", "forest": "#058931", "plum": "#a049d4", "ocean": "#00859a", "graphite": "#404249"}

// BrandColor is the scheme's accent.
func (s *Settings) BrandColor() string {
	if c, ok := SchemeColors[s.Scheme]; ok {
		return c
	}
	return SchemeColors["fizzy"]
}
