package engine

import (
	"context"
	"fmt"
	"os/exec"
	"time"

	"github.com/Vaniell0/wallforge/internal/config"
)

// Swww drives the swww fork ("awww" in nixpkgs) for static image wallpapers.
// The daemon is started lazily on first Apply; Stop kills it.
//
// NixOS nixpkgs renamed the package and binaries to `awww` / `awww-daemon`.
// We use those names directly — upstream users can symlink if needed.
type Swww struct {
	cli        string
	daemon     string
	transition string
	duration   string
}

func NewSwww(cfg config.SwwwConfig) *Swww {
	return &Swww{
		cli:        "awww",
		daemon:     "awww-daemon",
		transition: cfg.Transition,
		duration:   cfg.Duration,
	}
}

func (s *Swww) Name() string { return "swww" }

func (s *Swww) Apply(path string) error {
	if err := s.ensureDaemon(); err != nil {
		return fmt.Errorf("%s daemon: %w", s.cli, err)
	}
	cmd := exec.Command(s.cli, "img",
		"--transition-type", s.transition,
		"--transition-duration", s.duration,
		path)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s img failed: %w: %s", s.cli, err, out)
	}
	return nil
}

func (s *Swww) Stop() error {
	cmd := exec.Command(s.cli, "kill")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("%s kill: %w: %s", s.cli, err, out)
	}
	return nil
}

// ensureDaemon starts awww-daemon if it is not already running.
// `awww query` succeeds only when the daemon is live.
func (s *Swww) ensureDaemon() error {
	if exec.Command(s.cli, "query").Run() == nil {
		return nil
	}
	if err := exec.Command(s.daemon).Start(); err != nil {
		return fmt.Errorf("start %s: %w", s.daemon, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for {
		if exec.Command(s.cli, "query").Run() == nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("%s did not become ready in time", s.daemon)
		case <-time.After(50 * time.Millisecond):
		}
	}
}
