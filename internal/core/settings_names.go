package core

import (
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/apierr"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Settings › Names (what things are called, prices, starting setups),
// Views (how lists open) and Address (where people reach Runwell, and its
// time zone): the install's own words and whereabouts, for owners.

func init() { routeSets = append(routeSets, (*App).settingsNameRoutes) }

// IndexViews are how lists may open.
var IndexViews = []string{"cards", "table"}

func (a *App) settingsNameRoutes(mux *http.ServeMux) {
	ms := need("manage_settings")
	a.handle(mux, "GET /api/settings/names", a.showNames, ms, tool("show_names", "Show what this install calls things"))
	a.handle(mux, "PATCH /api/settings/names", a.updateNames, ms, tool("update_names", "Rename things",
		note("terminology: { client: { one:, other: }, engagement: {…}, scope_item: {…}, work: {…}, labels: { project: { one:, other:, prefix:, enabled: }, work_order: {…}, service: {…} } }. Send the whole set: anything left out goes back to its default."),
		bodyParam("terminology", "object", "The words, as above.", true)))
	a.handle(mux, "PATCH /api/settings/start", a.updateStart, ms, tool("apply_starting_setup", "Use a starting setup's names",
		note("Replaces every name in Settings > Names (and the prices switch) with a starting setup's: business, team or personal."),
		bodyParam("start", "string", "business, team or personal.", true)))
	a.handle(mux, "PATCH /api/settings/prices", a.updatePrices, ms, tool("update_prices", "Turn prices on agreements on or off",
		note("prices: false hides money everywhere (prices, totals, agreed amounts, price fields), for planning without it; true shows it wherever an agreement has some."),
		body("setting", `{prices: boolean (required)}`, true)))
	a.handle(mux, "GET /api/settings/views", a.showViews, ms, tool("show_views", "Show how lists open"))
	a.handle(mux, "PATCH /api/settings/views", a.updateViews, ms, tool("update_views", "Set how lists open",
		bodyParam("index_view", "string", "cards or table.", true)))
	a.handle(mux, "GET /api/settings/address", a.showAddress, ms, tool("show_address", "Show the address links in Runwell's email point to"))
	a.handle(mux, "PATCH /api/settings/address", a.updateAddress, ms, tool("update_address", "Set the address people reach Runwell at, or its time zone",
		note("app_url: the install's address, like https://runwell.example.com (a bare host means https). Every link in email (approvals, portal sign-ins, invitations, notifications) points there. time_zone: the install's zone, as a Rails name (\"Pacific Time (US & Canada)\") or IANA (\"America/Los_Angeles\")."),
		body("setting", `{app_url, time_zone}`, true)))
}

// ---- names -------------------------------------------------------------------

func (a *App) namesJSON(s *Settings) Obj {
	terms := Obj{}
	for _, k := range TermKeys {
		terms = terms.Set(k, O("one", s.Term(k, 1), "other", s.Term(k, 2)))
	}
	labels := Obj{}
	for _, l := range Labels {
		labels = labels.Set(l, O("one", s.LabelName(l, 1), "other", s.LabelName(l, 2), "prefix", s.LabelPrefix(l), "enabled", s.LabelEnabled(l)))
	}
	defaults := Obj{}
	for _, k := range TermKeys {
		d := defaultTerms[k]
		defaults = defaults.Set(k, O("one", d[0], "other", d[1]))
	}
	labelDefaults := Obj{}
	for _, l := range Labels {
		d := defaultLabels[l]
		labelDefaults = labelDefaults.Set(l, O("one", d[0], "other", d[1], "prefix", d[2]))
	}
	starts := []Obj{}
	for _, st := range Starts {
		starts = append(starts, O("key", st.Key, "name", st.Name, "description", st.Description))
	}
	return O("summary", "What this install calls things", "terms", terms, "labels", labels,
		"terminology", s.Terminology, "defaults", O("terms", defaults, "labels", labelDefaults), "prices", s.Prices, "starts", starts)
}

func (a *App) showNames(c *Ctx) error {
	s, err := a.settings(c.R.Context())
	if err != nil {
		return err
	}
	c.Summary = "What this install calls things"
	return c.JSON(200, a.namesJSON(s))
}

var nonPrefix = regexp.MustCompile(`[^A-Z0-9]`)

// terminologyFrom keeps only what differs from a default, so a blank field
// means "use the default".
func terminologyFrom(input map[string]any) map[string]any {
	out := map[string]any{}
	pick := func(m map[string]any, keys ...string) map[string]any {
		e := map[string]any{}
		for _, k := range keys {
			if v := strings.TrimSpace(str(m[k])); v != "" {
				e[k] = v
			}
		}
		return e
	}
	for _, k := range TermKeys {
		m, _ := input[k].(map[string]any)
		if e := pick(m, "one", "other"); len(e) > 0 {
			out[k] = e
		}
	}
	labels := map[string]any{}
	in, _ := input["labels"].(map[string]any)
	for _, l := range Labels {
		m, _ := in[l].(map[string]any)
		e := pick(m, "one", "other", "prefix")
		if p, ok := e["prefix"]; ok {
			if p = nonPrefix.ReplaceAllString(strings.ToUpper(p.(string)), ""); p != "" {
				e["prefix"] = p
			} else {
				delete(e, "prefix")
			}
		}
		if v, ok := m["enabled"]; ok && (v == false || v == "0" || v == "false") {
			e["enabled"] = false
		}
		if len(e) > 0 {
			labels[l] = e
		}
	}
	if len(labels) > 0 {
		out["labels"] = labels
	}
	return out
}

func (a *App) updateNames(c *Ctx) error {
	var body struct {
		Terminology map[string]any `json:"terminology"`
	}
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	terms := terminologyFrom(body.Terminology)
	if err := a.write(ctx, c.Actor, func(t *Tx) error {
		return t.update("settings", s.ID, map[string]any{"terminology": string(store.MustJSON(terms))})
	}); err != nil {
		return err
	}
	if s, err = a.settings(ctx); err != nil {
		return err
	}
	c.Summary = "Names saved."
	return c.JSON(200, a.namesJSON(s).Set("summary", c.Summary))
}

func (a *App) updateStart(c *Ctx) error {
	var body struct {
		Start string `json:"start"`
	}
	if err := c.OptionalBody(&body); err != nil {
		return err
	}
	i := -1
	for j, st := range Starts {
		if st.Key == body.Start {
			i = j
		}
	}
	if i < 0 {
		return &apierr.Error{Status: 422, Code: "invalid", Message: "Pick business, team or personal."}
	}
	ctx := c.R.Context()
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.applyStart(body.Start) }); err != nil {
		return err
	}
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	c.Summary = "Names now start from " + strings.ToLower(Starts[i].Name) + ": " + s.Term("client", 2) + ", " + s.Term("engagement", 2) + ", " + s.Term("work", 2) + "."
	return c.JSON(200, a.namesJSON(s).Set("summary", c.Summary))
}

