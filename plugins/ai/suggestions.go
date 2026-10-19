package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Suggestions on records (AiSuggestion, Ai::Suggestions): what a request
// should become, the promises and work in a note, the day ahead, the scope
// of a draft and a check before it's sent, the work a scope item needs, a
// reminder for a commitment, the week with a client. Each asks the model
// for an answer in a strict JSON shape, in the background, from the record
// as the person's own show tool gives it, and is used in one click: applied
// through the agent tools (or the write verbs) as that person, in their own
// request, or dismissed.

type definition struct {
	kind   string
	label  string
	types  []string
	fast   bool
	schema map[string]any
	prompt func(ctx context.Context, g *gen) (string, error)
	apply  func(ctx context.Context, a *applying) (string, error)
}

// gen is what a prompt is built from: the person, the record and, for a
// note, the record it's on.
type gen struct {
	api     *plugin.API
	caller  *plugin.Caller
	typ     string
	id      int64
	on      string // a note's record, "Client:4"
	payload map[string]any
}

type applying struct {
	gen
	req       *plugin.Request
	selection []string
}

// refused is a suggestion the person can't use as it is, in words.
type refused struct{ msg string }

func (r *refused) Error() string { return r.msg }

func refuseUse(format string, args ...any) error { return &refused{fmt.Sprintf(format, args...)} }

// ---- schema helpers (strict: every field present, nothing else) --------------

func sObject(props map[string]any) map[string]any {
	req := make([]string, 0, len(props))
	for _, k := range sortedKeys(props) {
		req = append(req, k)
	}
	return map[string]any{"type": "object", "properties": props, "required": req, "additionalProperties": false}
}

func sList(item map[string]any) map[string]any { return map[string]any{"type": "array", "items": item} }

func sText(desc string) map[string]any {
	if desc == "" {
		return map[string]any{"type": "string"}
	}
	return map[string]any{"type": "string", "description": desc}
}

func sDate() map[string]any {
	return map[string]any{"type": "string", "description": `YYYY-MM-DD, or "" when there's no date`}
}

