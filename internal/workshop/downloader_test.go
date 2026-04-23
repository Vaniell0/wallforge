package workshop

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func makeZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func newFakeServer(t *testing.T, zipBytes []byte, statusSequence []string) *httptest.Server {
	t.Helper()
	var idx int64
	mux := http.NewServeMux()

	mux.HandleFunc("/api/download/request", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]string{"uuid": "test-uuid"})
	})
	mux.HandleFunc("/api/download/status", func(w http.ResponseWriter, r *http.Request) {
		i := atomic.AddInt64(&idx, 1) - 1
		if i >= int64(len(statusSequence)) {
			i = int64(len(statusSequence)) - 1
		}
		_ = json.NewEncoder(w).Encode(map[string]map[string]string{
			"test-uuid": {"status": statusSequence[i]},
		})
	})
	mux.HandleFunc("/api/download/transmit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/zip")
		_, _ = w.Write(zipBytes)
	})

	return httptest.NewServer(mux)
}

func TestProxyDownloader_HappyPath(t *testing.T) {
	zipBytes := makeZip(t, map[string]string{
		"project.json": `{"type":"scene","file":"scene.pkg","title":"test"}`,
		"scene.pkg":    "binary-bytes",
	})
	srv := newFakeServer(t, zipBytes, []string{"queued", "transmitting", "prepared"})
	defer srv.Close()

	d := NewProxyDownloader(t.TempDir())
	d.BaseURL = srv.URL
	d.PollInterval = 1 * time.Millisecond
	d.Timeout = 2 * time.Second

	dir, err := d.Download(context.Background(), "123456789")
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(dir, "project.json")); err != nil {
		t.Errorf("project.json missing in extracted dir: %v", err)
	}
	proj, err := ParseDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if proj.Title != "test" {
		t.Errorf("extracted project.json has wrong title: %q", proj.Title)
	}
}

func TestProxyDownloader_Cached(t *testing.T) {
	cache := t.TempDir()
	itemDir := ItemDir(cache, "555")
	if err := os.MkdirAll(itemDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(itemDir, "project.json"),
		[]byte(`{"type":"scene"}`), 0o644); err != nil {
		t.Fatal(err)
	}

	// Server must not be hit.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("cached item should skip network, got request: %s", r.URL.Path)
		http.Error(w, "should not hit", 500)
	}))
	defer srv.Close()

	d := NewProxyDownloader(cache)
	d.BaseURL = srv.URL
	got, err := d.Download(context.Background(), "555")
	if err != nil {
		t.Fatal(err)
	}
	if got != itemDir {
		t.Errorf("cache dir mismatch: got %q, want %q", got, itemDir)
	}
}

func TestProxyDownloader_ErrorStatus(t *testing.T) {
	srv := newFakeServer(t, nil, []string{"error"})
	defer srv.Close()

	d := NewProxyDownloader(t.TempDir())
	d.BaseURL = srv.URL
	d.PollInterval = 1 * time.Millisecond
	d.Timeout = 1 * time.Second

	if _, err := d.Download(context.Background(), "1"); err == nil {
		t.Error("expected error from server-reported error status")
	}
}

func TestParseID(t *testing.T) {
	cases := []struct{ in, want string }{
		{"123456", "123456"},
		{"https://steamcommunity.com/sharedfiles/filedetails/?id=123456", "123456"},
		{"https://steamcommunity.com/sharedfiles/filedetails/?id=123456&foo=bar", "123456"},
	}
	for _, c := range cases {
		if got := parseID(c.in); got != c.want {
			t.Errorf("parseID(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestExtractZip_ZipSlipRejected(t *testing.T) {
	// Craft a zip with a path escaping the target dir.
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	w, _ := zw.Create("../escape.txt")
	_, _ = w.Write([]byte("bad"))
	_ = zw.Close()

	dst := t.TempDir()
	err := extractZip(buf.Bytes(), dst)
	if err == nil || !strings.Contains(err.Error(), "zip slip") {
		t.Errorf("expected zip slip rejection, got %v", err)
	}
}
