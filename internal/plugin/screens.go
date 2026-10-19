package plugin

import (
	"fmt"
	"slices"
	"strings"
)

// A plugin's screens are data the app draws: no plugin code runs in the
// browser. A screen calls its Source (a GET route of the plugin's, with the
// screen's query string, and record=<Type>:<id> on a record panel) and
// draws the parts it has from the answer, in this order: Text, Stats,
// Detail, Form, Table. The keys a part names are keys in that answer.

// Screen is one of a plugin's screens: a page (/p/<key>/<id>), a panel, a
// card on home, a settings page or a quick action's form.
type Screen struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Source string `json:"source"`
	// Text is the key of a paragraph to show first.
	Text string `json:"text,omitempty"`
	// Stats is the key of a row of figures: [{label, value, hint}].
	Stats  string  `json:"stats,omitempty"`
	Detail *Detail `json:"detail,omitempty"`
	Form   *Form   `json:"form,omitempty"`
	Table  *Table  `json:"table,omitempty"`
	// Filters are links that set a query parameter and redraw (This week,
	// Last week).
	Filters []Filter `json:"filters,omitempty"`
	// Actions are buttons in the screen's header.
	Actions []Action `json:"actions,omitempty"`
}

// Detail is a list of labelled values: the key of [{label, value, href,
// date, time, badge, multiline}].
type Detail struct {
	Items string `json:"items"`
}

// Form is a form posted to a route of the plugin's: {"<record>": {field:
// value}}, with the screen's query (record=…) on the address. A 422's
// errors mark the fields; an answer's "message" is shown, and the screen
// is drawn again from its source.
type Form struct {
	// Record is the key holding the values the form starts with; blank
	// starts empty. It is also what the values are sent under ("values"
	// when blank).
	Record string  `json:"record,omitempty"`
	Save   string  `json:"save"`
	Method string  `json:"method,omitempty"` // POST unless it says PATCH or PUT
	Submit string  `json:"submit,omitempty"` // the button: "Save" by default
	Fields []Field `json:"fields"`
	// Reset empties the form after a save (a log, not a record being edited).
	Reset bool `json:"reset,omitempty"`
}

// FieldTypes are the inputs a form can have.
var FieldTypes = []string{"text", "textarea", "number", "date", "email", "url", "select", "checkbox", "password"}

// Field is one input.
type Field struct {
	Name        string   `json:"name"`
	Label       string   `json:"label"`
	Type        string   `json:"type,omitempty"` // text by default
	Placeholder string   `json:"placeholder,omitempty"`
	Help        string   `json:"help,omitempty"`
	Required    bool     `json:"required,omitempty"`
	Options     []Option `json:"options,omitempty"`
	// OptionsFrom is the key in the source's answer holding [{value, label}].
	OptionsFrom string `json:"options_from,omitempty"`
	// Default is today's date for a date field when "today".
	Default string `json:"default,omitempty"`
}

// Option is a choice in a select or a filter.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Table is rows: the key holding them, its columns, and what each row can
// do.
type Table struct {
	Rows       string   `json:"rows"`
	Key        string   `json:"key,omitempty"` // each row's id field: "id" by default
	Columns    []Column `json:"columns"`
	RowActions []Action `json:"row_actions,omitempty"`
	Empty      string   `json:"empty,omitempty"` // what an empty table says
}

// Column is one of a table's: the row's Key, drawn as Kind says.
type Column struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Link makes the value a link to another of the plugin's screens; Href
	// the key of a path or URL in the row (a record's "url").
	Link *Link  `json:"link,omitempty"`
	Href string `json:"href,omitempty"`
	// Sub is the key of a smaller line under the value.
	Sub string `json:"sub,omitempty"`
	// Num aligns it right; Badge, Date and Time draw it as one.
	Num   bool `json:"num,omitempty"`
	Badge bool `json:"badge,omitempty"`
	Date  bool `json:"date,omitempty"`
	Time  bool `json:"time,omitempty"`
}

// Link is another of the plugin's screens, with query params taken from
// the row: {"id": "id"} sends ?id=<row.id>.
type Link struct {
	Screen string            `json:"screen"`
	Params map[string]string `json:"params,omitempty"`
}

// Action is a button: to another screen (Screen), or a call to a route
// (Post, sent Body with Params from the row or record on the address),
// after Confirm when it says so. Only someone who may call the route sees
// it.
type Action struct {
	Label   string            `json:"label"`
	Screen  string            `json:"screen,omitempty"`
	Post    string            `json:"post,omitempty"`
	Method  string            `json:"method,omitempty"` // POST unless it says DELETE, PATCH or PUT
	Params  map[string]string `json:"params,omitempty"`
	Body    map[string]any    `json:"body,omitempty"`
	Confirm string            `json:"confirm,omitempty"`
	Danger  bool              `json:"danger,omitempty"`
	// ShowIf shows it on a row only when the row's key is true (a person's
	// own entries: {"show_if": "mine"}).
	ShowIf string `json:"show_if,omitempty"`
}

// Filter is a query parameter the screen sets from a choice.
type Filter struct {
	Param   string   `json:"param"`
	Label   string   `json:"label"`
	Options []Option `json:"options,omitempty"`
	// OptionsFrom is the key in the answer holding [{value, label}] (for
	// choices that depend on who's looking).
	OptionsFrom string `json:"options_from,omitempty"`
}

