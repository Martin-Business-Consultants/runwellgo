package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
)

// Names people can @mention that aren't people (an AI agent), and what
// happens when they do.
//
// "mentionables": a plugin offers names (Module.Mentionables), listed in
// every @ picker while it's on and drawn as a mention chip, written
// <span data-mentionable="<key>:<id>">@Name</span>. When a note on a record
// mentions one, or another plugin relays a mention from its own content
// (a chat message), the core delivers mention.created to the plugin that
// offers it, with the person who mentioned it as the call's caller (so it
// can read as them through the tools, never change anything without their
// approval). It answers where it was mentioned with API.Reply: a note on
// the record written as the mentionable ("Researcher · via AI for Ted"), or
// a reply in the relaying plugin's own thread (Module.OnMentionReply).
//
// "mentions:relay": a plugin whose content can mention (a chat) passes
// those mentions on with API.RelayMention, as the person who wrote them,
// and takes the replies in its OnMentionReply.
//
// API.Emit (always) is an event of the plugin's own (<key>.<what>) for
// other plugins that hook it, with no record and no caller.

// Mentionable is a name a plugin offers to @mention.
type Mentionable struct {
	// ID is the plugin's own id for it ("7"); the app names it "<key>:<id>".
	ID     string `json:"id"`
	Handle string `json:"handle"` // what's typed after @: lowercase letters, digits, _ . -
	Name   string `json:"name"`
	// Avatar is an app path to its picture (one of the plugin's GET routes,
	// /api/x/<key>/…), or "" for its initials.
	Avatar      string `json:"avatar,omitempty"`
	Description string `json:"description,omitempty"`
	Kind        string `json:"kind"` // "agent"
	// Roles may mention it (owner, manager, member); everyone when empty.
	Roles []string `json:"roles,omitempty"`
	// Paused ones are listed (and mentioning one is answered), marked paused.
	Paused bool `json:"paused,omitempty"`
}

// Key is its full name, as the app and the mention chip write it.
func (m Mentionable) key(plugin string) string { return plugin + ":" + m.ID }

// MentionedBy is whether someone with role may mention it.
func (m Mentionable) MentionedBy(role string) bool {
	return len(m.Roles) == 0 || slices.Contains(m.Roles, role)
}

// Mention is a mention a plugin relays from its own content
// (mentions:relay): who wrote it is the route's caller.
type Mention struct {
	// Mentionable is "<key>:<id>", as the chip names it.
	Mentionable string
	// Text and HTML are what was written.
	Text string
	HTML string
	// Path is where it was written, in the app ("/chat?c=3&thread=12");
	// Subject names it ("#general").
	Path    string
	Subject string
	// Context is what was said before it (a thread so far), as plain text.
	Context string
	// Ref is the relaying plugin's own address for replies ("12": the
	// message to reply under), handed back in OnMentionReply.
	Ref string
}

// MentionEvent is mention.created's data: what the mentioned plugin hears.
type MentionEvent struct {
	ID int64 `json:"id"` // the mention, to Reply to
	// Mentionable is the plugin's own id for what was mentioned.
	Mentionable string `json:"mentionable"`
	// Source is "note" (a note on a record) or the relaying plugin's key.
	Source string `json:"source"`
	// Record is the record a note is on ("Todo:12"); NoteID the note.
	Record  string `json:"record,omitempty"`
	NoteID  int64  `json:"note_id,omitempty"`
	Author  Person `json:"author"`
	Text    string `json:"text"`
	HTML    string `json:"html"`
	Path    string `json:"path"`
	Subject string `json:"subject,omitempty"`
	Context string `json:"context,omitempty"`
	At      string `json:"occurred_at"`
}

// Person is someone on staff, by id and name.
type Person struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

// Reply answers a mention where it was made (mentionables): Body is text
// or HTML. Replace is an earlier reply's id (what Reply answered), to
// change it rather than add another ("On it…" becoming the answer).
type Reply struct {
	Mention int64
	Body    string
	Replace string
}

// MentionReply is a reply, as a relaying plugin's OnMentionReply takes it.
type MentionReply struct {
	Ref string // the Mention's Ref
	// As is who's replying: the mentionable, ID "<key>:<id>"; Via is its
	// plugin's name.
	As      Mentionable
	Via     string
	For     Person // who mentioned it
	HTML    string // cleaned the core's way
	Text    string
	Replace string
}

