package ai

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
	"github.com/Martin-Business-Consultants/runwellgo/internal/store"
)

// Settings › AI, one row (AiSetting): the provider, its key (sealed with
// the install's key, never sent back), an address for the owner's own
// server, the models (picked from the provider's own list, fetched with the
// key), a monthly budget in cents, whether clients may ask, and the
// switch. Nothing is sent anywhere until it's on.

type setting struct {
	ID          int64
	Enabled     bool
	Provider    string
	APIKey      string // sealed
	APIBase     string
	Model       string
	FastModel   string
	BudgetCents *int64
	Portal      bool
	LastTest    map[string]any
	ModelList   map[string]any
}

const settingColumns = `id, enabled, COALESCE(provider, ''), COALESCE(api_key, ''), COALESCE(api_base, ''), COALESCE(model, ''), COALESCE(fast_model, ''), monthly_budget_cents, portal, last_test, model_list`

// loadSetting is the row, made when there's none.
func loadSetting(ctx context.Context, q plugin.Querier) (*setting, error) {
	s := &setting{}
	var budget sql.NullInt64
	var test, list sql.NullString
	err := q.QueryRow(ctx, `SELECT `+settingColumns+` FROM ai_settings ORDER BY id LIMIT 1`).
		Scan(&s.ID, &s.Enabled, &s.Provider, &s.APIKey, &s.APIBase, &s.Model, &s.FastModel, &budget, &s.Portal, &test, &list)
	if errors.Is(err, sql.ErrNoRows) {
		now := store.Now().DB()
		if _, err := q.Exec(ctx, `INSERT INTO ai_settings (enabled, portal, created_at, updated_at) VALUES (0, 0, ?, ?)`, now, now); err != nil {
			return nil, err
		}
		return loadSetting(ctx, q)
	}
	if err != nil {
		return nil, err
	}
	if budget.Valid {
		s.BudgetCents = &budget.Int64
	}
	_ = json.Unmarshal([]byte(test.String), &s.LastTest)
	_ = json.Unmarshal([]byte(list.String), &s.ModelList)
	return s, nil
}

func (s *setting) info() *providerInfo { return providerFor(s.Provider) }

// ready is switched on, with what it needs to reach the provider.
func (s *setting) ready() bool {
	p := s.info()
	return s.Enabled && p != nil && (s.APIKey != "" || !p.NeedsKey)
}

func (s *setting) providerName() string {
	if p := s.info(); p != nil {
		return p.Name
	}
	return ""
}

func (s *setting) shortName() string {
	if p := s.info(); p != nil {
		return p.shortName()
	}
	return ""
}

// modelFor is the model as the provider takes it: the one picked, else the
// recommended one (the fast one falls back to the main one picked).
func (s *setting) modelFor(fast bool) string {
	m := ""
	if fast {
		m = s.FastModel
	}
	if m == "" {
		m = s.Model
	}
	if m == "" {
		if p := s.info(); p != nil {
			m = p.Model
			if fast {
				m = p.Fast
			}
		}
	}
	return strings.TrimPrefix(m, "opencode/")
}

// base is the provider's address: the owner's own when set.
func (s *setting) base() string {
	if b := strings.TrimRight(strings.TrimSpace(s.APIBase), "/"); b != "" {
		return b
	}
	if p := s.info(); p != nil {
		return p.Base
	}
	return ""
}

// llm is the model reached the way it's served, with the key opened.
func (s *setting) llm(api *plugin.API, fast bool) (*llm, error) {
	return s.llmFor(api, s.modelFor(fast))
}

// llmFor is model (one of the provider's) reached the way it's served.
func (s *setting) llmFor(api *plugin.API, model string) (*llm, error) {
	if s.info() == nil {
		return nil, notNow("AI isn't set up: Settings > Plugins > AI.")
	}
	key, err := api.Unseal(s.APIKey)
	if err != nil {
		return nil, notNow("The saved API key can't be read here (it was saved by another install). Paste it again in Settings › AI.")
	}
	l := &llm{provider: s.Provider, protocol: route(s.Provider, model), base: s.base(), key: key, model: model}
	if l.protocol == "anthropic" && !strings.HasSuffix(l.base, "/v1") {
		l.base += "/v1"
	}
	if l.client, err = clientFor(api, l.base); err != nil {
		return nil, err
	}
	return l, nil
}

