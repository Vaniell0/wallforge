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
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Vaniell0/wallforge/internal/apply"
	"github.com/Vaniell0/wallforge/internal/config"
	"github.com/Vaniell0/wallforge/internal/engine"
	"github.com/Vaniell0/wallforge/internal/library"
	"github.com/Vaniell0/wallforge/internal/state"
	"github.com/Vaniell0/wallforge/internal/steam"
	"github.com/Vaniell0/wallforge/internal/watchdog"
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

	// libraryIndex maps library Item.ID (short hash) to its absolute
	// path. Refreshed on every /api/library request so the preview
	// handler never serves arbitrary paths — only ones we've just
	// indexed. Mutex-guarded because handlers run concurrently.
	mu           sync.Mutex
	libraryIndex map[string]string

	// userPaused tracks "the user clicked Pause in this UI session" so
	// the status panel can reflect manual intent. The watchdog process
	// runs in a separate unit and may also pause/resume — we don't try
	// to reconcile both views; whoever acted last wins, and the user
	// can always hit Reload to re-read current sysfs/ppd state.
	userPaused bool
}

// New constructs a Server bound to addr. The listener isn't opened until
// Run is called. Warns to stderr when addr resolves to a non-loopback
// host — the API is unauthenticated, exposing it on a public interface
// hands wallforge control to anyone on the network.
func New(cfg config.Config, addr string) (*Server, error) {
	if addr == "" {
		return nil, errors.New("webui: empty addr")
	}
	if !isLoopbackAddr(addr) {
		fmt.Fprintf(os.Stderr,
			"wallforge: WARNING — serving on %q (non-loopback). "+
				"The API has no authentication; anyone reaching this socket "+
				"can change wallpapers and stop backends.\n", addr)
	}
	s := &Server{cfg: cfg, addr: addr, libraryIndex: map[string]string{}}
	s.http = &http.Server{
		Addr:              addr,
		Handler:           s.routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s, nil
}

// isLoopbackAddr reports whether the host portion of a bind addr
// (e.g. "127.0.0.1:7777") is a loopback. Empty / wildcard hosts
// ("", "0.0.0.0", "::") count as non-loopback because they bind on
// every interface.
func isLoopbackAddr(addr string) bool {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Bare ":7777" splits to host="", which means wildcard.
		return false
	}
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	// Hostname like "localhost". Resolve all addrs and require every
	// one of them to be loopback — partial loopback (a hostname that
	// resolves to both 127.0.0.1 and a public IP) is unsafe.
	addrs, err := net.LookupHost(host)
	if err != nil || len(addrs) == 0 {
		return false
	}
	for _, a := range addrs {
		ip := net.ParseIP(a)
		if ip == nil || !ip.IsLoopback() {
			return false
		}
	}
	return true
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
	mux.HandleFunc("GET /api/library", s.handleLibrary)
	mux.HandleFunc("GET /api/status", s.handleStatus)
	mux.HandleFunc("GET /api/power", s.handlePower)
	mux.HandleFunc("GET /preview/{id}", s.handlePreview)
	// State-mutating endpoints go through guardCSRF — see the helper
	// for the full rationale. /api/apply already requires a JSON body
	// (which triggers a CORS preflight), but the others previously
	// accepted simple POSTs that any cross-origin page could fire.
	mux.HandleFunc("POST /api/apply", guardCSRF(s.handleApply))
	mux.HandleFunc("POST /api/stop", guardCSRF(s.handleStop))
	mux.HandleFunc("POST /api/power/pause", guardCSRF(s.handlePowerPause))
	mux.HandleFunc("POST /api/power/resume", guardCSRF(s.handlePowerResume))

	return logRequests(mux)
}

// guardCSRF blocks cross-origin POSTs from drive-by browser tabs.
//
// Modern browsers send Sec-Fetch-Site on every request: same-origin /
// same-site / cross-site / none (curl, fetch from extensions). We
// accept same-origin and "none" (the latter covers direct API clients
// and curl), reject everything else. Browsers without Sec-Fetch
// support fall back to the Origin header — same-host origin only.
//
// This is layered defense: the listener is loopback by default, but
// DNS rebinding + a malicious page in the same browser session could
// still hit 127.0.0.1. Without the guard, /api/power/pause and
// /api/stop are simple-request CSRF targets — no preflight, no cookie
// needed (the API is unauthenticated), just a single fetch().
func guardCSRF(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		site := r.Header.Get("Sec-Fetch-Site")
		if site != "" {
			if site != "same-origin" && site != "none" {
				http.Error(w, "cross-origin requests blocked", http.StatusForbidden)
				return
			}
			next(w, r)
			return
		}
		// Sec-Fetch-Site missing → fall back to Origin.
		origin := r.Header.Get("Origin")
		if origin == "" {
			// No browser context (curl, native client) — allow.
			next(w, r)
			return
		}
		if u, err := url.Parse(origin); err == nil && u.Host == r.Host {
			next(w, r)
			return
		}
		http.Error(w, "cross-origin requests blocked", http.StatusForbidden)
	}
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
	writeJSON(w, map[string]string{"last_applied": s.getLastApplied()})
}

// LibraryItemDTO is the JSON shape for /api/library — essentially
// library.Item without the absolute Path (clients don't need it; the
// apply handler accepts the ID and we resolve server-side).
type LibraryItemDTO struct {
	ID    string `json:"id"`
	Kind  string `json:"kind"`
	Title string `json:"title"`
	Root  string `json:"root"`
}