// MentionEnv is what the host reaches for mentions, beside Env.
type MentionEnv interface {
	// RecordMention keeps a mention actor made in source's content of one
	// of owner's mentionables: what owner hears, and actor as its caller.
	RecordMention(ctx context.Context, owner, source string, actor Actor, m Mention) (MentionEvent, *Caller, error)
	// ReplyToMention answers a mention of one of plugin's mentionables.
	ReplyToMention(ctx context.Context, plugin string, r Reply) (string, error)
}

var errNoMentions = errors.New("this Runwell doesn't relay mentions")

func (a *API) mentionEnv() (MentionEnv, error) {
	e, ok := a.h.env.(MentionEnv)
	if !ok {
		return nil, errNoMentions
	}
	return e, nil
}

// ---- the host's side --------------------------------------------------------------------

var handleFormat = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,39}$`)

// mentionCache keeps each plugin's mentionables for a short while: the @
// pickers ask on every page.
type mentionCache struct {
	mu   sync.Mutex
	list map[string][]Mentionable
	at   map[string]time.Time
}

const mentionTTL = time.Minute

// Mentionables are the names the enabled plugins granted "mentionables"
// offer, ID "<key>:<id>", by plugin then name.
func (h *Host) Mentionables(ctx context.Context) []Mentionable {
	out := []Mentionable{}
	for _, p := range h.List() {
		if !p.Enabled || p.module == nil || p.module.Mentionables == nil || !p.Granted.Mentionables {
			continue
		}
		for _, m := range h.mentionablesOf(ctx, p) {
			m.ID = m.key(p.Key)
			out = append(out, m)
		}
	}
	return out
}

// Mentionable is one by its full name ("ai:7"), and its plugin's key; nil
// when no enabled plugin offers it.
func (h *Host) Mentionable(ctx context.Context, full string) (*Mentionable, string) {
	key, id, ok := strings.Cut(full, ":")
	if !ok {
		return nil, ""
	}
	p := h.Get(key)
	if p == nil || !p.Enabled || p.module == nil || p.module.Mentionables == nil || !p.Granted.Mentionables {
		return nil, ""
	}
	for _, m := range h.mentionablesOf(ctx, p) {
		if m.ID == id {
			return &m, key
		}
	}
	return nil, ""
}

func (h *Host) mentionablesOf(ctx context.Context, p *Plugin) []Mentionable {
	h.mentions.mu.Lock()
	if h.mentions.list == nil {
		h.mentions.list, h.mentions.at = map[string][]Mentionable{}, map[string]time.Time{}
	}
	if at, ok := h.mentions.at[p.Key]; ok && time.Since(at) < mentionTTL {
		list := h.mentions.list[p.Key]
		h.mentions.mu.Unlock()
		return list
	}
	h.mentions.mu.Unlock()
	_, api := h.enabled(p.Key, nil)
	if api == nil {
		return nil
	}
	var list []Mentionable
	err := guard(ctx, "plugin "+p.Key+" mentionables", func(ctx context.Context) error {
		var err error
		list, err = p.module.Mentionables(ctx, api)
		return err
	})
	if err != nil {
		h.log.Warn("plugin mentionables", "plugin", p.Key, "err", err)
		return nil
	}
	kept := list[:0]
	for _, m := range list {
		if m.ID == "" || strings.ContainsAny(m.ID, ": \"<>") || !handleFormat.MatchString(m.Handle) || strings.TrimSpace(m.Name) == "" {
			h.log.Warn("plugin mentionable left out: it needs an id, a name and a handle", "plugin", p.Key, "id", m.ID)
			continue
		}
		if m.Kind == "" {
			m.Kind = "agent"
		}
		kept = append(kept, m)
	}
	h.mentions.mu.Lock()
	h.mentions.list[p.Key], h.mentions.at[p.Key] = kept, time.Now()
	h.mentions.mu.Unlock()
	return kept
}

// forgetMentionables drops a plugin's cached names (or all, for "").
func (h *Host) forgetMentionables(key string) {
	h.mentions.mu.Lock()
	defer h.mentions.mu.Unlock()
	if key == "" {
		h.mentions.at = nil
		h.mentions.list = nil
		return
	}
	delete(h.mentions.at, key)
}

// DeliverMention hands mention.created to the plugin that offers what was
// mentioned, in the background, with the person who mentioned it as the
// call's caller (actor as the core knows them): it reads as them, and any
// change waits on their approval.
func (h *Host) DeliverMention(ctx context.Context, key string, ev MentionEvent, caller *Caller, actor Actor) {
	body, err := json.Marshal(ev)
	if err != nil {
		return
	}
	h.spawn(ctx, "plugin mention", func(ctx context.Context) {
		p, api := h.enabled(key, actor)
		if p == nil || p.module.OnEvent == nil || !p.Granted.Mentionables {
			return
		}
		api.caller = caller
		err := guard(ctx, "plugin "+key+" on mention.created", func(ctx context.Context) error {
			return p.module.OnEvent(ctx, api, "mention.created", body)
		})
		if err != nil {
			h.log.Warn("plugin mention", "plugin", key, "err", err)
		}
	})
}

// ReplyTo hands a reply to the plugin a mention was relayed from, as its
// OnMentionReply takes it; it answers the reply's id there.
func (h *Host) ReplyTo(ctx context.Context, key string, r MentionReply) (string, error) {
	p, api := h.enabled(key, nil)
	if p == nil || p.module.OnMentionReply == nil || !p.Granted.MentionsRelay {
		return "", fmt.Errorf("%s isn't on to take the reply", key)
	}
	var id string
	err := guard(ctx, "plugin "+key+" mention reply", func(ctx context.Context) error {
		var err error
		id, err = p.module.OnMentionReply(ctx, api, r)
		return err
	})
	return id, err
}

// ---- the plugin's side ------------------------------------------------------------------

// MentionablesChanged tells the host this plugin's names changed (one
// added, renamed, paused), so pickers show them at once (mentionables).
func (a *API) MentionablesChanged() {
	a.h.forgetMentionables(a.key)
}

// Caller is who the call is for: a route's caller, or the person who
// mentioned one of the plugin's mentionables (mention.created); nil for
// the system (an event, a task).
func (a *API) Caller() *Caller { return a.caller }

// RelayMention passes on a mention in the plugin's own content, written by
// the route's caller (mentions:relay), to the plugin that offers it. It
// answers false (and does nothing) when nothing on offers it, or the
// caller's role may not mention it.
func (a *API) RelayMention(ctx context.Context, m Mention) (bool, error) {
	if err := a.need("mentions:relay", a.grants.MentionsRelay); err != nil {
		return false, err
	}
	if a.caller == nil || a.actor == nil {
		return false, errors.New("a mention is relayed as the person who wrote it, from their request")
	}
	found, owner := a.h.Mentionable(ctx, m.Mentionable)
	if found == nil || !found.MentionedBy(a.caller.Role) {
		return false, nil
	}
	e, err := a.mentionEnv()
	if err != nil {
		return false, err
	}
	m.Text, m.Context = truncate(m.Text, 8000), truncate(m.Context, 16000)
	ev, caller, err := e.RecordMention(ctx, owner, a.key, a.actor, m)
	if err != nil {
		return false, err
	}
	a.h.DeliverMention(context.WithoutCancel(ctx), owner, ev, caller, a.actor)
	return true, nil
}

// Reply answers a mention of one of the plugin's mentionables where it
// was made (mentionables), and answers the reply's id (to Replace later).
func (a *API) Reply(ctx context.Context, r Reply) (string, error) {
	if err := a.need("mentionables", a.grants.Mentionables); err != nil {
		return "", err
	}
	if strings.TrimSpace(r.Body) == "" {
		return "", errors.New("a reply says something")
	}
	e, err := a.mentionEnv()
	if err != nil {
		return "", err
	}
	return e.ReplyToMention(ctx, a.key, r)
}

var emitFormat = regexp.MustCompile(`^[a-z][a-z0-9_]*\.[a-z][a-z_.]*[a-z]$`)

// Emit sends an event of the plugin's own, "<key>.<what>"
// (chat.message.mentioned), to the plugins that hook it, in the
// background (always): there's no record and no caller.
func (a *API) Emit(ctx context.Context, name string, data any) error {
	if !strings.HasPrefix(name, a.key+".") || !emitFormat.MatchString(name) {
		return fmt.Errorf("a plugin's events are named %s.<what>, in lowercase letters, . and _", a.key)
	}
	a.h.Emit(context.WithoutCancel(ctx), name, data)
	return nil
}
