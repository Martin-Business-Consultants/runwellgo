package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Settings › Reports: the Google Maps key (sealed, never sent back to the
// app's settings), a Map ID for a styled map, the Client custom field that
// holds an address, and what clients see in their portal.

// setting is a value kept in reports_settings; "" when there's none.
func setting(ctx context.Context, api *plugin.API, key string) string {
	var v string
	if err := api.QueryRow(ctx, `SELECT value FROM reports_settings WHERE key = ?`, key).Scan(&v); err != nil && !errors.Is(err, sql.ErrNoRows) {
		api.Log("reports: " + err.Error())
	}
	return v
}

func putSetting(ctx context.Context, api *plugin.API, key, value string) error {
	if value == "" {
		_, err := api.Exec(ctx, `DELETE FROM reports_settings WHERE key = ?`, key)
		return err
	}
	_, err := api.Exec(ctx, `INSERT INTO reports_settings (key, value, updated_at) VALUES (?, ?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, key, value, now())
	return err
}

// googleKey is the Maps key (for the browser) or, for geocoding, the server
// key when there's one; "" when none is set or it can't be opened.
func googleKey(ctx context.Context, api *plugin.API, server bool) string {
	sealed := ""
	if server {
		sealed = setting(ctx, api, "google_server_key")
	}
	if sealed == "" {
		sealed = setting(ctx, api, "google_key")
	}
	if sealed == "" {
		return ""
	}
	plain, err := api.Unseal(sealed)
	if err != nil {
		api.Log("reports: the Google key can't be opened: " + err.Error())
		return ""
	}
	return plain
}

// Sections are what a client's progress report can show, in order; money
// is off unless the owner turns it on.
var Sections = []struct{ Key, Label, Help string }{
	{"engagements", "Their engagements and progress", "Each one and how much of the shared work is done."},
	{"work_done", "Work done over time", "A chart of shared work finished, month by month."},
	{"on_time", "On-time delivery", "The share of work with a due date finished by it."},
	{"requests", "Requests answered", "How many of their requests were answered, and how fast."},
	{"commitments", "Commitments kept", "Promises you made them, kept on time."},
	{"next", "What's next", "Upcoming shared work and due dates, and anything waiting on them."},
	{"money", "Money agreed", "What's agreed, by engagement. Off unless you turn it on."},
}

var defaultSections = []string{"engagements", "work_done", "on_time", "requests", "commitments", "next"}

// portalSettings is whether clients see their report, and which sections.
type portalSettings struct {
	On       bool     `json:"on"`
	Sections []string `json:"sections"`
}

func portalConfig(ctx context.Context, api *plugin.API) portalSettings {
	p := portalSettings{On: true, Sections: defaultSections}
	if raw := setting(ctx, api, "portal"); raw != "" {
		var saved portalSettings
		if json.Unmarshal([]byte(raw), &saved) == nil {
			p = saved
		}
	}
	if p.Sections == nil {
		p.Sections = []string{}
	}
	return p
}

// field is a custom field, as the address picker shows it.
type field struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kind  string `json:"kind"`
}

// clientFields are the Client custom fields.
func clientFields(ctx context.Context, api *plugin.API) ([]field, error) {
	list, err := api.Records(ctx, plugin.Query{Type: "CustomField", Status: "Client", Limit: 200})
	if err != nil {
		return nil, err
	}
	out := []field{}
	for _, f := range list {
		k, _ := f["key"].(string)
		l, _ := f["label"].(string)
		kind, _ := f["kind"].(string)
		out = append(out, field{k, l, kind})
	}
	return out, nil
}

// addressField is the field that holds a client's address: the one chosen
// in Settings › Reports, else one whose key or label says address. auto is
// true when it was picked that way; none is chosen when the setting is
// "none".
func addressField(ctx context.Context, api *plugin.API) (f field, auto bool) {
	fields, err := clientFields(ctx, api)
	if err != nil {
		api.Log("reports: " + err.Error())
		return field{}, false
	}
	chosen := setting(ctx, api, "address_field")
	if chosen == "none" {
		return field{}, false
	}
	if chosen != "" {
		for _, f := range fields {
			if f.Key == chosen {
				return f, false
			}
		}
	}
	return pickAddress(fields), true
}

// pickAddress is the field that most looks like an address: one keyed or
// labelled exactly that first, then one that mentions it.
func pickAddress(fields []field) field {
	for _, f := range fields {
		if strings.EqualFold(f.Key, "address") || strings.EqualFold(strings.TrimSpace(f.Label), "address") {
			return f
		}
	}
	for _, f := range fields {
		if strings.Contains(strings.ToLower(f.Key+" "+f.Label), "address") && f.Kind != "link" {
			return f
		}
	}
	return field{}
}

// mapConfig is how the page draws its map: Google Maps (with the key the
// browser needs to load it) when a key is set and the page may load it,
// else the built-in globe, saying why.
func mapConfig(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	key := googleKey(ctx, api, false)
	out := map[string]any{"provider": "globe", "map_id": setting(ctx, api, "map_id"), "can_manage": req.Actor.Can("manage_settings")}
	switch {
	case key == "":
		out["message"] = "Add a Google Maps key in Settings › Reports for street-level maps."
	case !api.Grants().ContentExternal:
		out["message"] = "Google Maps is set up, but Reports may not load content from other sites: tick “Load content from other sites” for Reports in Settings › Plugins."
	default:
		// The Maps JavaScript API runs in the browser, so the browser has
		// the key: restrict it to this site's address in Google Cloud.
		out["provider"], out["key"] = "google", key
	}
	return plugin.JSON(200, out), nil
}

// showSettings is Settings › Reports.
func showSettings(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	fields, err := clientFields(ctx, api)
	if err != nil {
		return nil, err
	}
	addr, auto := addressField(ctx, api)
	site, err := api.Site(ctx)
	if err != nil {
		return nil, err
	}
	h := home(ctx, api, site)
	counts := map[string]int{}
	rows, err := api.Query(ctx, `SELECT status, count(*) FROM reports_address_places GROUP BY status`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		var n int
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		counts[s] = n
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	sections := []map[string]any{}
	for _, s := range Sections {
		sections = append(sections, map[string]any{"key": s.Key, "label": s.Label, "help": s.Help})
	}
	hidden, err := hiddenClients(ctx, api)
	if err != nil {
		return nil, err
	}
	g := api.Grants()
	return plugin.JSON(200, map[string]any{
		"google_key_set": setting(ctx, api, "google_key") != "", "google_server_key_set": setting(ctx, api, "google_server_key") != "",
		"map_id": setting(ctx, api, "map_id"), "content_granted": g.ContentExternal, "secrets_granted": g.Secrets, "portal_granted": g.Portal,
		"fields": fields, "address_field": addr.Key, "address_auto": auto && addr.Key != "", "address_none": setting(ctx, api, "address_field") == "none",
		"geocoded": counts["ok"], "not_found": counts["not_found"], "failed": counts["error"],
		"geocoder": map[bool]string{true: "Google", false: "OpenStreetMap"}[googleKey(ctx, api, true) != ""],
		"home":     h, "home_override": setting(ctx, api, "home") != "",
		"portal": portalConfig(ctx, api), "sections": sections, "hidden_clients": hidden,
	}), nil
}

// saveSettings saves the Map ID, the address field and the portal's
// sections; a changed address field looks every client up again.
func saveSettings(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	var in struct {
		MapID        *string         `json:"map_id"`
		AddressField *string         `json:"address_field"`
		Portal       *portalSettings `json:"portal"`
	}
	if r := decodeBody(req, "settings", &in); r != nil {
		return r, nil
	}
	errs := map[string][]string{}
	if in.MapID != nil {
		id := strings.TrimSpace(*in.MapID)
		if len(id) > 64 || strings.ContainsAny(id, " /?&#<>\"'") {
			errs["map_id"] = []string{"is a Map ID from Google Cloud, like 8e0a97af9386fef"}
		} else if err := putSetting(ctx, api, "map_id", id); err != nil {
			return nil, err
		}
	}
	if in.AddressField != nil {
		key := strings.TrimSpace(*in.AddressField)
		fields, err := clientFields(ctx, api)
		if err != nil {
			return nil, err
		}
		ok := key == "" || key == "none" || slices.ContainsFunc(fields, func(f field) bool { return f.Key == key })
		if !ok {
			errs["address_field"] = []string{"isn't one of the client fields"}
		} else if key != setting(ctx, api, "address_field") {
			if err := putSetting(ctx, api, "address_field", key); err != nil {
				return nil, err
			}
			// Another field: what was looked up was another field's.
			if _, err := api.Exec(ctx, `DELETE FROM reports_address_places`); err != nil {
				return nil, err
			}
			kickGeocode(api, 0)
		}
	}
	if in.Portal != nil {
		keep := []string{}
		for _, s := range Sections {
			if slices.Contains(in.Portal.Sections, s.Key) {
				keep = append(keep, s.Key)
			}
		}
		raw, _ := json.Marshal(portalSettings{On: in.Portal.On, Sections: keep})
		if err := putSetting(ctx, api, "portal", string(raw)); err != nil {
			return nil, err
		}
	}
	if len(errs) > 0 {
		return plugin.Invalid(errs), nil
	}
	return showSettings(ctx, api, req)
}

// setGoogleKey seals and keeps a key: {key, server} (server: the one for
// geocoding only). It's never answered back.
func setGoogleKey(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	var in struct {
		Key    string `json:"key"`
		Server bool   `json:"server"`
	}
	if r := decodeBody(req, "google", &in); r != nil {
		return r, nil
	}
	in.Key = strings.TrimSpace(in.Key)
	if len(in.Key) < 20 || len(in.Key) > 100 || strings.ContainsAny(in.Key, " /?&#<>\"'") {
		return plugin.Invalid(map[string][]string{"key": {"doesn't look like a Google API key (it starts AIza…)"}}), nil
	}
	sealed, err := api.Seal(in.Key)
	if err != nil {
		if r := plugin.Answer(err); r != nil {
			return r, nil
		}
		return plugin.JSON(409, map[string]any{"error": "secrets", "message": "Reports may not keep secrets: tick “Keep secrets” for Reports in Settings › Plugins."}), nil
	}
	name := "google_key"
	if in.Server {
		name = "google_server_key"
	}
	if err := putSetting(ctx, api, name, sealed); err != nil {
		return nil, err
	}
	// Places that OpenStreetMap couldn't find get another go with Google.
	if _, err := api.Exec(ctx, `DELETE FROM reports_address_places WHERE status <> 'ok'`); err != nil {
		return nil, err
	}
	kickGeocode(api, 0)
	return plugin.JSON(200, map[string]any{"summary": "Saved the key. It isn't shown again.", "set": true}), nil
}

func clearGoogleKey(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	name := "google_key"
	if req.Query.Get("server") == "1" {
		name = "google_server_key"
	}
	if err := putSetting(ctx, api, name, ""); err != nil {
		return nil, err
	}
	return plugin.JSON(200, map[string]any{"summary": "Removed the key.", "set": false}), nil
}

// geocodeNow looks every client's address up again, in the background.
func geocodeNow(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if _, err := api.Exec(ctx, `DELETE FROM reports_address_places WHERE status <> 'ok'`); err != nil {
		return nil, err
	}
	kickGeocode(api, 0)
	return plugin.JSON(202, map[string]any{"summary": "Looking addresses up. It takes about a second a client."}), nil
}

// hiddenClients are the clients whose report is switched off.
func hiddenClients(ctx context.Context, api *plugin.API) ([]map[string]any, error) {
	rows, err := api.Query(ctx, `SELECT client_id FROM reports_portal_hidden ORDER BY client_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := []map[string]any{}
	if len(ids) == 0 {
		return out, nil
	}
	list, err := api.Records(ctx, plugin.Query{Type: "Client", IDs: ids, Limit: 500})
	if err != nil {
		return nil, err
	}
	for _, c := range list {
		out = append(out, map[string]any{"id": c["id"], "name": c["name"], "path": fmt.Sprintf("/clients/%v", c["id"])})
	}
	return out, nil
}
