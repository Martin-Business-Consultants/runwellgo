package outbound

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSkipsLinkLocal(t *testing.T) {
	t.Parallel()
	for _, u := range []string{"http://169.254.169.254/latest/meta-data/", "http://[fe80::1]/", "http://0.0.0.0:8080/"} {
		if err := CheckURL(u); err == nil {
			t.Errorf("CheckURL(%q) allowed it", u)
		}
	}
	// What the dialer sees decides, whatever the URL said: a redirect there
	// is refused too. Loopback (a site on the same box) is fine.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/hop" {
			http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	get := func(path string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, srv.URL+path, nil)
		if err != nil {
			t.Fatal(err)
		}
		return Client.Do(req)
	}
	res, err := get("/ok")
	if err != nil {
		t.Fatalf("loopback: %v", err)
	}
	res.Body.Close()
	res, err = get("/hop")
	if err == nil {
		res.Body.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "may reach") {
		t.Fatalf("redirect to link-local: %v", err)
	}
}
