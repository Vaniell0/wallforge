package engine

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// killByExeName sends SIGTERM to every process whose argv[0] basename
// equals name.
//
// Why not pkill, comm, or /proc/<pid>/exe?
//   - `pkill -x` compares against /proc/<pid>/comm, which the kernel
//     truncates to TASK_COMM_LEN-1 = 15 chars. Long names like
//     `linux-wallpaperengine` (21) silently never match.
//   - `pkill -f` matches against the full cmdline and easily catches
//     shells whose argv happens to contain the target string — it can
//     kill the caller.
//   - /proc/<pid>/exe points at the real on-disk binary. Nix wraps
//     many packages via makeWrapper; mpvpaper's exe is
//     `.mpvpaper-wrapped`, not `mpvpaper`. That's a true difference
//     we don't want to match against.
//
// /proc/<pid>/cmdline preserves the argv the process was launched with,
// which stays stable through wrapper exec chains and long names alike.
// We compare the basename of argv[0].
func killByExeName(name string) error {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return err
	}
	var lastErr error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		pid, err := parsePID(e.Name())
		if err != nil {
			continue
		}
		data, err := os.ReadFile("/proc/" + e.Name() + "/cmdline")
		if err != nil || len(data) == 0 {
			continue
		}
		// cmdline is NUL-separated argv; take argv[0] only.
		if i := bytes.IndexByte(data, 0); i >= 0 {
			data = data[:i]
		}
		if filepath.Base(string(data)) != name {
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

// setNice lowers the scheduling priority of pid via PRIO_PROCESS.
// Use setNicePGroup instead when the child is going to fork children
// (lwpe spawns CEF helpers, mpv may fork decoder threads).
func setNice(pid, nice int) error {
	return setpriorityClamped(syscall.PRIO_PROCESS, pid, nice)
}

// setNicePGroup lowers the scheduling priority of every process in
// pgid's process group. Used after cmd.Start() with SysProcAttr.Setsid
// (which makes the child a session leader and pgid == pid). Catches
// subprocesses forked between cmd.Start and setpriority — PRIO_PROCESS
// would only renice the original child, leaving CEF helpers / decoder
// threads at default nice. Tests cover the short-lived-pid ESRCH path.
func setNicePGroup(pgid, nice int) error {
	return setpriorityClamped(syscall.PRIO_PGRP, pgid, nice)
}

// setpriorityClamped is the shared implementation. nice=0 is a no-op
// (saves us from spurious EPERM in restricted CI); out-of-range
// values are silently clamped to the kernel-accepted [-20, 19].
// We only ever pass positive values from wallforge — "be polite to
// the foreground" — which needs no capabilities. ESRCH (the target
// already exited) returns the wrapped error so the caller can log it
// without it becoming fatal.
func setpriorityClamped(which, who, nice int) error {
	if nice == 0 {
		return nil
	}
	if nice > 19 {
		nice = 19
	}
	if nice < -20 {
		nice = -20
	}
	if err := syscall.Setpriority(which, who, nice); err != nil {
		return fmt.Errorf("setpriority(which=%d, who=%d, nice=%d): %w", which, who, nice, err)
	}
	return nil
}
