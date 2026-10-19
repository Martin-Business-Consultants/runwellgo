package core

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Removing someone for good, and handing their work to someone else:
// deactivating keeps them (and is undone with a click); removing moves
// everything they're responsible for to a successor, deletes how they sign
// in, their agent and their row, and keeps history true: their events
// keep their name in events.actor (never the successor's id), so history
// and the audit log still name them. Plugins hear user.removed and move
// their own data.

func init() { routeSets = append(routeSets, (*App).personRemovalRoutes) }

func (a *App) personRemovalRoutes(mux *http.ServeMux) {
	mp := need("manage_people")
	a.handle(mux, "GET /api/settings/people/{id}/removal", a.previewPersonRemoval, mp, tool("preview_person_removal", "Preview removing someone",
		note("What removing them would hand to someone else (work they lead or are on, notes, commitments, …) and what would go (sign-ins, tokens, their agent), with counts, and whether you may. Deactivating (deactivate_person) is the reversible option."),
		next("remove_person", "deactivate_person")))
	a.handle(mux, "POST /api/settings/people/{id}/removal", a.removePerson, mp, tool("remove_person", "Remove someone and hand their work to someone else",
		note("Permanent. Moves everything they're responsible for to successor_id (an active person), deletes their sign-ins, tokens and agent, and removes them; history keeps their name. confirm is their name or email address. Preview with preview_person_removal first; deactivate_person is the reversible option."),
		bodyParam("successor_id", "integer", "Who gets their work: an active person's id.", true),
		bodyParam("confirm", "string", "Their name or email address, to confirm.", true)))
}

// removalColumn is one column that points at a person: moved to the
// successor, or (Group in removalGone) deleted with its row.
type removalColumn struct {
	Group, Table, Column string
}

// removalMoves are every column that means "this person is responsible for
// it" (or did it, where the record is theirs to carry on): each moves to
// the successor. assignments.user_id is moved by hand (a duplicate goes).
var removalMoves = []removalColumn{
	{"work_lead", "todos", "owner_id"},
	{"work_added", "todos", "created_by_id"},
	{"work_added", "assignments", "assigner_id"},
	{"commitments", "commitments", "user_id"},
	{"notes", "notes", "author_id"},
	{"documents", "documents", "uploaded_by_id"},
	{"engagements", "engagements", "created_by_id"},
	{"agreements", "agreement_versions", "sent_by_id"},
	{"agreements", "approvals", "recorded_by_id"},
	{"requests", "requests", "triaged_by_id"},
	{"questions", "questions", "user_id"},
	{"setup", "invitations", "invited_by_id"},
	{"setup", "webhook_endpoints", "created_by_id"},
	{"setup", "plugin_changes", "requested_by_id"},
	{"setup", "upgrades", "requested_by_id"},
	{"mentions", "mentions", "mentioner_id"},
	{"mentions", "plugin_mentions", "author_id"},
	{"mentions", "notifications", "creator_id"},
}

// removalGone are the rows that go with them: how they sign in, what was
// addressed to them alone, and what they downloaded.
var removalGone = []removalColumn{
	{"sign_ins", "sessions", "user_id"},
	{"sign_ins", "access_tokens", "user_id"},
	{"sign_ins", "account_tokens", "user_id"},
	{"sign_ins", "oauth_grants", "user_id"},
	{"sign_ins", "identities", "user_id"},
	{"notifications", "notifications", "user_id"},
	{"notifications", "plugin_notifications", "user_id"},
	{"notifications", "mentions", "mentionee_id"},
	{"exports", "exports", "user_id"},
}

// removalGroup is one line of the preview.
type removalGroup struct {
	Key, Label string
	Count      int
}

// removalLabels are the preview's words for each group, in order, with the
// install's terms.
func removalLabels(s *Settings, gone bool) [][2]string {
	if gone {
		return [][2]string{
			{"sign_ins", "Sign-ins and tokens"},
			{"agent", "Their agent"},
			{"notifications", "Their notifications"},
			{"exports", "Exports they made"},
		}
	}
	return [][2]string{
		{"work_lead", s.Term("work", 2) + " they lead"},
		{"work_on", s.Term("work", 2) + " they’re on"},
		{"work_added", s.Term("work", 2) + " they added or assigned"},
		{"commitments", s.Term("commitment", 2)},
		{"notes", "Notes they wrote"},
		{"documents", "Documents they uploaded"},
		{"engagements", s.Term("engagement", 2) + " they started"},
		{"agreements", "Agreements they sent or recorded"},
		{"requests", "Requests they triaged"},
		{"questions", "Questions for them"},
		{"setup", "Invitations, webhooks and updates they set up"},
		{"mentions", "Mentions and notifications they sent"},
	}
}

