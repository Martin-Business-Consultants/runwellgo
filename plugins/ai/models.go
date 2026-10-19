package ai

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// The models to pick from, for Settings › AI, an agent's form and Ask's
// picker: never a blank list and never a box to type a name into first.
// The provider's own list once it's been fetched with the key; until then
// (no key yet, an error, a server that's off) the models known for that
// provider, its recommended ones first, with why the live list isn't shown.
// Each says its context size and price where they're known, and which is
// recommended for the main and the fast role.

type knownModel struct {
	ID      string
	Context int // tokens
}

// knownModels are each provider's usual models, the recommended ones
// among them.
var knownModels = map[string][]knownModel{
	"anthropic":  {{"claude-opus-5-5", 1000000}, {"claude-sonnet-5", 1000000}, {"claude-haiku-5-5", 200000}, {"claude-opus-5", 1000000}, {"claude-sonnet-4-5", 200000}, {"claude-haiku-4-5", 200000}},
	"openai":     {{"gpt-5.6", 400000}, {"gpt-5.4-mini", 400000}, {"gpt-5", 400000}},
	"gemini":     {{"gemini-pro-latest", 1000000}, {"gemini-flash-latest", 1000000}},
	"openrouter": {{"anthropic/claude-sonnet-5", 1000000}, {"anthropic/claude-haiku-4-5", 200000}, {"anthropic/claude-opus-5-5", 1000000}, {"openai/gpt-5.6", 400000}, {"google/gemini-pro-latest", 1000000}},
	"opencode":   {{"claude-sonnet-5", 1000000}, {"claude-haiku-4-5", 200000}, {"claude-opus-5-5", 1000000}, {"gpt-5.6", 400000}, {"qwen3.8-max", 0}},
	"ollama":     {{"llama4", 0}, {"qwen3", 0}, {"gemma3", 0}},
}

// modelOption is one model to pick.
type modelOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Context     string `json:"context,omitempty"`
	Price       string `json:"price,omitempty"`
	Recommended string `json:"recommended,omitempty"` // main, fast or both
}

func contextText(n int) string {
	switch {
	case n >= 1000000:
		return fmt.Sprintf("%dM context", n/1000000)
	case n > 0:
		return fmt.Sprintf("%dK context", n/1000)
	}
	return ""
}

// priceText is a model's list price, per million tokens in and out.
func priceText(provider, model string) string {
	if provider == "ollama" {
		return "free, on your server"
	}
	id := model
	if i := strings.LastIndex(id, "/"); i >= 0 {
		id = id[i+1:]
	}
	for _, x := range prices {
		if strings.HasPrefix(id, x.prefix) {
			return fmt.Sprintf("$%s / $%s per M", money(x.p.in), money(x.p.out))
		}
	}
	return ""
}

func money(d float64) string {
	s := fmt.Sprintf("%.2f", d)
	return strings.TrimSuffix(strings.TrimSuffix(s, "0"), ".0")
}

// modelOptions are the models for the provider chosen: the live list when
// there is one, else the known ones, and a quiet word on which it is.
func modelOptions(s *setting) map[string]any {
	p := s.info()
	if p == nil {
		return map[string]any{"options": []modelOption{}, "live": false, "note": "Choose a provider first."}
	}
	contexts := map[string]int{}
	for _, k := range knownModels[p.Key] {
		contexts[k.ID] = k.Context
	}
	ids := s.modelChoices()
	live := len(ids) > 0
	note := ""
	if !live {
		for _, k := range knownModels[p.Key] {
			ids = append(ids, k.ID)
		}
		switch {
		case p.NeedsKey && s.APIKey == "":
			note = "Showing known models: paste your API key to see all of yours."
		case s.ModelList != nil && s.ModelList["provider"] == s.Provider && str(s.ModelList["error"]) != "":
			note = "Showing known models: " + p.shortName() + "’s list couldn’t be fetched (" + truncate(str(s.ModelList["error"]), 120) + ")."
		case s.ModelList != nil && s.ModelList["provider"] == s.Provider && s.ModelList["at"] == nil:
			note = "Showing known models while " + p.shortName() + "’s list loads."
		default:
			note = "Showing known models: save to fetch " + p.shortName() + "’s own list."
		}
	}
	// The recommended ones first, then the ones chosen, then the rest.
	for _, m := range []string{p.Fast, p.Model, strings.TrimPrefix(s.FastModel, "opencode/"), strings.TrimPrefix(s.Model, "opencode/")} {
		if m == "" {
			continue
		}
		if i := slices.Index(ids, m); i >= 0 {
			ids = slices.Delete(ids, i, i+1)
		}
		ids = append([]string{m}, ids...)
	}
	out := []modelOption{}
	for _, id := range ids {
		o := modelOption{ID: id, Label: id, Context: contextText(contexts[id]), Price: priceText(p.Key, id)}
		switch {
		case id == p.Model && id == p.Fast:
			o.Recommended = "both"
		case id == p.Model:
			o.Recommended = "main"
		case id == p.Fast:
			o.Recommended = "fast"
		}
		out = append(out, o)
	}
	return map[string]any{"options": out, "live": live, "note": nullStr(note), "recommended": map[string]string{"main": p.Model, "fast": p.Fast}}
}

// knownFor is every provider's known models and recommended ones, for the
// settings page to switch providers without a round trip.
func knownFor() map[string]any {
	out := map[string]any{}
	for _, p := range providers {
		list := []modelOption{}
		for _, k := range knownModels[p.Key] {
			o := modelOption{ID: k.ID, Label: k.ID, Context: contextText(k.Context), Price: priceText(p.Key, k.ID)}
			switch {
			case k.ID == p.Model && k.ID == p.Fast:
				o.Recommended = "both"
			case k.ID == p.Model:
				o.Recommended = "main"
			case k.ID == p.Fast:
				o.Recommended = "fast"
			}
			list = append(list, o)
		}
		out[p.Key] = list
	}
	return out
}

// listModels is the models to pick from, for anyone on staff (Ask's
// picker, an agent's form): the provider's, or the known ones.
func listModels(ctx context.Context, api *plugin.API, req *plugin.Request) (*plugin.Response, error) {
	s, err := loadSetting(ctx, api)
	if err != nil {
		return nil, err
	}
	out := modelOptions(s)
	out["main"], out["fast"] = s.modelFor(false), s.modelFor(true)
	out["provider"] = nullStr(s.Provider)
	return plugin.JSON(200, out), nil
}
