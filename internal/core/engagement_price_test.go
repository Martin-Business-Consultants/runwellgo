package core

import (
	"encoding/json"
	"strings"
	"testing"
)

// An engagement's price is whole cents, apart from any agreement: set when
// it starts or changed later, cleared with null, answered as price_cents
// beside the formatted price, and refused (Rails' words) when it isn't a
// whole, non-negative number of cents.
func TestEngagementPrice(t *testing.T) {
	ti := newTestInstall(t)
	client := ti.client("Harbor")
	create := func(fields map[string]any) (int, map[string]any) {
		fields["client_id"], fields["title"] = client, "Site"
		return ti.call("POST", "/api/engagements", map[string]any{"engagement": fields})
	}

	// Created with a price, and without one.
	_, priced := create(map[string]any{"price_cents": 150000})
	ref := dig(priced, "engagement", "ref").(string)
	if c, p := dig(priced, "engagement", "price_cents"), dig(priced, "engagement", "price"); c != float64(150000) || p != "$1,500.00" {
		t.Fatalf("created at %v (%v), want 150000 ($1,500.00)", c, p)
	}
	_, plain := create(map[string]any{})
	if c, p := dig(plain, "engagement", "price_cents"), dig(plain, "engagement", "price"); c != nil || p != nil {
		t.Fatalf("created with no price at %v (%v), want null", c, p)
	}
	if _, typed := create(map[string]any{"price_cents": "2500"}); dig(typed, "engagement", "price_cents") != float64(2500) {
		t.Fatalf("a price sent as text is %v, want 2500", dig(typed, "engagement", "price_cents"))
	}
	if _, blank := create(map[string]any{"price_cents": ""}); dig(blank, "engagement", "price_cents") != nil {
		t.Fatalf("a blank price is %v, want null", dig(blank, "engagement", "price_cents"))
	}

	// Changed, kept when something else changes, and cleared.
	updated := ti.must(200, "PATCH", "/api/engagements/"+ref, map[string]any{"engagement": map[string]any{"price_cents": 99}})
	if c, p := dig(updated, "engagement", "price_cents"), dig(updated, "engagement", "price"); c != float64(99) || p != "$0.99" {
		t.Fatalf("updated to %v (%v), want 99 ($0.99)", c, p)
	}
	updated = ti.must(200, "PATCH", "/api/engagements/"+ref, map[string]any{"engagement": map[string]any{"title": "Site rebuild"}})
	if c := dig(updated, "engagement", "price_cents"); c != float64(99) {
		t.Fatalf("changing the title made the price %v", c)
	}
	updated = ti.must(200, "PATCH", "/api/engagements/"+ref, map[string]any{"engagement": map[string]any{"price_cents": 0}})
	if c, p := dig(updated, "engagement", "price_cents"), dig(updated, "engagement", "price"); c != float64(0) || p != "$0.00" {
		t.Fatalf("a zero price is %v (%v), want 0 ($0.00), not none", c, p)
	}
	updated = ti.must(200, "PATCH", "/api/engagements/"+ref, map[string]any{"engagement": map[string]any{"price_cents": nil}})
	if c, p := dig(updated, "engagement", "price_cents"), dig(updated, "engagement", "price"); c != nil || p != nil {
		t.Fatalf("cleared to %v (%v), want null", c, p)
	}

	// Refused, on create and update alike, leaving it as it was.
	ti.must(200, "PATCH", "/api/engagements/"+ref, map[string]any{"engagement": map[string]any{"price_cents": 5000}})
	for _, c := range []struct {
		price any
		want  string
	}{
		{-1, "must be greater than or equal to 0"},
		{12.5, "must be an integer"},
		{"12.5", "must be an integer"},
		{"lots", "is not a number"},
		{true, "is not a number"},
		{map[string]any{"cents": 1}, "is not a number"},
	} {
		for _, try := range []func() (int, map[string]any){
			func() (int, map[string]any) { return create(map[string]any{"price_cents": c.price}) },
			func() (int, map[string]any) {
				return ti.call("PATCH", "/api/engagements/"+ref, map[string]any{"engagement": map[string]any{"price_cents": c.price}})
			},
		} {
			status, out := try()
			errs, _ := dig(out, "errors", "price_cents").([]any)
			if status != 422 || len(errs) != 1 || errs[0] != c.want {
				t.Errorf("price_cents %#v: %d %v, want 422 %q", c.price, status, out["errors"], c.want)
			}
		}
	}
	if shown := ti.must(200, "GET", "/api/engagements/"+ref, nil); dig(shown, "engagement", "price_cents") != float64(5000) {
		t.Fatalf("a refused change moved the price to %v", dig(shown, "engagement", "price_cents"))
	}

	// Lists carry it and sort by it (unpriced last).
	list := ti.must(200, "GET", "/api/engagements?state=all&sort=price&direction=desc", nil)
	var got []any
	for _, e := range list["engagements"].([]any) {
		got = append(got, e.(map[string]any)["price_cents"])
	}
	if len(got) < 3 || got[0] != float64(5000) || got[1] != float64(2500) || got[len(got)-1] != nil {
		t.Fatalf("sorted by price, the list is %v", got)
	}
	if client := ti.must(200, "GET", "/api/clients/"+itoa(client), nil); !strings.Contains(stringOf(dig(client, "client", "engagements")), "price_cents") {
		t.Fatal("the client's engagements don't carry their price")
	}

	// Agents (and the CLI, which runs the same tools) are told about it.
	out := ti.must(200, "GET", "/api/agent/tools", nil)
	for _, tl := range out["tools"].([]any) {
		tool := tl.(map[string]any)
		if name := tool["name"]; name == "create_engagement" || name == "update_engagement" {
			if !strings.Contains(stringOf(tool["params"]), "price_cents") {
				t.Errorf("%s doesn't take price_cents: %v", name, tool["params"])
			}
		}
	}
}

// stringOf is v as JSON, to look for a key in.
func stringOf(v any) string { b, _ := json.Marshal(v); return string(b) }