func (s *Server) handleLibrary(w http.ResponseWriter, r *http.Request) {
	items, err := library.Scan(s.cfg.Library.Roots)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// Refresh the ID→Path map so subsequent /preview/{id} / /api/apply
	// calls can resolve just-scanned items. A client that never calls
	// /api/library can't trigger a library apply — that's by design.
	s.mu.Lock()
	s.libraryIndex = make(map[string]string, len(items))
	for _, it := range items {
		s.libraryIndex[it.ID] = it.Path
	}
	s.mu.Unlock()

	dto := make([]LibraryItemDTO, 0, len(items))
	for _, it := range items {
		dto = append(dto, LibraryItemDTO{
			ID: it.ID, Kind: it.Kind, Title: it.Title, Root: it.Root,
		})
	}
	writeJSON(w, dto)
}

func (s *Server) resolveLibraryID(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.libraryIndex[id]
	return p, ok
}

// getLastApplied / setLastApplied centralise access to the field —
// without these, /api/power (read every 30s by every open tab) races
// against /api/apply (write on every click). go test -race confirms.
func (s *Server) getLastApplied() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.lastApplied
}

func (s *Server) setLastApplied(v string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastApplied = v
}

// handlePreview serves a thumbnail for a single item. Steam Workshop
// items look up project.json for the preview filename; library items
// just serve the image/video itself. Either way we look up the path
// server-side rather than trusting a URL fragment — no raw filesystem
// paths flow through the handler boundary.
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if strings.HasPrefix(id, "lib_") {
		path, ok := s.resolveLibraryID(id)
		if !ok {
			http.Error(w, "unknown library id (did you call /api/library first?)", http.StatusNotFound)
			return
		}
		http.ServeFile(w, r, path)
		return
	}
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
	// Library IDs are server-side only — the client sends the ID back
	// and we translate to the absolute path before handing off to apply.
	if strings.HasPrefix(req.Input, "lib_") {
		path, ok := s.resolveLibraryID(req.Input)
		if !ok {
			http.Error(w, "unknown library id (did you call /api/library first?)", http.StatusNotFound)
			return
		}
		req.Input = path
	}
	res, err := apply.ByInput(s.cfg, req.Input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setLastApplied(req.Input)
	s.mu.Lock()
	s.userPaused = false
	s.mu.Unlock()
	writeJSON(w, res)
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	errs := engine.StopAll(s.cfg)
	s.mu.Lock()
	s.userPaused = true
	s.mu.Unlock()
	if len(errs) == 0 {
		s.setLastApplied("")
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	writeJSON(w, map[string]any{"ok": false, "errors": msgs})
}

// PowerDTO is the JSON shape returned by /api/power. Mirrors the
// snapshot the watchdog acts on plus the UI-tracked manual pause flag,
// so the front-end can render a single status strip without doing
// detection itself.
type PowerDTO struct {
	AC               bool   `json:"ac"`
	Profile          string `json:"profile"`
	PowerSaverPolicy string `json:"power_saver_policy"`
	Mode             string `json:"mode"`   // normal | low-power | paused
	Reason           string `json:"reason"` // empty for normal
	UserPaused       bool   `json:"user_paused"`
	LastApplied      string `json:"last_applied"`
}

func (s *Server) currentPower() PowerDTO {
	// We don't keep a Watchdog instance around — the serve unit is
	// separate from the watchdog unit — so we just call the same
	// detection helpers. Polling on demand is cheap (one sysfs read +
	// one short subprocess) and keeps the UI honest about the current
	// state regardless of whether the watchdog is even running.
	policy := watchdog.ParsePolicy(s.cfg.Watchdog.PowerSaverPolicy)
	w := watchdog.New(0, policy, nil)
	snap := w.Snapshot()
	mode, reason := watchdog.EffectiveMode(snap, policy)

	s.mu.Lock()
	user := s.userPaused
	last := s.lastApplied
	s.mu.Unlock()

	return PowerDTO{
		AC:               snap.Power != watchdog.StateBattery,
		Profile:          snap.Profile.String(),
		PowerSaverPolicy: policy.String(),
		Mode:             mode.String(),
		Reason:           reason,
		UserPaused:       user,
		LastApplied:      last,
	}
}

func (s *Server) handlePower(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.currentPower())
}

func (s *Server) handlePowerPause(w http.ResponseWriter, r *http.Request) {
	errs := engine.StopAll(s.cfg)
	s.mu.Lock()
	s.userPaused = true
	s.mu.Unlock()
	if len(errs) == 0 {
		writeJSON(w, map[string]any{"ok": true})
		return
	}
	msgs := make([]string, 0, len(errs))
	for _, e := range errs {
		msgs = append(msgs, e.Error())
	}
	// Even on partial errors we mark the UI as paused — the user's
	// intent was to stop, and at least some backends did.
	writeJSON(w, map[string]any{"ok": false, "errors": msgs})
}

func (s *Server) handlePowerResume(w http.ResponseWriter, r *http.Request) {
	// Resume target priority: lastApplied (this serve session) → state
	// file (persisted across processes). Falling back to the state
	// file means clicking Resume right after `wallforge serve` starts
	// still does the right thing.
	target := s.getLastApplied()
	if target == "" {
		entry, err := state.Load()
		if err == nil {
			target = entry.Input
		}
	}
	if target == "" {
		http.Error(w, "no wallpaper to resume — apply one first", http.StatusBadRequest)
		return
	}
	res, err := apply.ByInput(s.cfg, target)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	s.setLastApplied(target)
	s.mu.Lock()
	s.userPaused = false
	s.mu.Unlock()
	writeJSON(w, res)
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