// validateScreens checks the screens and every place that names one.
func (m *Manifest) validateScreens() []string {
	var errs []string
	add := func(format string, args ...any) { errs = append(errs, fmt.Sprintf(format, args...)) }
	seen := map[string]bool{}
	routeFor := func(method, path string) *Route {
		if method == "" {
			method = "POST"
		}
		return m.Route(method, strings.SplitN(path, "?", 2)[0])
	}
	checkAction := func(where string, a Action) {
		switch {
		case a.Label == "":
			add("%s needs a label", where)
		case (a.Screen == "") == (a.Post == ""):
			add("%s goes to a screen or posts to a route, one of them", where)
		case a.Screen != "" && m.Screen(a.Screen) == nil:
			add("%s names screen %q, which there isn't", where, a.Screen)
		case a.Post != "" && routeFor(a.Method, a.Post) == nil:
			add("%s posts to %s %s, which isn't one of the plugin's routes", where, a.Method, a.Post)
		}
	}
	for i, s := range m.Screens {
		where := fmt.Sprintf("screens[%d]", i)
		if !idFormat.MatchString(s.ID) || seen[s.ID] {
			add("%s needs an id of its own, in lowercase letters, digits, - and _", where)
		}
		seen[s.ID] = true
		if s.Title == "" {
			add("%s needs a title", where)
		}
		if r := m.Route("GET", s.Source); r == nil {
			add("%s.source must be one of the plugin's GET routes", where)
		}
		if s.Text == "" && s.Stats == "" && s.Detail == nil && s.Form == nil && s.Table == nil {
			add("%s draws nothing: give it text, stats, detail, a form or a table", where)
		}
		if f := s.Form; f != nil {
			method := strings.ToUpper(f.Method)
			if method == "" {
				method = "POST"
			}
			if method == "GET" || method == "DELETE" || routeFor(method, f.Save) == nil {
				add("%s.form.save must be one of the plugin's POST, PATCH or PUT routes", where)
			}
			if len(f.Fields) == 0 {
				add("%s.form needs fields", where)
			}
			for j, fd := range f.Fields {
				typ := fd.Type
				if typ == "" {
					typ = "text"
				}
				switch {
				case fd.Name == "" || fd.Label == "":
					add("%s.form.fields[%d] needs a name and a label", where, j)
				case !slices.Contains(FieldTypes, typ):
					add("%s.form.fields[%d].type is one of %s", where, j, strings.Join(FieldTypes, ", "))
				case typ == "select" && len(fd.Options) == 0 && fd.OptionsFrom == "":
					add("%s.form.fields[%d] is a select with no options or options_from", where, j)
				}
			}
		}
		if t := s.Table; t != nil {
			if t.Rows == "" || len(t.Columns) == 0 {
				add("%s.table needs rows and columns", where)
			}
			for j, c := range t.Columns {
				if c.Key == "" || c.Label == "" {
					add("%s.table.columns[%d] needs a key and a label", where, j)
				}
			}
			for j, a := range t.RowActions {
				checkAction(fmt.Sprintf("%s.table.row_actions[%d]", where, j), a)
			}
		}
		if d := s.Detail; d != nil && d.Items == "" {
			add("%s.detail needs items", where)
		}
		for j, f := range s.Filters {
			if f.Param == "" || (len(f.Options) == 0 && f.OptionsFrom == "") {
				add("%s.filters[%d] needs a param and options", where, j)
			}
		}
		for j, a := range s.Actions {
			checkAction(fmt.Sprintf("%s.actions[%d]", where, j), a)
		}
	}
	// Links are checked once every screen is known.
	for i, s := range m.Screens {
		if s.Table == nil {
			continue
		}
		for j, c := range s.Table.Columns {
			if c.Link != nil && m.Screen(c.Link.Screen) == nil {
				add("screens[%d].table.columns[%d].link names screen %q, which there isn't", i, j, c.Link.Screen)
			}
		}
	}
	screen := func(where, id string) *Screen {
		s := m.Screen(id)
		if s == nil {
			add("%s names screen %q, which there isn't", where, id)
		}
		return s
	}
	if m.Nav != nil {
		if m.Nav.Label == "" {
			add("nav needs a label")
		}
		if m.Nav.Path != "" {
			if !strings.HasPrefix(m.Nav.Path, "/") || strings.HasPrefix(m.Nav.Path, "/api") {
				add("nav.path is an app path, like /my-day")
			}
		} else {
			screen("nav.page", m.Nav.Page)
		}
	}
	for i, p := range m.Panels {
		if Slots[p.Slot] == "" {
			add("panels[%d].slot is one of client_panel, engagement_panel, todo_panel, home_top", i)
		}
		if p.Label == "" {
			add("panels[%d] needs a label", i)
		}
		screen(fmt.Sprintf("panels[%d].screen", i), p.Screen)
	}
	for i, h := range m.Home {
		if h.Title == "" {
			add("home[%d] needs a title", i)
		}
		screen(fmt.Sprintf("home[%d].screen", i), h.Screen)
	}
	if m.Settings != nil {
		if m.Settings.Label == "" {
			add("settings needs a label")
		}
		screen("settings.screen", m.Settings.Screen)
	}
	for i, q := range m.QuickActions {
		if q.Label == "" {
			add("quick_actions[%d] needs a label", i)
		}
		for _, t := range q.Types {
			if !slices.Contains(RecordTypes, t) {
				add("quick_actions[%d].types are Client, Engagement or Todo, not %q", i, t)
			}
		}
		if s := screen(fmt.Sprintf("quick_actions[%d].screen", i), q.Screen); s != nil && s.Form == nil {
			add("quick_actions[%d].screen must have a form", i)
		}
	}
	return errs
}
