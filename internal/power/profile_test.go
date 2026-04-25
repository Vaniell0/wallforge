package power

import (
	"errors"
	"os/exec"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		in   string
		want Profile
	}{
		{"performance", ProfilePerformance},
		{"balanced", ProfileBalanced},
		{"power-saver", ProfilePowerSaver},
		{"  performance\n", ProfilePerformance},
		{"PERFORMANCE", ProfilePerformance},
		{"", ProfileUnknown},
		{"low-latency", ProfileUnknown}, // hypothetical future profile
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			if got := Parse(c.in); got != c.want {
				t.Errorf("Parse(%q) = %v, want %v", c.in, got, c.want)
			}
		})
	}
}

func TestProfileString(t *testing.T) {
	cases := map[Profile]string{
		ProfileUnknown:     "unknown",
		ProfilePerformance: "performance",
		ProfileBalanced:    "balanced",
		ProfilePowerSaver:  "power-saver",
	}
	for p, want := range cases {
		if got := p.String(); got != want {
			t.Errorf("Profile(%d).String() = %q, want %q", p, got, want)
		}
	}
}

// stubRunner returns a runner that produces a fake exec.Cmd whose
// Output() yields the supplied stdout. We achieve that by wiring
// `echo -n <out>` through /bin/sh — every NixOS dev env has both, and
// it keeps the test from hard-depending on a writable temp script.
func stubRunner(out string) runner {
	return func(name string, args ...string) *exec.Cmd {
		// We're meant to run `powerprofilesctl get`; replace with echo.
		_ = name
		_ = args
		return exec.Command("printf", "%s", out)
	}
}

func TestDetectWith_KnownProfiles(t *testing.T) {
	cases := []struct {
		out  string
		want Profile
	}{
		{"performance\n", ProfilePerformance},
		{"balanced", ProfileBalanced},
		{"power-saver\n", ProfilePowerSaver},
		{"future-mode", ProfileUnknown},
	}
	for _, c := range cases {
		t.Run(c.out, func(t *testing.T) {
			got, err := detectWith(stubRunner(c.out))
			if err != nil {
				t.Fatalf("detectWith: %v", err)
			}
			if got != c.want {
				t.Errorf("detectWith = %v, want %v", got, c.want)
			}
		})
	}
}

func TestDetectWith_NotInstalled(t *testing.T) {
	missing := func(name string, args ...string) *exec.Cmd {
		_ = name
		_ = args
		return exec.Command("/this/binary/does/not/exist/powerprofilesctl-stub")
	}
	got, err := detectWith(missing)
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
	if got != ProfileUnknown {
		t.Errorf("got = %v, want ProfileUnknown", got)
	}
}
