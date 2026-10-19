package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// What the Ask panel draws (ai/chats/_panel and ai/messages/_list): the
// chat's title, the person's and the AI's words, what the AI looked at,
// each change it proposed with its fields to check and Approve or Decline,
// one-click questions while it's empty, and why it can't answer, if so.

// chatAnswer is the panel for a person's chat: their tools name the actions.
func chatAnswer(ctx context.Context, api *plugin.API, c *chat, page string, caller *plugin.Caller) (*plugin.Response, error) {
	tools, _ := api.Tools(ctx, caller)
	return panelAnswer(ctx, api, c, page, tools)
}

func panelAnswer(ctx context.Context, api *plugin.API, c *chat, page string, tools []plugin.ToolInfo) (*plugin.Response, error) {
	site, _ := api.Site(ctx)
	msgs, err := loadMessages(ctx, api, c.ID)
	if err != nil {
		return nil, err
	}
	replying := c.replying()
	out, waiting := transcript(ctx, api, msgs, tools, replying)
	heading, record := "Ask anything", ""
	if c.ContactID != 0 {
		heading = "Ask about your work with " + site.Name
	} else if c.SubjectType != "" {
		record = c.record()
		if recs, err := api.Records(ctx, plugin.Query{Type: c.SubjectType, ID: c.SubjectID}); err == nil && len(recs) > 0 {
			heading = str(recs[0]["display_name"])
			if c.SubjectType == "Engagement" {
				heading = strings.TrimSpace(str(recs[0]["ref"]) + " " + str(recs[0]["title"]))
			}
		}
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
		if ex, err := chatExcluded(ctx, api, c); err == nil && ex {
			reason = "AI is off for this " + clientWord(ctx, api, false) + ": the owner keeps it out of AI."
		}
	}
	if c.UserID != 0 && c.ContactID == 0 {
		addFeedback(ctx, api, out, c.UserID)
	}
	// What can be done again: regenerate the last answer, edit the last
	// question (not while it answers or a change waits).
	lastUser, lastAnswer := int64(0), int64(0)
	for _, m := range msgs {
		if m.Role == "user" {
			lastUser, lastAnswer = m.ID, 0
		} else if m.Finish != "agent" {
			lastAnswer = m.ID
		}
	}
	answer := map[string]any{
		"chat": map[string]any{"id": c.ID, "title": nullStr(c.Title), "heading": heading, "record": nullStr(record),
			"model": nullStr(c.Model), "model_name": pickedModel(s, c.Model)},
		"last_question_id": nullID(lastUser),
		"last_answer_id":   nullID(lastAnswer),
		"images":           takesImages(s.Provider) && c.ContactID == 0,
		"provider_name":    s.shortName(),
		"messages":         out,
		"replying":         replying,
		"waiting":          waiting,
		"unavailable":      nullStr(reason),
		"note":             "It acts as you, and asks before changing anything.",
		"prompts":          []string{},
	}
	if c.ContactID != 0 {
		answer["note"] = "It answers from your portal, and asks before sending anything."
	}
	if len(msgs) == 0 {
		if c.ContactID != 0 {
			answer["prompts"] = []string{"Where are we on my " + strings.ToLower(term(site, "engagement", true)) + "?", "What are you waiting on from me?", "What’s coming up next?", "I need something new: help me ask for it"}
		} else {
			answer["prompts"] = prompts(site, c.SubjectType, heading, page)
		}
	}
	return plugin.JSON(200, answer), nil
}

