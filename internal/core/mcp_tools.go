package core

import (
	"maps"
	"net/http"
	"regexp"
	"slices"
	"strings"
)

// What a tool is to an AI app: who it's offered to (by role, by whether
// it's the portal's, by whether the connection is read-only), its input
// schema, its annotations and its description, as Rails' Agent::Tool and
// Agent::Catalogue had them.

// read is whether the tool only looks (a GET).
func (t *toolSpec) read() bool { return t.Method == http.MethodGet || t.Method == http.MethodHead }

// portalTool is whether the tool is a client's (the portal's), run as their
// contact.
func (t *toolSpec) portalTool() bool { return t.rt != nil && t.rt.portal }

// perm is the permission the tool's route needs, if any.
func (t *toolSpec) perm() string {
	if t.rt == nil {
		return ""
	}
	return t.rt.perm
}

// availableTo is whether a person on staff with role is offered the tool:
// a staff tool whose permission the role holds (a plugin's that is off is
// held by nobody).
func (a *App) availableTo(t *toolSpec, role string) bool {
	return !t.portalTool() && (t.perm() == "" || a.can(role, t.perm()))
}

// toolsFor is the catalogue for whoever is asking: a person on staff (or
// their agent) gets the staff tools their role allows, a client's contact
// the portal's; a read-only connection only the tools that look.
func (a *App) toolsFor(actor *Actor) []*toolSpec {
	readOnly := actor != nil && actor.Token != nil && actor.Token.ReadOnly
	var out []*toolSpec
	for _, t := range a.catalogue() {
		switch {
		case actor == nil:
			continue
		case actor.Contact != nil:
			if !t.portalTool() || t == apiRequestTool {
				continue
			}
		case actor.User != nil:
			if !a.availableTo(t, actor.User.Person().Role) {
				continue
			}
		default:
			continue
		}
		if readOnly && !t.read() && t != apiRequestTool {
			continue
		}
		out = append(out, t)
	}
	return out
}

// withheldTools are the staff tools this person isn't offered, and why: what
// `me` says when a tool seems to be missing.
func (a *App) withheldTools(actor *Actor) []Obj {
	out := []Obj{}
	if actor == nil || actor.User == nil {
		return out
	}
	for _, t := range a.tools {
		if t.portalTool() || a.availableTo(t, actor.User.Person().Role) {
			continue
		}
		why := "needs " + t.perm()
		if key, _, ok := strings.Cut(t.perm(), ":"); ok && !slices.ContainsFunc(a.permissions(), func(p Permission) bool { return p.Key == t.perm() }) {
			why = "the " + key + " plugin is off"
		}
		out = append(out, O("name", t.Name, "why", why))
	}
	return out
}

// customFieldsSchema is the install's own fields (Settings › Fields), by key.
var customFieldsSchema = map[string]any{"type": "object", "additionalProperties": true,
	"description": "Custom fields by key, from list_custom_fields: text; money in whole cents; dates YYYY-MM-DD; true/false; a choice’s exact value. A blank removes a value."}

// idParams are parameters holding an id, and the kind of record they name
// (Agent::Resolver::PARAMS): each also takes a name, which the call
// resolves.
var idParams = map[string]string{"client_id": "Client", "contact_id": "Contact", "engagement_id": "Engagement", "owner_id": "User",
	"user_id": "User", "todo_id": "Todo", "scope_item_id": "ScopeItem", "request_id": "Request"}

// inputSchema is a tool's arguments as JSON Schema, for MCP: the path
// parameters, then its params, a wrapped body as the nested object it is
// ({"client": {"name": …}}), and confirm and idempotency_key where they
// apply.
func (t *toolSpec) inputSchema() map[string]any {
	props := map[string]any{}
	required := []string{}
	for _, p := range t.Params {
		var s map[string]any
		switch {
		case p.In == "path":
			s = map[string]any{"type": "string", "description": pathParamDesc(p.Name)}
			if strings.HasSuffix(p.Name, "id") {
				s["type"] = []string{"integer", "string"}
			}
		case p.Type == "file":
			s = map[string]any{"type": "object", "description": strOr(p.Desc, "A file"),
				"properties": map[string]any{"filename": map[string]any{"type": "string"}, "content_base64": map[string]any{"type": "string"}, "content_type": map[string]any{"type": "string"}},
				"required":   []string{"filename", "content_base64"}}
		case p.Name == "custom_fields":
			s = customFieldsSchema
		case p.Type == "object":
			s = shapeSchema(p.Desc)
		case p.Type == "array":
			s = map[string]any{"type": "array", "description": p.Desc}
			if items := shapeSchema(strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(p.Desc), "["), "]")); items["properties"] != nil {
				s["items"] = items
			}
		case idParams[p.Name] != "" && p.Type == "integer":
			s = map[string]any{"type": []string{"integer", "string"}, "description": "The " + strings.ToLower(humanize(snake(idParams[p.Name]))) + "’s id, or its name to look up"}
		case p.Type == "":
			s = map[string]any{"type": "string", "description": p.Desc}
		default:
			s = map[string]any{"type": p.Type, "description": p.Desc}
		}
		props[p.Name] = s
		if p.Required {
			required = append(required, p.Name)
		}
	}
	if t.Confirm != "" {
		props["confirm"] = map[string]any{"type": "boolean", "description": "Set true once you have shown the person the preview and they agreed."}
	}
	if !t.read() && t != apiRequestTool {
		props["idempotency_key"] = map[string]any{"type": "string", "description": "Optional: any unique string for this change. Sent again with the same key (a retry), the tool answers as before instead of doing it twice."}
	}
	return map[string]any{"type": "object", "properties": props, "required": required}
}

