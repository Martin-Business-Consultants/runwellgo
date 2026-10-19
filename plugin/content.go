package plugin

import (
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
)

// "content:external": a plugin whose own screens load something
// from another site (a map's script, tiles and fonts) names those sites in
// its manifest's Content, by Content-Security-Policy directive. The app's
// pages allow only themselves otherwise; while the plugin is on and granted
// content:external, the core adds its sources to the policy, and takes them
// out again the moment it's off. A source is a host (maps.googleapis.com),
// a wildcard under a domain (*.gstatic.com), optionally with https://, or
// one of the few keywords a vendor's script needs (blob: and data: where
// they make sense, 'unsafe-eval' for script-src); never a bare *, http: or
// 'unsafe-inline' widening.

// ContentDirectives are the directives a plugin may add sources to.
var ContentDirectives = []string{"script-src", "style-src", "img-src", "font-src", "connect-src", "frame-src", "worker-src"}

var contentHost = regexp.MustCompile(`^(https://)?(\*\.)?[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)

// contentKeywords are the non-host sources each directive takes.
var contentKeywords = map[string][]string{
	"script-src":  {"blob:", "'unsafe-eval'"},
	"img-src":     {"blob:", "data:"},
	"font-src":    {"data:"},
	"connect-src": {"blob:", "data:"},
	"worker-src":  {"blob:"},
	"frame-src":   {},
	"style-src":   {},
}

// validateContent checks the manifest's Content.
func (m *Manifest) validateContent() []string {
	var errs []string
	if len(m.Content) > 0 && !m.Permissions.ContentExternal {
		errs = append(errs, "content needs permissions.content:external")
	}
	for dir, sources := range m.Content {
		if !slices.Contains(ContentDirectives, dir) {
			errs = append(errs, fmt.Sprintf("content directives are %s, not %q", strings.Join(ContentDirectives, ", "), dir))
			continue
		}
		for _, s := range sources {
			if !contentHost.MatchString(s) && !slices.Contains(contentKeywords[dir], s) {
				errs = append(errs, fmt.Sprintf("content[%s] names hosts (maps.googleapis.com, *.gstatic.com), not %q", dir, s))
			}
		}
	}
	return errs
}

// contentHosts are the sites Content names, for its card.
func (m *Manifest) contentHosts() []string {
	seen := map[string]bool{}
	var out []string
	for _, sources := range m.Content {
		for _, s := range sources {
			h := strings.TrimPrefix(s, "https://")
			if contentHost.MatchString(h) && !seen[h] {
				seen[h] = true
				out = append(out, h)
			}
		}
	}
	sort.Strings(out)
	return out
}

// ContentSources are the sources the plugins that are on and granted
// content:external add to the app's Content-Security-Policy, by directive,
// each list sorted and without repeats; nil when none does.
func (h *Host) ContentSources() map[string][]string {
	var out map[string][]string
	for _, p := range h.List() {
		if !p.Enabled || p.module == nil || !p.Granted.ContentExternal {
			continue
		}
		// The module's own manifest: what this build declares, not what's
		// stored.
		for dir, sources := range p.module.Manifest.Content {
			if out == nil {
				out = map[string][]string{}
			}
			for _, s := range sources {
				if !slices.Contains(out[dir], s) {
					out[dir] = append(out[dir], s)
				}
			}
		}
	}
	for dir := range out {
		sort.Strings(out[dir])
	}
	return out
}
