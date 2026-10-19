package core

import (
	"context"
	"net/http"
)

// The sidebar's live parts: the counts beside its items and the plugins'
// pages, in the person's own order.

func init() { routeSets = append(routeSets, (*App).navRoutes) }

func (a *App) navRoutes(mux *http.ServeMux) {
	a.handle(mux, "GET /api/nav", a.showNav)
}

// navCounts are the numbers beside sidebar items, by badge name; areas add
// theirs in an init.
var navCounts = map[string]func(a *App, ctx context.Context, actor *Actor) (int, error){}

// navPlugins, when set (the plugins area), lists the plugins' pages for
// the person: [{href, label, key}].
var navPlugins func(a *App, ctx context.Context, actor *Actor) ([]Obj, error)

func (a *App) showNav(c *Ctx) error {
	ctx := c.R.Context()
	counts := Obj{}
	for name, count := range navCounts {
		n, err := count(a, ctx, c.Actor)
		if err != nil {
			return err
		}
		counts = counts.Set(name, n)
	}
	plugins := []Obj{}
	if navPlugins != nil {
		var err error
		if plugins, err = navPlugins(a, ctx, c.Actor); err != nil {
			return err
		}
	}
	return c.JSON(200, O("counts", counts, "plugins", plugins))
}
