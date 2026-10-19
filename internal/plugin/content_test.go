package plugin

import (
	"slices"
	"strings"
	"testing"
)

// Content names hosts and a few keywords, under directives it may widen,
// and only with content:external.
func TestContentValidates(t *testing.T) {
	t.Parallel()
	ok := Manifest{Key: "maps", Permissions: Grants{ContentExternal: true}, Content: map[string][]string{
		"script-src": {"https://maps.googleapis.com", "*.gstatic.com", "'unsafe-eval'"}, "worker-src": {"blob:"}, "img-src": {"data:"},
	}}
	if errs := ok.validateContent(); len(errs) > 0 {
		t.Fatalf("valid content refused: %v", errs)
	}
	for name, m := range map[string]Manifest{
		"no grant":        {Content: map[string][]string{"script-src": {"maps.googleapis.com"}}},
		"a star":          {Permissions: Grants{ContentExternal: true}, Content: map[string][]string{"script-src": {"*"}}},
		"http":            {Permissions: Grants{ContentExternal: true}, Content: map[string][]string{"script-src": {"http://evil.test"}}},
		"unsafe-inline":   {Permissions: Grants{ContentExternal: true}, Content: map[string][]string{"style-src": {"'unsafe-inline'"}}},
		"a path":          {Permissions: Grants{ContentExternal: true}, Content: map[string][]string{"connect-src": {"api.test/x"}}},
		"other directive": {Permissions: Grants{ContentExternal: true}, Content: map[string][]string{"default-src": {"api.test"}}},
		"eval elsewhere":  {Permissions: Grants{ContentExternal: true}, Content: map[string][]string{"connect-src": {"'unsafe-eval'"}}},
		"frame-ancestors": {Permissions: Grants{ContentExternal: true}, Content: map[string][]string{"frame-ancestors": {"api.test"}}},
	} {
		if errs := m.validateContent(); len(errs) == 0 {
			t.Errorf("%s: accepted", name)
		}
	}
	if hosts := ok.contentHosts(); !slices.Equal(hosts, []string{"*.gstatic.com", "maps.googleapis.com"}) {
		t.Errorf("hosts %v", hosts)
	}
}

// The host adds a plugin's content only while it's on and granted it.
func TestContentSourcesNeedOnAndGranted(t *testing.T) {
	t.Parallel()
	m := &Module{Manifest: Manifest{Key: "maps", Name: "Maps", Version: "1", Permissions: Grants{ContentExternal: true},
		Content: map[string][]string{"script-src": {"maps.googleapis.com"}}}}
	h := &Host{modules: map[string]*Module{"maps": m}}
	set := func(on, granted bool) {
		h.plugins = map[string]*Plugin{"maps": {Key: "maps", Manifest: m.Manifest, Enabled: on, Granted: Grants{ContentExternal: granted}, module: m}}
	}
	set(true, false)
	if h.ContentSources() != nil {
		t.Fatal("added while not granted")
	}
	set(false, true)
	if h.ContentSources() != nil {
		t.Fatal("added while off")
	}
	set(true, true)
	if got := h.ContentSources()["script-src"]; !slices.Equal(got, []string{"maps.googleapis.com"}) {
		t.Fatalf("got %v", got)
	}
	if !strings.Contains(strings.Join(m.Manifest.Additions(), " "), "Loads content from: maps.googleapis.com") {
		t.Fatal("its card doesn't say where it loads from")
	}
}
