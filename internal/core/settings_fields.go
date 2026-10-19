package core

import (
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/v3/internal/store"
)

// Settings › Fields: what this install records about clients, engagements
// and work beyond the core. A field's key is set once from its label and
// never changes, so agents and imports can rely on it; its kind is fixed
// once it has values; archiving hides it and keeps them.

func init() { routeSets = append(routeSets, (*App).fieldRoutes) }

func (a *App) fieldRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	kinds := strings.Join(choiceKeysOf(), ", ")
	a.handle(mux, "GET /api/settings/fields", a.listFields, ms, tool("list_custom_fields", "List custom fields",
		note("Every field an install added to clients (Client), engagements (Engagement) and work (Todo), with the key to set it by (custom_fields: { key: value } on create/update tools) and its kind.")))
	a.handle(mux, "POST /api/settings/fields", a.createField, ms, tool("create_custom_field", "Add a custom field",
		note("kind: "+kinds+". choices (for choice): separated by commas. listed: a column in the index table. client_visible: shown in the client portal (engagements)."),
		body("field", `{model_type: Client|Engagement|Todo, label (required), kind, choices, listed: boolean, client_visible: boolean}`, true)))
	a.handle(mux, "PATCH /api/settings/fields/{id}", a.updateField, ms, tool("update_custom_field", "Change a custom field",
		note("Rename it, change its choices, whether it's listed or shown to clients, its position (1 is first), or restore an archived one (archived: false). Its kind and key don't change."),
		body("field", `{label, choices, listed: boolean, client_visible: boolean, position: integer, archived: boolean}`, true)))
	a.handle(mux, "DELETE /api/settings/fields/{id}", a.archiveField, ms, tool("archive_custom_field", "Archive a custom field",
		note("Hides it from forms, pages and agents. Its values are kept, and update_custom_field with archived: false brings it back.")))
}

func choiceKeysOf() []string {
	out := []string{}
	for _, k := range CustomFieldKinds {
		out = append(out, k.Key)
	}
	return out
}

func kindLabel(kind string) string {
	for _, k := range CustomFieldKinds {
		if k.Key == kind {
			return k.Label
		}
	}
	return kind
}

// fieldModelName is a model's word, "Clients" for Client.
func fieldModelName(s *Settings, model string, count int) string {
	return s.Term(map[string]string{"Client": "client", "Engagement": "engagement", "Todo": "work"}[model], count)
}

func fieldJSON(f *CustomField) Obj {
	o := O("id", f.ID, "model_type", f.ModelType, "key", f.Key, "label", f.Label, "kind", f.Kind, "kind_label", kindLabel(f.Kind),
		"position", f.Position, "listed", f.Listed, "client_visible", f.ClientVisible)
	if f.Kind == "choice" {
		o = o.Set("choices", f.Choices)
	}
	return o.Set("archived", f.ArchivedAt.Valid)
}

func (a *App) listFields(c *Ctx) error {
	ctx := c.R.Context()
	rows, err := a.store.QueryContext(ctx, `SELECT `+customFieldColumns+` FROM custom_fields ORDER BY position, id`)
	if err != nil {
		return err
	}
	fields, err := store.ScanAll(rows, func(r *sqlRows) (*CustomField, error) { return scanCustomField(r) })
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	list := []Obj{}
	archived := 0
	for _, model := range CustomFieldModels { // grouped by model, as the page shows them
		for _, f := range fields {
			if f.ModelType == model {
				list = append(list, fieldJSON(f))
				if f.ArchivedAt.Valid {
					archived++
				}
			}
		}
	}
	summary := "No custom fields"
	if len(list) > 0 {
		word := "fields"
		if len(list) == 1 {
			word = "field"
		}
		summary = fmt.Sprintf("%d custom %s", len(list)-archived, word)
		if archived > 0 {
			summary += fmt.Sprintf(", %d archived", archived)
		}
	}
	models := []Obj{}
	for _, m := range CustomFieldModels {
		models = append(models, O("model", m, "name", fieldModelName(s, m, 2)))
	}
	kinds := []Obj{}
	for _, k := range CustomFieldKinds {
		kinds = append(kinds, O("key", k.Key, "label", k.Label))
	}
	c.Summary = summary
	return c.JSON(200, O("summary", summary, "fields", list, "models", models, "kinds", kinds))
}

var nonKey = regexp.MustCompile(`[^a-z0-9]+`)

// fieldKey is a label as a key: "Matter number" → "matter_number".
func fieldKey(label string) string {
	return strings.Trim(nonKey.ReplaceAllString(strings.ToLower(label), "_"), "_")
}