// clientFor reaches base: a cloud provider through the declared hosts, the
// owner's own server only when an owner allowed this server's network.
func clientFor(api *plugin.API, base string) (*http.Client, error) {
	u, err := url.Parse(base)
	if err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
		return nil, notNow("The address %q isn't a web address. Fix it in Settings › AI.", base)
	}
	if u.Scheme == "https" && slices.Contains(hosts, u.Hostname()) {
		c := api.HTTP()
		c.Timeout = 3 * time.Minute
		return c, nil
	}
	c, err := api.PrivateHTTP()
	if err != nil {
		return nil, notNow("Runwell may not reach %s yet: an owner can let the AI plugin reach it in Settings › Plugins (tick “Reach this server’s network”).", u.Host)
	}
	cp := *c
	cp.Timeout = 3 * time.Minute
	return &cp, nil
}

// ---- budget and kept-out clients --------------------------------------------------------

// monthStart is the first of this month, in the install's zone.
func monthStart(ctx context.Context, api *plugin.API) time.Time {
	loc := time.UTC
	if site, err := api.Site(ctx); err == nil {
		if site.Location != nil {
			loc = site.Location
		}
	}
	now := time.Now().In(loc)
	return time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, loc)
}

// spent is this month's spend in dollars, every chat and suggestion.
func spent(ctx context.Context, api *plugin.API) (float64, error) {
	var total sql.NullFloat64
	err := api.QueryRow(ctx, `SELECT SUM(total_cost) FROM ai_usages WHERE created_at >= ?`, store.TimeOf(monthStart(ctx, api)).DB()).Scan(&total)
	return total.Float64, err
}

func (s *setting) overBudget(ctx context.Context, api *plugin.API) (bool, error) {
	if s.BudgetCents == nil {
		return false, nil
	}
	d, err := spent(ctx, api)
	return d*100 >= float64(*s.BudgetCents), err
}

