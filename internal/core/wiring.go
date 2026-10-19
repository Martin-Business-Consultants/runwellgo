package core

import "context"

// What ties areas together where one provides what another asks for.

func init() {
	// Settings › Your account › Your menu arranges every plugin page there
	// is; the sidebar (api_plugins.go's navPlugins) shows the person's
	// arrangement of the ones they may open.
	pluginNavItems = func(a *App, ctx context.Context) ([]Obj, error) {
		out := []Obj{}
		for _, p := range a.enabledPlugins() {
			if n := p.Manifest.Nav; n != nil {
				out = append(out, O("key", p.Key, "label", n.Label, "href", navHref(p.Key, n)))
			}
		}
		return out, nil
	}
}