func sChoice(values []string, desc string) map[string]any {
	enum := make([]any, len(values))
	for i, v := range values {
		enum[i] = v
	}
	m := map[string]any{"type": "string", "enum": enum}
	if desc != "" {
		m["description"] = desc
	}
	return m
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// ---- the kinds ---------------------------------------------------------------------------

var definitions = map[string]*definition{}

func define(d *definition) { definitions[d.kind] = d }

func init() {
	define(&definition{kind: "triage", label: "Suggest how to triage", types: []string{"Request"},
		schema: sObject(map[string]any{
			"become":         sChoice([]string{"new_engagement", "scope", "work", "commitment", "dismiss"}, ""),
			"engagement_ref": sText(`For scope, work or a commitment: the engagement's ref, or ""`),
			"label":          sChoice([]string{"project", "work_order", "service", ""}, "For a new engagement: its kind"),
			"owner":          sChoice([]string{"us", "client"}, "For a commitment: who promises"),
			"due_on":         sDate(),
			"out_of_scope":   map[string]any{"type": "boolean", "description": "True when it asks for more than the engagement's agreed scope"},
			"reason":         sText("One or two sentences: why"),
		}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			req, err := g.record(ctx, "Request")
			if err != nil {
				return "", err
			}
			engagements := "none: no client yet"
			if clientID := clientOf(ctx, g.api, req); clientID != 0 {
				recs, err := g.api.Records(ctx, plugin.Query{Type: "Engagement", ClientID: clientID, Limit: 50})
				if err != nil {
					return "", err
				}
				var parts []string
				for _, e := range recs {
					state := "open"
					if e["closed"] == true {
						state = "closed"
					}
					parts = append(parts, fmt.Sprintf("%s %s (%s)", str(e["ref"]), str(e["title"]), state))
				}
				engagements = strings.Join(parts, "; ")
			}
			return fmt.Sprintf(`Triage this request: should it become a new engagement, scope on an existing engagement's agreement (a change), work on an engagement, a commitment (a dated promise), or be dismissed? Prefer existing engagements when it fits one; say if it's beyond what was agreed.

The client's engagements: %s

The request:
%s`, engagements, g.show(ctx, "show_request", "id", strconv.FormatInt(g.id, 10))), nil
		},
		apply: func(ctx context.Context, a *applying) (string, error) {
			if !a.caller.Can("triage_requests") {
				return "", refuseUse("Only people who may triage can use this.")
			}
			p := a.payload
			id := strconv.FormatInt(a.id, 10)
			ref := strings.ToUpper(strings.TrimSpace(str(p["engagement_ref"])))
			due := parseDate(str(p["due_on"]))
			var err error
			switch str(p["become"]) {
			case "new_engagement":
				args := map[string]any{"id": id}
				if l := str(p["label"]); l != "" {
					args["label"] = l
				}
				err = a.run(ctx, "promote_request_to_engagement", args)
			case "scope":
				if ref == "" {
					return "", refuseUse("Pick the engagement yourself: %s isn't one of theirs.", str(p["engagement_ref"]))
				}
				err = a.run(ctx, "promote_request_to_change", map[string]any{"id": id, "engagement_ref": ref})
			case "work":
				if ref == "" {
					return "", refuseUse("Pick the engagement yourself.")
				}
				args := map[string]any{"id": id, "engagement_ref": ref}
				if due != "" {
					args["due_on"] = due
				}
				err = a.run(ctx, "promote_request_to_todo", args)
			case "commitment":
				if due == "" {
					due = time.Now().AddDate(0, 0, 7).Format(time.DateOnly)
				}
				args := map[string]any{"id": id, "due_on": due, "owner_kind": "us", "user_id": a.caller.ID}
				if str(p["owner"]) == "client" {
					args["owner_kind"] = "client"
					delete(args, "user_id")
				}
				if ref != "" {
					args["engagement_ref"] = ref
				}
				err = a.run(ctx, "promote_request_to_commitment", args)
			case "dismiss":
				err = a.run(ctx, "dismiss_request", map[string]any{"id": id, "reason": truncate(str(p["reason"]), 250)})
			default:
				return "", refuseUse("The suggestion didn't say what to do; triage it yourself.")
			}
			if err != nil {
				return "", err
			}
			return "Triaged as suggested.", nil
		},
	})

	define(&definition{kind: "note_actions", label: "Find promises and work", types: []string{"Note"}, fast: true,
		schema: sObject(map[string]any{
			"commitments": sList(sObject(map[string]any{"owner": sChoice([]string{"us", "client"}, ""), "description": sText("What will be done, starting with a verb"), "due_on": sDate()})),
			"tasks":       sList(sObject(map[string]any{"title": sText(""), "engagement_ref": sText(`The engagement it belongs to, or ""`), "due_on": sDate()})),
		}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			note, onJSON, err := g.note(ctx)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf(`Read this note and list what it commits anyone to. Commitments are promises with a date between us and the client ("Rosa will send the logo by Friday": owner client; "we'll share the draft by the 14th": owner us). Tasks are work for us to do. Only what the note says; nothing invented. Today is %s.

It's on %s: %s

The note (%s, %s):
%s`, today(ctx, g.api), g.on, truncate(onJSON, 4000), str(note["kind"]), dateOf(str(note["occurred_at"])), truncate(plainText(str(note["body_html"])+str(note["body"])), 6000)), nil
		},
		apply: func(ctx context.Context, a *applying) (string, error) {
			clientID, engagement, err := a.noteParents(ctx)
			if err != nil {
				return "", err
			}
			made := 0
			for i, item := range rows(a.payload["commitments"]) {
				if !contains(a.selection, fmt.Sprintf("commitments-%d", i)) || clientID == 0 {
					continue
				}
				record := fmt.Sprintf("Client:%d", clientID)
				if engagement != nil {
					record = fmt.Sprintf("Engagement:%d", intOf(engagement["id"]))
				}
				due := parseDate(str(item["due_on"]))
				if due == "" {
					due = time.Now().AddDate(0, 0, 7).Format(time.DateOnly)
				}
				c := plugin.Commitment{Record: record, Description: str(item["description"]), DueOn: due, OwnerKind: "us", UserID: a.caller.ID}
				if str(item["owner"]) == "client" {
					c.OwnerKind, c.UserID = "client", 0
				}
				if _, err := a.api.AddCommitment(ctx, c); err != nil {
					return "", err
				}
				made++
			}
			for i, item := range rows(a.payload["tasks"]) {
				if !contains(a.selection, fmt.Sprintf("tasks-%d", i)) {
					continue
				}
				ref := ""
				if r := strings.ToUpper(strings.TrimSpace(str(item["engagement_ref"]))); r != "" && clientID != 0 {
					if recs, err := a.api.Records(ctx, plugin.Query{Type: "Engagement", Ref: r}); err == nil && len(recs) > 0 && clientOf(ctx, a.api, recs[0]) == clientID {
						ref = r
					}
				}
				if ref == "" && engagement != nil {
					ref = str(engagement["ref"])
				}
				if ref == "" {
					continue
				}
				todo := map[string]any{"title": truncate(str(item["title"]), 250)}
				if due := parseDate(str(item["due_on"])); due != "" {
					todo["due_on"] = due
				}
				if err := a.run(ctx, "create_work", map[string]any{"ref": ref, "todo": todo}); err != nil {
					return "", err
				}
				made++
			}
			return fmt.Sprintf("Added %d.", made), nil
		},
	})

	define(&definition{kind: "day", label: "Plan my day", types: []string{"User"},
		schema: sObject(map[string]any{
			"summary":    sText("Two to four sentences on the day ahead"),
			"priorities": sList(sObject(map[string]any{"record": sText(`"Type:id"`), "what": sText(""), "why": sText("")})),
		}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			first, _, _ := strings.Cut(g.caller.Name, " ")
			return fmt.Sprintf(`Write %s's plan for today (%s): a short summary of what matters, then up to five priorities in order, each naming its record as "Type:id" with why it comes first. From what needs them now:

%s`, first, today(ctx, g.api), g.show(ctx, "briefing", "", "")), nil
		},
	})

	define(&definition{kind: "scope_draft", label: "Draft the scope", types: []string{"Engagement"},
		schema: sObject(map[string]any{
			"items": sList(sObject(map[string]any{"description": sText("One deliverable, as the client will read it"), "internal_estimate": sText(`Rough effort for us, internal, or ""`)})),
		}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			e, err := g.record(ctx, "Engagement")
			if err != nil {
				return "", err
			}
			requests := "none"
			if clientID := clientOf(ctx, g.api, e); clientID != 0 {
				recs, err := g.api.Records(ctx, plugin.Query{Type: "Request", ClientID: clientID, Open: true, Limit: 10})
				if err == nil && len(recs) > 0 {
					var lines []string
					for _, r := range recs {
						lines = append(lines, "- "+str(r["subject"]))
					}
					requests = strings.Join(lines, "\n")
				}
			}
			site, _ := g.api.Site(ctx)
			return fmt.Sprintf(`Draft the scope items for this %s's agreement: clear deliverables the client can say yes to, in their words, each one thing. Skip what's already on the draft. Five to ten items at most.

%s

Their open requests:
%s`, strings.ToLower(term(site, "engagement", false)), g.show(ctx, "show_engagement", "ref", str(e["ref"])), requests), nil
		},
		apply: func(ctx context.Context, a *applying) (string, error) {
			e, err := a.record(ctx, "Engagement")
			if err != nil {
				return "", err
			}
			draft, err := a.draft(ctx, e, true)
			if err != nil {
				return "", err
			}
			picked := 0
			for i, item := range rows(a.payload["items"]) {
				if !contains(a.selection, fmt.Sprintf("items-%d", i)) {
					continue
				}
				si := map[string]any{"description": truncate(str(item["description"]), 250), "price_cents": 0}
				if est := str(item["internal_estimate"]); est != "" {
					si["internal_estimate"] = est
				}
				if err := a.run(ctx, "add_scope_item", map[string]any{"id": draft, "scope_item": si}); err != nil {
					return "", err
				}
				picked++
			}
			return fmt.Sprintf("Added %d to the draft.", picked), nil
		},
	})

	define(&definition{kind: "agreement_check", label: "Check before sending", types: []string{"Engagement"},
		schema: sObject(map[string]any{
			"summary": sText("The summary the client reads: plain, warm, two to four sentences, never internal estimates"),
			"issues":  sList(sObject(map[string]any{"item": sText(""), "problem": sText("")})),
		}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			e, err := g.record(ctx, "Engagement")
			if err != nil {
				return "", err
			}
			items, summary := "no draft", "none"
			versions, err := g.api.Records(ctx, plugin.Query{Type: "AgreementVersion", EngagementID: g.id, Limit: 50})
			if err != nil {
				return "", err
			}
			for _, v := range versions {
				if v["draft"] != true {
					continue
				}
				if s := str(v["summary"]); s != "" {
					summary = s
				}
				scope, err := g.api.Records(ctx, plugin.Query{Type: "ScopeItem", AgreementVersionID: intOf(v["id"]), Limit: 200})
				if err != nil {
					return "", err
				}
				var lines []string
				for _, si := range scope {
					line := fmt.Sprintf("- %s (price %d cents", str(si["description"]), intOf(si["price_cents"]))
					if est := str(si["internal_estimate"]); est != "" {
						line += ", internal estimate " + est
					}
					lines = append(lines, line+")")
				}
				items = strings.Join(lines, "\n")
			}
			return fmt.Sprintf(`Check this draft agreement before it goes to the client. List real problems only: vague or overlapping items, missing prices where others have them, anything the client couldn't say yes to as written. Then write the summary the client will read; never mention internal estimates.

%s: %s
Current summary: %s
Items:
%s`, str(e["ref"]), str(e["title"]), summary, items), nil
		},
		apply: func(ctx context.Context, a *applying) (string, error) {
			e, err := a.record(ctx, "Engagement")
			if err != nil {
				return "", err
			}
			draft, err := a.draft(ctx, e, false)
			if err != nil {
				return "", err
			}
			if draft == 0 {
				return "", refuseUse("There's no draft any more.")
			}
			if err := a.run(ctx, "update_agreement_version", map[string]any{"id": draft, "agreement_version": map[string]any{"summary": truncate(str(a.payload["summary"]), 250)}}); err != nil {
				return "", err
			}
			return "Summary updated.", nil
		},
	})

	define(&definition{kind: "plan_work", label: "Plan the work", types: []string{"ScopeItem"},
		schema: sObject(map[string]any{
			"tasks": sList(sObject(map[string]any{"title": sText(""), "owner_id": map[string]any{"type": []any{"integer", "null"}, "description": "Who should do it, from the team, or null"},
				"due_on": sDate(), "why": sText("Why this owner, briefly")})),
		}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			return fmt.Sprintf(`Break this agreed scope item into the work that delivers it: three to eight concrete tasks in order, each with the best owner from the team (by role and how much they have open) and a due date if the engagement suggests one. Today is %s.

%s

The team:
%s`, today(ctx, g.api), g.show(ctx, "show_scope_item", "id", strconv.FormatInt(g.id, 10)), team(ctx, g.api)), nil
		},
		apply: func(ctx context.Context, a *applying) (string, error) {
			item, err := a.record(ctx, "ScopeItem")
			if err != nil {
				return "", err
			}
			e, _ := item["engagement"].(map[string]any)
			ref := str(e["ref"])
			if ref == "" {
				return "", refuseUse("Couldn't find the engagement this belongs to.")
			}
			active := map[int64]bool{}
			if people, err := a.api.Records(ctx, plugin.Query{Type: "User", Open: true, Limit: 500}); err == nil {
				for _, p := range people {
					active[intOf(p["id"])] = true
				}
			}
			picked := 0
			for i, task := range rows(a.payload["tasks"]) {
				if !contains(a.selection, fmt.Sprintf("tasks-%d", i)) {
					continue
				}
				todo := map[string]any{"title": truncate(str(task["title"]), 250), "scope_item_id": a.id}
				if owner := intOf(task["owner_id"]); active[owner] {
					todo["owner_id"] = owner
				}
				if due := parseDate(str(task["due_on"])); due != "" {
					todo["due_on"] = due
				}
				if err := a.run(ctx, "create_work", map[string]any{"ref": ref, "todo": todo}); err != nil {
					return "", err
				}
				picked++
			}
			return fmt.Sprintf("Added %d to %s.", picked, ref), nil
		},
	})

	define(&definition{kind: "reminder", label: "Draft a reminder", types: []string{"Commitment"}, fast: true,
		schema: sObject(map[string]any{"message": sText("The message to send, ready to paste")}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			c, err := g.record(ctx, "Commitment")
			if err != nil {
				return "", err
			}
			site, _ := g.api.Site(ctx)
			theirs := !strings.HasPrefix(str(c["owner"]), "us") && !strings.HasPrefix(str(c["owner"]), "user:")
			who, ask := "we promised them", "An honest update: where it stands and, if it will slip, the new date to offer."
			if theirs {
				who, ask = "they promised us", "A gentle nudge: what we need, and why it matters now."
			}
			overdue := ""
			if c["overdue"] == true {
				overdue = " (overdue)"
			}
			client, _ := c["client"].(map[string]any)
			line := fmt.Sprintf("%s will %s, by %s%s. %s: %s.", str(c["owner_name"]), str(c["description"]), str(c["due_on"]), overdue, term(site, "client", false), str(client["display_name"]))
			if e, ok := c["engagement"].(map[string]any); ok && str(e["ref"]) != "" {
				line += fmt.Sprintf(" %s: %s.", term(site, "engagement", false), str(e["display_name"]))
			}
			return fmt.Sprintf(`Write a short, friendly message about this commitment (%s), ready to paste into an email or chat. %s No subject line, no placeholders. Today is %s.

%s`, who, ask, today(ctx, g.api), line), nil
		},
	})

	define(&definition{kind: "client_week", label: "Summarise the week", types: []string{"Client"},
		schema: sObject(map[string]any{"summary": sText("A short update on the past week and what's next, for us or to send them")}),
		prompt: func(ctx context.Context, g *gen) (string, error) {
			site, _ := g.api.Site(ctx)
			return fmt.Sprintf(`Summarise the past week with this %s (to %s): what moved, what we're waiting on them for, what we've promised, what's next. Five to eight lines, plain, no internal estimates.

%s`, strings.ToLower(term(site, "client", false)), today(ctx, g.api), g.show(ctx, "show_client", "id", strconv.FormatInt(g.id, 10))), nil
		},
		apply: func(ctx context.Context, a *applying) (string, error) {
			if err := a.run(ctx, "add_note", map[string]any{"record": fmt.Sprintf("Client:%d", a.id),
				"note": map[string]any{"body": str(a.payload["summary"]), "kind": "internal", "source": "Runwell AI"}}); err != nil {
				return "", err
			}
			return "Saved as a note.", nil
		},
	})
}

// ---- building prompts --------------------------------------------------------------------

// record is the suggestion's record, as Records reads it.
func (g *gen) record(ctx context.Context, typ string) (map[string]any, error) {
	recs, err := g.api.Records(ctx, plugin.Query{Type: typ, ID: g.id})
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return nil, refuseUse("Couldn't find that record any more.")
	}
	return recs[0], nil
}

