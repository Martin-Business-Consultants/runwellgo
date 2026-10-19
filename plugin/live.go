package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// What reaches people while they're in the app, for a plugin whose screens
// change as others act (a chat): events pushed to their open tabs, and
// notifications in their bell.
//
// "live": API.Publish sends an event to the people it names. The core keeps
// one stream per open tab (GET /api/live, server-sent events) carrying
// only what's addressed to the person signed in, and the app hands each
// plugin's events to its screens (ui/src/lib/live.svelte.js). An event is
// small (64 KB at most), best effort and not stored: a tab that was away
// is told to read again. API.Online says who has a tab open.
//
// "notify": API.Notify puts a notification in a person's bell and on
// /notifications, beside the core's own, opening the app path it names;
// emailed when it asks. Unread ones of the same Group collapse into one, so
// a busy conversation is one notification, not fifty. A plugin's
// notifications show only while it's on.
//
// API.RichText is always there: HTML cleaned the way the core cleans rich
// text, its plain text and the people it @mentions.

// Notification is something for one person to see.
type Notification struct {
	UserID int64 // who it's for: a person on staff
	// FromID is who it's from (their avatar beside it); 0 for nobody.
	FromID int64
	Title  string // "Sarah mentioned you in #general"
	Body   string // plain text, a line or two
	// Path is where opening it goes: a path in the app ("/chat?c=12").
	Path string
	// Source and Subject say what it's about ("Chat", "#general").
	Source  string
	Subject string
	// Group collapses this into an unread notification of the same group
	// (its count goes up, its words are the latest), emailed only once.
	Group string
	// Email sends it by email too, through Settings › Email.
	Email bool
}

// RichText is HTML as the core keeps it.
type RichText struct {
	HTML     string  // cleaned: the tags and links rich text keeps
	Text     string  // its plain text
	Mentions []int64 // the people it @mentions, each once
	// Mentionables are the plugins' names it @mentions ("ai:7"), each once
	// (mentions.go).
	Mentionables []string
}

// LiveEnv is what the host reaches for these, beside Env: the core
// implements it (an Env without it, a test's, answers that it can't).
type LiveEnv interface {
	// Publish pushes an event to the people named, in their open tabs;
	// Online says which of them have one open.
	Publish(plugin, topic string, userIDs []int64, data json.RawMessage)
	Online(userIDs []int64) map[int64]bool
	// Notify puts a notification in a person's bell.
	Notify(ctx context.Context, plugin string, n Notification) error
	// RichText cleans HTML the core's way, with its text and mentions.
	RichText(html string) RichText
}

var errNoLive = errors.New("this Runwell can't push live events or notify people")

func (a *API) liveEnv() (LiveEnv, error) {
	e, ok := a.h.env.(LiveEnv)
	if !ok {
		return nil, errNoLive
	}
	return e, nil
}

var topicFormat = regexp.MustCompile(`^[a-z][a-z0-9_.:-]{0,60}$`)

const maxLiveEvent = 64 << 10

// Publish pushes data, as JSON, to the open tabs of the people userIDs
// names (live), as an event on topic. It's best effort: someone with no tab
// open misses it, and reads again when they come back.
func (a *API) Publish(ctx context.Context, topic string, userIDs []int64, data any) error {
	if err := a.need("live", a.grants.Live); err != nil {
		return err
	}
	if !topicFormat.MatchString(topic) {
		return fmt.Errorf("a topic is lowercase letters, digits and . _ : -, not %q", topic)
	}
	body, err := json.Marshal(data)
	if err != nil {
		return err
	}
	if len(body) > maxLiveEvent {
		return errors.New("a live event is at most 64 KB: send an id and let the app read the rest")
	}
	e, err := a.liveEnv()
	if err != nil || len(userIDs) == 0 {
		return err
	}
	e.Publish(a.key, topic, userIDs, body)
	return nil
}

// Online is which of the people named have the app open in a tab now
// (live).
func (a *API) Online(userIDs []int64) (map[int64]bool, error) {
	if err := a.need("live", a.grants.Live); err != nil {
		return nil, err
	}
	e, err := a.liveEnv()
	if err != nil {
		return nil, err
	}
	return e.Online(userIDs), nil
}

// Notify puts n in its person's bell (notify).
func (a *API) Notify(ctx context.Context, n Notification) error {
	if err := a.need("notify", a.grants.Notify); err != nil {
		return err
	}
	n.Title, n.Body = strings.TrimSpace(n.Title), strings.TrimSpace(n.Body)
	switch {
	case n.UserID <= 0:
		return errors.New("a notification is for someone: UserID")
	case n.Title == "":
		return errors.New("a notification needs a title")
	case !strings.HasPrefix(n.Path, "/") || strings.HasPrefix(n.Path, "//"):
		return fmt.Errorf("a notification opens a path in the app, like /chat, not %q", n.Path)
	}
	n.Title, n.Body = truncate(n.Title, 200), truncate(n.Body, 1000)
	e, err := a.liveEnv()
	if err != nil {
		return err
	}
	return e.Notify(ctx, a.key, n)
}

// RichText is html cleaned the core's way, with its text and mentions.
func (a *API) RichText(html string) RichText {
	if e, ok := a.h.env.(LiveEnv); ok {
		return e.RichText(html)
	}
	return RichText{HTML: html, Text: html}
}
