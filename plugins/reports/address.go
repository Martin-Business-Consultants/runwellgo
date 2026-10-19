package reports

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Clients placed by their address: the Client custom field Settings ›
// Reports names, looked up (Google's Geocoding API with a key, else
// Nominatim) in the background, when the plugin starts (its task's first
// run), when a client is added or their address changes, and every 15
// minutes for what's still to do. What was found is kept with the address
// it was found for (reports_address_places), so a changed address is looked
// up again and an unchanged one never is.

// addressPlace is a client's address and where it was found.
type addressPlace struct {
	Address string
	Status  string // ok, not_found, error
	Place   Place
}

// addressPlaces are the addresses found, by client id (status ok).
func addressPlaces(ctx context.Context, api *plugin.API) (map[int64]addressPlace, error) {
	rows, err := api.Query(ctx, `SELECT client_id, address, status, COALESCE(label, ''), COALESCE(region, ''), COALESCE(country, ''), COALESCE(country_code, ''), COALESCE(lat, 0), COALESCE(lng, 0), COALESCE(provider, ''), updated_at FROM reports_address_places`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]addressPlace{}
	for rows.Next() {
		var id int64
		var a addressPlace
		var provider string
		if err := rows.Scan(&id, &a.Address, &a.Status, &a.Place.Label, &a.Place.Region, &a.Place.Country, &a.Place.CountryCode, &a.Place.Lat, &a.Place.Lng, &provider, &a.Place.UpdatedAt); err != nil {
			return nil, err
		}
		a.Place.Source, a.Place.Address, a.Place.SetBy = "address", a.Address, provider
		out[id] = a
	}
	return out, rows.Err()
}

// cleanAddress is an address on one line: its lines joined by commas,
// spaces squeezed.
func cleanAddress(v any) string {
	s, _ := v.(string)
	var parts []string
	for _, line := range strings.FieldsFunc(s, func(r rune) bool { return r == '\n' || r == '\r' }) {
		if l := strings.Join(strings.Fields(line), " "); l != "" {
			parts = append(parts, strings.TrimRight(l, ","))
		}
	}
	return truncate(strings.Join(parts, ", "), 300)
}

// clientAddresses are each client's address (the field's value), by id,
// for the clients given (all when none).
func clientAddresses(ctx context.Context, api *plugin.API, key string, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		m, err := api.Metrics(ctx, plugin.MetricsQuery{Since: time.Now().AddDate(0, 0, -1)})
		if err != nil {
			return nil, err
		}
		for _, c := range m.Clients {
			ids = append(ids, c.ID)
		}
	}
	for start := 0; start < len(ids); start += 400 {
		batch := ids[start:min(start+400, len(ids))]
		list, err := api.Records(ctx, plugin.Query{Type: "Client", IDs: batch, Limit: 500})
		if err != nil {
			return nil, err
		}
		for _, c := range list {
			id, _ := c["id"].(int64)
			fields, _ := c["custom_fields"].(map[string]any)
			out[id] = cleanAddress(fields[key])
		}
	}
	return out, nil
}

var (
	sweepMu  sync.Mutex
	sweeping bool
	queued   = map[int64]bool{} // clients to look up next; 0 is everyone
	lastFull time.Time
)

// kickGeocode looks client's address up (every client's, for 0) in the
// background, unless a sweep is running, which picks it up.
func kickGeocode(api *plugin.API, client int64) {
	sweepMu.Lock()
	queued[client] = true
	if sweeping {
		sweepMu.Unlock()
		return
	}
	sweeping = true
	sweepMu.Unlock()
	err := api.Background("geocode", func(ctx context.Context, api *plugin.API) {
		defer func() {
			if r := recover(); r != nil {
				stopSweeping()
				panic(r)
			}
		}()
		runSweeps(ctx, api)
	})
	if err != nil {
		stopSweeping()
		api.Log("reports: " + err.Error())
	}
}

// runSweeps looks up what's queued until nothing is (or its time is up:
// the rest waits for the next kick or the task), then marks the sweep done.
func runSweeps(ctx context.Context, api *plugin.API) {
	for {
		sweepMu.Lock()
		todo := queued
		if len(todo) == 0 || ctx.Err() != nil {
			sweeping = false
			sweepMu.Unlock()
			return
		}
		queued = map[int64]bool{}
		sweepMu.Unlock()
		var ids []int64
		if !todo[0] {
			for id := range todo {
				ids = append(ids, id)
			}
		}
		if err := geocodeClients(ctx, api, ids); err != nil {
			api.Log("reports: geocoding addresses: " + err.Error())
		}
	}
}