// show is a record as the person's show tool gives it (what an agent over
// MCP would see), trimmed.
func (g *gen) show(ctx context.Context, tool, key, value string) string {
	args := map[string]any{}
	if key != "" {
		args[key] = value
	}
	out, err := g.api.RunTool(ctx, g.caller, tool, args)
	if err != nil {
		return "{}"
	}
	if d, ok := out["data"]; ok {
		return trim(d)
	}
	return trim(out)
}

// note is the note a note_actions suggestion is about, found in its
// record's show answer, and that answer.
func (g *gen) note(ctx context.Context) (map[string]any, string, error) {
	typ, id, ok := strings.Cut(g.on, ":")
	for _, t := range readTypes {
		if t.typ != typ || !ok {
			continue
		}
		key := id
		if typ == "Engagement" {
			if recs, err := g.api.Records(ctx, plugin.Query{Type: "Engagement", ID: intOf(id)}); err == nil && len(recs) > 0 {
				key = str(recs[0]["ref"])
			}
		}
		out, err := g.api.RunTool(ctx, g.caller, t.tool, map[string]any{t.key: key})
		if err != nil {
			return nil, "", err
		}
		if n := findNote(out, g.id); n != nil {
			return n, trim(out["data"]), nil
		}
	}
	return nil, "", refuseUse("Couldn't find that note.")
}