// removal is what removing p would do: who they are (and their agent),
// what moves and what goes, and why it may not happen.
type removal struct {
	Person  *User
	AgentID int64
	Moves   []removalGroup
	Gone    []removalGroup
	Refusal string // why the caller may not, "" when they may
	Status  int
}

// ids are the person and their agent: what the columns may hold.
func (r *removal) ids() []any {
	if r.AgentID != 0 {
		return []any{r.Person.ID, r.AgentID}
	}
	return []any{r.Person.ID}
}

func (r *removal) in() string { return "(" + store.Placeholders(len(r.ids())) + ")" }

// planRemoval counts what removing p would move and remove, and says
// whether actor may: not themselves, not the last active owner, and an
// owner only by an owner.
func (a *App) planRemoval(ctx context.Context, db store.Querier, actor *Actor, p *User) (*removal, error) {
	r := &removal{Person: p}
	if err := db.QueryRowContext(ctx, `SELECT id FROM users WHERE agent_owner_id = ?`, p.ID).Scan(&r.AgentID); err != nil && !isNoRows(err) {
		return nil, err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return nil, err
	}
	in, ids := r.in(), r.ids()
	counts := map[string]int{}
	for _, m := range removalMoves {
		n, err := store.Count(ctx, db, `SELECT count(*) FROM `+m.Table+` WHERE `+m.Column+` IN `+in, ids...)
		if err != nil {
			return nil, err
		}
		counts[m.Group] += n
	}
	// Work they're on beside work they lead (the lead has an assignment too).
	if counts["work_on"], err = store.Count(ctx, db, `SELECT count(*) FROM assignments x JOIN todos t ON t.id = x.todo_id
		WHERE x.user_id IN `+in+` AND (t.owner_id IS NULL OR t.owner_id NOT IN `+in+`)`, append(ids, ids...)...); err != nil {
		return nil, err
	}
	gone := map[string]int{}
	for _, g := range removalGone {
		n, err := store.Count(ctx, db, `SELECT count(*) FROM `+g.Table+` WHERE `+g.Column+` IN `+in, ids...)
		if err != nil {
			return nil, err
		}
		gone[g.Group] += n
	}
	if r.AgentID != 0 {
		gone["agent"] = 1
	}
	for _, l := range removalLabels(s, false) {
		r.Moves = append(r.Moves, removalGroup{l[0], l[1], counts[l[0]]})
	}
	for _, l := range removalLabels(s, true) {
		r.Gone = append(r.Gone, removalGroup{l[0], l[1], gone[l[0]]})
	}

	me := actor.User.Person()
	switch {
	case me.ID == p.ID:
		r.Refusal, r.Status = "You can’t remove yourself. Another owner can.", 422
	case p.Role == "owner" && me.Role != "owner":
		r.Refusal, r.Status = "Only an owner can remove an owner.", 403
	case p.Role == "owner" && p.Active():
		n, err := otherActiveOwners(ctx, db, p.ID)
		if err != nil {
			return nil, err
		}
		if n == 0 {
			r.Refusal, r.Status = needAnOwner, 422
		}
	}
	return r, nil
}

func groupsJSON(groups []removalGroup) []Obj {
	out := make([]Obj, 0, len(groups))
	for _, g := range groups {
		out = append(out, O("key", g.Key, "label", g.Label, "count", g.Count))
	}
	return out
}

// removalSuccessors are who may take someone's work: active people, not them.
func (a *App) removalSuccessors(ctx context.Context, p *User) ([]Obj, error) {
	rows, err := a.store.QueryContext(ctx, `SELECT `+userColumns+` FROM users WHERE deactivated_at IS NULL AND agent_owner_id IS NULL AND id != ? ORDER BY name`, p.ID)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (Obj, error) {
		u, err := scanUser(r)
		if err != nil {
			return nil, err
		}
		return O("id", u.ID, "name", u.DisplayName(), "email_address", u.Email, "role", u.Role), nil
	})
}

