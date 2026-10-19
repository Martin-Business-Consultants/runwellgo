package reports

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// Place is where a client (or home) is: set by someone (source search,
// pin, agent) or, Approximate, the city of its time zone.
type Place struct {
	Label       string  `json:"label"`
	Region      string  `json:"region,omitempty"`
	Country     string  `json:"country"`
	CountryCode string  `json:"country_code"`
	Lat         float64 `json:"lat"`
	Lng         float64 `json:"lng"`
	Source      string  `json:"source"` // search, pin, agent (by hand); address; zone when approximate
	Approximate bool    `json:"approximate"`
	// Address is the address it was found from (source address).
	Address   string `json:"address,omitempty"`
	SetBy     string `json:"set_by,omitempty"`
	UpdatedAt string `json:"updated_at,omitempty"`
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// places are the places someone set, by client id.
func places(ctx context.Context, api *plugin.API) (map[int64]Place, error) {
	rows, err := api.Query(ctx, `SELECT client_id, label, COALESCE(region, ''), COALESCE(country, ''), COALESCE(country_code, ''), lat, lng, source, COALESCE(set_by, ''), updated_at FROM reports_client_places`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]Place{}
	for rows.Next() {
		var id int64
		var p Place
		if err := rows.Scan(&id, &p.Label, &p.Region, &p.Country, &p.CountryCode, &p.Lat, &p.Lng, &p.Source, &p.SetBy, &p.UpdatedAt); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// zoneFor is a place from a time zone: its city, approximate; ok false when
// the zone is one we can't place.
func zoneFor(zone string) (Place, bool) {
	z, ok := placeOfZone(zone)
	if !ok {
		return Place{}, false
	}
	return Place{Label: z.City, Country: countryName(z.Country), CountryCode: z.Country, Lat: z.Lat, Lng: z.Lng, Source: "zone", Approximate: true}, true
}

// spread nudges approximate pins that share a city apart, each by its own
// small offset (the same every time), so they don't sit on each other.
func spread(p Place, id int64, n int) Place {
	if n < 2 {
		return p
	}
	h := fnv.New32a()
	fmt.Fprint(h, id)
	angle := float64(h.Sum32()%360) * math.Pi / 180
	r := 0.6 + float64(h.Sum32()%7)*0.12 // degrees
	p.Lat += r * math.Sin(angle)
	p.Lng += r * math.Cos(angle) / math.Max(0.3, math.Cos(p.Lat*math.Pi/180))
	return p
}

// placeFor is where a client is: where someone set it by hand, else where
// their address is, else (approximate) their time zone's city; ok false
// when there's none of these.
func placeFor(id int64, zone string, set map[int64]Place, addrs map[int64]addressPlace) (Place, bool) {
	if p, ok := set[id]; ok {
		return p, true
	}
	if a, ok := addrs[id]; ok && a.Status == "ok" {
		return a.Place, true
	}
	return zoneFor(zone)
}

// Home is where the business is: the client marked internal ("This is
// us"), placed like any client, by hand or from its address (From
// "internal"); else the place set in Settings › Reports (an override, for
// an install with no internal client or no address for it: "override");
// else the city of the install's time zone ("zone").
type Home struct {
	Place
	From       string `json:"from"`
	ClientID   int64  `json:"client_id,omitempty"`
	ClientName string `json:"client_name,omitempty"`
}

// homeOf works home out from the clients, the places set and the
// addresses found.
func homeOf(ctx context.Context, api *plugin.API, site plugin.Site, clients []plugin.ClientMetrics, set map[int64]Place, addrs map[int64]addressPlace) Home {
	var us *plugin.ClientMetrics
	for i, c := range clients {
		if c.Internal && c.Status != "former" && (us == nil || c.ID < us.ID) {
			us = &clients[i]
		}
	}
	if us != nil {
		if p, ok := placeFor(us.ID, "", set, addrs); ok {
			return Home{Place: p, From: "internal", ClientID: us.ID, ClientName: us.Name}
		}
	}
	h := Home{From: "zone"}
	if us != nil {
		h.ClientID, h.ClientName = us.ID, us.Name
	}
	if raw := setting(ctx, api, "home"); raw != "" {
		var p Place
		if json.Unmarshal([]byte(raw), &p) == nil && p.Label != "" {
			h.Place, h.From = p, "override"
			return h
		}
	}
	zone := site.TimeZone
	if us != nil && us.TimeZone != "" {
		zone = us.TimeZone
	}
	if p, ok := zoneFor(zone); ok {
		h.Place = p
		return h
	}
	h.Place, _ = zoneFor("Eastern Time (US & Canada)")
	return h
}

// home works home out on its own (Settings, the home routes).
func home(ctx context.Context, api *plugin.API, site plugin.Site) Home {
	m, err := api.Metrics(ctx, plugin.MetricsQuery{Since: time.Now().AddDate(0, 0, -1)})
	if err != nil {
		api.Log("reports: " + err.Error())
		m = &plugin.Metrics{}
	}
	set, err := places(ctx, api)
	if err != nil {
		api.Log("reports: " + err.Error())
	}
	addrs, err := addressPlaces(ctx, api)
	if err != nil {
		api.Log("reports: " + err.Error())
	}
	return homeOf(ctx, api, site, m.Clients, set, addrs)
}

// placeInput is a place as the app or an agent gives it: a query to look
// up, or the place itself.
type placeInput struct {
	Query       string   `json:"query"`
	Label       string   `json:"label"`
	Region      string   `json:"region"`
	Country     string   `json:"country"`
	CountryCode string   `json:"country_code"`
	Lat         *float64 `json:"lat"`
	Lng         *float64 `json:"lng"`
	Source      string   `json:"source"`
}

// resolve is the place in, looked up when it's a query; a response when
// it doesn't hold up.
func resolve(ctx context.Context, api *plugin.API, req *plugin.Request, in placeInput) (Place, *plugin.Response, error) {
	if strings.TrimSpace(in.Query) != "" && in.Lat == nil {
		found, err := geocode(ctx, api, req, in.Query)
		if err != nil {
			return Place{}, plugin.JSON(http.StatusBadGateway, map[string]any{"error": "geocoder", "message": "Couldn't look that place up just now: " + err.Error()}), nil //nolint:nilerr // answered as a 502
		}
		if len(found) == 0 {
			return Place{}, plugin.Invalid(map[string][]string{"query": {"No place matches that. Try a city and its country."}}), nil
		}
		p := found[0]
		p.Source = "search"
		if req.Actor != nil && req.Actor.Agent {
			p.Source = "agent"
		}
		return p, nil, nil
	}
	errs := map[string][]string{}
	if in.Lat == nil || *in.Lat < -90 || *in.Lat > 90 {
		errs["lat"] = []string{"must be between -90 and 90"}
	}
	if in.Lng == nil || *in.Lng < -180 || *in.Lng > 180 {
		errs["lng"] = []string{"must be between -180 and 180"}
	}
	if len(errs) > 0 {
		return Place{}, plugin.Invalid(errs), nil
	}
	in.Label = strings.TrimSpace(in.Label)
	if in.Label == "" {
		// A dropped pin: name the place it's in, when the geocoder can.
		if p, err := reverse(ctx, api, req, *in.Lat, *in.Lng); err == nil && p.Label != "" {
			in.Label, in.Region, in.Country, in.CountryCode = p.Label, p.Region, p.Country, p.CountryCode
		} else {
			in.Label = fmt.Sprintf("%.2f, %.2f", *in.Lat, *in.Lng)
		}
	}
	code := strings.ToUpper(strings.TrimSpace(in.CountryCode))
	country := strings.TrimSpace(in.Country)
	if country == "" && code != "" {
		country = countryName(code)
	}
	src := in.Source
	switch {
	case req.Actor != nil && req.Actor.Agent:
		src = "agent"
	case src != "search":
		src = "pin"
	}
	return Place{Label: truncate(in.Label, 120), Region: truncate(strings.TrimSpace(in.Region), 120), Country: truncate(country, 80), CountryCode: truncate(code, 3),
		Lat: round(*in.Lat), Lng: round(*in.Lng), Source: src}, nil, nil
}

func round(f float64) float64 { return math.Round(f*1e5) / 1e5 }

func truncate(s string, n int) string {
	if r := []rune(s); len(r) > n {
		return string(r[:n])
	}
	return s
}

func pathID(req *plugin.Request) (int64, error) {
	id, err := strconv.ParseInt(req.Params["id"], 10, 64)
	if err != nil || id <= 0 {
		return 0, plugin.ErrNoRecord
	}
	return id, nil
}

func decodeBody(req *plugin.Request, wrap string, into any) *plugin.Response {
	if len(req.Body) == 0 {
		return plugin.Invalid(map[string][]string{wrap: {"is missing"}})
	}
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(req.Body, &outer); err != nil {
		return plugin.JSON(http.StatusBadRequest, map[string]any{"error": "bad_request", "message": "The body isn't JSON."})
	}
	raw, ok := outer[wrap]
	if !ok {
		raw = req.Body
	}
	if err := json.Unmarshal(raw, into); err != nil {
		return plugin.JSON(http.StatusBadRequest, map[string]any{"error": "bad_request", "message": "The " + wrap + " isn't what this takes."})
	}
	return nil
}

// setPlace puts a client on the map.
func setPlace(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	id, err := pathID(req)
	if err != nil {
		return plugin.Answer(err), nil
	}
	client, err := api.Record(ctx, fmt.Sprintf("Client:%d", id))
	if err != nil {
		if r := plugin.Answer(err); r != nil {
			return r, nil
		}
		return nil, err
	}
	var in placeInput
	if r := decodeBody(req, "place", &in); r != nil {
		return r, nil
	}
	p, bad, err := resolve(ctx, api, req, in)
	if bad != nil || err != nil {
		return bad, err
	}
	who := ""
	var whoID any
	if req.Actor != nil {
		who, whoID = req.Actor.Name, req.Actor.ID
	}
	p.SetBy, p.UpdatedAt = who, now()
	if _, err := api.Exec(ctx, `INSERT INTO reports_client_places (client_id, label, region, country, country_code, lat, lng, source, set_by_id, set_by, updated_at) VALUES (?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT (client_id) DO UPDATE SET label = excluded.label, region = excluded.region, country = excluded.country, country_code = excluded.country_code,
		lat = excluded.lat, lng = excluded.lng, source = excluded.source, set_by_id = excluded.set_by_id, set_by = excluded.set_by, updated_at = excluded.updated_at`,
		id, p.Label, p.Region, p.Country, p.CountryCode, p.Lat, p.Lng, p.Source, whoID, who, p.UpdatedAt); err != nil {
		return nil, err
	}
	name, _ := client["name"].(string)
	where := p.Label
	if p.Country != "" && p.Country != p.Label {
		where += ", " + p.Country
	}
	return plugin.JSON(200, map[string]any{"summary": fmt.Sprintf("%s is on the map in %s.", name, where), "client_id": id, "place": p}), nil
}

// clearPlace forgets the place set by hand: back to their address, or
// their time zone's city.
func clearPlace(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	id, err := pathID(req)
	if err != nil {
		return plugin.Answer(err), nil
	}
	if _, err := api.Exec(ctx, `DELETE FROM reports_client_places WHERE client_id = ?`, id); err != nil {
		return nil, err
	}
	addrs, err := addressPlaces(ctx, api)
	if err != nil {
		return nil, err
	}
	if a, ok := addrs[id]; ok && a.Status == "ok" {
		return plugin.JSON(200, map[string]any{"summary": "Placed by their address again: " + a.Place.Label + ".", "client_id": id, "place": a.Place}), nil
	}
	return plugin.JSON(200, map[string]any{"summary": "Placed by their time zone again, approximately.", "client_id": id}), nil
}

// site0 is the install, logged when it can't be read.
func site0(ctx context.Context, api *plugin.API) plugin.Site {
	s, err := api.Site(ctx)
	if err != nil {
		api.Log("reports: " + err.Error())
	}
	return s
}

// setHome sets the override for home (people who manage settings), used
// while there's no internal client with an address.
func setHome(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	var in placeInput
	if r := decodeBody(req, "place", &in); r != nil {
		return r, nil
	}
	p, bad, err := resolve(ctx, api, req, in)
	if bad != nil || err != nil {
		return bad, err
	}
	if req.Actor != nil {
		p.SetBy = req.Actor.Name
	}
	p.UpdatedAt = now()
	raw, _ := json.Marshal(p)
	if _, err := api.Exec(ctx, `INSERT INTO reports_settings (key, value, updated_at) VALUES ('home', ?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`, string(raw), p.UpdatedAt); err != nil {
		return nil, err
	}
	return plugin.JSON(200, map[string]any{"summary": "Home is " + p.Label + ", unless the client marked internal has an address.", "home": home(ctx, api, site0(ctx, api))}), nil
}

func clearHome(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if _, err := api.Exec(ctx, `DELETE FROM reports_settings WHERE key = 'home'`); err != nil {
		return nil, err
	}
	site, err := api.Site(ctx)
	if err != nil {
		return nil, err
	}
	h := home(ctx, api, site)
	return plugin.JSON(200, map[string]any{"summary": "Home is " + h.Label + " again.", "home": h}), nil
}

// searchPlaces looks a place up (?q=), for the place picker.
func searchPlaces(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	q := strings.TrimSpace(req.Query.Get("q"))
	if len([]rune(q)) < 2 {
		return plugin.JSON(200, map[string]any{"places": []Place{}}), nil
	}
	found, err := geocode(ctx, api, req, q)
	if err != nil {
		return plugin.JSON(http.StatusBadGateway, map[string]any{"error": "geocoder", "message": "Couldn't look places up just now. Try again in a moment, or drop the pin on the globe."}), nil //nolint:nilerr // answered as a 502
	}
	return plugin.JSON(200, map[string]any{"places": found}), nil
}

// The geocoder: Nominatim's usage policy asks for one request a second at
// most, a User-Agent that says who's asking, and results kept rather than
// asked for again.
var (
	geoMu   sync.Mutex
	geoLast time.Time
)

const geoCacheDays = 90

func geocode(ctx context.Context, api *plugin.API, req *plugin.Request, query string) ([]Place, error) {
	key := strings.ToLower(strings.Join(strings.Fields(query), " "))
	var raw, fetched string
	err := api.QueryRow(ctx, `SELECT results, fetched_at FROM reports_geocode_cache WHERE query = ?`, key).Scan(&raw, &fetched)
	if err == nil {
		if t, perr := time.Parse(time.RFC3339, fetched); perr == nil && time.Since(t) < geoCacheDays*24*time.Hour {
			var out []Place
			if json.Unmarshal([]byte(raw), &out) == nil {
				return out, nil
			}
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	out, err := nominatimSearch(ctx, api, req, "/search", url.Values{"q": {query}, "limit": {"6"}})
	if err != nil {
		return nil, err
	}
	b, _ := json.Marshal(out)
	if _, err := api.Exec(ctx, `INSERT INTO reports_geocode_cache (query, results, fetched_at) VALUES (?, ?, ?) ON CONFLICT (query) DO UPDATE SET results = excluded.results, fetched_at = excluded.fetched_at`, key, string(b), now()); err != nil {
		return nil, err
	}
	return out, nil
}

func firstOf(s ...string) string {
	for _, v := range s {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// reverse is the place a point is in (a dropped pin's city), cached like a
// search.
func reverse(ctx context.Context, api *plugin.API, req *plugin.Request, lat, lng float64) (Place, error) {
	key := fmt.Sprintf("@%.2f,%.2f", lat, lng)
	var raw string
	if err := api.QueryRow(ctx, `SELECT results FROM reports_geocode_cache WHERE query = ?`, key).Scan(&raw); err == nil {
		var out []Place
		if json.Unmarshal([]byte(raw), &out) == nil && len(out) > 0 {
			return out[0], nil
		}
	}
	out, err := nominatimSearch(ctx, api, req, "/reverse", url.Values{"lat": {fmt.Sprintf("%.5f", lat)}, "lon": {fmt.Sprintf("%.5f", lng)}, "zoom": {"10"}})
	if err != nil {
		return Place{}, err
	}
	if len(out) == 0 {
		return Place{}, errors.New("nowhere named there")
	}
	b, _ := json.Marshal(out)
	if _, err := api.Exec(ctx, `INSERT INTO reports_geocode_cache (query, results, fetched_at) VALUES (?, ?, ?) ON CONFLICT (query) DO UPDATE SET results = excluded.results, fetched_at = excluded.fetched_at`, key, string(b), now()); err != nil {
		return Place{}, err
	}
	return out[0], nil
}

// nominatimSearch asks Nominatim (path /search or /reverse), a request a
// second at most, and reads its answer as places.
func nominatimSearch(ctx context.Context, api *plugin.API, req *plugin.Request, path string, q url.Values) ([]Place, error) {
	geoMu.Lock()
	wait := time.Until(geoLast.Add(time.Second))
	if wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			geoMu.Unlock()
			return nil, ctx.Err()
		}
	}
	geoLast = time.Now()
	geoMu.Unlock()

	q.Set("format", "jsonv2")
	q.Set("addressdetails", "1")
	q.Set("accept-language", "en")
	site, _ := api.Site(ctx)
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+nominatim+path+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	agent := "Runwell-Reports/1.0"
	if site.URL != "" {
		agent += " (+" + site.URL + ")"
	}
	if site.ContactEmail != "" {
		agent += " " + site.ContactEmail
	}
	r.Header.Set("User-Agent", agent)
	res, err := api.HTTP().Do(r)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the geocoder answered %d", res.StatusCode)
	}
	type hit struct {
		Lat     string            `json:"lat"`
		Lon     string            `json:"lon"`
		Name    string            `json:"name"`
		Address map[string]string `json:"address"`
	}
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	var hits []hit
	if path == "/reverse" {
		var one hit
		if err := json.Unmarshal(body, &one); err != nil {
			return nil, err
		}
		if one.Lat != "" {
			hits = []hit{one}
		}
	} else if err := json.Unmarshal(body, &hits); err != nil {
		return nil, err
	}
	out := []Place{}
	for _, h := range hits {
		lat, err1 := strconv.ParseFloat(h.Lat, 64)
		lng, err2 := strconv.ParseFloat(h.Lon, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		a := h.Address
		label := firstOf(a["city"], a["town"], a["village"], a["municipality"], a["hamlet"], a["suburb"], h.Name, a["county"], a["state"], a["country"])
		region := firstOf(a["state"], a["region"], a["county"])
		if region == label {
			region = ""
		}
		code := strings.ToUpper(a["country_code"])
		out = append(out, Place{Label: label, Region: region, Country: firstOf(a["country"], countryName(code)), CountryCode: code, Lat: round(lat), Lng: round(lng), Source: "search"})
	}
	return out, nil
}
