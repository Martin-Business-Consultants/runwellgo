package chat

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// The people on staff, as chat shows them: the name they chose here (else
// their Runwell name), their picture (else their Runwell avatar), their
// status, and whether they're here: online (a tab open and in use), away
// (idle, or outside the hours they keep) or offline.

type person struct {
	ID          int64
	Name, Email string
	Role        string
}

// hours are when someone is available, in their own zone: outside them
// they show as away and nothing notifies them (it stays unread).
type hours struct {
	On    bool   `json:"on"`
	Days  []int  `json:"days"`  // 0 Sunday … 6 Saturday
	Start string `json:"start"` // "09:00"
	End   string `json:"end"`   // "17:30"
	Zone  string `json:"zone"`  // an IANA zone
}

type profile struct {
	UserID       int64
	DisplayName  string
	StatusText   string
	StatusEmoji  string
	Hours        hours
	Notify       string // all, mentions (and direct messages) or none
	Sound        bool
	Browser      bool
	AvatarFileID int64
	Active       bool   // the tab says it's in use (not idle)
	SeenAt       string // the last word from a tab
}

var notifyLevels = []string{"all", "mentions", "none"}

func defaultProfile(id int64) profile {
	return profile{UserID: id, Notify: "mentions", Sound: true, Active: true,
		Hours: hours{Days: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "17:00"}}
}

// staff is everyone active on staff, by id.
func staff(ctx context.Context, api *plugin.API) (map[int64]person, error) {
	list, err := api.Records(ctx, plugin.Query{Type: "User", Open: true, Limit: 500})
	if err != nil {
		return nil, err
	}
	out := map[int64]person{}
	for _, r := range list {
		p := person{ID: intOf(r["id"]), Name: str(r["name"]), Email: str(r["email_address"]), Role: str(r["role"])}
		if p.ID > 0 {
			out[p.ID] = p
		}
	}
	return out, nil
}

func staffIDs(people map[int64]person) []int64 {
	out := make([]int64, 0, len(people))
	for id := range people {
		out = append(out, id)
	}
	slices.Sort(out)
	return out
}

// profiles are the chat profiles of the people named (a default for
// those who have none yet).
func profiles(ctx context.Context, q plugin.Querier, userIDs []int64) (map[int64]profile, error) {
	out := map[int64]profile{}
	for _, id := range userIDs {
		out[id] = defaultProfile(id)
	}
	if len(userIDs) == 0 {
		return out, nil
	}
	rows, err := q.Query(ctx, `SELECT user_id, COALESCE(display_name, ''), COALESCE(status_text, ''), COALESCE(status_emoji, ''), COALESCE(hours, ''),
		notify, sound, browser, COALESCE(avatar_file_id, 0), active, COALESCE(seen_at, '') FROM chat_profiles WHERE user_id IN (`+placeholders(len(userIDs))+`)`, anys(userIDs)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		p := defaultProfile(0)
		var h string
		if err := rows.Scan(&p.UserID, &p.DisplayName, &p.StatusText, &p.StatusEmoji, &h, &p.Notify, &p.Sound, &p.Browser, &p.AvatarFileID, &p.Active, &p.SeenAt); err != nil {
			return nil, err
		}
		if h != "" {
			_ = json.Unmarshal([]byte(h), &p.Hours)
		}
		out[p.UserID] = p
	}
	return out, rows.Err()
}

func profileOf(ctx context.Context, q plugin.Querier, userID int64) (profile, error) {
	m, err := profiles(ctx, q, []int64{userID})
	return m[userID], err
}

// saveProfile writes a profile whole.
func saveProfile(ctx context.Context, q plugin.Querier, p profile) error {
	h, _ := json.Marshal(p.Hours)
	_, err := q.Exec(ctx, `INSERT INTO chat_profiles (user_id, display_name, status_text, status_emoji, hours, notify, sound, browser, avatar_file_id, active, seen_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id) DO UPDATE SET display_name = excluded.display_name, status_text = excluded.status_text, status_emoji = excluded.status_emoji,
		hours = excluded.hours, notify = excluded.notify, sound = excluded.sound, browser = excluded.browser, avatar_file_id = excluded.avatar_file_id,
		active = excluded.active, seen_at = excluded.seen_at, updated_at = excluded.updated_at`,
		p.UserID, p.DisplayName, p.StatusText, p.StatusEmoji, string(h), p.Notify, p.Sound, p.Browser, nullID(p.AvatarFileID), p.Active, p.SeenAt, stamp())
	return err
}

func nullID(id int64) any {
	if id <= 0 {
		return nil
	}
	return id
}

// clock reads "09:30" as minutes into the day.
func clock(s string) (int, bool) {
	var h, m int
	if _, err := fmt.Sscanf(s, "%d:%d", &h, &m); err != nil || h < 0 || h > 24 || m < 0 || m > 59 || len(s) != 5 || (h == 24 && m > 0) {
		return 0, false
	}
	return h*60 + m, true
}

// available is whether now falls in the hours (always, when they keep none).
func (h hours) available(now time.Time, site *time.Location) bool {
	if !h.On {
		return true
	}
	loc := site
	if h.Zone != "" {
		if l, err := time.LoadLocation(h.Zone); err == nil {
			loc = l
		}
	}
	if loc == nil {
		loc = time.UTC
	}
	t := now.In(loc)
	if !slices.Contains(h.Days, int(t.Weekday())) {
		return false
	}
	start, ok1 := clock(h.Start)
	end, ok2 := clock(h.End)
	if !ok1 || !ok2 {
		return true
	}
	at := t.Hour()*60 + t.Minute()
	if end <= start { // overnight: 22:00–06:00
		return at >= start || at < end
	}
	return at >= start && at < end
}

