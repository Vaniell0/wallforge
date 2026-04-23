package engine

import (
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

	args := []string{}
	if w.screen != "" {
		args = append(args, "--screen-root", w.screen)
	}
	if w.fpsCap > 0 {
		args = append(args, "--fps", fmt.Sprintf("%d", w.fpsCap))
	}
	if w.silent {
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