// fieldChoices are choices given as text (commas or lines) or a list.
func fieldChoices(v any) []string {
	var raw []string
	switch x := v.(type) {
	case []any:
		for _, c := range x {
			raw = append(raw, str(c))
		}
	default:
		raw = regexp.MustCompile(`[\n,]`).Split(str(v), -1)
	}
	out := []string{}
	for _, c := range raw {
		if c = strings.TrimSpace(c); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

func (a *App) loadField(c *Ctx, db store.Querier) (*CustomField, error) {
	id, err := c.IntParam("id")
	if err != nil {
		return nil, err
	}
	f, err := scanCustomField(db.QueryRowContext(c.R.Context(), `SELECT `+customFieldColumns+` FROM custom_fields WHERE id = ?`, id))
	if isNoRows(err) {
		return nil, notFound("field")
	}
	return f, err
}

func (a *App) createField(c *Ctx) error {
	body, err := c.Wrapped("field")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	f := &CustomField{ModelType: str(body["model_type"]), Label: strings.TrimSpace(str(body["label"])), Kind: str(body["kind"]),
		Choices: fieldChoices(body["choices"]), Listed: boolOf(body["listed"]), ClientVisible: boolOf(body["client_visible"])}
	if f.Kind == "" {
		f.Kind = "text"
	}
	f.Key = fieldKey(f.Label)
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		bad := apierr.Invalid{}
		if !slices.Contains(CustomFieldModels, f.ModelType) {
			bad.Add("model_type", "is not included in the list")
		}
		if !slices.Contains(choiceKeysOf(), f.Kind) {
			bad.Add("kind", "is not included in the list")
		}
		if f.Label == "" {
			bad.Add("label", "can't be blank")
		}
		if f.Key == "" {
			bad.Add("key", "can't be blank")
		} else if n, err := store.Count(ctx, t, `SELECT count(*) FROM custom_fields WHERE model_type = ? AND key = ?`, f.ModelType, f.Key); err != nil {
			return err
		} else if n > 0 {
			bad.Add("key", "is already used by another field")
		}
		if f.Kind == "choice" && len(f.Choices) == 0 {
			bad.Add("choices", "are needed for a choice field")
		}
		if err := fullInvalid(bad); err != nil {
			return err
		}
		last, err := store.Count(ctx, t, `SELECT COALESCE(max(position), 0) FROM custom_fields WHERE model_type = ?`, f.ModelType)
		if err != nil {
			return err
		}
		f.Position = last + 1
		now := store.Now().DB()
		f.ID, err = t.Insert(`INSERT INTO custom_fields (model_type, label, key, kind, choices, position, listed, client_visible, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			f.ModelType, f.Label, f.Key, f.Kind, string(store.MustJSON(f.Choices)), f.Position, f.Listed, f.ClientVisible, now, now)
		return err
	})
	if err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	c.Summary = "Added " + f.Label + " to " + strings.ToLower(fieldModelName(s, f.ModelType, 2)) + "."
	return c.JSON(201, O("summary", c.Summary, "field", fieldJSON(f)))
}

// moveField puts f at position (1 is first, clamped) among its model's
// fields, shifting the others.
func (t *Tx) moveField(f *CustomField, position int) error {
	ids, err := store.Int64s(t.Ctx, t, `SELECT id FROM custom_fields WHERE model_type = ? AND id != ? ORDER BY position, id`, f.ModelType, f.ID)
	if err != nil {
		return err
	}
	position = max(1, min(position, len(ids)+1))
	ids = slices.Insert(ids, position-1, f.ID)
	for i, id := range ids {
		if _, err := t.Exec(`UPDATE custom_fields SET position = ? WHERE id = ?`, i+1, id); err != nil {
			return err
		}
	}
	f.Position = position
	return nil
}

func (a *App) updateField(c *Ctx) error {
	body, err := c.Wrapped("field")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	var f *CustomField
	err = a.write(ctx, c.Actor, func(t *Tx) error {
		if f, err = a.loadField(c, t); err != nil {
			return err
		}
		set := map[string]any{}
		bad := apierr.Invalid{}
		if has(body, "label") {
			if f.Label = strings.TrimSpace(str(body["label"])); f.Label == "" {
				bad.Add("label", "can't be blank")
			}
			set["label"] = f.Label
		}
		if has(body, "choices") {
			f.Choices = fieldChoices(body["choices"])
			set["choices"] = string(store.MustJSON(f.Choices))
		}
		if f.Kind == "choice" && len(f.Choices) == 0 {
			bad.Add("choices", "are needed for a choice field")
		}
		if has(body, "listed") {
			f.Listed = boolOf(body["listed"])
			set["listed"] = f.Listed
		}
		if has(body, "client_visible") {
			f.ClientVisible = boolOf(body["client_visible"])
			set["client_visible"] = f.ClientVisible
		}
		if v, ok := body["archived"]; ok && v != nil {
			if boolOf(v) {
				if !f.ArchivedAt.Valid {
					f.ArchivedAt = store.Now()
				}
			} else {
				f.ArchivedAt = store.Time{}
			}
			set["archived_at"] = f.ArchivedAt.DB()
		}
		if err := fullInvalid(bad); err != nil {
			return err
		}
		if err := t.update("custom_fields", f.ID, set); err != nil {
			return err
		}
		if p, ok := intOf(body["position"]); ok {
			return t.moveField(f, int(p))
		}
		return nil
	})
	if err != nil {
		return err
	}
	c.Summary = "Saved " + f.Label + "."
	return c.JSON(200, O("summary", c.Summary, "field", fieldJSON(f)))
}

func (a *App) archiveField(c *Ctx) error {
	ctx := c.R.Context()
	var f *CustomField
	err := a.write(ctx, c.Actor, func(t *Tx) error {
		var err error
		if f, err = a.loadField(c, t); err != nil {
			return err
		}
		f.ArchivedAt = store.Now()
		return t.update("custom_fields", f.ID, map[string]any{"archived_at": f.ArchivedAt.DB()})
	})
	if err != nil {
		return err
	}
	c.Summary = "Archived " + f.Label + ". Its values are kept."
	return c.JSON(200, O("summary", c.Summary, "field", fieldJSON(f)))
}