// transcript is a chat's messages as the app draws them: each reply's
// tool calls, what they did, and the changes waiting on the person.
func transcript(ctx context.Context, api *plugin.API, msgs []*storedMessage, tools []plugin.ToolInfo, replying bool) ([]map[string]any, bool) {
	byName := map[string]plugin.ToolInfo{}
	for _, t := range tools {
		byName[t.Name] = t
	}
	people := map[int64]string{}
	maps.Copy(people, peopleNames(ctx, api))
	waiting := false
	out := []map[string]any{}
	for _, m := range msgs {
		calls := []map[string]any{}
		for _, call := range m.Calls {
			var args struct {
				Query     string         `json:"query"`
				Record    string         `json:"record"`
				Name      string         `json:"name"`
				Arguments map[string]any `json:"arguments"`
			}
			_ = json.Unmarshal([]byte(call.Args), &args)
			action, known := byName[args.Name]
			isAction := call.Name == "run_tool" || call.Name == "portal_action"
			proposal := isAction && (call.Approval != "" || (known && !action.ReadOnly))
			state := "done"
			switch {
			case call.Approval == "denied":
				state = "declined"
			case call.Result != nil && call.IsError:
				state = "failed"
			case call.Result != nil:
				state = "done"
			case call.Approval == "approved":
				state = "running"
			case !replying && m.Finish == "tool_use" && isAction:
				state, proposal = "pending", true
				waiting = true
			}
			item := map[string]any{"id": call.ID, "tool_call_id": call.CallID, "name": call.Name, "activity": activity(call.Name, args.Query, args.Record, args.Name, action), "proposal": proposal, "state": state}
			if proposal {
				item["details"] = details(args.Arguments, people)
				item["reaches_client"] = nullStr(action.Confirm)
			}
			if call.Result != nil {
				item["summary"] = nullStr(resultSummary(*call.Result))
			}
			calls = append(calls, item)
		}
		item := map[string]any{"id": m.ID, "role": m.Role, "content": m.Content, "failed": m.failed(), "created_at": m.At.UTC().Format(time.RFC3339), "tool_calls": calls,
			"thinking": nullStr(m.Thinking), "stopped": m.Finish == "stopped", "images": 0}
		var raw struct {
			Images []image `json:"images"`
			RunID  int64   `json:"run_id"`
			Handle string  `json:"handle"`
		}
		_ = json.Unmarshal([]byte(m.Raw), &raw)
		if m.Role == "user" {
			item["images"] = len(raw.Images)
		}
		if m.Finish == "agent" {
			item["agent_run"] = map[string]any{"id": raw.RunID, "handle": raw.Handle, "url": fmt.Sprintf("/ai/runs/%d", raw.RunID)}
		}
		out = append(out, item)
	}
	return out, waiting
}

// addFeedback marks each answer with the person's thumbs up or down.
func addFeedback(ctx context.Context, api *plugin.API, out []map[string]any, userID int64) {
	rows, err := api.Query(ctx, `SELECT f.ai_message_id, f.rating FROM ai_feedback f WHERE f.user_id = ?
		AND f.ai_message_id IN (SELECT id FROM ai_messages WHERE ai_chat_id = (SELECT ai_chat_id FROM ai_messages WHERE id = ?))`, userID, firstID(out))
	if err != nil {
		return
	}
	rated := map[int64]string{}
	_ = eachRow(rows, func(rows *sql.Rows) error { // thumbs are a decoration: what was read is enough
		var id int64
		var r int
		if rows.Scan(&id, &r) == nil {
			rated[id] = map[int]string{1: "up", -1: "down"}[r]
		}
		return nil
	})
	for _, m := range out {
		m["feedback"] = nullStr(rated[intOf(m["id"])])
	}
}

func firstID(out []map[string]any) int64 {
	if len(out) == 0 {
		return 0
	}
	return intOf(out[0]["id"])
}

// activity is what a tool call was, in words (ai_activity).
func activity(tool, query, record, action string, info plugin.ToolInfo) string {
	switch tool {
	case "search":
		return "Searched for “" + truncate(query, 60) + "”"
	case "read_record":
		return "Read " + record
	case "find_tools":
		return "Looked for a way to “" + truncate(query, 60) + "”"
	case "run_tool", "portal_action":
		if info.Title != "" {
			return info.Title
		}
		return humanize(action)
	}
	return humanize(tool)
}

