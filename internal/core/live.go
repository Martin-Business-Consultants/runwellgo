package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/plugin"
)

// Live: events pushed to people's open tabs. A plugin granted "live"
// publishes an event to the people it names (plugin.API.Publish); each tab
// of a person on staff keeps one stream open (GET /api/live, server-sent
// events) that carries only what's addressed to them, and the app hands
// each plugin's events to its screens (ui/src/lib/live.svelte.js). The core
// adds its own: "_core" "nav" when a person's sidebar counts or bell
// changed. Events are best effort and kept only briefly in memory (the
// last few hundred), so a tab that reconnects with Last-Event-ID gets what
// it missed, and one that was away longer is told to read again ("resync").
//
// The stream is there only while a plugin that's on was granted live; with
// none, /api/live answers 404 and nothing changes.

const (
	liveKept      = 512              // events kept for tabs that reconnect
	liveBuffer    = 64               // events a slow tab may fall behind before it's cut off
	liveHeartbeat = 25 * time.Second // a comment, so proxies keep the stream open
	liveLifetime  = 4 * time.Minute  // a stream ends before the server's write timeout; the tab reconnects
	liveTabs      = 12               // streams one person may hold open
	liveGrace     = 15 * time.Second // a tab reconnecting still counts as open
)

type liveEvent struct {
	id    int64
	users map[int64]bool
	body  []byte // {"plugin", "topic", "data"}
}

type liveSub struct {
	user int64
	ch   chan *liveEvent
	gone chan struct{} // closed when the hub cuts it off (a slow tab, shutdown)
	once sync.Once
}

func (s *liveSub) cut() { s.once.Do(func() { close(s.gone) }) }

// liveHub routes events to the streams open for each person.
type liveHub struct {
	mu     sync.Mutex
	seq    int64
	subs   map[int64]map[*liveSub]bool
	kept   []*liveEvent
	left   map[int64]time.Time // when each person's last stream closed
	closed bool
}

func (h *liveHub) publish(plugin, topic string, users []int64, data json.RawMessage) {
	body, err := json.Marshal(map[string]any{"plugin": plugin, "topic": topic, "data": data})
	if err != nil {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.seq++
	ev := &liveEvent{id: h.seq, users: map[int64]bool{}, body: body}
	for _, u := range users {
		ev.users[u] = true
	}
	h.kept = append(h.kept, ev)
	if len(h.kept) > liveKept {
		h.kept = h.kept[len(h.kept)-liveKept:]
	}
	for u := range ev.users {
		for s := range h.subs[u] {
			select {
			case s.ch <- ev:
			default:
				// Too far behind: cut it off; it reconnects and catches up.
				s.cut()
			}
		}
	}
}

// subscribe opens a stream for user; after is the last event the tab had
// (Last-Event-ID), and missed is what it missed since, or resync when that
// isn't kept any more.
func (h *liveHub) subscribe(user, after int64) (s *liveSub, missed []*liveEvent, resync bool, latest int64, ok bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.closed || len(h.subs[user]) >= liveTabs {
		return nil, nil, false, 0, false
	}
	if h.subs == nil {
		h.subs = map[int64]map[*liveSub]bool{}
	}
	if h.subs[user] == nil {
		h.subs[user] = map[*liveSub]bool{}
	}
	s = &liveSub{user: user, ch: make(chan *liveEvent, liveBuffer), gone: make(chan struct{})}
	h.subs[user][s] = true
	if after > 0 && after < h.seq {
		if len(h.kept) == 0 || h.kept[0].id > after+1 {
			resync = true
		} else {
			for _, ev := range h.kept {
				if ev.id > after && ev.users[user] {
					missed = append(missed, ev)
				}
			}
		}
	}
	return s, missed, resync, h.seq, true
}

func (h *liveHub) unsubscribe(s *liveSub) {
	h.mu.Lock()
	defer h.mu.Unlock()
	delete(h.subs[s.user], s)
	if len(h.subs[s.user]) == 0 {
		delete(h.subs, s.user)
		if h.left == nil {
			h.left = map[int64]time.Time{}
		}
		h.left[s.user] = time.Now()
	}
}

// online is which of users have a stream open (or had one a moment ago,
// reconnecting).
func (h *liveHub) online(users []int64) map[int64]bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	out := map[int64]bool{}
	for _, u := range users {
		out[u] = len(h.subs[u]) > 0 || time.Since(h.left[u]) < liveGrace
	}
	return out
}