func pathParamDesc(name string) string {
	switch name {
	case "ref", "engagement_ref":
		return "The engagement's ref, like WO-12 or S-4, or its title"
	case "id":
		return "The record's id, or its name to look up"
	case "key":
		return "The plugin's key"
	}
	return "The " + strings.ToLower(humanize(strings.TrimSuffix(name, "_id"))) + "’s id, or its name"
}

// snake is "ScopeItem" → "scope_item".
func snake(s string) string { return strings.TrimSuffix(searchTable(s), "s") }

// annotations are MCP's hints about what a call does.
func (t *toolSpec) annotations() map[string]any {
	return map[string]any{"title": t.Summary, "readOnlyHint": t.read(), "destructiveHint": t.Method == http.MethodDelete,
		"idempotentHint": t.read() || slices.Contains([]string{"PATCH", "PUT", "DELETE"}, t.Method), "openWorldHint": t.Confirm != ""}
}

// description is what MCP shows of a tool: what it does, whether it
// reaches the client, and what's often next.
func (t *toolSpec) description() string {
	parts := []string{strOr(t.Notes, t.Summary)}
	if t.Notes != "" && !strings.HasPrefix(t.Notes, t.Summary) {
		parts[0] = t.Summary + ". " + t.Notes
	}
	if t.Confirm != "" {
		parts = append(parts, "Reaches the client: "+t.Confirm+" Call once to get a preview, then again with confirm: true.")
	}
	if len(t.Next) > 0 {
		parts = append(parts, "Often next: "+strings.Join(t.Next, ", ")+".")
	}
	return strings.Join(parts, "\n\n")
}

// ---- body shapes ---------------------------------------------------------------

// A wrapped body is declared by the shape it takes, in words:
//
//	body("client", `{name (required), status: active|dormant|former, internal: boolean, custom_fields: {key: value}}`, true)
//
// shapeSchema reads that into the nested JSON Schema Rails' Agent::Tool
// made from `params: { client: { name: "string!", status: [...] } }`:
// each field, (required) or a trailing ! marking it required, a type after
// the colon (string, text, integer, number, boolean, date, integer[]), a|b|c
// for one of those values, {…} for an object and [{…}] for a list of them.
// Words it doesn't know stay as the field's description. A description
// that isn't a shape is kept as the object's description.
func shapeSchema(desc string) map[string]any {
	d := strings.TrimSpace(desc)
	out := map[string]any{"type": "object"}
	if !strings.HasPrefix(d, "{") || !strings.HasSuffix(d, "}") {
		if d != "" {
			out["description"] = d
		}
		out["additionalProperties"] = true
		return out
	}
	inner := d[1 : len(d)-1]
	props := map[string]any{}
	required := []string{}
	for _, item := range splitTop(inner) {
		name, spec, note, req := shapeField(item)
		if name == "" {
			continue
		}
		if name == "key" && spec == "value" { // {key: value}: any keys
			out["additionalProperties"] = true
			continue
		}
		var s map[string]any
		if name == "custom_fields" {
			s = customFieldsSchema
		} else {
			s = fieldSchema(name, spec)
		}
		if note != "" {
			s = withDesc(s, note)
		}
		props[name] = s
		if req {
			required = append(required, name)
		}
	}
	if len(props) > 0 {
		out["properties"] = props
	}
	if len(required) > 0 {
		out["required"] = required
	}
	return out
}

var parenNote = regexp.MustCompile(`\s*\(([^()]*)\)`)

