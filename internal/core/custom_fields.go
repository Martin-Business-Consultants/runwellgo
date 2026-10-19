package core

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Custom fields: what an install records about its clients, engagements or
// work that the core doesn't (Settings › Fields). Information only: nothing
// acts on one. Values are text (money in whole cents, dates ISO 8601),
// assigned by field key as forms and agents send them:
//
//	{"client": {"custom_fields": {"website": "acme.example", "renewal": "2026-12-01"}}}
//
// A blank removes a value; keys not given are left as they are.

// CustomFieldModels are the types that take custom fields.
var CustomFieldModels = []string{"Client", "Engagement", "Todo"}

// CustomFieldKinds are what a field may hold, with their names.
var CustomFieldKinds = []struct{ Key, Label string }{
	{"text", "Text"}, {"long_text", "Long text"}, {"number", "Number"}, {"money", "Money"},
	{"date", "Date"}, {"boolean", "Yes or no"}, {"choice", "Choice"}, {"link", "Link"},
}

// CustomField is one field.
type CustomField struct {
	ID            int64      `json:"id"`
	ModelType     string     `json:"model_type"`
	Label         string     `json:"label"`
	Key           string     `json:"key"`
	Kind          string     `json:"kind"`
	Choices       []string   `json:"choices"`
	Position      int        `json:"position"`
	Listed        bool       `json:"listed"`
	ClientVisible bool       `json:"client_visible"`
	ArchivedAt    store.Time `json:"archived_at"`
}

const customFieldColumns = `id, model_type, label, key, kind, choices, position, listed, client_visible, archived_at`

func scanCustomField(r interface{ Scan(...any) error }) (*CustomField, error) {
	var f CustomField
	var choices store.JSON
	if err := r.Scan(&f.ID, &f.ModelType, &f.Label, &f.Key, &f.Kind, &choices, &f.Position, &f.Listed, &f.ClientVisible, &f.ArchivedAt); err != nil {
		return nil, err
	}
	f.Choices = []string{}
	for _, c := range choices.List() {
		f.Choices = append(f.Choices, str(c))
	}
	return &f, nil
}

// customFields are a model's active fields, in order.
func (a *App) customFields(ctx context.Context, db store.Querier, model string) ([]*CustomField, error) {
	rows, err := db.QueryContext(ctx, `SELECT `+customFieldColumns+` FROM custom_fields WHERE model_type = ? AND archived_at IS NULL ORDER BY position`, model)
	if err != nil {
		return nil, err
	}
	return store.ScanAll(rows, func(r *sqlRows) (*CustomField, error) { return scanCustomField(r) })
}

var numberRE = regexp.MustCompile(`^-?\d+(\.\d+)?$`)
var centsRE = regexp.MustCompile(`^-?\d+$`)
var schemeRE = regexp.MustCompile(`(?i)^[a-z][a-z0-9+.-]*://`)
var webRE = regexp.MustCompile(`(?i)^https?://[^\s/]+\.[^\s]+$`)

// Normalize is what an input becomes in the column, "" for blank (remove
// it); an error in words for what this kind can't hold.
func (f *CustomField) Normalize(input any) (string, error) {
	s := strings.TrimSpace(fmt.Sprint(input))
	if input == nil {
		s = ""
	}
	if s == "" && f.Kind != "boolean" {
		return "", nil
	}
	switch f.Kind {
	case "text":
		return truncate(s, 500), nil
	case "number":
		if !numberRE.MatchString(s) {
			return "", fmt.Errorf("should be a number")
		}
	case "money":
		if !centsRE.MatchString(s) {
			if fl, ok := input.(float64); ok && fl == float64(int64(fl)) {
				return strconv.FormatInt(int64(fl), 10), nil
			}
			return "", fmt.Errorf("should be whole cents, like 150000 for $1,500.00")
		}
		n, _ := strconv.ParseInt(s, 10, 64)
		return strconv.FormatInt(n, 10), nil
	case "date":
		d, err := time.Parse("2006-01-02", s)
		if err != nil {
			return "", fmt.Errorf("should be a date, YYYY-MM-DD")
		}
		return d.Format("2006-01-02"), nil
	case "boolean":
		if boolOf(input) {
			return "true", nil
		}
		return "false", nil
	case "choice":
		if !slices.Contains(f.Choices, s) {
			return "", fmt.Errorf("should be one of %s", strings.Join(f.Choices, ", "))
		}
	case "link":
		url := s
		if !schemeRE.MatchString(url) {
			url = "https://" + url
		}
		if !webRE.MatchString(url) {
			return "", fmt.Errorf("should be a web address")
		}
		return url, nil
	}
	return s, nil
}