func (a *App) previewPersonRemoval(c *Ctx) error {
	ctx := c.R.Context()
	p, err := a.person(ctx, c)
	if err != nil {
		return err
	}
	r, err := a.planRemoval(ctx, a.store, c.Actor, p)
	if err != nil {
		return err
	}
	successors, err := a.removalSuccessors(ctx, p)
	if err != nil {
		return err
	}
	moving := 0
	var parts []string
	for _, g := range r.Moves {
		moving += g.Count
		if g.Count > 0 {
			parts = append(parts, fmt.Sprintf("%s: %d", strings.ToLower(g.Label[:1])+g.Label[1:], g.Count))
		}
	}
	summary := "Removing " + p.DisplayName() + " hands nothing over"
	if len(parts) > 0 {
		summary = "Removing " + p.DisplayName() + " hands over " + strings.Join(parts, ", ")
	}
	summary += "; their sign-ins, tokens and agent go. History keeps their name."
	if r.Refusal != "" {
		summary = r.Refusal
	}
	c.Summary = summary
	return c.JSON(200, O("summary", summary, "person", O("id", p.ID, "name", p.DisplayName(), "email_address", p.Email, "role", p.Role, "active", p.Active()),
		"moves", groupsJSON(r.Moves), "removes", groupsJSON(r.Gone), "moving", moving,
		"allowed", r.Refusal == "", "refusal", nullable(r.Refusal), "successors", orEmpty(successors),
		"confirm_with", p.DisplayName()))
}

func (a *App) removePerson(c *Ctx) error {
	ctx := c.R.Context()
	p, err := a.person(ctx, c)
	if err != nil {
		return err
	}
	var in struct {
		SuccessorID any    `json:"successor_id"`
		Confirm     string `json:"confirm"`
	}
	if err := c.Body(&in); err != nil {
		return err
	}
	var succ *User
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		r, err := a.planRemoval(ctx, t, c.Actor, p)
		if err != nil {
			return err
		}
		if r.Refusal != "" {
			code := "refused"
			if r.Status == 403 {
				code = "forbidden"
			}
			return &apierr.Error{Status: r.Status, Code: code, Message: r.Refusal}
		}
		inv := apierr.Invalid{}
		sid, _ := intOf(in.SuccessorID)
		if sid != 0 {
			if succ, err = scanUser(t.QueryRow(`SELECT `+userColumns+` FROM users WHERE id = ?`, sid)); err != nil && !isNoRows(err) {
				return err
			}
		}
		switch {
		case sid == 0:
			inv.Add("successor_id", "can't be blank")
		case succ == nil || succ.IsAgent():
			inv.Add("successor_id", "isn't someone on the team")
		case succ.ID == p.ID:
			inv.Add("successor_id", "can't be the person you're removing")
		case !succ.Active():
			inv.Add("successor_id", "is deactivated: choose someone active")
		}
		typed := strings.ToLower(strings.TrimSpace(in.Confirm))
		if typed == "" || (typed != strings.ToLower(strings.TrimSpace(p.Name)) && typed != strings.ToLower(p.Email)) {
			inv.Add("confirm", "doesn't match their name or email address")
		}
		if err := fullInvalid(inv); err != nil {
			return err
		}
		return t.removePerson(r, succ)
	})
	if err != nil {
		return err
	}
	c.Summary = fmt.Sprintf("Removed %s. Their work is now %s’s.", p.DisplayName(), succ.DisplayName())
	return c.JSON(200, O("summary", c.Summary, "successor", O("id", succ.ID, "name", succ.DisplayName())))
}

