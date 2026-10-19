package core

import (
	"cmp"
	"context"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// What happened since an agent last looked: every event (work moved,
// requests in, agreements sent and decided, notes and documents added…)
// after a cursor, oldest first. A scheduled bot keeps the cursor it was
// given and passes it back next time, so it hears each change once. The
// audit log's subjects (people, settings) are left out.

func init() { routeSets = append(routeSets, (*App).changeRoutes) }

func (a *App) changeRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/changes", a.listChanges, tool("changes", "What changed since a moment, or since the last call",
		note(`Events oldest first. First call: since (an ISO time, or "24h", "7d"; default 24h). Then pass back the cursor you were given as after, to hear only what's new. Narrow with client_id (or a client's name) or engagement_ref. Answers up to limit (100, at most 500) with more: true when there are others.`),
		query("since", "string", `An ISO time, or like "24h" or "7d" (default 24h).`),
		query("after", "integer", "The cursor a previous call gave: only what came after it."),
		query("client_id", "integer", "Only this client's changes."),
		query("engagement_ref", "string", "Only this engagement's changes."),
		query("limit", "integer", "How many at most (100, at most 500).")))
}

const changesMax = 500

var sinceAgo = regexp.MustCompile(`(?i)^(\d+)\s*([hd])$`)

// changesSince is where a first call starts: an ISO time, "24h", "7d".
func changesSince(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().Add(-24 * time.Hour), nil
	}
	if m := sinceAgo.FindStringSubmatch(s); m != nil {
		n, _ := strconv.Atoi(m[1])
		unit := time.Hour
		if strings.EqualFold(m[2], "d") {
			unit = 24 * time.Hour
		}
		return time.Now().Add(-time.Duration(n) * unit), nil
	}
	for _, layout := range []string{time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02T15:04", "2006-01-02"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, nil
		}
	}
	return time.Time{}, apierr.BadRequest("since should be an ISO time, or like 24h or 7d")
}