func excludedClients(ctx context.Context, api *plugin.API) (map[int64]bool, error) {
	rows, err := api.Query(ctx, `SELECT client_id FROM ai_exclusions`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// available is why AI can't be used now ("" when it can): switched on,
// within budget.
func available(ctx context.Context, api *plugin.API, s *setting) (string, error) {
	if !s.ready() {
		return "AI isn’t switched on here.", nil
	}
	over, err := s.overBudget(ctx, api)
	if err != nil {
		return "", err
	}
	if over {
		return "This month's AI budget is spent: Settings > Plugins > AI.", nil
	}
	return "", nil
}

// ---- the settings page ------------------------------------------------------------------

func showSettings(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	if s.Provider != "" && (s.ModelList == nil || s.ModelList["provider"] != s.Provider) {
		refreshModelsLater(ctx, api, s)
	}
	dollars, err := spent(ctx, api)
	if err != nil {
		return nil, err
	}
	excluded, err := excludedClients(ctx, api)
	if err != nil {
		return nil, err
	}
	keptOut := []map[string]any{}
	if len(excluded) > 0 {
		ids := make([]int64, 0, len(excluded))
		for id := range excluded {
			ids = append(ids, id)
		}
		if keptOut, err = api.Records(ctx, plugin.Query{Type: "Client", IDs: ids, Limit: 500}); err != nil {
			return nil, err
		}
	}
	summary := "AI is off."
	if s.ready() {
		summary = fmt.Sprintf("AI is on, through %s (%s). %s spent this month.", s.providerName(), s.modelFor(false), dollarsText(dollars, 2))
	}
	from := store.TimeOf(monthStart(ctx, api)).DB()
	byPerson, people := []map[string]any{}, map[int64]string{}
	rows, err := api.Query(ctx, `SELECT c.user_id, c.contact_id, SUM(u.total_cost) FROM ai_usages u JOIN ai_chats c ON c.id = u.ai_chat_id
		WHERE u.created_at >= ? GROUP BY c.user_id, c.contact_id ORDER BY SUM(u.total_cost) DESC`, from)
	if err != nil {
		return nil, err
	}
	type spend struct {
		user, contact sql.NullInt64
		cost          float64
	}
	var spends []spend
	for rows.Next() {
		var x spend
		var c sql.NullFloat64
		if err := rows.Scan(&x.user, &x.contact, &c); err != nil {
			rows.Close()
			return nil, err
		}
		x.cost = c.Float64
		spends = append(spends, x)
	}
	rows.Close()
	for id, name := range peopleNames(ctx, api) {
		people[id] = name
	}
	for _, x := range spends {
		who := "Someone"
		switch {
		case x.user.Valid && people[x.user.Int64] != "":
			who = people[x.user.Int64]
		case x.contact.Valid:
			who = "Clients, in their portal"
		}
		byPerson = append(byPerson, map[string]any{"user_id": nullInt(x.user), "name": who, "cost": dollarsText(x.cost, 3)})
	}
	byKind := map[string]string{}
	rows, err = api.Query(ctx, `SELECT c.purpose, SUM(u.total_cost) FROM ai_usages u JOIN ai_chats c ON c.id = u.ai_chat_id WHERE u.created_at >= ? GROUP BY c.purpose`, from)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var purpose string
		var c sql.NullFloat64
		if err := rows.Scan(&purpose, &c); err != nil {
			rows.Close()
			return nil, err
		}
		byKind[purpose] = dollarsText(c.Float64, 3)
	}
	rows.Close()
	used := []string{}
	rows, err = api.Query(ctx, `SELECT kind, SUM(state = 'accepted'), COUNT(*) FROM ai_suggestions WHERE created_at >= ? GROUP BY kind ORDER BY kind`, from)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind string
		var accepted, all int
		if err := rows.Scan(&kind, &accepted, &all); err != nil {
			rows.Close()
			return nil, err
		}
		used = append(used, fmt.Sprintf("%s %d of %d", strings.ReplaceAll(kind, "_", " "), accepted, all))
	}
	rows.Close()
	choices := s.modelChoices()
	var list map[string]any
	if s.ModelList != nil && s.ModelList["provider"] == s.Provider {
		list = s.ModelList
	}
	loading := list != nil && list["at"] == nil && recent(str(list["requested_at"]), 2*time.Minute)
	test := s.LastTest
	testing := test != nil && test["at"] == nil && recent(str(test["requested_at"]), 2*time.Minute)
	keptOutRefs := []map[string]any{}
	for _, c := range keptOut {
		keptOutRefs = append(keptOutRefs, map[string]any{"type": c["type"], "id": c["id"], "record": c["record"], "display_name": c["display_name"], "url": c["url"]})
	}
	return plugin.JSON(200, map[string]any{
		"summary": summary, "enabled": s.Enabled, "ready": s.ready(), "provider": nullStr(s.Provider), "model": nullStr(s.modelFor(false)),
		"fast_model": nullStr(s.modelFor(true)), "portal": s.Portal, "monthly_budget_cents": s.BudgetCents, "spent_cents": int64(math.Round(dollars * 100)),
		"kept_out": keptOutRefs,
		// What the page draws beside them.
		"setting": map[string]any{"enabled": s.Enabled, "provider": s.Provider, "api_key_saved": s.APIKey != "", "api_base": s.APIBase,
			"model": strings.TrimPrefix(s.Model, "opencode/"), "fast_model": strings.TrimPrefix(s.FastModel, "opencode/"), "monthly_budget_cents": s.BudgetCents, "portal": s.Portal},
		"providers": providers,
		"models": map[string]any{"choices": choices, "loading": loading, "error": nullStr(str(list["error"])),
			"missing": s.Provider != "" && list == nil, "provider_name": s.shortName()},
		"test":  map[string]any{"testing": testing, "results": test["results"], "at": test["at"], "provider_name": s.providerName()},
		"spent": dollarsText(dollars, map[bool]int{true: 3, false: 2}[dollars < 1]), "budget": budgetText(s.BudgetCents),
		"by_person": byPerson, "by_kind": byKind, "suggestions_used": used,
		"private_network": api.Grants().PrivateHTTP,
		// What the model pickers offer: the provider's list, or the known
		// models while it can't be had, and every provider's known ones.
		"model_options": modelOptions(s), "known_models": knownFor(),
	}), nil
}

// modelChoices are the provider's models to pick from once fetched, with
// the ones already chosen among them.
func (s *setting) modelChoices() []string {
	out := []string{}
	if s.ModelList == nil || s.ModelList["provider"] != s.Provider {
		return out
	}
	ids, _ := s.ModelList["ids"].([]any)
	for _, id := range ids {
		out = append(out, str(id))
	}
	if len(out) == 0 {
		return out
	}
	for _, m := range []string{s.Model, s.FastModel} {
		m = strings.TrimPrefix(m, "opencode/")
		if m != "" && !slices.Contains(out, m) {
			out = append(out, m)
		}
	}
	return out
}

