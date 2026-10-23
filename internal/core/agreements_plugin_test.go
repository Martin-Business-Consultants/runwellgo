package core

import (
	"fmt"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/plugins/agreements"
)

// Agreements are a default plugin: on from the first run; switched off,
// their routes and tools are gone, engagements are underway from the start
// and nothing else breaks; the agreements already made stay in the
// database and come back when it's on again. Removed, it stays off.
func TestAgreementsPluginSwitch(t *testing.T) {
	ti := newTestInstall(t)
	a, ctx := ti.a, t.Context()

	// Not installed yet (a first run before defaults): on.
	if !a.agreementsOn(ctx) {
		t.Fatal("agreements are off before the default plugins were installed")
	}
	a.plugins.EnsureDefaults(ctx)
	if p := a.plugins.Get(agreements.Key); p == nil || !p.Enabled {
		t.Fatalf("the agreements plugin isn't installed and on by default: %+v", p)
	}

	// On: a new engagement starts as a draft agreement.
	client := ti.client("Harbor")
	eng := ti.must(201, "POST", "/api/engagements", map[string]any{"engagement": map[string]any{"client_id": client, "title": "Site"}})
	ref := dig(eng, "engagement", "ref").(string)
	if st := dig(eng, "engagement", "state"); st != "draft" {
		t.Fatalf("a new engagement is %v, want draft", st)
	}
	versions := dig(eng, "engagement", "agreement_versions").([]any)
	if len(versions) != 1 || dig(eng, "engagement", "agreements") != true {
		t.Fatalf("a new engagement has %d versions (agreements %v)", len(versions), dig(eng, "engagement", "agreements"))
	}
	draftID := int64(versions[0].(map[string]any)["id"].(float64))
	ti.must(201, "POST", fmt.Sprintf("/api/agreement_versions/%d/scope_items", draftID), map[string]any{"scope_item": map[string]any{"description": "Homepage", "price_cents": 50000}})
	tools := ti.toolNames()
	for _, name := range []string{"create_agreement_version", "send_agreement", "add_scope_item", "record_decision", "promote_request_to_change"} {
		if !tools[name] {
			t.Errorf("with agreements on, %s isn't offered", name)
		}
	}
	if me := ti.me(); me.Get("agreements") != true {
		t.Fatalf("the session says agreements are %v", me.Get("agreements"))
	}

	// Off.
	off := false
	if _, err := a.plugins.Configure(ctx, nil, agreements.Key, nil, &off); err != nil {
		t.Fatal(err)
	}
	if a.agreementsOn(ctx) {
		t.Fatal("agreements are on after switching the plugin off")
	}
	shown := ti.must(200, "GET", "/api/engagements/"+ref, nil)
	if st, label := dig(shown, "engagement", "state"), dig(shown, "engagement", "state_label"); st != "approved" || label != "Active" {
		t.Fatalf("with agreements off the engagement is %v (%v), want approved (Active)", st, label)
	}
	if n := len(dig(shown, "engagement", "agreement_versions").([]any)); n != 0 || dig(shown, "engagement", "agreements") != false {
		t.Fatalf("with agreements off the engagement shows %d versions (agreements %v)", n, dig(shown, "engagement", "agreements"))
	}
	for _, r := range []struct{ method, path string }{
		{"POST", "/api/engagements/" + ref + "/agreement_versions"},
		{"PATCH", fmt.Sprintf("/api/agreement_versions/%d", draftID)},
		{"POST", fmt.Sprintf("/api/agreement_versions/%d/send_out", draftID)},
		{"POST", fmt.Sprintf("/api/agreement_versions/%d/scope_items", draftID)},
		{"GET", "/api/approve/no-such-token"},
	} {
		if status, out := ti.call(r.method, r.path, map[string]any{}); status != 404 {
			t.Errorf("with agreements off, %s %s answers %d: %v", r.method, r.path, status, out)
		}
	}
	tools = ti.toolNames()
	for _, name := range []string{"create_agreement_version", "send_agreement", "add_scope_item", "record_decision", "email_approval_link", "promote_request_to_change"} {
		if tools[name] {
			t.Errorf("with agreements off, %s is still offered", name)
		}
	}
	for _, name := range []string{"create_engagement", "show_engagement", "list_engagements", "create_work", "briefing"} {
		if !tools[name] {
			t.Errorf("with agreements off, %s isn't offered", name)
		}
	}
	if me := ti.me(); me.Get("agreements") != false {
		t.Fatalf("the session says agreements are %v", me.Get("agreements"))
	}

	// Engagements carry on: a new one is underway with no draft, work goes
	// on it, and the list, home and briefing answer.
	fresh := ti.must(201, "POST", "/api/engagements", map[string]any{"engagement": map[string]any{"client_id": client, "title": "Retainer"}})
	freshRef := dig(fresh, "engagement", "ref").(string)
	if st := dig(fresh, "engagement", "state"); st != "approved" {
		t.Fatalf("a new engagement with agreements off is %v, want approved", st)
	}
	var drafts int
	if err := a.store.QueryRowContext(ctx, `SELECT count(*) FROM agreement_versions v JOIN engagements e ON e.id = v.engagement_id WHERE e.ref = ?`, freshRef).Scan(&drafts); err != nil || drafts != 0 {
		t.Fatalf("a new engagement with agreements off got %d versions (%v)", drafts, err)
	}
	ti.must(201, "POST", "/api/engagements/"+freshRef+"/work", map[string]any{"todo": map[string]any{"title": "Kickoff"}})
	list := ti.must(200, "GET", "/api/engagements?state=approved", nil)
	if n := len(list["engagements"].([]any)); n != 2 {
		t.Fatalf("the Active list shows %d engagements, want both", n)
	}
	for _, f := range list["filters"].([]any) {
		if v := f.(map[string]any)["value"]; v == "draft" || v == "sent" || v == "changes_requested" {
			t.Errorf("with agreements off the list offers the %v filter", v)
		}
	}
	briefing := ti.must(200, "GET", "/api/briefing", nil)
	for _, s := range briefing["sections"].([]any) {
		if k := s.(map[string]any)["key"]; k == "awaiting_client" || k == "drafts" {
			t.Errorf("with agreements off the briefing has %v", k)
		}
	}
	ti.must(200, "GET", "/api/briefing?home=1", nil)
	// The portal shows the client no agreement, and nothing awaiting them.
	portal, err := a.portalEngagements(ctx, client, "")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range portal {
		if e.state() != "approved" || e.pending() != nil || len(e.Versions) != 0 {
			t.Errorf("with agreements off the portal shows %s as %s with %d versions", e.Ref, e.state(), len(e.Versions))
		}
	}
	ti.must(200, "GET", "/api/search?q=Site", nil)

	// What was agreed stays in the database, untouched.
	var rows, items int
	if err := a.store.QueryRowContext(ctx, `SELECT count(*), (SELECT count(*) FROM scope_items WHERE agreement_version_id = ?) FROM agreement_versions WHERE id = ?`, draftID, draftID).Scan(&rows, &items); err != nil || rows != 1 || items != 1 {
		t.Fatalf("with agreements off the draft is %d rows with %d items (%v)", rows, items, err)
	}

	// On again: it's back as it was.
	on := true
	if _, err := a.plugins.Configure(ctx, nil, agreements.Key, nil, &on); err != nil {
		t.Fatal(err)
	}
	shown = ti.must(200, "GET", "/api/engagements/"+ref, nil)
	if st := dig(shown, "engagement", "state"); st != "draft" || len(dig(shown, "engagement", "agreement_versions").([]any)) != 1 {
		t.Fatalf("with agreements back on the engagement is %v with %v", st, dig(shown, "engagement", "agreement_versions"))
	}
	if !ti.toolNames()["send_agreement"] {
		t.Fatal("with agreements back on, send_agreement isn't offered")
	}

	// Removed, it stays off, and the defaults don't put it back.
	if err := a.plugins.Remove(ctx, nil, agreements.Key, true); err != nil {
		t.Fatal(err)
	}
	a.plugins.EnsureDefaults(ctx)
	if a.plugins.Get(agreements.Key) != nil || a.agreementsOn(ctx) {
		t.Fatal("a removed agreements plugin came back on")
	}
	if status, _ := ti.call("POST", "/api/engagements/"+ref+"/agreement_versions", map[string]any{}); status != 404 {
		t.Fatalf("with the plugin removed, starting a version answers %d", status)
	}
	if err := a.store.QueryRowContext(ctx, `SELECT count(*) FROM agreement_versions WHERE id = ?`, draftID).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("removing the plugin (purging its data) took the agreements with it: %d (%v)", rows, err)
	}
}
