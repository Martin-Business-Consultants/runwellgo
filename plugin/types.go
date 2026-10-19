package plugin

import (
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/mail"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// The core's types a plugin meets, named here so a plugin in its own
// module needs nothing from Runwell's internal packages.

// Time is a nullable timestamp, the core's column type: written to SQLite
// as Rails writes it (UTC text), answered in JSON as Rails does (ISO 8601).
type Time = store.Time

// Now is the current time, in UTC.
func Now() Time { return store.Now() }

// TimeOf is t in UTC; the zero time is null.
func TimeOf(t time.Time) Time { return store.TimeOf(t) }

// MailMessage is an email, as Env.SendMail sends it (API.Mail builds one).
type MailMessage = mail.Message

// DB is the install's database, as the Host is built with (New); a plugin
// reaches its own tables through its *API, and its tests open one with
// plugintest.
type DB = store.DB
