// Package workspace binds wallpapers to Hyprland workspaces and runs a
// daemon that swaps the wallpaper when the active workspace changes.
//
// Bindings live in $XDG_STATE_HOME/wallforge/workspaces.json and are
// edited with `wallforge workspace bind|unbind|list`. The daemon opens
// Hyprland's .socket2 event stream, watches for workspace switches and
// calls apply.ByInput for the matching input.
package workspace

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// Bindings maps a workspace identifier (numeric ID or named workspace)
// to the wallpaper input that should render on it. The input follows
// the same grammar as `wallforge apply` — path or Steam Workshop ID.
type Bindings struct {
	// ByWorkspace keys on the workspace name/ID Hyprland sends on the
	// event socket. For numbered workspaces that's the decimal as a
	// string ("1", "2"); named workspaces use the raw name ("web", etc).
	ByWorkspace map[string]string `json:"by_workspace"`
}

// Path returns the bindings file location. Parallel to state.Path but
// deliberately in its own file — per-workspace bindings are a
// configuration-ish thing the user edits, not a derived state value
// written on every apply.
func Path() string {
	if x := os.Getenv("XDG_STATE_HOME"); x != "" {
		return filepath.Join(x, "wallforge", "workspaces.json")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return filepath.Join(".", "wallforge-workspaces.json")
	}
	return filepath.Join(home, ".local", "state", "wallforge", "workspaces.json")
}

// Load returns the stored bindings. A missing file returns an empty
// Bindings + nil error — first-time users have nothing to load.
func Load() (Bindings, error) {
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return Bindings{ByWorkspace: map[string]string{}}, nil
		}
		return Bindings{}, fmt.Errorf("read %s: %w", path, err)
	}
	var b Bindings
	if err := json.Unmarshal(data, &b); err != nil {
		return Bindings{}, fmt.Errorf("parse %s: %w", path, err)
	}
	if b.ByWorkspace == nil {
		b.ByWorkspace = map[string]string{}
	}
	return b, nil
}

// Save atomically rewrites the bindings file.
func Save(b Bindings) error {
	path := Path()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	if b.ByWorkspace == nil {
		b.ByWorkspace = map[string]string{}
	}
	data, err := json.MarshalIndent(b, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// SocketPath returns the Hyprland event socket. Hyprland exports
// HYPRLAND_INSTANCE_SIGNATURE in every graphical session; the socket
// lives under $XDG_RUNTIME_DIR/hypr/<signature>/.socket2.sock.
func SocketPath() (string, error) {
	sig := os.Getenv("HYPRLAND_INSTANCE_SIGNATURE")
	if sig == "" {
		return "", errors.New("HYPRLAND_INSTANCE_SIGNATURE not set — is Hyprland running in this session?")
	}
	runDir := os.Getenv("XDG_RUNTIME_DIR")
	if runDir == "" {
		return "", errors.New("XDG_RUNTIME_DIR not set")
	}
	return filepath.Join(runDir, "hypr", sig, ".socket2.sock"), nil
}

// ParseEvent splits a single line of the Hyprland event stream into
// its "EVENT" and "DATA" halves. The separator is ">>" (two chars);
// DATA can contain further "," separators depending on the event.
func ParseEvent(line string) (event, data string) {
	if i := strings.Index(line, ">>"); i >= 0 {
		return line[:i], line[i+2:]
	}
	return line, ""
}

// WorkspaceIDFromEvent pulls the workspace identifier out of a
// "workspace>>..." or "workspacev2>>..." DATA payload. The v2 event is
// "ID,NAME"; the original is just "NAME". We return the name because
// the user writes bindings against whatever identifier Hyprland uses
// in practice — for numeric workspaces the ID and name coincide.
func WorkspaceIDFromEvent(event, data string) (string, bool) {
	switch event {
	case "workspace":
		return data, true
	case "workspacev2":
		// "<id>,<name>" — prefer the name since it's the user-visible
		// label (matches hyprctl output).
		if i := strings.Index(data, ","); i >= 0 {
			return data[i+1:], true
		}
		return data, true
	}
	return "", false
}

// Runner is the per-workspace daemon. apply is the side-effect that
// runs when a switched-to workspace has a binding; the production
// caller passes apply.ByInput (ignoring the Result).
type Runner struct {
	apply func(input string) error

	mu       sync.Mutex
	lastApp  string // de-dupe rapid-fire events that would hit the same binding
	snapshot Bindings
}

// NewRunner returns a Runner that loads bindings lazily on first event.
func NewRunner(apply func(input string) error) *Runner {
	return &Runner{apply: apply}
}

// Run reads newline-delimited events from r and dispatches. The reader
// is typically a unix socket connection; the caller is responsible for
// closing it. Run returns when r reaches EOF or ctx is cancelled.
func (rn *Runner) Run(ctx context.Context, r io.Reader) error {
	scanner := bufio.NewScanner(r)
	// Hyprland event lines are short; the default 64KB buffer is
	// plenty but make the grow-behaviour explicit anyway.
	scanner.Buffer(make([]byte, 0, 64*1024), 64*1024)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rn.handleLine(scanner.Text())
	}
	return scanner.Err()
}

func (rn *Runner) handleLine(line string) {
	event, data := ParseEvent(line)
	ws, ok := WorkspaceIDFromEvent(event, data)
	if !ok {
		return
	}
	// Reload bindings on every event — the user may have edited them
	// while the daemon is running. Cost is negligible (one small JSON
	// read) and avoids needing a watcher for the bindings file.
	b, err := Load()
	if err != nil {
		return
	}
	rn.mu.Lock()
	rn.snapshot = b
	input, hasBinding := b.ByWorkspace[ws]
	lastApplied := rn.lastApp
	rn.mu.Unlock()
	if !hasBinding || input == lastApplied {
		return
	}
	if err := rn.apply(input); err != nil {
		// Silent-skip: a bad binding shouldn't crash the daemon.
		// Real observability lives in the systemd journal via
		// whatever cmdApply already prints.
		return
	}
	rn.mu.Lock()
	rn.lastApp = input
	rn.mu.Unlock()
}

// Dial opens the Hyprland event socket using SocketPath(). Separated
// from Run so tests can substitute an io.Pipe or net.Listen instead.
func Dial() (net.Conn, error) {
	sock, err := SocketPath()
	if err != nil {
		return nil, err
	}
	conn, err := net.Dial("unix", sock)
	if err != nil {
		return nil, fmt.Errorf("dial %s: %w", sock, err)
	}
	return conn, nil
}