// stopSweeping marks the sweep done when it couldn't start or panicked.
func stopSweeping() {
	sweepMu.Lock()
	sweeping = false
	sweepMu.Unlock()
}

// kickIfStale starts a full sweep when none ran in this process for ten
// minutes (the page opened: places should be current).
func kickIfStale(api *plugin.API) {
	sweepMu.Lock()
	stale := time.Since(lastFull) > 10*time.Minute
	sweepMu.Unlock()
	if stale {
		kickGeocode(api, 0)
	}
}

// geocodeTask is the every-15-minutes sweep (its first run is just after
// the plugin is switched on).
func geocodeTask(ctx context.Context, api *plugin.API) error {
	sweepMu.Lock()
	queued[0] = true
	if sweeping {
		sweepMu.Unlock()
		return nil
	}
	sweeping = true
	sweepMu.Unlock()
	defer func() {
		if r := recover(); r != nil {
			stopSweeping()
			panic(r)
		}
	}()
	runSweeps(ctx, api)
	return nil
}

// geocodeClients looks up the addresses of the clients given (all when
// none) that changed since they were last looked up, or failed an hour or
// more ago, and forgets those that no longer have one.
func geocodeClients(ctx context.Context, api *plugin.API, ids []int64) error {
	f, _ := addressField(ctx, api)
	full := len(ids) == 0
	if full {
		sweepMu.Lock()
		lastFull = time.Now()
		sweepMu.Unlock()
	}
	if f.Key == "" {
		if full {
			_, err := api.Exec(ctx, `DELETE FROM reports_address_places`)
			return err
		}
		return nil
	}
	addrs, err := clientAddresses(ctx, api, f.Key, ids)
	if err != nil {
		return err
	}
	have, err := allAddressRows(ctx, api)
	if err != nil {
		return err
	}
	if full {
		for id := range have {
			if _, ok := addrs[id]; !ok {
				if _, err := api.Exec(ctx, `DELETE FROM reports_address_places WHERE client_id = ?`, id); err != nil {
					return err
				}
			}
		}
	}
	for id, addr := range addrs {
		if ctx.Err() != nil {
			return nil // the rest next time
		}
		row, ok := have[id]
		if addr == "" {
			if ok {
				if _, err := api.Exec(ctx, `DELETE FROM reports_address_places WHERE client_id = ?`, id); err != nil {
					return err
				}
			}
			continue
		}
		if ok && row.address == addr && (row.status != "error" || time.Since(row.at) < time.Hour) {
			continue
		}
		p, provider, err := geocodeAddress(ctx, api, addr)
		status, msg := "ok", ""
		switch {
		case errors.Is(err, errNotFound):
			status = "not_found"
		case err != nil:
			status, msg = "error", truncate(err.Error(), 300)
		}
		if _, err := api.Exec(ctx, `INSERT INTO reports_address_places (client_id, address, status, label, region, country, country_code, lat, lng, provider, error, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?,?)
			ON CONFLICT (client_id) DO UPDATE SET address = excluded.address, status = excluded.status, label = excluded.label, region = excluded.region, country = excluded.country,
			country_code = excluded.country_code, lat = excluded.lat, lng = excluded.lng, provider = excluded.provider, error = excluded.error, updated_at = excluded.updated_at`,
			id, addr, status, p.Label, p.Region, p.Country, p.CountryCode, p.Lat, p.Lng, provider, msg, now()); err != nil {
			return err
		}
	}
	return nil
}

type addressRow struct {
	address, status string
	at              time.Time
}

func allAddressRows(ctx context.Context, api *plugin.API) (map[int64]addressRow, error) {
	rows, err := api.Query(ctx, `SELECT client_id, address, status, updated_at FROM reports_address_places`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]addressRow{}
	for rows.Next() {
		var id int64
		var r addressRow
		var at string
		if err := rows.Scan(&id, &r.address, &r.status, &at); err != nil {
			return nil, err
		}
		r.at, _ = time.Parse(time.RFC3339, at)
		out[id] = r
	}
	return out, rows.Err()
}

var errNotFound = errors.New("no place matches that address")

