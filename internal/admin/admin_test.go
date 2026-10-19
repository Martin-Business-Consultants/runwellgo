package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The page's policy is 'self' alone, widened only by the sources the
// request carries (plugins that are on and granted content:external).
func TestCSPWidensByPluginSources(t *testing.T) {
	t.Parallel()
	h := Handler()
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", nil))
	plain := w.Header().Get("Content-Security-Policy")
	if strings.Contains(plain, "googleapis") || !strings.Contains(plain, "script-src 'self' 'unsafe-inline';") {
		t.Fatalf("plain policy: %s", plain)
	}
	r := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/reports", nil)
	r = r.WithContext(WithSources(r.Context(), map[string][]string{"script-src": {"https://maps.googleapis.com"}, "worker-src": {"blob:"}}))
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	got := w.Header().Get("Content-Security-Policy")
	if !strings.Contains(got, "script-src 'self' 'unsafe-inline' https://maps.googleapis.com;") || !strings.Contains(got, "worker-src 'self' blob:") ||
		!strings.Contains(got, "connect-src 'self';") {
		t.Fatalf("widened policy: %s", got)
	}
}