func (a *App) updatePrices(c *Ctx) error {
	body, err := c.Wrapped("setting")
	if err != nil {
		return err
	}
	on := boolOf(body["prices"])
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("settings", s.ID, map[string]any{"prices": on}) }); err != nil {
		return err
	}
	c.Summary = "Agreements no longer show prices."
	if on {
		c.Summary = "Agreements show prices where they have them."
	}
	return c.JSON(200, O("summary", c.Summary, "prices", on))
}

// ---- views -------------------------------------------------------------------

func (a *App) showViews(c *Ctx) error {
	s, err := a.settings(c.R.Context())
	if err != nil {
		return err
	}
	c.Summary = "Lists open as " + s.IndexView
	return c.JSON(200, O("summary", c.Summary, "index_view", s.IndexView, "choices", IndexViews))
}

func (a *App) updateViews(c *Ctx) error {
	var body struct {
		IndexView string `json:"index_view"`
	}
	if err := c.Body(&body); err != nil {
		return err
	}
	if !slices.Contains(IndexViews, body.IndexView) {
		bad := apierr.Invalid{}
		bad.Add("index_view", "is not included in the list")
		return fullInvalid(bad)
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("settings", s.ID, map[string]any{"index_view": body.IndexView}) }); err != nil {
		return err
	}
	c.Summary = "Lists now open as " + body.IndexView + "."
	return c.JSON(200, O("summary", c.Summary, "index_view", body.IndexView, "choices", IndexViews))
}