// geocodeAddress is where an address is: Google's answer when there's a
// key, else OpenStreetMap's, trying it without its first line (the street)
// when the whole of it finds nothing, so a client is at least in their
// town.
func geocodeAddress(ctx context.Context, api *plugin.API, addr string) (Place, string, error) {
	if key := googleKey(ctx, api, true); key != "" {
		p, err := googleGeocode(ctx, api, key, addr)
		return p, "Google", err
	}
	found, err := geocode(ctx, api, nil, addr)
	if err == nil && len(found) == 0 {
		if _, rest, ok := strings.Cut(addr, ","); ok && strings.TrimSpace(rest) != "" {
			found, err = geocode(ctx, api, nil, strings.TrimSpace(rest))
		}
	}
	if err != nil {
		return Place{}, "OpenStreetMap", err
	}
	if len(found) == 0 {
		return Place{}, "OpenStreetMap", errNotFound
	}
	return found[0], "OpenStreetMap", nil
}

// googleGeocode asks Google's Geocoding API, its answers cached like
// Nominatim's.
func googleGeocode(ctx context.Context, api *plugin.API, key, addr string) (Place, error) {
	cacheKey := "google:" + strings.ToLower(strings.Join(strings.Fields(addr), " "))
	var raw string
	if err := api.QueryRow(ctx, `SELECT results FROM reports_geocode_cache WHERE query = ?`, cacheKey).Scan(&raw); err == nil {
		var out []Place
		if json.Unmarshal([]byte(raw), &out) == nil {
			if len(out) == 0 {
				return Place{}, errNotFound
			}
			return out[0], nil
		}
	}
	q := url.Values{"address": {addr}, "key": {key}, "language": {"en"}}
	r, err := http.NewRequestWithContext(ctx, "GET", "https://"+googleAPIs+"/maps/api/geocode/json?"+q.Encode(), nil)
	if err != nil {
		return Place{}, err
	}
	res, err := api.HTTP().Do(r)
	if err != nil {
		return Place{}, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return Place{}, err
	}
	out, err := readGoogle(body)
	if err != nil {
		return Place{}, err
	}
	b, _ := json.Marshal(out)
	if _, err := api.Exec(ctx, `INSERT INTO reports_geocode_cache (query, results, fetched_at) VALUES (?, ?, ?) ON CONFLICT (query) DO UPDATE SET results = excluded.results, fetched_at = excluded.fetched_at`, cacheKey, string(b), now()); err != nil {
		return Place{}, err
	}
	if len(out) == 0 {
		return Place{}, errNotFound
	}
	return out[0], nil
}

// readGoogle reads a Geocoding API answer as places; an error in Google's
// words when it refused (a bad key, the API not enabled, over quota).
func readGoogle(body []byte) ([]Place, error) {
	var ans struct {
		Status       string `json:"status"`
		ErrorMessage string `json:"error_message"`
		Results      []struct {
			Formatted  string `json:"formatted_address"`
			Components []struct {
				Long  string   `json:"long_name"`
				Short string   `json:"short_name"`
				Types []string `json:"types"`
			} `json:"address_components"`
			Geometry struct {
				Location struct {
					Lat float64 `json:"lat"`
					Lng float64 `json:"lng"`
				} `json:"location"`
			} `json:"geometry"`
		} `json:"results"`
	}
	if err := json.Unmarshal(body, &ans); err != nil {
		return nil, err
	}
	switch ans.Status {
	case "OK", "ZERO_RESULTS":
	default:
		return nil, fmt.Errorf("Google answered %s: %s", ans.Status, ans.ErrorMessage)
	}
	out := []Place{}
	for _, res := range ans.Results {
		comp := map[string][2]string{}
		for _, c := range res.Components {
			for _, t := range c.Types {
				if _, ok := comp[t]; !ok {
					comp[t] = [2]string{c.Long, c.Short}
				}
			}
		}
		label := firstOf(comp["locality"][0], comp["postal_town"][0], comp["sublocality"][0], comp["administrative_area_level_2"][0], comp["administrative_area_level_1"][0], comp["country"][0])
		region := comp["administrative_area_level_1"][0]
		if region == label {
			region = ""
		}
		out = append(out, Place{Label: label, Region: region, Country: comp["country"][0], CountryCode: strings.ToUpper(comp["country"][1]),
			Lat: round(res.Geometry.Location.Lat), Lng: round(res.Geometry.Location.Lng), Source: "address", Address: res.Formatted})
	}
	return out, nil
}
