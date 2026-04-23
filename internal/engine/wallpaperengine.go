package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/vaniello/wallforge/internal/config"
)

// WallpaperEngine drives Almamu/linux-wallpaperengine for Steam Workshop
// scene and web wallpapers (the native WE format).
//
// lwpe is a foreground process — like mpvpaper, we detach it (setsid) and
// kill previous instances with pkill. The binary accepts a workshop
// project directory as positional argument and options for screen selection
// and FPS capping.
type WallpaperEngine struct {
	screen string
	fpsCap int
	silent bool
}

func NewWallpaperEngine(cfg config.WpeConfig) *WallpaperEngine {
	return &WallpaperEngine{
		screen: cfg.Screen,
		fpsCap: cfg.Fps,
		silent: cfg.Silent,
	}
}

func (w *WallpaperEngine) Name() string { return "linux-wallpaperengine" }

// Apply renders the project directory referenced by path. The directory
// must contain a project.json (validated by the caller via engine.Detect).
func (w *WallpaperEngine) Apply(path string) error {
	if _, err := exec.LookPath("linux-wallpaperengine"); err != nil {
		return fmt.Errorf(
			"linux-wallpaperengine not found in PATH: %w\n\n"+
				"Build it with `nix build .#linux-wallpaperengine` from the "+
				"project root, or include it in your system packages.", err)
	}
	_ = w.Stop()

	// Without --screen-root lwpe opens a plain window instead of taking
	// over the wallpaper layer. Honor an explicit config value; otherwise
	// ask Hyprland for the active monitors and apply to all of them.
	screens := []string{}
	if w.screen != "" {
		screens = []string{w.screen}
	} else {
		screens = detectHyprlandMonitors()
	}

	args := []string{}
	if w.fpsCap > 0 {
		args = append(args, "--fps", fmt.Sprintf("%d", w.fpsCap))
	}
	if w.silent {
		args = append(args, "--silent")
	}
	if len(screens) == 0 {
		// Last-resort: let lwpe fall back to whatever it picks. Will
		// almost certainly open a window, but better to run than error
		// out — tells the user what to set in config.
		args = append(args, path)
	} else {
		for _, s := range screens {
			args = append(args, "--screen-root", s, "--bg", path)
		}
	}

	cmd := exec.Command("linux-wallpaperengine", args...)

	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open /dev/null: %w", err)
	}
	defer devnull.Close()
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start linux-wallpaperengine: %w", err)
	}
	return cmd.Process.Release()
}

func (w *WallpaperEngine) Stop() error {
	// /proc/<pid>/comm is truncated at TASK_COMM_LEN=15, so
	// `pkill -x linux-wallpaperengine` (21 chars) never matches. `pkill -f`
	// over a broad pattern also catches shells whose argv happens to
	// contain the string. Walk /proc by hand and match on comm directly.
	return killByComm("linux-wallpaper")
}

// killByComm sends SIGTERM to every process whose /proc/<pid>/comm equals
// name. Kernel comm is at most 15 characters (TASK_COMM_LEN - 1), so
// callers must truncate longer names to match.
func killByComm(name string) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	var lastErr error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/comm")
		if err != nil {
			continue
		}
		got := string(data)
		if len(got) > 0 && got[len(got)-1] == '\n' {
			got = got[:len(got)-1]
		}
		if got != name {
			continue
		}
		pid, err := parsePID(e.Name())
		if err != nil {
			continue
		}
		if p, err := os.FindProcess(pid); err == nil {
			if err := p.Signal(syscall.SIGTERM); err != nil &&
				err.Error() != "os: process already finished" {
				lastErr = err
			}
		}
	}
	return lastErr
}

func parsePID(s string) (int, error) {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, fmt.Errorf("not a pid: %q", s)
		}
		n = n*10 + int(c-'0')
	}
	return n, nil
}

// detectHyprlandMonitors asks hyprctl for the active output names.
// Returns nil if hyprctl isn't available or returns non-JSON — callers
// treat the empty slice as "don't inject --screen-root".
func detectHyprlandMonitors() []string {
	out, err := exec.Command("hyprctl", "monitors", "-j").Output()
	if err != nil {
		return nil
	}
	var monitors []struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(out, &monitors); err != nil {
		return nil
	}
	names := make([]string, 0, len(monitors))
	for _, m := range monitors {
		if m.Name != "" {
			names = append(names, m.Name)
		}
	}
	return names
}