func updateSettings(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	if req.Actor.Agent {
		return refuse(http.StatusForbidden, "The AI provider and its key are set by a person in the browser."), nil
	}
	in := wrapped(params(req), "ai_setting")
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	was := *s
	set := func(key string, fn func(v any)) {
		if v, ok := in[key]; ok {
			fn(v)
		}
	}
	errs := map[string][]string{}
	set("enabled", func(v any) { s.Enabled = truthy(v) })
	set("portal", func(v any) { s.Portal = truthy(v) })
	set("provider", func(v any) { s.Provider = strings.TrimSpace(str(v)) })
	set("api_base", func(v any) { s.APIBase = strings.TrimSpace(str(v)) })
	set("model", func(v any) { s.Model = strings.TrimSpace(str(v)) })
	set("fast_model", func(v any) { s.FastModel = strings.TrimSpace(str(v)) })
	set("monthly_budget_cents", func(v any) {
		text := strings.TrimSpace(str(v))
		if text == "" {
			s.BudgetCents = nil
			return
		}
		n := intOf(v)
		if fmt.Sprint(n) != text || n < 0 {
			errs["monthly_budget_cents"] = append(errs["monthly_budget_cents"], "must be a whole number of cents, 0 or more")
			return
		}
		s.BudgetCents = &n
	})
	key := strings.TrimSpace(str(in["api_key"]))
	if s.Provider != "" && providerFor(s.Provider) == nil {
		errs["provider"] = append(errs["provider"], "is not included in the list")
	}
	if s.APIBase != "" {
		if u, err := url.Parse(s.APIBase); err != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") {
			errs["api_base"] = append(errs["api_base"], "must be a web address, like http://localhost:11434/v1")
		}
	}
	// Another provider's models mean nothing here: back to its recommended
	// ones until some are picked.
	if was.Provider != "" && s.Provider != was.Provider {
		if _, ok := in["model"]; !ok || s.Model == was.Model {
			s.Model = ""
		}
		if _, ok := in["fast_model"]; !ok || s.FastModel == was.FastModel {
			s.FastModel = ""
		}
	}
	if s.Enabled {
		if s.Provider == "" {
			errs["provider"] = append(errs["provider"], "is needed to switch AI on")
		} else if p := s.info(); p != nil && p.NeedsKey && key == "" && s.APIKey == "" {
			errs["api_key"] = append(errs["api_key"], "is needed for "+p.Name)
		}
	}
	if len(errs) > 0 {
		return plugin.Invalid(errs), nil
	}
	if key != "" {
		if s.APIKey, err = api.Seal(key); err != nil {
			return nil, err
		}
	}
	if _, err := api.Exec(ctx, `UPDATE ai_settings SET enabled = ?, provider = ?, api_key = ?, api_base = ?, model = ?, fast_model = ?, monthly_budget_cents = ?, portal = ?, updated_at = ? WHERE id = ?`,
		s.Enabled, nullStr(s.Provider), nullStr(s.APIKey), nullStr(s.APIBase), nullStr(s.Model), nullStr(s.FastModel), s.BudgetCents, s.Portal, store.Now().DB(), s.ID); err != nil {
		return nil, err
	}
	api.Log(fmt.Sprintf("settings.ai by %s: on %v, provider %q, model %q", req.Actor.Name, s.Enabled, s.Provider, s.modelFor(false)))
	if s.Provider != "" && (s.Provider != was.Provider || key != "" || s.APIBase != was.APIBase) {
		refreshModelsLater(ctx, api, s)
	}
	message := "AI settings saved."
	switch {
	case s.ready():
		message = "AI is on, through " + s.providerName() + "."
	case !s.Enabled:
		message = "AI settings saved, and AI is off."
	}
	return plugin.JSON(200, map[string]any{"message": message, "summary": message}), nil
}