func (a *App) listChanges(c *Ctx) error {
	ctx := c.R.Context()
	limit := 100
	if raw := c.Query("limit"); raw != "" {
		n, _ := strconv.Atoi(raw)
		limit = max(1, min(n, changesMax))
	}
	security := store.Placeholders(len(SecuritySubjects))
	where, args := `e.subject_type NOT IN (`+security+`)`, idArgsStrings(SecuritySubjects)
	after, hasAfter := int64(0), c.Query("after") != ""
	if hasAfter {
		after, _ = strconv.ParseInt(c.Query("after"), 10, 64)
		where += ` AND e.id > ?`
		args = append(args, after)
	} else {
		since, err := changesSince(c.Query("since"))
		if err != nil {
			return err
		}
		where += ` AND e.occurred_at >= ?`
		args = append(args, store.TimeOf(since).DB())
	}
	clientID, engagementID, scopeName, err := a.changesScope(ctx, c)
	if err != nil {
		return err
	}
	scoped := clientID != 0 || engagementID != 0
	fetch := limit + 1
	if scoped {
		fetch = changesMax * 4
	}
	rows, err := a.store.QueryContext(ctx, `SELECT e.id, e.subject_type, e.subject_id, e.kind, e.payload, e.occurred_at, COALESCE(e.actor, ''), e.source,
			COALESCE(u.name, ''), COALESCE(u.email_address, '')
		FROM events e LEFT JOIN users u ON u.id = e.actor_user_id WHERE `+where+` ORDER BY e.id LIMIT ?`, append(args, fetch)...)
	if err != nil {
		return err
	}
	type change struct {
		id, subjectID            int64
		typ, kind, actor, source string
		userName, userEmail      string
		payload                  store.JSON
		at                       store.Time
	}
	all, err := store.ScanAll(rows, func(r *sqlRows) (change, error) {
		var ch change
		err := r.Scan(&ch.id, &ch.typ, &ch.subjectID, &ch.kind, &ch.payload, &ch.at, &ch.actor, &ch.source, &ch.userName, &ch.userEmail)
		return ch, err
	})
	if err != nil {
		return err
	}
	known := map[string]subject{}
	gone := map[string]bool{}
	lookup := func(typ string, id int64) (subject, bool, error) {
		key := fmt.Sprintf("%s:%d", typ, id)
		if s, ok := known[key]; ok {
			return s, true, nil
		}
		if gone[key] {
			return subject{}, false, nil
		}
		s, ok, err := a.subjectInfo(c.R.Context(), typ, id)
		if err != nil {
			return s, false, err
		}
		if ok {
			known[key] = s
		} else {
			gone[key] = true
		}
		return s, ok, nil
	}
	var events []change
	for _, ch := range all {
		if scoped {
			s, ok, err := lookup(ch.typ, ch.subjectID)
			if err != nil {
				return err
			}
			if !ok || (engagementID != 0 && s.EngagementID != engagementID) || (engagementID == 0 && s.ClientID != clientID) {
				continue
			}
		}
		events = append(events, ch)
	}
	more := len(events) > limit
	events = events[:min(len(events), limit)]
	var cursor int64
	switch {
	case len(events) > 0:
		cursor = events[len(events)-1].id
	case hasAfter:
		cursor = after
	default:
		_ = a.store.QueryRowContext(ctx, `SELECT COALESCE(max(id), 0) FROM events`).Scan(&cursor)
	}
	list := make([]Obj, 0, len(events))
	for _, ch := range events {
		by := cmp.Or(ch.userName, ch.userEmail, ch.actor, "system")
		source := ch.source
		if source == "agent" {
			source = "via " + strOr(ch.actor, "an agent")
		}
		var ref any
		if s, ok, err := lookup(ch.typ, ch.subjectID); err != nil {
			return err
		} else if ok {
			ref = a.agentRef(ch.typ, ch.subjectID, s.Ref, s.Name, s.Path)
		}
		list = append(list, O("id", ch.id, "what", eventTitle(ch.kind), "kind", ch.kind, "details", nullable(eventDetails(ch.payload.Map())),
			"at", ch.at, "by", by, "source", source, "subject", ref))
	}
	c.Summary = changesSummary(len(events), scoped, scopeName, more, cursor)
	if more {
		c.Crumbs = []Crumb{step("Hear the rest", "changes", "after", cursor)}
	}
	return c.JSON(200, O("summary", c.Summary, "cursor", cursor, "more", more, "changes", list))
}

// changesSummary is how many changes there are, for the scope named if
// any, and how to hear the rest.
func changesSummary(n int, scoped bool, scopeName string, more bool, cursor int64) string {
	switch {
	case n == 0 && scoped:
		return "Nothing new for " + scopeName + "."
	case n == 0:
		return "Nothing new."
	}
	summary := pluralize(n, "change")
	if more {
		summary += fmt.Sprintf(", and more: call again with after: %d", cursor)
	}
	return summary + "."
}

// changesScope is the client (client_id) or engagement (engagement_ref)
// changes are asked for, and its name; zeros for all of them.
func (a *App) changesScope(ctx context.Context, c *Ctx) (clientID, engagementID int64, scopeName string, err error) {
	if v := c.Query("client_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, 0, "", notFound("client")
		}
		if err := a.store.QueryRowContext(ctx, `SELECT name FROM clients WHERE id = ?`, id).Scan(&scopeName); isNoRows(err) {
			return 0, 0, "", notFound("client")
		} else if err != nil {
			return 0, 0, "", err
		}
		clientID = id
	}
	if ref := c.Query("engagement_ref"); ref != "" {
		if err := a.store.QueryRowContext(ctx, `SELECT id, ref FROM engagements WHERE ref = ?`, strings.ToUpper(ref)).Scan(&engagementID, &scopeName); isNoRows(err) {
			return 0, 0, "", notFound("engagement")
		} else if err != nil {
			return 0, 0, "", err
		}
	}
	return clientID, engagementID, scopeName, nil
}

func idArgsStrings(list []string) []any {
	out := make([]any, len(list))
	for i, s := range list {
		out[i] = s
	}
	return out
}
