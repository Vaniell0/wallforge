package engine

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// WallpaperEngine drives Almamu/linux-wallpaperengine for Steam Workshop
// scene and web wallpapers (the native WE format).
//
// lwpe is a foreground process — like mpvpaper, we detach it (setsid) and
// kill previous instances with pkill. The binary accepts a workshop
// project directory as positional argument and options for screen selection
// and FPS capping.
type WallpaperEngine struct {
	// Screen selects the target monitor by name (Hyprland / xrandr name).
	// Empty string lets lwpe pick the primary output.
	Screen string

	// FpsCap is the --fps option. 0 leaves it to lwpe's default (30).
	FpsCap int

	// Silent passes --silent to suppress in-scene audio.
	Silent bool
}

func NewWallpaperEngine() *WallpaperEngine {
	return &WallpaperEngine{
		FpsCap: 30,
		Silent: true,
	}
}

func (w *WallpaperEngine) Name() string { return "linux-wallpaperengine" }

// Apply renders the project directory referenced by path. The directory
// must contain a project.json (validated by the caller via engine.Detect).
func (w *WallpaperEngine) Apply(path string) error {
	if _, err := exec.LookPath("linux-wallpaperengine"); err != nil {
		return fmt.Errorf(
			"linux-wallpaperengine not found in PATH: %w\n\n"+
				"This backend is the blocker for Steam Workshop scene/web "+
				"content. A Nix derivation skeleton is provided under "+
				"nix/linux-wallpaperengine.nix — it still needs real hashes "+
				"and a successful first build.", err)
	}
	_ = w.Stop()

	args := []string{}
	if w.Screen != "" {
		args = append(args, "--screen-root", w.Screen)
	}
	if w.FpsCap > 0 {
		args = append(args, "--fps", fmt.Sprintf("%d", w.FpsCap))
	}
	if w.Silent {
		args = append(args, "--silent")
	}
	args = append(args, path)

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
	cmd := exec.Command("pkill", "-x", "linux-wallpaperengine")
	if err := cmd.Run(); err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok && exitErr.ExitCode() == 1 {
			return nil
		}
		return fmt.Errorf("pkill linux-wallpaperengine: %w", err)
	}
	return nil
}
