package core

import (
	"context"
	"fmt"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// A brief: everything a local AI needs to do one piece of work and report
// back, as Markdown: what the work is and where it sits, what the client
// agreed, what's been said, what's promised, how to update Runwell with the
// `runwell` command (or MCP). Copied by the AI button on a todo, and served
// to agents by brief_work. Staff only: it includes internal notes, never
// estimates (Todo::Brief).

func (a *App) briefTodo(c *Ctx) error {
	ctx := c.R.Context()
	id, err := c.IntParam("id")
	if err != nil {
		return err
	}
	td, err := a.todo(ctx, a.store, id)
	if err != nil {
		return err
	}
	brief, err := a.todoBrief(ctx, td, c.Actor.User.Person())
	if err != nil {
		return err
	}
	if c.Query("format") == "text" || strings.HasPrefix(c.R.Header.Get("Accept"), "text/plain") {
		c.W.Header().Set("Content-Type", "text/plain; charset=utf-8")
		_, err := c.W.Write([]byte(brief))
		return err
	}
	c.Summary = fmt.Sprintf("Brief for %s (Todo:%d)", td.Title, td.ID)
	c.Crumbs = []Crumb{step("Start it", "update_work", "id", id, "todo", map[string]any{"status": "in_progress"}),
		step("Leave a note", "add_note", "record", fmt.Sprintf("Todo:%d", id))}
	return c.JSON(200, O("summary", c.Summary, "todo", a.agentRef("Todo", td.ID, "", td.Title, fmt.Sprintf("/work/%d", td.ID)), "brief", brief))
}

func (a *App) todoBrief(ctx context.Context, td *Todo, person *User) (string, error) {
	base := a.BaseURL()
	record := fmt.Sprintf("Todo:%d", td.ID)
	engs, err := a.workEngagements(ctx, a.store, []int64{td.EngagementID})
	if err != nil {
		return "", err
	}
	e := engs[td.EngagementID]
	if e == nil {
		return "", notFound("engagement")
	}
	s, err := a.settings(ctx)
	if err != nil {
		return "", err
	}
	assigned, err := todoAssignees(ctx, a.store, []int64{td.ID})
	if err != nil {
		return "", err
	}
	owners := todoOwners(td, assigned[td.ID])
	names, err := workPeople(ctx, a.store, owners)
	if err != nil {
		return "", err
	}
	var sections []string

	// The heading: where the work sits.
	lines := []string{"# " + td.Title, "",
		fmt.Sprintf("Runwell work item %s: %s/work/%d", record, base, td.ID),
		"Client: " + e.ClientName,
		fmt.Sprintf("%s: %s %s", s.LabelName(e.Label, 1), e.Ref, e.Title)}
	if td.ScopeItemID != 0 {
		var item string
		if a.store.QueryRowContext(ctx, `SELECT description FROM scope_items WHERE id = ?`, td.ScopeItemID).Scan(&item) == nil {
			lines = append(lines, "Scope item: "+item)
		}
	}
	status := "Status: " + humanize(td.Status)
	if td.OwnerID != 0 {
		status += " · Lead: " + names[td.OwnerID]
		var others []string
		for _, id := range owners {
			if id != td.OwnerID {
				others = append(others, names[id])
			}
		}
		if len(others) > 0 {
			status += " · Also on it: " + workSentence(others)
		}
	} else {
		status += " · Unassigned"
	}
	if td.DueOn != "" {
		status += " · Due: " + td.DueOn
	}
	sections = append(sections, strings.Join(append(lines, status), "\n"))

	text := workPlain(td.Description)
	if text == "" {
		text = "(No description. Read the notes below, and ask if it's unclear.)"
	}
	sections = append(sections, "## The work\n\n"+text)

	// What the client approved, as they saw it: the measure for whether
	// this is in scope.
	var snap store.JSON
	err = a.store.QueryRowContext(ctx, `SELECT av.snapshot FROM agreement_versions av
		WHERE av.engagement_id = ? AND EXISTS (SELECT 1 FROM approvals ap WHERE ap.agreement_version_id = av.id AND ap.decision = 'approved')
		ORDER BY av.number DESC, av.id DESC LIMIT 1`, e.ID).Scan(&snap)
	switch {
	case isNoRows(err):
		sections = append(sections, "## Agreed scope\n\nNothing approved yet for "+e.Ref+": treat the scope as unsettled.")
	case err != nil:
		return "", err
	default:
		m := snap.Map()
		parts := []string{"## Agreed scope (" + e.Ref + ", as the client approved it)"}
		if summary := strings.TrimSpace(str(m["summary"])); summary != "" {
			parts = append(parts, summary)
		}
		var items []string
		for _, it := range asList(m["items"]) {
			if im, ok := it.(map[string]any); ok {
				items = append(items, "- "+str(im["description"]))
			}
		}
		if len(items) > 0 {
			parts = append(parts, strings.Join(items, "\n"))
		}
		sections = append(sections, strings.Join(parts, "\n\n"))
	}

	// Recent notes, newest first.
	rows, err := a.store.QueryContext(ctx, `SELECT n.occurred_at, COALESCE(NULLIF(u.name, ''), u.email_address, 'someone'), n.kind, n.body
		FROM notes n LEFT JOIN users u ON u.id = n.author_id WHERE n.subject_type = 'Todo' AND n.subject_id = ? ORDER BY n.occurred_at DESC LIMIT 5`, td.ID)
	if err != nil {
		return "", err
	}
	notes, err := store.ScanAll(rows, func(r *sqlRows) (string, error) {
		var at store.Time
		var who, kind, body string
		if err := r.Scan(&at, &who, &kind, &body); err != nil {
			return "", err
		}
		return fmt.Sprintf("- %s %s (%s): %s", at.In(a.zone(ctx)).Format("2006-01-02"), who, kind, truncate(strings.Join(strings.Fields(workPlain(body)), " "), 400)), nil
	})
	if err != nil {
		return "", err
	}
	if len(notes) > 0 {
		sections = append(sections, "## Recent notes (newest first)\n\n"+strings.Join(notes, "\n"))
	}

	// What's promised on the engagement.
	open, err := a.loadCommitments(ctx, a.store, `cm.engagement_id = ? AND cm.resolution IS NULL ORDER BY cm.due_on, cm.id`, e.ID)
	if err != nil {
		return "", err
	}
	if len(open) > 0 {
		list := make([]string, len(open))
		for i, cm := range open {
			list[i] = fmt.Sprintf("- %s · %s · due %s", cm.Description, cm.OwnerName, cm.DueOn)
		}
		sections = append(sections, "## Open commitments on "+e.Ref+"\n\n"+strings.Join(list, "\n"))
	}

	agent, personName := "your agent", "your person"
	if person != nil {
		personName = person.DisplayName()
		var name string
		if a.store.QueryRowContext(ctx, `SELECT CASE WHEN name = '' THEN email_address ELSE name END FROM users WHERE agent_owner_id = ?`, person.ID).Scan(&name) == nil {
			agent = name
		}
	}
	sections = append(sections, fmt.Sprintf(`## Working with Runwell

You act as %[1]s: everything you do is recorded as %[1]s, with the app's name, and has exactly %[2]s's permissions.

Sign in once with the `+"`runwell`"+` command (or connect an MCP client to %[3]s/mcp):

`+"```sh"+`
curl -fsSL %[3]s/install/cli | sh    # if you don't have it yet
runwell login %[3]s
`+"```"+`

Then, as you work:

`+"```sh"+`
runwell show_work --id %[4]d                                  # the item, fresh
runwell update_work --id %[4]d --todo.status in_progress      # when you start
runwell add_note --record %[5]s --note.body "What you did, found or decided"
runwell update_work --id %[4]d --todo.status blocked          # stuck: add a note saying why
runwell update_work --id %[4]d --todo.status in_review        # finished: a person checks it and marks it done
`+"```"+`

`+"`runwell tools`"+` lists everything you can do; `+"`runwell help <tool>`"+` explains one.`, agent, personName, base, td.ID, record))

	sections = append(sections, `## Ground rules

- Stay within the agreed scope. If the work needs more than was agreed, stop and leave a note saying so rather than doing it.
- Leave a note when you start, when you make a decision, and when you finish, so a person can follow what happened.
- When you're finished, set the status to in_review, not done: a person checks the work and marks it done.
- Anything that reaches the client (sending, emailing, making something visible to them) will ask for confirmation. Don't confirm it on a person's behalf.`)

	return strings.Join(sections, "\n\n") + "\n", nil
}

// workSentence is Rails' to_sentence: "a", "a and b", "a, b, and c".
func workSentence(words []string) string {
	switch len(words) {
	case 0:
		return ""
	case 1:
		return words[0]
	case 2:
		return words[0] + " and " + words[1]
	}
	return strings.Join(words[:len(words)-1], ", ") + ", and " + words[len(words)-1]
}