func findNote(v any, id int64) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		if notes, ok := x["notes"].([]any); ok {
			for _, n := range notes {
				if m, ok := n.(map[string]any); ok && intOf(m["id"]) == id {
					return m
				}
			}
		}
		for _, inner := range x {
			if n := findNote(inner, id); n != nil {
				return n
			}
		}
	case []any:
		for _, inner := range x {
			if n := findNote(inner, id); n != nil {
				return n
			}
		}
	}
	return nil
}

// noteParents are the client and engagement of the record a note is on.
func (a *applying) noteParents(ctx context.Context) (int64, map[string]any, error) {
	typ, id, _ := strings.Cut(a.on, ":")
	recs, err := a.api.Records(ctx, plugin.Query{Type: typ, ID: intOf(id)})
	if err != nil {
		return 0, nil, err
	}
	if len(recs) == 0 {
		return 0, nil, refuseUse("Couldn't find what the note is on any more.")
	}
	rec := recs[0]
	var engagement map[string]any
	if typ == "Engagement" {
		engagement = rec
	} else if e, ok := rec["engagement"].(map[string]any); ok && intOf(e["id"]) != 0 {
		engagement = e
	}
	return clientOf(ctx, a.api, rec), engagement, nil
}

// draft is the engagement's draft agreement version's id; with make, one is
// started when there's none.
func (a *applying) draft(ctx context.Context, e map[string]any, make bool) (int64, error) {
	versions, err := a.api.Records(ctx, plugin.Query{Type: "AgreementVersion", EngagementID: intOf(e["id"]), Limit: 50})
	if err != nil {
		return 0, err
	}
	for _, v := range versions {
		if v["draft"] == true {
			return intOf(v["id"]), nil
		}
	}
	if !make {
		return 0, nil
	}
	out, err := a.api.RunTool(ctx, a.caller, "create_agreement_version", map[string]any{"ref": str(e["ref"])})
	if err != nil {
		return 0, err
	}
	if out["status"] == "error" {
		return 0, refuseUse("%s", str(out["summary"]))
	}
	if rec := firstRecord(out["data"], "AgreementVersion"); rec != nil {
		return intOf(rec["id"]), nil
	}
	return 0, refuseUse("Couldn't start a draft.")
}

