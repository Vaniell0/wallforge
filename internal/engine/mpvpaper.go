package engine

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"github.com/Vaniell0/wallforge/internal/config"
)

// Mpvpaper drives the `mpvpaper` tool for video and GIF wallpapers.
//
// We deliberately omit mpvpaper's -f (self-fork) flag: with -f the
// short-lived parent that we niced exits, and the daemonized child
// keeps decoding at default priority. Letting mpvpaper run in the
// "foreground" + Setsid + Process.Release achieves the same detach
// without losing the niceness we just set. Children inherit the
// pgid, so PRIO_PGRP catches every helper thread mpv forks.
type Mpvpaper struct {
	target  string
	mpvOpts string
	nice    int
}

func NewMpvpaper(cfg config.MpvpaperConfig) *Mpvpaper {
	return &Mpvpaper{target: cfg.Target, mpvOpts: cfg.MpvOpts, nice: cfg.Nice}
}

func (m *Mpvpaper) Name() string { return "mpvpaper" }

func (m *Mpvpaper) Apply(path string) error {
	// Ensure no previous instance is running before spawning a new one —
	// mpvpaper will happily launch multiple copies and leak GPU memory.
	_ = m.Stop()

	args := []string{
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
	// Setsid creates a new session AND new process group with PGID =
	// child PID. setNicePGroup below uses that.
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start mpvpaper: %w", err)
	}
	// Renice the whole process group so any decoder / GL helper
	// threads mpv forks land politely too. Errors only log.
	if err := setNicePGroup(cmd.Process.Pid, m.nice); err != nil {
		fmt.Fprintf(os.Stderr, "wallforge mpvpaper: %v\n", err)
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