var dateOnly = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// details are a proposed change's fields as "Field: value" lines a person
// can check: nested fields flattened, dates as people say them, people by
// name.
func details(args map[string]any, people map[int64]string) [][2]string {
	var out [][2]string
	var walk func(m map[string]any)
	walk = func(m map[string]any) {
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := m[k]
			if inner, ok := v.(map[string]any); ok {
				walk(inner)
				continue
			}
			var shown string
			switch x := v.(type) {
			case []any:
				parts := []string{}
				for _, p := range x {
					parts = append(parts, str(p))
				}
				shown = strings.Join(parts, ", ")
			default:
				shown = str(x)
				switch {
				case dateOnly.MatchString(shown):
					if t, err := time.Parse(time.DateOnly, shown); err == nil {
						shown = t.Format("January 2, 2006")
					}
				case (k == "owner_id" || k == "user_id") && people[intOf(x)] != "":
					shown = people[intOf(x)]
				default:
					shown = truncate(shown, 200)
				}
			}
			out = append(out, [2]string{humanize(strings.TrimSuffix(k, "_id")), shown})
		}
	}
	walk(args)
	if out == nil {
		out = [][2]string{}
	}
	return out
}

// resultSummary is the line a tool's answer said, when it did.
func resultSummary(result string) string {
	var m map[string]any
	if json.Unmarshal([]byte(result), &m) != nil {
		return truncate(result, 200)
	}
	if s := str(m["summary"]); s != "" {
		return s
	}
	return str(m["error"])
}

func humanize(s string) string {
	s = strings.ReplaceAll(s, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// prompts are questions worth one click on this kind of record, or page
// (ai_prompts).
func prompts(site plugin.Site, subjectType, name, page string) []string {
	lower := func(k string, plural bool) string { return strings.ToLower(term(site, k, plural)) }
	switch subjectType {
	case "Client":
		return []string{"Summarize " + name + "’s week", "What are we waiting on them for, and what have we promised?", "Draft this week’s update for " + name, "What’s at risk with " + name + "?"}
	case "Engagement":
		ref, _, _ := strings.Cut(name, " ")
		return []string{"Where are we on " + ref + "?", "What’s overdue or blocked here?", "Draft the scope for the next " + lower("scope_item", false) + "s", "Summarize what changed this week"}
	case "Todo":
		return []string{"What’s needed to finish this?", "Split this into smaller " + lower("work", true), "Who should own this?", "Summarize the notes so far"}
	case "Request":
		return []string{"What should this become?", "Is this in the agreed scope?", "Draft a reply", "Who asked, and what else have they asked for?"}
	case "ScopeItem":
		return []string{"Plan the " + lower("work", false) + " for this", "What’s left to deliver?", "What could go wrong here?", "Summarize this for the " + lower("client", false)}
	}
	switch {
	case strings.HasPrefix(page, "/settings/names"):
		return []string{"Set Runwell up for how we work: suggest names for our things", "We’re planning a wedding: set Runwell up for it", "What do these names change?", "Suggest names for a design studio"}
	case strings.HasPrefix(page, "/settings/appearance"):
		return []string{"Write a theme from our brand colors", "Make it calmer and easier to read", "Make dark mode easier on the eyes", "Suggest a warmer accent color"}
	case strings.HasPrefix(page, "/settings/fields"):
		return []string{"Suggest custom fields we’d use", "Which fields are never filled in?", "What should be a field rather than a note?", "Tidy up our fields"}
	case strings.HasPrefix(page, "/settings/audit"):
		return []string{"Anything unusual in the last week’s sign-ins?", "Who changed settings this month?", "What was deleted this week?", "Summarize today’s activity"}
	case strings.HasPrefix(page, "/requests"):
		return []string{"Triage the open requests: suggest what each should become", "Which requests are waiting longest?", "Which requests look out of scope?", "Summarize this week’s requests"}
	case strings.HasPrefix(page, "/commitments"):
		return []string{"Which promises are at risk this week?", "Draft reminders for what we’re waiting on", "What did we miss last week?", "What’s due tomorrow?"}
	case strings.HasPrefix(page, "/work"):
		return []string{"What’s blocked, and why?", "Who has too much on?", "What’s due this week?", "What finished this week?"}
	}
	return []string{"What needs me today?", "What changed since yesterday?", "Find overdue " + lower("commitment", true), "Plan my week"}
}
