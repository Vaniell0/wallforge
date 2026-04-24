// Package webui serves a local browser UI for applying wallpapers,
// browsing the Steam Workshop subscription library and controlling the
// running backends. Bound to 127.0.0.1 by default — there's no auth on
// any of this because the only caller we trust is the same user.
package webui

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"path/filepath"
	"time"

	"github.com/Vaniell0/wallforge/internal/apply"
	"github.com/Vaniell0/wallforge/internal/config"
	"github.com/Vaniell0/wallforge/internal/engine"
	"github.com/Vaniell0/wallforge/internal/steam"
	"github.com/Vaniell0/wallforge/internal/workshop"
)

//go:embed static
var staticFS embed.FS

// Server bundles the HTTP listener with the user config so handlers can
// reach steam/engine without wiring up dependencies at each call site.
type Server struct {
	cfg  config.Config
	addr string
	http *http.Server

	// lastApplied is the most recent item the UI applied — handy for the
	// "current wallpaper" strip. Not persisted across restarts; the whole
	// point of the web-UI is interactive use, not source of truth.
	lastApplied string
}

// New constructs a Server bound to addr. The listener isn't opened until
// Run is called.
func New(cfg config.Config, addr string) (*Server, error) {
	if addr == "" {
		return nil, errors.New("webui: empty addr")
	}
	s := &Server{cfg: cfg, addr: addr}
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

// Run starts the server and blocks until ctx is cancelled. Shutdown is
// given a short grace window so in-flight apply calls can finish cleanly.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.http.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		return s.http.Shutdown(shutdownCtx)
	}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()

	static, err := fs.Sub(staticFS, "static")
	if err != nil {
		// embed is compile-time, so a failure here means we shipped a
		// broken binary — fail loud rather than silently.
		panic(fmt.Errorf("webui: fs.Sub: %w", err))
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServer(http.FS(static))))

	mux.HandleFunc("GET /{$}", s.handleIndex)
	mux.HandleFunc("GET /api/items", s.handleItems)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /preview/{id}", s.handlePreview)
	mux.HandleFunc("POST /api/apply", s.handleApply)
	mux.HandleFunc("POST /api/stop", s.handleStop)

	return logRequests(mux)
}

// ItemDTO is the JSON shape returned by /api/items — one row per
// subscribed Workshop item.
type ItemDTO struct {
	ID         string   `json:"id"`
	Type       string   `json:"type"`
	Title      string   `json:"title"`
	Tags       []string `json:"tags"`
	HasPreview bool     `json:"has_preview"`
	Broken     bool     `json:"broken"` // project.json missing / unparseable
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

func (s *Server) handleItems(w http.ResponseWriter, r *http.Request) {
	items, err := steam.List(s.cfg.Steam.Root)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	dto := make([]ItemDTO, 0, len(items))
	for _, it := range items {
		d := ItemDTO{ID: it.ID}
		if it.Project == nil {
			d.Broken = true
			d.Title = "(no project.json)"
			dto = append(dto, d)
			continue
		}
		d.Type = string(it.Project.Type)
		d.Title = it.Project.Title
		d.Tags = it.Project.Tags
		d.HasPreview = it.Project.Preview != ""
		dto = append(dto, d)
	}
	writeJSON(w, dto)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]string{"last_applied": s.lastApplied})
}

// handlePreview serves the project.preview file for a single workshop
// item. The preview filename comes from project.json; we re-parse it
// instead of trusting an untrusted query parameter to avoid path
// traversal into arbitrary Steam content.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if !apply.IsNumericID(id) {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	dir, err := steam.Resolve(s.cfg.Steam.Root, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusNotFound)
		return
	}
	proj, err := workshop.ParseDir(dir)
	if err != nil || proj == nil || proj.Preview == "" {
		http.Error(w, "no preview", http.StatusNotFound)
		return
	}
	// Belt-and-braces: filepath.Base strips any embedded traversal in
	// case a hostile project.json tries "../../etc/passwd".
	name := filepath.Base(proj.Preview)
	http.ServeFile(w, r, filepath.Join(dir, name))
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Input string `json:"input"` // ID or path
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad JSON: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.Input == "" {
		http.Error(w, "missing input", http.StatusBadRequest)
		return
	}
	res, err := apply.ByInput(s.cfg, req.Input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.lastApplied = req.Input
	writeJSON(w, res)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	errs := engine.StopAll(s.cfg)
	if len(errs) == 0 {
		s.lastApplied = ""
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	writeJSON(w, map[string]any{"ok": false, "errors": msgs})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		// Header is already committed — log path isn't available, so
		// just swallow: the client will see a truncated body.
		_ = err
	}
}

// logRequests prints a line per request to stderr. Nothing fancy — the
// intent is to make an "is this alive?" check trivial from the terminal
// where `wallforge serve` is running.
func logRequests(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &statusRecorder{ResponseWriter: w, status: 200}
		h.ServeHTTP(rw, r)
		fmt.Printf("webui: %s %s %d %s\n",
			r.Method, r.URL.Path, rw.status, time.Since(start).Round(time.Millisecond))
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}