func firstRecord(v any, typ string) map[string]any {
	switch x := v.(type) {
	case map[string]any:
		if x["type"] == typ && intOf(x["id"]) != 0 {
			return x
		}
		for _, inner := range x {
			if r := firstRecord(inner, typ); r != nil {
				return r
			}
		}
	case []any:
		for _, inner := range x {
			if r := firstRecord(inner, typ); r != nil {
				return r
			}
		}
	}
	return nil
}

// run is one change, as the person, in their own request: its error in
// the tool's words.
func (a *applying) run(ctx context.Context, tool string, args map[string]any) error {
	out, err := a.api.RunTool(ctx, a.caller, tool, args)
	if err != nil {
		return err
	}
	if out["status"] == "error" {
		msg := str(out["summary"])
		if errs, ok := out["errors"].(map[string]any); ok {
			for field, list := range errs {
				if l, ok := list.([]any); ok && len(l) > 0 {
					msg += fmt.Sprintf(" %s %s.", humanize(field), str(l[0]))
				}
			}
		}
		return refuseUse("%s", strings.TrimSpace(msg))
	}
	return nil
}

// team is the people on staff and how much each has open.
func team(ctx context.Context, api *plugin.API) string {
	people, err := api.Records(ctx, plugin.Query{Type: "User", Open: true, Limit: 200})
	if err != nil {
		return ""
	}
	var lines []string
	for _, p := range people {
		open, _ := api.Records(ctx, plugin.Query{Type: "Todo", OwnerID: intOf(p["id"]), Open: true, Limit: 500})
		lines = append(lines, fmt.Sprintf("- %s (id %d, %s): %d open", str(p["name"]), intOf(p["id"]), str(p["role"]), len(open)))
	}
	return strings.Join(lines, "\n")
}

