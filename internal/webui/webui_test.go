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
