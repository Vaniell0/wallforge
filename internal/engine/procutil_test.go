package engine

import (
	"os/exec"
	"syscall"
	"testing"
)

// niceOf returns the conventional nice value [-20, 19] for pid by
// inverting the Linux getpriority encoding (which returns 20 - nice
// in [1, 40] for backward POSIX compatibility).
func niceOf(t *testing.T, pid int) int {
	t.Helper()
	prio, err := syscall.Getpriority(syscall.PRIO_PROCESS, pid)
	if err != nil {
		t.Fatalf("getpriority(%d): %v", pid, err)
	}
	return 20 - prio
}

func TestSetNice_Zero(t *testing.T) {
	// nice=0 must short-circuit — no syscall fired, so even an invalid
	// pid is fine. Guards against spurious EPERM in restricted CI.
	if err := setNice(1, 0); err != nil {
		t.Errorf("setNice(_, 0) = %v, want nil", err)
	}
}

func TestSetNice_AdjustsChild(t *testing.T) {
	// Start a sleep we own and verify setNice actually moves the
	// kernel value. Positive deltas are always permitted.
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()

	before := niceOf(t, cmd.Process.Pid)
	if err := setNice(cmd.Process.Pid, before+5); err != nil {
		t.Fatalf("setNice: %v", err)
	}
	after := niceOf(t, cmd.Process.Pid)
	if after != before+5 {
		t.Errorf("nice after setNice = %d, want %d", after, before+5)
	}
}

func TestSetNice_Clamp(t *testing.T) {
	// Out-of-range values must be clamped silently — the kernel would
	// EINVAL on raw 999, but our wrapper lands on 19.
	cmd := exec.Command("sleep", "60")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	if err := setNice(cmd.Process.Pid, 999); err != nil {
		t.Errorf("setNice(clamp) = %v, want nil", err)
	}
	if got := niceOf(t, cmd.Process.Pid); got != 19 {
		t.Errorf("nice after clamp = %d, want 19", got)
	}
}

func TestSetNice_ShortLivedExited(t *testing.T) {
	// Setting nice on a pid that already exited must return ESRCH
	// without crashing or applying to a recycled pid. mpvpaper-with-f
	// would hit this every Apply before the H2 fix; keep a regression
	// test now that the fix removed -f.
	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sh: %v", err)
	}
	pid := cmd.Process.Pid
	if _, err := cmd.Process.Wait(); err != nil {
		t.Fatalf("wait: %v", err)
	}
	// pid is now reapable / reaped. setNice must surface an error,
	// not panic, not silently no-op.
	err := setNice(pid, 5)
	if err == nil {
		t.Error("setNice on exited pid returned nil; want ESRCH-style error")
	}
}

func TestSetNicePGroup_AdjustsLeader(t *testing.T) {
	// Spawn a sleep with Setsid so it's its own session leader (and
	// pgid). PRIO_PGRP on its pid must move the leader's nice — same
	// observable as PRIO_PROCESS for a single-process group, but
	// confirms the codepath.
	cmd := exec.Command("sleep", "60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		t.Fatalf("start sleep: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	}()
	before := niceOf(t, cmd.Process.Pid)
	if err := setNicePGroup(cmd.Process.Pid, before+7); err != nil {
		t.Fatalf("setNicePGroup: %v", err)
	}
	after := niceOf(t, cmd.Process.Pid)
	if after != before+7 {
		t.Errorf("nice after PRIO_PGRP = %d, want %d", after, before+7)
	}
}