// refreshModelsLater asks the provider for its models in the background
// (AiModelsJob), so the page offers them to pick from.
func refreshModelsLater(ctx context.Context, api *plugin.API, s *setting) {
	if s.Provider == "" {
		return
	}
	list, _ := json.Marshal(map[string]any{"provider": s.Provider, "requested_at": time.Now().UTC().Format(time.RFC3339)})
	if _, err := api.Exec(ctx, `UPDATE ai_settings SET model_list = ? WHERE id = ?`, string(list), s.ID); err != nil {
		api.Log("models: " + err.Error())
		return
	}
	_ = api.Background("models", func(ctx context.Context, api *plugin.API) {
		s, err := loadSetting(ctx, api)
		if err != nil || s.Provider == "" {
			return
		}
		out := map[string]any{"provider": s.Provider, "at": time.Now().UTC().Format(time.RFC3339)}
		l, err := s.llm(api, false)
		var ids []string
		if err == nil {
			ids, err = l.models(ctx)
		}
		if err != nil {
			out["error"] = truncate(ctxErr(ctx, err).Error(), 250)
		} else {
			out["ids"] = ids
		}
		raw, _ := json.Marshal(out)
		if _, err := api.Exec(context.WithoutCancel(ctx), `UPDATE ai_settings SET model_list = ? WHERE id = ? AND provider = ?`, string(raw), s.ID, s.Provider); err != nil {
			api.Log("models: " + err.Error())
		}
	})
}

func refreshModels(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	if s.Provider == "" {
		return refuse(http.StatusUnprocessableEntity, "Choose a provider first."), nil
	}
	refreshModelsLater(ctx, api, s)
	return plugin.JSON(202, map[string]any{"message": "Fetching " + s.shortName() + "’s models…"}), nil
}

// testSettings asks each model the install uses for a one-word answer, in
// the background (AiTestJob); the page polls for what each said.
func testSettings(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	if !s.ready() {
		return refuse(http.StatusUnprocessableEntity, "Switch AI on with a provider (and its key) first, then test it."), nil
	}
	test, _ := json.Marshal(map[string]any{"requested_at": time.Now().UTC().Format(time.RFC3339)})
	if _, err := api.Exec(ctx, `UPDATE ai_settings SET last_test = ? WHERE id = ?`, string(test), s.ID); err != nil {
		return nil, err
	}
	caller := req.Actor
	err = api.Background("test", func(ctx context.Context, api *plugin.API) {
		results := []map[string]any{}
		seen := map[string]bool{}
		for _, fast := range []bool{false, true} {
			model := s.modelFor(fast)
			if seen[model] {
				continue
			}
			seen[model] = true
			started := time.Now()
			r := map[string]any{"model": model, "route": routeName(s.Provider, model)}
			reply, err := oneShot(ctx, api, s, caller, fast, "Reply with the single word: ready")
			if err != nil {
				r["ok"], r["error"] = false, truncate(ctxErr(ctx, err).Error(), 250)
			} else {
				r["ok"], r["reply"], r["seconds"] = true, truncate(reply, 40), math.Round(time.Since(started).Seconds()*10)/10
			}
			results = append(results, r)
		}
		raw, _ := json.Marshal(map[string]any{"at": time.Now().UTC().Format(time.RFC3339), "results": results})
		if _, err := api.Exec(context.WithoutCancel(ctx), `UPDATE ai_settings SET last_test = ? WHERE id = ?`, string(raw), s.ID); err != nil {
			api.Log("test: " + err.Error())
		}
	})
	if err != nil {
		return refuse(http.StatusConflict, err.Error()), nil
	}
	return plugin.JSON(202, map[string]any{"message": "Asking " + s.providerName() + "…"}), nil
}

// oneShot asks one question with no tools, counted in the month's spend
// on a chat of its own.
func oneShot(ctx context.Context, api *plugin.API, s *setting, caller *plugin.Caller, fast bool, question string) (string, error) {
	l, err := s.llm(api, fast)
	if err != nil {
		return "", err
	}
	chatID, err := insertChat(ctx, api, caller.ID, 0, "", 0, "suggestion", "Connection test")
	if err != nil {
		return "", err
	}
	ans, err := l.turn(ctx, ask{System: "You are checking that a connection works.", Turns: []turn{{Role: "user", Text: question}}, MaxTokens: 2000})
	if err != nil {
		return "", err
	}
	recordUsage(ctx, api, chatID, 0, l, ans.Usage)
	return ans.Text, nil
}

func recent(at string, within time.Duration) bool {
	t, err := time.Parse(time.RFC3339, at)
	return err == nil && time.Since(t) < within
}

func dollarsText(d float64, precision int) string {
	return "$" + fmt.Sprintf("%.*f", precision, d)
}

func budgetText(cents *int64) any {
	if cents == nil {
		return nil
	}
	return fmt.Sprintf("$%d.%02d", *cents/100, *cents%100)
}

func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func nullInt(n sql.NullInt64) any {
	if !n.Valid {
		return nil
	}
	return n.Int64
}