// shapeField is one "name (note): spec" item: its name, its spec, any note,
// and whether it's required.
func shapeField(item string) (name, spec, note string, required bool) {
	item = strings.TrimSpace(item)
	head, spec, _ := cutTop(item, ':')
	spec = strings.TrimSpace(spec)
	var notes []string
	head = parenNote.ReplaceAllStringFunc(head, func(m string) string {
		n := strings.TrimSpace(parenNote.FindStringSubmatch(m)[1])
		if n == "required" {
			required = true
		} else if n != "" {
			notes = append(notes, n)
		}
		return ""
	})
	if !strings.HasPrefix(spec, "{") && !strings.HasPrefix(spec, "[") {
		spec = parenNote.ReplaceAllStringFunc(spec, func(m string) string {
			n := strings.TrimSpace(parenNote.FindStringSubmatch(m)[1])
			if n == "required" {
				required = true
			} else if n != "" {
				notes = append(notes, n)
			}
			return ""
		})
	}
	name = strings.TrimSpace(head)
	if before, ok := strings.CutSuffix(name, "!"); ok {
		name, required = before, true
	}
	if before, ok := strings.CutSuffix(spec, "!"); ok {
		spec, required = before, true
	}
	if strings.ContainsAny(name, " \"'") {
		return "", "", "", false
	}
	return name, strings.TrimSpace(spec), strings.Join(notes, "; "), required
}

// fieldSchema is one field's schema from its spec.
func fieldSchema(name, spec string) map[string]any {
	switch {
	case spec == "" && idParams[name] != "":
		return map[string]any{"type": []string{"integer", "string"}, "description": "The " + strings.ToLower(humanize(snake(idParams[name]))) + "’s id, or its name to look up"}
	case strings.HasPrefix(spec, "{"):
		return shapeSchema(spec)
	case strings.HasPrefix(spec, "[") && strings.HasSuffix(spec, "]"):
		inner := strings.TrimSpace(spec[1 : len(spec)-1])
		if strings.HasPrefix(inner, "{") {
			return map[string]any{"type": "array", "items": shapeSchema(inner)}
		}
		return map[string]any{"type": "array", "items": fieldSchema(name, inner)}
	case strings.HasSuffix(spec, "[]"):
		return map[string]any{"type": "array", "items": fieldSchema("", strings.TrimSuffix(spec, "[]"))}
	}
	switch strings.ToLower(spec) {
	case "", "string":
		return map[string]any{"type": "string"}
	case "text", "rich text", "html":
		return map[string]any{"type": "string", "description": "Rich text: HTML (<p>, <strong>, lists) or plain text"}
	case "date", "yyyy-mm-dd":
		return map[string]any{"type": "string", "format": "date", "description": "YYYY-MM-DD"}
	case "integer", "int", "cents", "id":
		if idParams[name] != "" {
			return map[string]any{"type": []string{"integer", "string"}, "description": "The " + strings.ToLower(humanize(snake(idParams[name]))) + "’s id, or its name to look up"}
		}
		return map[string]any{"type": "integer"}
	case "number":
		return map[string]any{"type": "number"}
	case "boolean", "bool", "true|false":
		return map[string]any{"type": "boolean"}
	case "object":
		return map[string]any{"type": "object", "additionalProperties": true}
	case "array", "list":
		return map[string]any{"type": "array"}
	}
	if strings.Contains(spec, "|") && !strings.Contains(spec, " ") {
		var values []string
		for v := range strings.SplitSeq(spec, "|") {
			if v = strings.TrimSpace(v); v != "" {
				values = append(values, v)
			}
		}
		return map[string]any{"type": "string", "enum": values}
	}
	return map[string]any{"type": "string", "description": spec}
}

func withDesc(s map[string]any, note string) map[string]any {
	out := make(map[string]any, len(s)+1)
	maps.Copy(out, s)
	if d, ok := out["description"].(string); ok && d != "" {
		out["description"] = d + " (" + note + ")"
	} else {
		out["description"] = note
	}
	return out
}

// splitTop splits s on commas outside braces, brackets and parentheses.
func splitTop(s string) []string {
	var out []string
	depth, start := 0, 0
	for i, r := range s {
		switch r {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case ',':
			if depth == 0 {
				out = append(out, s[start:i])
				start = i + 1
			}
		}
	}
	return append(out, s[start:])
}

// cutTop cuts s at the first sep outside braces, brackets and parentheses.
func cutTop(s string, sep rune) (before, after string, found bool) {
	depth := 0
	for i, r := range s {
		switch r {
		case '{', '[', '(':
			depth++
		case '}', ']', ')':
			depth--
		case sep:
			if depth == 0 {
				return s[:i], s[i+1:], true
			}
		}
	}
	return s, "", false
}
