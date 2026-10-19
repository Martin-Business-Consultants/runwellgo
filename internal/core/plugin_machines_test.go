package core

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Martin-Business-Consultants/runwellgo/internal/plugin"
)

// A machine route answers only a token its own plugin issued: not a
// made-up one, not a person's session, and the token reaches nothing else.
func TestMachineRoutesTakeOnlyTheirPluginsTokens(t *testing.T) {
	a, err := NewApp(Config{DataDir: t.TempDir(), NoDefaultPlugins: true, Logger: slog.New(slog.NewTextHandler(io.Discard, nil))})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { a.Close() })
	srv := httptest.NewServer(a.Handler())
	defer srv.Close()
	ctx := t.Context()
	if _, err := a.plugins.Install(ctx, nil, "runners"); err != nil {
		t.Fatal(err)
	}
	on := true
	if _, err := a.plugins.Configure(ctx, nil, "runners", nil, &on); err != nil {
		t.Fatal(err)
	}
	owner := &plugin.Caller{ID: 1, Name: "Owner", Role: "owner", Capabilities: []string{"runners:manage", "runners:use"}}
	res, err := a.plugins.Serve(ctx, "runners", "createRunner", &plugin.Request{Method: "POST", Body: []byte(`{"runner":{"name":"Box"}}`), Actor: owner}, nil)
	if err != nil {
		t.Fatal(err)
	}
	var made struct{ Token string }
	_ = json.Unmarshal(res.Body, &made)
	if !strings.HasPrefix(made.Token, "rwm_") {
		t.Fatalf("no token: %s", res.Body)
	}
	call := func(method, path, token string) int {
		req, _ := http.NewRequestWithContext(t.Context(), method, srv.URL+path, strings.NewReader(`{}`))
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	if got := call("POST", "/api/x/runners/machine/heartbeat", made.Token); got != 200 {
		t.Fatalf("the runner's heartbeat: %d", got)
	}
	if got := call("POST", "/api/x/runners/machine/heartbeat", "rwm_madeup"); got != 401 {
		t.Fatalf("a made-up token: %d", got)
	}
	if got := call("POST", "/api/x/runners/machine/heartbeat", ""); got != 401 {
		t.Fatalf("no token: %d", got)
	}
	if got := call("GET", "/api/x/runners/runners", made.Token); got != 401 {
		t.Fatalf("a machine token reached a person's route: %d", got)
	}
	if got := call("GET", "/api/clients", made.Token); got != 401 {
		t.Fatalf("a machine token reached the core: %d", got)
	}
	// Switched off, the plugin's machines can't call in.
	off := false
	if _, err := a.plugins.Configure(ctx, nil, "runners", nil, &off); err != nil {
		t.Fatal(err)
	}
	if got := call("POST", "/api/x/runners/machine/heartbeat", made.Token); got != 404 {
		t.Fatalf("with the plugin off: %d", got)
	}
}
