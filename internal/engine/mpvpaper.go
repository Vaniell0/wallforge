package engine

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/vaniello/wallforge/internal/config"
)

// Mpvpaper drives the `mpvpaper` tool for video and GIF wallpapers.
//
// Unlike swww/awww, mpvpaper is not a daemon — each invocation spawns a
// foreground process that must be detached (setsid) and re-spawned on every
// Apply. Stop uses `pkill` to terminate any running mpvpaper instances.
type Mpvpaper struct {
	target  string
	mpvOpts string
}

func NewMpvpaper(cfg config.MpvpaperConfig) *Mpvpaper {
	return &Mpvpaper{target: cfg.Target, mpvOpts: cfg.MpvOpts}
}

func (m *Mpvpaper) Name() string { return "mpvpaper" }

func (m *Mpvpaper) Apply(path string) error {
	// Ensure no previous instance is running before spawning a new one —
	// mpvpaper will happily launch multiple copies and leak GPU memory.
	_ = m.Stop()

	args := []string{
		"-f", // fork into background after window attach
		"-o", m.mpvOpts,
	}
	if m.target == "" {
		args = append(args, "*")
	} else {
		args = append(args, m.target)
	}
	args = append(args, path)

	cmd := exec.Command("mpvpaper", args...)

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
		return fmt.Errorf("start mpvpaper: %w", err)
	}
	// Detach so the child survives after the CLI exits.
	return cmd.Process.Release()
}

func (m *Mpvpaper) Stop() error {
	// pkill -x fails here too: nixpkgs wraps mpvpaper via makeWrapper,
	// so the running process is actually `.mpvpaper-wrapped` (17 chars,
	// truncated to `.mpvpaper-wrapp` in /proc/<pid>/comm). Match by the
	// real executable basename via /proc/<pid>/exe instead.
	return killByExeName("mpvpaper")
}
