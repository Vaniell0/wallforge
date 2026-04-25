// Package power detects the active system power profile via
// power-profiles-daemon (`powerprofilesctl get`). The watchdog and the
// web-UI use this alongside the AC/battery sysfs probe so wallforge can
// pause expensive backends when the user manually picks the power-saver
// profile, not just when the laptop is unplugged.
//
// Why shell out instead of subscribing to the D-Bus signal: the binary
// is a thin wrapper around the same call, sticking to exec keeps
// wallforge stdlib-only (no godbus dependency) and matches the existing
// posture of every other backend caller in this repo.
package power

import (
	"errors"
	"io/fs"
	"os/exec"
	"strings"
)

// Profile is the ppd profile name normalised to a finite enum so callers
// don't have to string-compare. Unknown covers both "ppd not installed"
// and "ppd returned a name we haven't seen" — callers should treat both
// as "no opinion, leave the system alone".
type Profile int

const (
	ProfileUnknown Profile = iota
	ProfilePerformance
	ProfileBalanced
	ProfilePowerSaver
)

func (p Profile) String() string {
	switch p {
	case ProfilePerformance:
		return "performance"
	case ProfileBalanced:
		return "balanced"
	case ProfilePowerSaver:
		return "power-saver"
	}
	return "unknown"
}

// ErrNotInstalled is returned when `powerprofilesctl` is missing from
// PATH. Callers can treat it as "ppd not configured on this host" and
// fall back to AC/battery-only logic.
var ErrNotInstalled = errors.New("power: powerprofilesctl not found in PATH")

// runner abstracts exec.Command so tests can stub the shell-out without
// needing the real binary. The default mirrors os/exec exactly.
type runner func(name string, args ...string) *exec.Cmd

var defaultRunner runner = exec.Command

// Detect runs `powerprofilesctl get` and parses its output. Errors are
// returned untouched so callers can distinguish "ppd not installed"
// (ErrNotInstalled) from "ppd ran but failed" — the watchdog cares
// about the difference: missing tool is benign, runtime failure is a
// log-worthy event.
func Detect() (Profile, error) {
	return detectWith(defaultRunner)
}

func detectWith(run runner) (Profile, error) {
	cmd := run("powerprofilesctl", "get")
	out, err := cmd.Output()
	if err != nil {
		// "Not installed" can surface two ways: LookPath fails before
		// fork (*exec.Error wrapping ErrNotFound) or fork/exec returns
		// ENOENT for an explicit absolute path. Both mean the user
		// has no ppd installed; map them to ErrNotInstalled so the
		// watchdog can downgrade gracefully.
		var execErr *exec.Error
		var pathErr *fs.PathError
		if errors.As(err, &execErr) || errors.As(err, &pathErr) || errors.Is(err, exec.ErrNotFound) {
			return ProfileUnknown, ErrNotInstalled
		}
		return ProfileUnknown, err
	}
	return Parse(strings.TrimSpace(string(out))), nil
}

// Parse maps a ppd profile name to the typed enum. Exposed so tests and
// future callers (e.g. UI rendering) don't have to duplicate the table.
// Unknown names map to ProfileUnknown rather than erroring — ppd may
// add new profiles and we don't want a rename to break the watchdog.
func Parse(s string) Profile {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "performance":
		return ProfilePerformance
	case "balanced":
		return ProfileBalanced
	case "power-saver":
		return ProfilePowerSaver
	}
	return ProfileUnknown
}
