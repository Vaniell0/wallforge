package webui

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Vaniell0/wallforge/internal/config"
)

// newTestServer spins up a Server whose steam.root points at a temp dir
// preloaded with a single workshop subdirectory + project.json, so the
// API handlers hit real filesystem code without needing an installed
// Steam client.
func newTestServer(t *testing.T) (*Server, string) {
	t.Helper()
	root := t.TempDir()
	wsDir := filepath.Join(root, "steamapps", "workshop", "content", "431960", "123")
	if err := os.MkdirAll(wsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	proj := `{
		"title": "Demo",
		"type": "image",
		"file": "bg.png",
		"preview": "preview.png",
		"tags": ["abstract", "blue"],
		"workshopid": "123"
	}`
	if err := os.WriteFile(filepath.Join(wsDir, "project.json"), []byte(proj), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wsDir, "preview.png"), []byte("PNGDATA"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Steam.Root = root

	srv, err := New(cfg, "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	return srv, root
}

func TestIndexServesHTML(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status: want 200, got %d", rr.Code)
	}
	if ct := rr.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("content-type: want text/html, got %q", ct)
	}
	if !strings.Contains(rr.Body.String(), "<title>Wallforge</title>") {
		t.Errorf("index body missing title tag")
	}
}

func TestItemsReturnsSubscription(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/items", nil)
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status: want 200, got %d", rr.Code)
	}
	var items []ItemDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d", len(items))
	}
	got := items[0]
	if got.ID != "123" || got.Title != "Demo" || got.Type != "image" || !got.HasPreview {
		t.Errorf("unexpected item: %+v", got)
	}
}

func TestPreviewServesFile(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/preview/123", nil)
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status: want 200, got %d", rr.Code)
	}
	if rr.Body.String() != "PNGDATA" {
		t.Errorf("preview body: want PNGDATA, got %q", rr.Body.String())
	}
}

func TestPreviewRejectsNonNumericID(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/preview/../etc/passwd", nil)
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	// Mux never matches the pattern for traversal strings — serves the
	// index page (404/405 are also acceptable, the key is "not the
	// preview handler"). The body must not be "PNGDATA".
	if strings.Contains(rr.Body.String(), "PNGDATA") {
		t.Errorf("preview handler leaked file to traversal request")
	}
}

func TestApplyRejectsEmptyInput(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/apply",
		strings.NewReader(`{"input":""}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("want 400, got %d", rr.Code)
	}
}

func TestStaticAssetsServed(t *testing.T) {
	srv, _ := newTestServer(t)
	for _, p := range []string{"/static/app.js", "/static/style.css"} {
		req := httptest.NewRequest(http.MethodGet, p, nil)
		rr := httptest.NewRecorder()
		srv.routes().ServeHTTP(rr, req)
		if rr.Code != 200 {
			t.Errorf("%s: want 200, got %d", p, rr.Code)
		}
		if rr.Body.Len() == 0 {
			t.Errorf("%s: empty body", p)
		}
	}
}

func TestLastApplied_NoRace(t *testing.T) {
	// Regression test for audit H1: lastApplied was read by /api/power
	// and /api/status without taking s.mu, while /api/apply and
	// /api/stop were writing it from other handlers. go test -race
	// caught the data race; this test reproduces the contention so a
	// future refactor that drops the mutex protection trips CI.
	srv, _ := newTestServer(t)

	const iters = 200
	done := make(chan struct{}, 3)

	go func() {
		for i := 0; i < iters; i++ {
			srv.setLastApplied("a")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < iters; i++ {
			srv.setLastApplied("b")
		}
		done <- struct{}{}
	}()
	go func() {
		for i := 0; i < iters; i++ {
			_ = srv.getLastApplied()
		}
		done <- struct{}{}
	}()
	for i := 0; i < 3; i++ {
		<-done
	}
}

func TestCSRFRejectsCrossOriginPost(t *testing.T) {
	// Drive-by browser fetch with cross-site Sec-Fetch-Site or with
	// an Origin from a different host must be rejected on every
	// state-mutating endpoint.
	srv, _ := newTestServer(t)
	endpoints := []string{
		"/api/apply",
		"/api/stop",
		"/api/power/pause",
		"/api/power/resume",
	}
	for _, ep := range endpoints {
		req := httptest.NewRequest(http.MethodPost, ep, nil)
		req.Header.Set("Sec-Fetch-Site", "cross-site")
		rr := httptest.NewRecorder()
		srv.routes().ServeHTTP(rr, req)
		if rr.Code != http.StatusForbidden {
			t.Errorf("%s with cross-site Sec-Fetch-Site: got %d, want 403", ep, rr.Code)
		}
	}
}

func TestCSRFAcceptsSameOriginPost(t *testing.T) {
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/stop", nil)
	req.Header.Set("Sec-Fetch-Site", "same-origin")
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	// Stop returns 200 even when no backends were running (StopAll
	// errors are not fatal). Just assert "not 403".
	if rr.Code == http.StatusForbidden {
		t.Errorf("same-origin POST got 403")
	}
}

func TestCSRFAcceptsBareCurl(t *testing.T) {
	// curl / native clients send neither Sec-Fetch-Site nor Origin.
	// Must pass through — the API has many non-browser users.
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodPost, "/api/stop", nil)
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code == http.StatusForbidden {
		t.Errorf("bare POST (no Sec-Fetch, no Origin) got 403")
	}
}

func TestPowerEndpointShape(t *testing.T) {
	// /api/power must return the new schema: mode + reason +
	// power_saver_policy. Front-end depends on these field names.
	srv, _ := newTestServer(t)
	req := httptest.NewRequest(http.MethodGet, "/api/power", nil)
	rr := httptest.NewRecorder()
	srv.routes().ServeHTTP(rr, req)
	if rr.Code != 200 {
		t.Fatalf("status %d", rr.Code)
	}
	var dto PowerDTO
	if err := json.Unmarshal(rr.Body.Bytes(), &dto); err != nil {
		t.Fatal(err)
	}
	// Mode must be one of the known values; profile is whatever the
	// host returns (likely "performance" or "unknown").
	switch dto.Mode {
	case "normal", "low-power", "paused":
	default:
		t.Errorf("unexpected mode: %q", dto.Mode)
	}
	if dto.PowerSaverPolicy == "" {
		t.Errorf("PowerSaverPolicy empty — should default to 'reduce'")
	}
}