func today(ctx context.Context, api *plugin.API) string {
	loc := time.UTC
	if site, err := api.Site(ctx); err == nil {
		if site.Location != nil {
			loc = site.Location
		}
	}
	return time.Now().In(loc).Format("January 2, 2006")
}

func dateOf(iso string) string {
	if len(iso) >= 10 {
		return iso[:10]
	}
	return iso
}

func parseDate(s string) string {
	if dateOnly.MatchString(strings.TrimSpace(s)) {
		if _, err := time.Parse(time.DateOnly, strings.TrimSpace(s)); err == nil {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

var tags = regexp.MustCompile(`<[^>]*>`)

func plainText(h string) string {
	return strings.TrimSpace(html.UnescapeString(tags.ReplaceAllString(strings.ReplaceAll(h, "</p>", "</p>\n"), "")))
}

func rows(v any) []map[string]any {
	var out []map[string]any
	if list, ok := v.([]any); ok {
		for _, r := range list {
			if m, ok := r.(map[string]any); ok {
				out = append(out, m)
			}
		}
	}
	return out
}

// ---- the suggestion rows -------------------------------------------------------------------

type suggestion struct {
	ID          int64
	UserID      int64
	SubjectType string
	SubjectID   int64
	ChatID      int64
	Kind        string
	State       string
	Payload     map[string]any
	Error       string
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

const suggestionColumns = `id, COALESCE(user_id, 0), COALESCE(subject_type, ''), COALESCE(subject_id, 0), COALESCE(ai_chat_id, 0), kind, state, payload, COALESCE(error, ''), created_at, updated_at`

func scanSuggestion(row interface{ Scan(...any) error }) (*suggestion, error) {
	s := &suggestion{}
	var payload sql.NullString
	var created, updated store.Time
	if err := row.Scan(&s.ID, &s.UserID, &s.SubjectType, &s.SubjectID, &s.ChatID, &s.Kind, &s.State, &payload, &s.Error, &created, &updated); err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(payload.String), &s.Payload)
	s.CreatedAt, s.UpdatedAt = created.Time, updated.Time
	return s, nil
}

// working is still being made (for at most three minutes).
func (s *suggestion) working() bool {
	return s.State == "working" && time.Since(s.CreatedAt) < 3*time.Minute
}

func (s *suggestion) json() map[string]any {
	state := s.State
	if state == "working" && !s.working() {
		state = "stale"
	}
	d := definitions[s.Kind]
	label := ""
	if d != nil {
		label = d.label
	}
	payload := map[string]any{}
	for k, v := range s.Payload {
		if !strings.HasPrefix(k, "_") {
			payload[k] = v
		}
	}
	return map[string]any{"id": s.ID, "kind": s.Kind, "label": label, "state": state, "payload": payload, "error": nullStr(s.Error),
		"record": fmt.Sprintf("%s:%d", s.SubjectType, s.SubjectID), "created_at": s.CreatedAt.UTC().Format(time.RFC3339), "updated_at": s.UpdatedAt.UTC().Format(time.RFC3339)}
}

// suggestionSubject reads the record a suggestion is asked for: "User:me",
// a note (with the record it's on), or a record of the kind's types.
func suggestionSubject(ctx context.Context, api *plugin.API, caller *plugin.Caller, d *definition, record, on string) (string, int64, string, bool) {
	typ, id, _ := strings.Cut(strings.TrimSpace(record), ":")
	if !contains(d.types, typ) {
		return "", 0, "", false
	}
	switch typ {
	case "User":
		return "User", caller.ID, "", true
	case "Note":
		n, err := strconv.ParseInt(id, 10, 64)
		if err != nil || n <= 0 {
			return "", 0, "", false
		}
		onType, onID, _, ok := subjectOf(ctx, api, on, chatSubjects)
		if !ok {
			return "", 0, "", false
		}
		return "Note", n, fmt.Sprintf("%s:%d", onType, onID), true
	}
	t, n, _, ok := subjectOf(ctx, api, record, append([]string{}, d.types...))
	return t, n, "", ok
}

// suggestionExcluded is whether the suggestion's record is kept out of AI.
func suggestionExcluded(ctx context.Context, api *plugin.API, typ string, id int64, on string) (bool, error) {
	if typ == "Note" {
		t, n, _ := strings.Cut(on, ":")
		return excludedRecord(ctx, api, t, intOf(n))
	}
	return excludedRecord(ctx, api, typ, id)
}

func newSuggestion(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return res, nil
	}
	p := params(req)
	d := definitions[str(p["kind"])]
	if d == nil {
		return missing("kind of suggestion"), nil
	}
	typ, id, on, ok := suggestionSubject(ctx, api, caller, d, str(p["record"]), str(p["on"]))
	if !ok {
		return missing("record"), nil
	}
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	reason, err := available(ctx, api, s)
	if err != nil {
		return nil, err
	}
	if reason == "" {
		if ex, err := suggestionExcluded(ctx, api, typ, id, on); err != nil {
			return nil, err
		} else if ex {
			reason = "AI isn’t available here."
		}
	}
	if reason != "" {
		return refuse(http.StatusUnprocessableEntity, reason), nil
	}
	payload := "null"
	if on != "" {
		raw, _ := json.Marshal(map[string]any{"_on": on})
		payload = string(raw)
	}
	now := store.Now().DB()
	ins, err := api.Exec(ctx, `INSERT INTO ai_suggestions (user_id, subject_type, subject_id, kind, state, payload, created_at, updated_at) VALUES (?, ?, ?, ?, 'working', ?, ?, ?)`,
		caller.ID, typ, id, d.kind, payload, now, now)
	if err != nil {
		return nil, err
	}
	sid, _ := ins.LastInsertId()
	g := &gen{caller: caller, typ: typ, id: id, on: on}
	if err := api.Background("suggestion", func(ctx context.Context, api *plugin.API) {
		g.api = api
		generate(ctx, api, sid, d, s, g)
	}); err != nil {
		_, _ = api.Exec(ctx, `UPDATE ai_suggestions SET state = 'failed', error = ? WHERE id = ?`, err.Error(), sid)
	}
	sg, err := scanSuggestion(api.QueryRow(ctx, `SELECT `+suggestionColumns+` FROM ai_suggestions WHERE id = ?`, sid))
	if err != nil {
		return nil, err
	}
	return plugin.JSON(201, map[string]any{"suggestion": sg.json()}), nil
}

// generate asks the fast or main model for the kind's structured answer
// (AiSuggestion#generate!), through a chat of its own whose usage says what
// it cost.
func generate(ctx context.Context, api *plugin.API, id int64, d *definition, s *setting, g *gen) {
	model := s.modelFor(d.fast)
	fail := func(err error) {
		msg := truncate(fmt.Sprintf("%s: %s", routeName(s.Provider, model), ctxErr(ctx, err).Error()), 300)
		if _, err := api.Exec(context.WithoutCancel(ctx), `UPDATE ai_suggestions SET state = 'failed', error = ?, updated_at = ? WHERE id = ?`, msg, store.Now().DB(), id); err != nil {
			api.Log("suggestion: " + err.Error())
		}
	}
	chatID, err := insertChat(ctx, api, g.caller.ID, 0, g.typ, g.id, "suggestion", d.label)
	if err != nil {
		fail(err)
		return
	}
	if _, err := api.Exec(ctx, `UPDATE ai_suggestions SET ai_chat_id = ? WHERE id = ?`, chatID, id); err != nil {
		fail(err)
		return
	}
	prompt, err := d.prompt(ctx, g)
	if err != nil {
		fail(err)
		return
	}
	l, err := s.llm(api, d.fast)
	if err != nil {
		fail(err)
		return
	}
	site, _ := api.Site(ctx)
	ans, err := l.turn(ctx, ask{System: staffSystem(site, g.caller), Turns: []turn{{Role: "user", Text: prompt}}, Schema: d.schema, Name: d.kind})
	if err != nil {
		fail(err)
		return
	}
	recordUsage(ctx, api, chatID, 0, l, ans.Usage)
	if ans.Stop == "refusal" {
		fail(errors.New("the model declined to answer this one"))
		return
	}
	parsed, err := parseJSON(ans.Text)
	if err != nil {
		fail(err)
		return
	}
	if g.on != "" {
		parsed["_on"] = g.on
	}
	raw, _ := json.Marshal(parsed)
	if _, err := api.Exec(context.WithoutCancel(ctx), `UPDATE ai_suggestions SET state = 'ready', payload = ?, error = NULL, updated_at = ? WHERE id = ?`, string(raw), store.Now().DB(), id); err != nil {
		api.Log("suggestion: " + err.Error())
	}
}

var fence = regexp.MustCompile("(?s)```(?:json)?\\s*(.*?)\\s*```")

// parseJSON reads a structured answer, from a code fence or around words
// when a model added them.
func parseJSON(text string) (map[string]any, error) {
	text = strings.TrimSpace(text)
	if m := fence.FindStringSubmatch(text); m != nil {
		text = m[1]
	}
	if i, j := strings.Index(text, "{"), strings.LastIndex(text, "}"); i >= 0 && j > i {
		text = text[i : j+1]
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(text), &out); err != nil || out == nil {
		return nil, errors.New("its answer wasn't in the expected shape; try again")
	}
	return out, nil
}

func ownSuggestion(ctx context.Context, api *plugin.API, req *plugin.Request) (*suggestion, *plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return nil, res, nil
	}
	s, err := scanSuggestion(api.QueryRow(ctx, `SELECT `+suggestionColumns+` FROM ai_suggestions WHERE id = ? AND user_id = ?`, pathID(req, "id"), caller.ID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, missing("suggestion"), nil
	}
	return s, nil, err
}

func showSuggestion(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, res, err := ownSuggestion(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	return plugin.JSON(200, map[string]any{"suggestion": s.json()}), nil
}

// latestSuggestion is the person's latest of a kind on a record (the day's
// from today only), and whether AI may suggest here now.
func latestSuggestion(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return res, nil
	}
	d := definitions[req.Query.Get("kind")]
	if d == nil {
		return missing("kind of suggestion"), nil
	}
	typ, id, on, ok := suggestionSubject(ctx, api, caller, d, req.Query.Get("record"), req.Query.Get("on"))
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	reason, err := available(ctx, api, s)
	if err != nil {
		return nil, err
	}
	out := map[string]any{"kind": d.kind, "label": d.label, "ready": s.ready(), "available": false, "suggestion": nil}
	if !ok || !s.ready() {
		return plugin.JSON(200, out), nil
	}
	ex, err := suggestionExcluded(ctx, api, typ, id, on)
	if err != nil {
		return nil, err
	}
	out["available"] = reason == "" && !ex
	out["excluded"] = ex
	since := "0000"
	if d.kind == "day" {
		since = store.TimeOf(time.Now().Add(-18 * time.Hour)).DB().(string)
	}
	sg, err := scanSuggestion(api.QueryRow(ctx, `SELECT `+suggestionColumns+` FROM ai_suggestions WHERE kind = ? AND subject_type = ? AND subject_id = ? AND user_id = ? AND created_at >= ?
		ORDER BY created_at DESC LIMIT 1`, d.kind, typ, id, caller.ID, since))
	if err == nil {
		out["suggestion"] = sg.json()
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	return plugin.JSON(200, out), nil
}

// useSuggestion uses one (with the items ticked, for a list) as the
// person, or dismisses it.
func useSuggestion(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, res, err := ownSuggestion(ctx, api, req)
	if res != nil || err != nil {
		return res, err
	}
	p := params(req)
	now := store.Now().DB()
	if str(p["decision"]) == "dismiss" {
		if _, err := api.Exec(ctx, `UPDATE ai_suggestions SET state = 'dismissed', updated_at = ? WHERE id = ?`, now, s.ID); err != nil {
			return nil, err
		}
		s.State = "dismissed"
		return plugin.JSON(200, map[string]any{"message": "Dismissed.", "suggestion": s.json()}), nil
	}
	if s.State != "ready" {
		return refuse(http.StatusConflict, "This suggestion isn't ready to use."), nil
	}
	d := definitions[s.Kind]
	if d == nil {
		return missing("kind of suggestion"), nil
	}
	message := "Noted."
	if d.apply != nil {
		var selection []string
		if list, ok := p["items"].([]any); ok {
			for _, v := range list {
				selection = append(selection, str(v))
			}
		}
		a := &applying{gen: gen{api: api, caller: req.Actor, typ: s.SubjectType, id: s.SubjectID, on: str(s.Payload["_on"]), payload: s.Payload}, req: req, selection: selection}
		message, err = d.apply(ctx, a)
		if err != nil {
			var r *refused
			if errors.As(err, &r) {
				return refuse(http.StatusUnprocessableEntity, r.msg), nil
			}
			if res := plugin.Answer(err); res != nil {
				return res, nil
			}
			return refuse(http.StatusUnprocessableEntity, err.Error()), nil
		}
	}
	if _, err := api.Exec(ctx, `UPDATE ai_suggestions SET state = 'accepted', updated_at = ? WHERE id = ?`, now, s.ID); err != nil {
		return nil, err
	}
	s.State = "accepted"
	return plugin.JSON(200, map[string]any{"message": message, "suggestion": s.json()}), nil
}

// ---- kept out of AI ----------------------------------------------------------------------

// exclude keeps a client out of AI: no assistant and no suggestions on its
// records. A person's call, never an agent's.
func exclude(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return setExclusion(ctx, api, req, true)
}

func include(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	return setExclusion(ctx, api, req, false)
}

func setExclusion(ctx context.Context, api *plugin.API, req *plugin.Request, out bool) (*plugin.Response, error) {
	caller, res := person(req)
	if res != nil {
		return refuse(http.StatusForbidden, "Keeping a client out of AI is a person's call."), nil
	}
	recs, err := api.Records(ctx, plugin.Query{Type: "Client", ID: pathID(req, "client_id")})
	if err != nil {
		return nil, err
	}
	if len(recs) == 0 {
		return missing(clientWord(ctx, api, false)), nil
	}
	client := recs[0]
	now := store.Now().DB()
	message := str(client["display_name"]) + " is kept out of AI."
	if out {
		_, err = api.Exec(ctx, `INSERT INTO ai_exclusions (client_id, user_id, created_at, updated_at) VALUES (?, ?, ?, ?) ON CONFLICT (client_id) DO NOTHING`, intOf(client["id"]), caller.ID, now, now)
	} else {
		_, err = api.Exec(ctx, `DELETE FROM ai_exclusions WHERE client_id = ?`, intOf(client["id"]))
		message = "AI works on " + str(client["display_name"]) + " again."
	}
	if err != nil {
		return nil, err
	}
	return plugin.JSON(200, map[string]any{"message": message, "excluded": out}), nil
}