// seenWithin is whether a tab has spoken for the person lately.
func seenWithin(s string, d time.Duration) bool {
	t, err := time.Parse(time.RFC3339Nano, s)
	return err == nil && time.Since(t) < d
}

// presenceOf is online, away or offline.
func presenceOf(p profile, online, available bool) string {
	switch {
	case !online:
		return "offline"
	case !available || !p.Active || !seenWithin(p.SeenAt, 3*time.Minute):
		return "away"
	}
	return "online"
}

// directory is everyone on staff with their profiles and presence: what
// a call needs to show people.
type directory struct {
	people   map[int64]person
	profiles map[int64]profile
	removed  map[int64]string // people removed from Runwell: "Name (removed)"
	online   map[int64]bool
	site     *time.Location
	now      time.Time
}

func loadDirectory(ctx context.Context, api *plugin.API) (*directory, error) {
	people, err := staff(ctx, api)
	if err != nil {
		return nil, err
	}
	all := staffIDs(people)
	prof, err := profiles(ctx, api, all)
	if err != nil {
		return nil, err
	}
	online, err := api.Online(all)
	if err != nil {
		return nil, err
	}
	removed, err := removedPeople(ctx, api)
	if err != nil {
		return nil, err
	}
	site, _ := api.Site(ctx)
	return &directory{people: people, profiles: prof, removed: removed, online: online, site: site.Location, now: time.Now()}, nil
}

// name is what chat calls someone: their chat name, else their own.
func (d *directory) name(id int64) string {
	if p := d.profiles[id]; p.DisplayName != "" {
		return p.DisplayName
	}
	if p, ok := d.people[id]; ok {
		return p.Name
	}
	if n, ok := d.removed[id]; ok {
		return n
	}
	return "Someone"
}

func (d *directory) firstName(id int64) string {
	if f := strings.Fields(d.name(id)); len(f) > 0 {
		return f[0]
	}
	return d.name(id)
}

func avatarURL(id int64, pr profile) string {
	if pr.AvatarFileID > 0 {
		return fmt.Sprintf("/api/x/chat/avatars/%d?v=%d", id, pr.AvatarFileID)
	}
	return fmt.Sprintf("/api/users/%d/avatar", id)
}

func (d *directory) available(id int64) bool {
	return d.profiles[id].Hours.available(d.now, d.site)
}

// personJSON is someone as chat shows them.
func (d *directory) personJSON(id int64) map[string]any {
	pr := d.profiles[id]
	p := d.people[id]
	available := d.available(id)
	full := p.Name
	if _, gone := d.removed[id]; gone && p.ID == 0 {
		full = d.name(id)
	}
	return map[string]any{"id": id, "name": d.name(id), "full_name": full, "role": p.Role,
		"avatar_url": avatarURL(id, pr), "status_text": pr.StatusText, "status_emoji": pr.StatusEmoji,
		"presence": presenceOf(pr, d.online[id], available), "available": available, "active": d.people[id].ID != 0}
}

func (d *directory) peopleJSON() []map[string]any {
	out := []map[string]any{}
	ids := staffIDs(d.people)
	slices.SortFunc(ids, func(a, b int64) int { return strings.Compare(strings.ToLower(d.name(a)), strings.ToLower(d.name(b))) })
	for _, id := range ids {
		out = append(out, d.personJSON(id))
	}
	return out
}

// ref is someone as a message names them.
func (d *directory) ref(id int64) map[string]any {
	return map[string]any{"id": id, "name": d.name(id), "avatar_url": avatarURL(id, d.profiles[id])}
}

// presence is a tab saying whether its person is using it (not idle):
// everyone hears when that changes who's online.
func presence(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	me, res := caller(req)
	if res != nil {
		return res, nil
	}
	if me.Agent {
		return ok(map[string]any{"ok": true})
	}
	active := truthy(params(req)["active"])
	before, err := profileOf(ctx, api, me.ID)
	if err != nil {
		return nil, err
	}
	wasHere := before.Active && seenWithin(before.SeenAt, 3*time.Minute)
	after := before
	after.Active, after.SeenAt = active, stamp()
	if err := saveProfile(ctx, api, after); err != nil {
		return nil, err
	}
	if wasHere != active {
		// A tab that just said so is open, even before its stream is.
		if err := announceAs(ctx, api, me.ID, true); err != nil {
			return nil, err
		}
	}
	return ok(map[string]any{"ok": true})
}

// announce tells everyone how someone shows now (presence, status, name,
// picture).
func announce(ctx context.Context, api *plugin.API, userID int64) error {
	return announceAs(ctx, api, userID, false)
}

func announceAs(ctx context.Context, api *plugin.API, userID int64, here bool) error {
	d, err := loadDirectory(ctx, api)
	if err != nil {
		return err
	}
	if here {
		d.online[userID] = true
	}
	return api.Publish(ctx, "person", staffIDs(d.people), d.personJSON(userID))
}

// errNoRows is sql's, for scans of the plugin's own rows.
func isNoRows(err error) bool { return errors.Is(err, sql.ErrNoRows) }