// Cast is the stored text as JSON: cents and numbers as numbers, dates as
// YYYY-MM-DD, yes or no as true or false.
func (f *CustomField) Cast(text string) any {
	switch f.Kind {
	case "money":
		n, _ := strconv.ParseInt(text, 10, 64)
		return n
	case "number":
		if strings.Contains(text, ".") {
			n, _ := strconv.ParseFloat(text, 64)
			return n
		}
		n, _ := strconv.ParseInt(text, 10, 64)
		return n
	case "boolean":
		return text == "true"
	}
	return text
}

// customValues are the values of a model's fields for each subject:
// subject id → field key → typed value. One query for the lot.
func (a *App) customValues(ctx context.Context, db store.Querier, model string, ids []int64) (map[int64]Obj, error) {
	out := map[int64]Obj{}
	if len(ids) == 0 {
		return out, nil
	}
	fields, err := a.customFields(ctx, db, model)
	if err != nil {
		return nil, err
	}
	byID := map[int64]*CustomField{}
	for _, f := range fields {
		byID[f.ID] = f
	}
	args := []any{model}
	for _, id := range ids {
		args = append(args, id)
	}
	rows, err := db.QueryContext(ctx, `SELECT subject_id, custom_field_id, value FROM custom_values WHERE subject_type = ? AND subject_id IN (`+store.Placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	type cv struct {
		subject, field int64
		value          string
	}
	values, err := store.ScanAll(rows, func(r *sqlRows) (cv, error) {
		var v cv
		return v, r.Scan(&v.subject, &v.field, &v.value)
	})
	if err != nil {
		return nil, err
	}
	have := map[int64]map[int64]string{}
	for _, v := range values {
		if have[v.subject] == nil {
			have[v.subject] = map[int64]string{}
		}
		have[v.subject][v.field] = v.value
	}
	for _, id := range ids {
		o := Obj{}
		for _, f := range fields { // in the fields' order
			if v, ok := have[id][f.ID]; ok {
				o = o.Set(f.Key, f.Cast(v))
			}
		}
		out[id] = o
	}
	return out, nil
}

// setCustomFields writes the inputs given (key → value) for a subject; a
// blank removes its value. Errors name the field, as the Rails core did.
func (t *Tx) setCustomFields(model string, id int64, inputs map[string]any) error {
	if len(inputs) == 0 {
		return nil
	}
	fields, err := t.App.customFields(t.Ctx, t, model)
	if err != nil {
		return err
	}
	byKey := map[string]*CustomField{}
	for _, f := range fields {
		byKey[f.Key] = f
	}
	inv := apierr.Invalid{}
	type write struct {
		f    *CustomField
		text string
	}
	var writes []write
	for key, input := range inputs {
		f := byKey[key]
		if f == nil {
			inv.Add("base", fmt.Sprintf("There’s no custom field “%s”", key))
			continue
		}
		text, err := f.Normalize(input)
		if err != nil {
			inv.Add("base", f.Label+" "+err.Error())
			continue
		}
		writes = append(writes, write{f, text})
	}
	if err := inv.Err(); err != nil {
		return err
	}
	now := store.Now().DB()
	for _, w := range writes {
		if w.text == "" {
			if _, err := t.Exec(`DELETE FROM custom_values WHERE custom_field_id = ? AND subject_type = ? AND subject_id = ?`, w.f.ID, model, id); err != nil {
				return err
			}
			continue
		}
		if _, err := t.Exec(`INSERT INTO custom_values (custom_field_id, subject_type, subject_id, value, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT (custom_field_id, subject_type, subject_id) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			w.f.ID, model, id, w.text, now, now); err != nil {
			return err
		}
	}
	return nil
}

// customSearchText is every custom value of a subject, as plain text, for
// the index.
func customSearchText(ctx context.Context, db store.Querier, model string, id int64) string {
	values, err := store.Strings(ctx, db, `SELECT value FROM custom_values WHERE subject_type = ? AND subject_id = ?`, model, id)
	if err != nil {
		return ""
	}
	for i, v := range values {
		values[i] = plainText(v)
	}
	return strings.Join(values, " ")
}

// ---- agent refs ------------------------------------------------------------

// agentRef is how a record introduces itself to an agent (the Rails
// agent_ref): its type, id, "Type:id", ref (an engagement's), name and URL.
func (a *App) agentRef(typ string, id int64, ref, display, path string) Obj {
	o := O("type", typ, "id", id, "record", fmt.Sprintf("%s:%d", typ, id))
	if ref != "" {
		o = o.Set("ref", ref)
	}
	o = o.Set("display_name", display)
	if path != "" {
		o = o.Set("url", a.BaseURL()+path)
	}
	return o
}

// userRef is a person as answers show them: {id, name}; nil for nobody.
func userRef(id int64, name string) any {
	if id == 0 {
		return nil
	}
	return O("id", id, "name", name)
}