// ---- address -----------------------------------------------------------------

var hostRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)*(:\d+)?$`)
var schemePrefix = regexp.MustCompile(`^[a-z]+://`)
var pathPart = regexp.MustCompile(`[/?#].*$`)

func (a *App) addressJSON(c *Ctx, s *Settings) Obj {
	set := a.cfg.BaseURL != "" || s.AppHost != ""
	url := a.BaseURL()
	summary := "Links in email point to " + url
	if !set {
		summary += " (not set: guessed)"
	}
	var seen string
	_ = a.store.QueryRowContext(c.R.Context(), `SELECT COALESCE(seen_host, '') FROM settings WHERE id = ?`, s.ID).Scan(&seen)
	proto := "http"
	if strings.HasPrefix(url, "https://") {
		proto = "https"
	}
	zoneFrom := "default"
	if os.Getenv("TIME_ZONE") != "" {
		zoneFrom = "TIME_ZONE"
	}
	return O("summary", summary, "url", url, "host", a.Host(), "protocol", proto, "set", set, "saved_host", nullable(s.AppHost),
		"app_host_env", nullable(os.Getenv("APP_HOST")), "app_url_env", nullable(os.Getenv("APP_URL")), "last_seen_host", nullable(seen),
		"time_zone", a.zoneName(c.R.Context()), "time_zone_saved", nullable(s.TimeZone), "time_zone_default_from", zoneFrom)
}

func (a *App) showAddress(c *Ctx) error {
	s, err := a.settings(c.R.Context())
	if err != nil {
		return err
	}
	out := a.addressJSON(c, s)
	c.Summary = str(out.Get("summary"))
	return c.JSON(200, out)
}

func (a *App) updateAddress(c *Ctx) error {
	body, err := c.Wrapped("setting")
	if err != nil {
		return err
	}
	ctx := c.R.Context()
	s, err := a.settings(ctx)
	if err != nil {
		return err
	}
	set := map[string]any{}
	bad := apierr.Invalid{}
	if has(body, "app_url") {
		raw := strings.TrimSpace(str(body["app_url"]))
		proto := "https"
		if m := regexp.MustCompile(`(?i)^(https?)://`).FindStringSubmatch(raw); m != nil {
			proto = strings.ToLower(m[1])
		}
		host := pathPart.ReplaceAllString(schemePrefix.ReplaceAllString(strings.ToLower(raw), ""), "")
		switch {
		case host == "":
			set["app_host"], set["app_protocol"] = nil, proto
		case !hostRE.MatchString(host):
			bad.Add("app_host", "should be a domain, like runwell.example.com")
		default:
			set["app_host"], set["app_protocol"] = host, proto
		}
	}
	if has(body, "time_zone") {
		zone, ok := normalizeZone(strings.TrimSpace(str(body["time_zone"])))
		if !ok {
			bad.Add("time_zone", "isn’t a time zone we know")
		} else {
			set["time_zone"] = nullable(zone)
		}
	}
	if len(bad) > 0 {
		if msgs, ok := bad["app_host"]; ok {
			delete(bad, "app_host")
			bad["base"] = append(bad["base"], "Address "+msgs[0])
		}
		return fullInvalid(bad)
	}
	if err := a.write(ctx, c.Actor, func(t *Tx) error { return t.update("settings", s.ID, set) }); err != nil {
		return err
	}
	if s, err = a.settings(ctx); err != nil {
		return err
	}
	c.Summary = "Links in email now point to " + a.BaseURL() + "."
	if has(body, "time_zone") {
		c.Summary = "Times now show in " + a.zoneName(ctx) + "."
	}
	return c.JSON(200, a.addressJSON(c, s).Set("summary", c.Summary))
}
