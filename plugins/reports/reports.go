// Package reports is Reports: a snapshot of the business (how each client
// is doing, the money agreed and the work delivered) around a map with a
// pin where each client is, and a progress report each client sees in
// their portal.
//
// Where a client is comes, first, from someone setting it by hand (a
// search or a pin dropped on the map, reports_client_places); then from
// their address (the Client custom field Settings › Reports names, looked
// up with Google's Geocoding API when a key is set, else OpenStreetMap's
// Nominatim, one request a second at most, kept in reports_address_places
// and reports_geocode_cache); and, failing both, from their time zone, at
// that zone's city, marked approximate. Home is the client marked internal
// ("This is us"), placed the same way; an override is kept for an install
// with no internal client or no address for it.
//
// The map is Google Maps when an owner adds a key (its script, tiles and
// fonts allowed through content:external), else the built-in globe, which
// works offline.
//
// It reads Runwell through the plugin API alone: metrics for the numbers
// (each client's counts, each engagement's state and value, dated facts for
// the trends), records:read for a client's card, their address and their
// shared work, records:write to add a note from it, portal for the client's
// own report. Its screens are its own Svelte (ui/src/plugins/reports).
// With it off, Runwell is as it was.
package reports

import (
	"context"
	"encoding/json"
	"slices"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Key is the plugin's key.
const Key = "reports"

// nominatim is the geocoder's host (OpenStreetMap's public Nominatim);
// googleAPIs is Google's (geocoding, with a key).
const (
	nominatim  = "nominatim.openstreetmap.org"
	googleAPIs = "maps.googleapis.com"
)

// content is what Google Maps loads in the page (its script, tiles, fonts
// and styles), added to the app's Content-Security-Policy while Reports is
// on and granted content:external.
var content = map[string][]string{
	"script-src":  {"https://maps.googleapis.com", "https://maps.gstatic.com", "https://*.googleapis.com", "https://*.gstatic.com"},
	"img-src":     {"https://maps.googleapis.com", "https://maps.gstatic.com", "https://*.googleapis.com", "https://*.gstatic.com"},
	"connect-src": {"https://maps.googleapis.com", "https://*.googleapis.com", "https://*.gstatic.com", "data:", "blob:"},
	"font-src":    {"https://fonts.gstatic.com"},
	"style-src":   {"https://fonts.googleapis.com"},
	"worker-src":  {"blob:"},
}

const schema = `
CREATE TABLE IF NOT EXISTS "reports_client_places" ("client_id" integer PRIMARY KEY NOT NULL, "label" varchar NOT NULL, "region" varchar, "country" varchar, "country_code" varchar, "lat" float NOT NULL, "lng" float NOT NULL, "source" varchar NOT NULL, "set_by_id" integer, "set_by" varchar, "updated_at" varchar NOT NULL);
CREATE TABLE IF NOT EXISTS "reports_geocode_cache" ("query" varchar PRIMARY KEY NOT NULL, "results" text NOT NULL, "fetched_at" varchar NOT NULL);
CREATE TABLE IF NOT EXISTS "reports_settings" ("key" varchar PRIMARY KEY NOT NULL, "value" text NOT NULL, "updated_at" varchar NOT NULL);
CREATE TABLE IF NOT EXISTS "reports_address_places" ("client_id" integer PRIMARY KEY NOT NULL, "address" text NOT NULL, "status" varchar NOT NULL, "label" varchar, "region" varchar, "country" varchar, "country_code" varchar, "lat" float, "lng" float, "provider" varchar, "error" varchar, "updated_at" varchar NOT NULL);
CREATE TABLE IF NOT EXISTS "reports_portal_hidden" ("client_id" integer PRIMARY KEY NOT NULL, "hidden_by" varchar, "updated_at" varchar NOT NULL);
`

func param(name, typ, desc string) plugin.ToolParam {
	return plugin.ToolParam{Name: name, Type: typ, Description: desc}
}

// filters are the report's filters, as tools take them.
var filters = []plugin.ToolParam{
	param("status", "", "active (the default), dormant, former or all."),
	param("health", "", "on_track, watch, at_risk or all (the default)."),
	param("type", "", "Only clients with an engagement of this type: project, work_order or service."),
}

// Module is the plugin, for the core to register.
func Module() *plugin.Module {
	return &plugin.Module{
		Manifest: plugin.Manifest{
			Key:      Key,
			Name:     "Reports",
			Version:  "1.1.0",
			Author:   "Runwell",
			Homepage: "https://github.com/Martin-Business-Consultants/runwellgo",
			Description: "A snapshot of the business on a map with a pin for every client: how each one is doing " +
				"and why, the money agreed, the work delivered, and how far your work reaches; and a progress report each client sees in their portal.",
			Permissions: plugin.Grants{RecordsRead: true, RecordsWrite: true, Tables: true, Metrics: true, Secrets: true, Portal: true, ContentExternal: true,
				HTTP: []string{nominatim, googleAPIs}},
			Content: content,
			Schema:  schema,
			Hooks:   []string{"client.created", "client.updated", "client.deleted", "user.removed"},
			Tasks:   []plugin.TaskSpec{{Name: "geocode", Every: "15m"}},
			Routes: []plugin.Route{
				{Method: "GET", Path: "/overview", Export: "overview"},
				{Method: "GET", Path: "/health", Export: "health", Tool: &plugin.Tool{
					Name: "show_client_health", Title: "Show how each client is doing",
					Description: "Every client scored out of 100, at risk first, with the reasons: late work, overdue commitments, agreements waiting on them, requests to triage, days since anyone touched them, and momentum (work done in the last four weeks against the four before). 80 and up is on track, 55 to 79 needs watching, under 55 is at risk.\n\nOften next: show_client_snapshot.",
					Query:       filters,
				}},
				{Method: "GET", Path: "/money", Export: "money", Tool: &plugin.Tool{
					Name: "show_money_report", Title: "Show the money report",
					Description: "What's agreed: by month (approved agreements, last 12 months), by client, by engagement type, recurring against fixed, and the pipeline (drafts, sent and waiting, changes requested). Nothing when the install hides prices.",
					Query:       filters,
				}},
				{Method: "GET", Path: "/delivery", Export: "delivery", Tool: &plugin.Tool{
					Name: "show_delivery_report", Title: "Show the delivery report",
					Description: "Work added and done by the week (26 weeks), the share of work with a due date done on time, how long agreements take to be approved, and requests in against triaged.",
					Query:       filters,
				}},
				{Method: "GET", Path: "/reach", Export: "reach", Tool: &plugin.Tool{
					Name: "show_reach_report", Title: "Show where clients are",
					Description: "Clients by country and time zone (each placed where someone set it by hand, else from their address, else approximately by its time zone), home (the client marked internal), and new clients by month.",
					Query:       filters,
				}},
				{Method: "GET", Path: "/clients/{id}", Export: "client", Tool: &plugin.Tool{
					Name: "show_client_snapshot", Title: "Show a client's snapshot",
					Description: "One client at a glance: where they are and their time now, health and why, money agreed (this year, by engagement), engagements and how far along each is, open and late work, requests, commitments due, the last touch and how long ago, and contacts with portal access.\n\nOften next: set_client_place.",
				}},
				{Method: "PUT", Path: "/clients/{id}/place", Export: "setPlace", Tool: &plugin.Tool{
					Name: "set_client_place", Title: "Set where a client is",
					Description: "Puts a client on the map by hand (it wins over their address). Either query (a place to look up on OpenStreetMap, like \"Lisbon, Portugal\"; the best match is taken) or lat and lng with a label (the city) and country_code.",
					Wrap:        "place", Body: `{query: a place to look up, or label, region, country, country_code (ISO, like PT), lat, lng}`,
				}},
				{Method: "DELETE", Path: "/clients/{id}/place", Export: "clearPlace", Tool: &plugin.Tool{
					Name: "clear_client_place", Title: "Forget where a client is",
					Description: "Forgets the place set by hand: back to their address (when it was found), else approximately their time zone.",
				}},
				{Method: "GET", Path: "/places", Export: "search"},
				{Method: "PUT", Path: "/home", Export: "setHome", Capability: "manage_settings"},
				{Method: "DELETE", Path: "/home", Export: "clearHome", Capability: "manage_settings"},
				{Method: "POST", Path: "/clients/{id}/notes", Export: "addNote"},
				{Method: "GET", Path: "/map", Export: "mapConfig"},
				{Method: "GET", Path: "/settings", Export: "settings", Capability: "manage_settings"},
				{Method: "PATCH", Path: "/settings", Export: "saveSettings", Capability: "manage_settings"},
				{Method: "PUT", Path: "/settings/google_key", Export: "setGoogleKey", Capability: "manage_settings"},
				{Method: "DELETE", Path: "/settings/google_key", Export: "clearGoogleKey", Capability: "manage_settings"},
				{Method: "POST", Path: "/geocode", Export: "geocodeNow", Capability: "manage_settings"},
				{Method: "GET", Path: "/clients/{id}/client_report", Export: "clientReport", Tool: &plugin.Tool{
					Name: "show_client_report", Title: "Show a client's progress report",
					Description: "What a client sees on their portal's progress page: their engagements and how far along each is, work done by the month, on-time delivery, requests answered and how fast, commitments we kept, and what's next; money agreed only when the owner shows it. Nothing internal (no health score, no notes, no other clients). Also says whether they see it (the page on, and not switched off for them).",
				}},
				{Method: "PUT", Path: "/clients/{id}/client_report", Export: "setClientReport", Capability: "manage_settings"},
				{Method: "GET", Path: "/portal/status", Export: "portalStatus", Portal: true},
				{Method: "GET", Path: "/portal/report", Export: "portalReport", Portal: true},
			},
			Nav: &plugin.NavPage{Label: "Reports", Path: "/reports", Icon: "globe"},
		},
		Handlers: map[string]plugin.Handler{
			"overview": overview, "health": healthReport, "money": moneyReport, "delivery": deliveryReport, "reach": reachReport,
			"client": clientSnapshot, "setPlace": setPlace, "clearPlace": clearPlace, "search": searchPlaces,
			"setHome": setHome, "clearHome": clearHome, "addNote": addNote,
			"mapConfig": mapConfig, "settings": showSettings, "saveSettings": saveSettings, "setGoogleKey": setGoogleKey, "clearGoogleKey": clearGoogleKey,
			"geocodeNow": geocodeNow, "clientReport": clientReport, "setClientReport": setClientReport, "portalStatus": portalStatus, "portalReport": portalReport,
		},
		Tasks:   map[string]plugin.Task{"geocode": geocodeTask},
		OnEvent: onEvent,
	}
}

// userRemoved keeps who placed a client on the map as a name (set_by) and
// drops the id that would point at no one. Nothing else in reports is a
// person's own.
func userRemoved(ctx context.Context, api *plugin.API, data json.RawMessage) error {
	var ev struct {
		Payload struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	if ev.Payload.ID == 0 {
		return nil
	}
	if _, err := api.Exec(ctx, `UPDATE reports_client_places SET set_by = ? WHERE set_by_id = ? AND COALESCE(set_by, '') = ''`, ev.Payload.Name, ev.Payload.ID); err != nil {
		return err
	}
	_, err := api.Exec(ctx, `UPDATE reports_client_places SET set_by_id = NULL WHERE set_by_id = ?`, ev.Payload.ID)
	return err
}

// onEvent forgets a deleted client's places, and looks a new client's
// address up, or a client's whose address changed.
func onEvent(ctx context.Context, api *plugin.API, kind string, data json.RawMessage) error {
	var ev struct {
		SubjectType string `json:"subject_type"`
		SubjectID   int64  `json:"subject_id"`
		Payload     struct {
			Changed []string `json:"changed"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(data, &ev); err != nil {
		return err
	}
	if kind == "user.removed" {
		return userRemoved(ctx, api, data)
	}
	if ev.SubjectType != "Client" {
		return nil
	}
	switch kind {
	case "client.deleted":
		for _, table := range []string{"reports_client_places", "reports_address_places", "reports_portal_hidden"} {
			if _, err := api.Exec(ctx, `DELETE FROM `+table+` WHERE client_id = ?`, ev.SubjectID); err != nil {
				return err
			}
		}
	case "client.created":
		kickGeocode(api, ev.SubjectID)
	case "client.updated":
		field, _ := addressField(ctx, api)
		if field.Key != "" && slices.Contains(ev.Payload.Changed, "custom_fields."+field.Key) {
			kickGeocode(api, ev.SubjectID)
		}
	}
	return nil
}
