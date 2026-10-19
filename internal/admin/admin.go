// Package admin is the app: a Svelte app (ui/), built into dist/ and
// embedded in the binary. Staff, the client portal and approval pages are
// all one app that routes in the browser, working the same /api the CLI and
// agents use, signed in by a session cookie instead of a token.
//
// dist/ is committed, so `go build` and `go install` need no Node; after
// changing ui/, `make ui` rebuilds it.
package admin

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"path"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// policy is the app's Content-Security-Policy, by directive, in order.
var policy = [][2]string{
	{"default-src", "'self'"}, {"img-src", "'self' data: blob: https:"}, {"font-src", "'self' data:"},
	{"style-src", "'self' 'unsafe-inline'"}, {"script-src", "'self' 'unsafe-inline'"}, {"connect-src", "'self'"},
	{"frame-ancestors", "'none'"}, {"base-uri", "'none'"}, {"form-action", "'self'"},
}

var csp = CSP(nil)

// CSP is the app's Content-Security-Policy with extra sources added by
// directive (what plugins that are on and granted content:external name;
// see plugin.Host.ContentSources). A directive the policy doesn't have
// (frame-src, worker-src) starts from 'self'.
func CSP(extra map[string][]string) string {
	var b strings.Builder
	done := map[string]bool{}
	for _, d := range policy {
		if b.Len() > 0 {
			b.WriteString("; ")
		}
		b.WriteString(d[0] + " " + d[1])
		for _, s := range extra[d[0]] {
			b.WriteString(" " + s)
		}
		done[d[0]] = true
	}
	for _, d := range []string{"frame-src", "worker-src"} {
		if len(extra[d]) > 0 && !done[d] {
			b.WriteString("; " + d + " 'self' " + strings.Join(extra[d], " "))
		}
	}
	return b.String()
}

type sourcesKey struct{}

// WithSources is ctx carrying the extra sources the page's policy adds
// (the core sets them per request, from the plugins that are on).
func WithSources(ctx context.Context, extra map[string][]string) context.Context {
	return context.WithValue(ctx, sourcesKey{}, extra)
}

// Handler serves the app: the built assets, and index.html for every other
// path (the browser routes).
func Handler() http.Handler {
	files, _ := fs.Sub(dist, "dist")
	index, _ := fs.ReadFile(files, "index.html")
	assets := http.FileServer(http.FS(files))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rel := path.Clean(r.URL.Path)
		h := w.Header()
		if extra, _ := r.Context().Value(sourcesKey{}).(map[string][]string); len(extra) > 0 {
			h.Set("Content-Security-Policy", CSP(extra))
		} else {
			h.Set("Content-Security-Policy", csp)
		}
		// Hashed build assets never change under their name.
		if strings.HasPrefix(rel, "/assets/") {
			h.Set("Cache-Control", "public, max-age=31536000, immutable")
			assets.ServeHTTP(w, r)
			return
		}
		if f, err := fs.Stat(files, strings.TrimPrefix(rel, "/")); err == nil && !f.IsDir() && rel != "/index.html" {
			assets.ServeHTTP(w, r)
			return
		}
		h.Set("Content-Type", "text/html; charset=utf-8")
		h.Set("Cache-Control", "no-cache")
		w.Write(index)
	})
}
