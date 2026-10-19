package reports

import (
	"strings"
	"testing"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

func TestZonesPlaceClients(t *testing.T) {
	if p, ok := placeOfZone("Eastern Time (US & Canada)"); !ok || p.City != "New York" || p.Country != "US" {
		t.Fatalf("eastern: %+v", p)
	}
	if p, ok := placeOfZone("Europe/Paris"); !ok || p.City != "Paris" {
		t.Fatalf("an IANA zone: %+v", p)
	}
	if _, ok := placeOfZone("Mars/Olympus"); ok {
		t.Fatal("placed an unknown zone")
	}
	for name, p := range zonePlaces {
		if p.Lat < -90 || p.Lat > 90 || p.Lng < -180 || p.Lng > 180 || len(p.Country) != 2 || p.IANA == "" {
			t.Errorf("%s: %+v", name, p)
		}
		if _, err := time.LoadLocation(p.IANA); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestHealthScore(t *testing.T) {
	now := time.Now()
	recent := now.AddDate(0, 0, -2)
	w := words{work1: "piece of work", workN: "pieces of work", commitment1: "commitment", commitmentN: "commitments"}
	calm := healthOf(plugin.ClientMetrics{ID: 1, Status: "active", LastTouchAt: &recent}, momentum{}, now, w)
	if calm.Score != 100 || calm.Band != "on_track" || len(calm.Reasons) != 0 {
		t.Fatalf("calm: %+v", calm)
	}
	old := now.AddDate(0, 0, -45)
	h := healthOf(plugin.ClientMetrics{ID: 1, Status: "active", LateWork: 5, OpenWork: 6, OverdueOurs: 1, LastTouchAt: &old}, momentum{}, now, w)
	// −30 late (capped), −12 overdue, −15 quiet, −10 nothing done.
	if h.Score != 33 || h.Band != "at_risk" || h.Reasons[0].Points != -30 || h.Reasons[0].Text != "5 pieces of work late" {
		t.Fatalf("troubled: %+v", h)
	}
	if g := healthOf(plugin.ClientMetrics{ID: 1, Status: "former"}, momentum{}, now, w); g.Band != "inactive" {
		t.Fatalf("former: %+v", g)
	}
	if g := healthOf(plugin.ClientMetrics{ID: 1, Status: "active", LastTouchAt: &recent, OpenWork: 1}, momentum{recent: 3, before: 1}, now, w); g.Score != 100 || g.Reasons[0].Points != 5 {
		t.Fatalf("momentum: %+v", g)
	}
}

func TestPerMonth(t *testing.T) {
	if perMonth(1200, "quarterly") != 400 || perMonth(1200, "monthly") != 1200 || perMonth(1200, "weekly") != 5200 {
		t.Fatal("per month")
	}
}

func TestModuleValidates(t *testing.T) {
	// A manifest that doesn't hold up panics the host at start.
	plugin.New(nil, nil, nil, nil, Module())
}

func TestPlacePrecedence(t *testing.T) {
	hand := map[int64]Place{1: {Label: "Lisbon", Source: "pin"}}
	addr := map[int64]addressPlace{1: {Status: "ok", Place: Place{Label: "Porto", Source: "address"}}, 2: {Status: "ok", Place: Place{Label: "Porto", Source: "address"}}, 3: {Status: "not_found"}}
	if p, _ := placeFor(1, "Lisbon", hand, addr); p.Label != "Lisbon" {
		t.Fatalf("by hand loses: %+v", p)
	}
	if p, _ := placeFor(2, "Lisbon", hand, addr); p.Label != "Porto" || p.Approximate {
		t.Fatalf("address loses: %+v", p)
	}
	if p, _ := placeFor(3, "Lisbon", hand, addr); p.Source != "zone" || !p.Approximate {
		t.Fatalf("not found isn't the zone: %+v", p)
	}
}

func TestAddresses(t *testing.T) {
	if got := cleanAddress("100 Middle Street\r\n  Portland,  ME 04101\n"); got != "100 Middle Street, Portland, ME 04101" {
		t.Fatalf("clean: %q", got)
	}
	f := pickAddress([]field{{"website", "Website", "link"}, {"hq", "Office address", "long_text"}})
	if f.Key != "hq" {
		t.Fatalf("picked %+v", f)
	}
	if f := pickAddress([]field{{"office_address", "Where", "text"}, {"addr", "Address", "text"}}); f.Key != "addr" {
		t.Fatalf("an exact label wins: %+v", f)
	}
	body := `{"status":"OK","results":[{"formatted_address":"1600 Broadway, Denver, CO 80202, USA","geometry":{"location":{"lat":39.742,"lng":-104.987}},
		"address_components":[{"long_name":"Denver","short_name":"Denver","types":["locality","political"]},{"long_name":"Colorado","short_name":"CO","types":["administrative_area_level_1"]},{"long_name":"United States","short_name":"US","types":["country"]}]}]}`
	ps, err := readGoogle([]byte(body))
	if err != nil || len(ps) != 1 || ps[0].Label != "Denver" || ps[0].Region != "Colorado" || ps[0].CountryCode != "US" || ps[0].Source != "address" {
		t.Fatalf("google: %+v %v", ps, err)
	}
	if _, err := readGoogle([]byte(`{"status":"REQUEST_DENIED","error_message":"The provided API key is invalid."}`)); err == nil || !strings.Contains(err.Error(), "invalid") {
		t.Fatalf("a refusal: %v", err)
	}
}