// close ends every stream (the server shutting down) and opens no more.
func (h *liveHub) close() {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.closed = true
	for _, subs := range h.subs {
		for s := range subs {
			s.cut()
		}
	}
}

// EndStreams ends the live streams so a shutdown needn't wait on them.
func (a *App) EndStreams() { a.live.close() }

func init() { routeSets = append(routeSets, (*App).liveRoutes) }

func (a *App) liveRoutes(mux *http.ServeMux) {
	// The app's own stream; no agent tool (an agent asks, it isn't pushed to).
	a.handle(mux, "GET /api/live", a.liveStream)
}

// liveOn is whether a plugin that's on may publish.
func (a *App) liveOn() bool {
	for _, p := range a.enabledPlugins() {
		if p.Granted.Live {
			return true
		}
	}
	return false
}

// liveStream is one tab's stream of the events addressed to the person
// signed in.
func (a *App) liveStream(c *Ctx) error {
	if !a.liveOn() {
		return notFound("live stream")
	}
	if c.Actor.User.IsAgent() {
		return forbidden()
	}
	user := c.Actor.User.Person().ID
	after, _ := strconv.ParseInt(c.R.Header.Get("Last-Event-ID"), 10, 64)
	if after == 0 {
		after, _ = strconv.ParseInt(c.Query("after"), 10, 64)
	}
	s, missed, resync, latest, ok := a.live.subscribe(user, after)
	if !ok {
		return &apierr.Error{Status: http.StatusTooManyRequests, Code: "too_many_tabs", Message: "Too many tabs are open for live updates; close a few and reload."}
	}
	defer a.live.unsubscribe(s)
	w := c.W
	rc := http.NewResponseController(w)
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache, no-transform")
	h.Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	write := func(format string, args ...any) bool {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			return false
		}
		return rc.Flush() == nil
	}
	// The id of where it stands, so a reconnect picks up from here.
	if !write("retry: 2000\nid: %d\nevent: hello\ndata: {}\n\n", latest) {
		return nil
	}
	if resync && !write("id: %d\nevent: resync\ndata: {}\n\n", latest) {
		return nil
	}
	for _, ev := range missed {
		if !write("id: %d\ndata: %s\n\n", ev.id, ev.body) {
			return nil
		}
	}
	beat := time.NewTicker(liveHeartbeat)
	defer beat.Stop()
	end := time.NewTimer(liveLifetime)
	defer end.Stop()
	ctx := c.R.Context()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-s.gone:
			return nil
		case <-end.C:
			return nil
		case <-beat.C:
			if !write(": ping\n\n") {
				return nil
			}
		case ev := <-s.ch:
			if !write("id: %d\ndata: %s\n\n", ev.id, ev.body) {
				return nil
			}
		}
	}
}

// navChanged tells a person's tabs their sidebar counts or bell changed.
func (a *App) navChanged(users ...int64) {
	a.live.publish("_core", "nav", users, json.RawMessage(`{}`))
}

// Publish and Online: the live grant (plugin.API.Publish / Online).
func (e pluginEnv) Publish(plugin, topic string, users []int64, data json.RawMessage) {
	e.a.live.publish(plugin, topic, users, data)
}

func (e pluginEnv) Online(users []int64) map[int64]bool { return e.a.live.online(users) }

var _ plugin.LiveEnv = pluginEnv{}