// removePerson does what r planned, handing everything to succ.
func (t *Tx) removePerson(r *removal, succ *User) error {
	p := r.Person
	in, ids := r.in(), r.ids()
	with := func(first ...any) []any { return append(first, ids...) }
	now := store.Now().DB()

	// What search shows them on is reindexed once it's theirs.
	reindex := map[string]string{
		"Todo":       `SELECT id FROM todos WHERE owner_id IN ` + in + ` OR created_by_id IN ` + in,
		"Commitment": `SELECT id FROM commitments WHERE user_id IN ` + in,
		"Note":       `SELECT id FROM notes WHERE author_id IN ` + in,
		"Document":   `SELECT id FROM documents WHERE uploaded_by_id IN ` + in,
		"Engagement": `SELECT id FROM engagements WHERE created_by_id IN ` + in,
		"Request":    `SELECT id FROM requests WHERE triaged_by_id IN ` + in,
	}
	for typ, q := range reindex {
		args := ids
		if typ == "Todo" {
			args = append(append([]any{}, ids...), ids...)
		}
		found, err := store.Int64s(t.Ctx, t, q, args...)
		if err != nil {
			return err
		}
		for _, id := range found {
			t.Reindex(typ, id)
		}
	}

	// History keeps who did it, never the successor: their name goes into
	// events.actor (what readers fall back to) and the id, which the
	// database won't let outlive the row, is cleared. What an app did for
	// them reads "Claude for Sarah".
	if _, err := t.Exec(`UPDATE events SET actor = CASE WHEN actor IS NULL OR actor = '' THEN ? ELSE actor || ' for ' || ? END, actor_user_id = NULL
		WHERE actor_user_id IN `+in, append([]any{p.DisplayName(), p.DisplayName()}, ids...)...); err != nil {
		return err
	}

	// Work they're on: the successor's already there, or takes their place.
	if _, err := t.Exec(`DELETE FROM assignments WHERE user_id IN `+in+` AND todo_id IN (SELECT todo_id FROM assignments WHERE user_id = ?)`, append(append([]any{}, ids...), succ.ID)...); err != nil {
		return err
	}
	if _, err := t.Exec(`UPDATE assignments SET user_id = ?, updated_at = ? WHERE user_id IN `+in, with(succ.ID, now)...); err != nil {
		return err
	}
	for _, m := range removalMoves {
		if _, err := t.Exec(`UPDATE `+m.Table+` SET `+m.Column+` = ? WHERE `+m.Column+` IN `+in, with(succ.ID)...); err != nil {
			return fmt.Errorf("%s.%s: %w", m.Table, m.Column, err)
		}
	}
	// A plugin notification they sent stays, from nobody in particular.
	if _, err := t.Exec(`UPDATE plugin_notifications SET from_id = NULL WHERE from_id IN `+in, ids...); err != nil {
		return err
	}
	// Their own invitation stays as the record it was, without them.
	if _, err := t.Exec(`UPDATE invitations SET user_id = NULL WHERE user_id IN `+in, ids...); err != nil {
		return err
	}

	// What goes: their exports (with the archives), the bell's rows for
	// them, how they sign in and what their tokens left behind.
	exports, err := store.Int64s(t.Ctx, t, `SELECT id FROM exports WHERE user_id IN `+in, ids...)
	if err != nil {
		return err
	}
	for _, id := range exports {
		if err := t.detach("Export", id, "archive"); err != nil {
			return err
		}
	}
	if _, err := t.Exec(`DELETE FROM notifications WHERE source_type = 'PluginNotification' AND source_id IN (SELECT id FROM plugin_notifications WHERE user_id IN `+in+`)`, ids...); err != nil {
		return err
	}
	for _, q := range []string{
		`DELETE FROM agent_calls WHERE access_token_id IN (SELECT id FROM access_tokens WHERE user_id IN ` + in + `)`,
		`DELETE FROM agent_idempotency_keys WHERE access_token_id IN (SELECT id FROM access_tokens WHERE user_id IN ` + in + `)`,
	} {
		if _, err := t.Exec(q, ids...); err != nil {
			return err
		}
	}
	for _, g := range removalGone {
		if _, err := t.Exec(`DELETE FROM `+g.Table+` WHERE `+g.Column+` IN `+in, ids...); err != nil {
			return fmt.Errorf("%s.%s: %w", g.Table, g.Column, err)
		}
	}
	for _, id := range ids {
		uid, _ := id.(int64) // r.ids() is user ids
		if err := t.detach("User", uid, "avatar"); err != nil {
			return err
		}
	}
	if _, err := t.Exec(`DELETE FROM users WHERE agent_owner_id = ?`, p.ID); err != nil {
		return err
	}
	if _, err := t.Exec(`DELETE FROM users WHERE id = ?`, p.ID); err != nil {
		return err
	}

	moved, gone := map[string]any{}, map[string]any{}
	for _, g := range r.Moves {
		moved[g.Key] = g.Count
	}
	for _, g := range r.Gone {
		gone[g.Key] = g.Count
	}
	payload := map[string]any{
		"id": p.ID, "name": p.DisplayName(), "email_address": p.Email,
		"successor_id": succ.ID, "successor_name": succ.DisplayName(),
		"moved": moved, "removed": gone,
	}
	if r.AgentID != 0 {
		payload["agent_id"] = r.AgentID
	}
	return t.Event("User", p.ID, "user.removed", payload)
}

// removedNames are the names of removed people, by id, from their
// user.removed events: what readers show once the row is gone.
func removedNames(ctx context.Context, db store.Querier, ids []int64) map[int64]string {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out
	}
	rows, err := db.QueryContext(ctx, `SELECT subject_id, COALESCE(json_extract(payload, '$.name'), '') FROM events
		WHERE subject_type = 'User' AND kind = 'user.removed' AND subject_id IN (`+store.Placeholders(len(ids))+`)`, idArgs(ids)...)
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var name string
		if rows.Scan(&id, &name) == nil && name != "" {
			out[id] = name
		}
	}
	_ = rows.Err() // best-effort: the names read so far
	return out
}
