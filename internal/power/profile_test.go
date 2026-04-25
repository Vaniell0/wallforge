package power

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"
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
// Output() yields the supplied stdout. We swap powerprofilesctl with
// `printf %s <out>` — fast and present in every dev environment.
func stubRunner(out string) runner {
	return func(ctx context.Context, name string, args ...string) *exec.Cmd {
		_ = name
		_ = args
		return exec.CommandContext(ctx, "printf", "%s", out)
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
	missing := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		_ = name
		_ = args
		return exec.CommandContext(ctx, "/this/binary/does/not/exist/powerprofilesctl-stub")
	}
	got, err := detectWith(missing)
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("err = %v, want ErrNotInstalled", err)
	}
	if got != ProfileUnknown {
		t.Errorf("got = %v, want ProfileUnknown", got)
	}
}

func TestDetectWith_Timeout(t *testing.T) {
	// Run a command that sleeps past the 2s detectTimeout. We don't
	// override detectTimeout to keep this honest about the production
	// behaviour, but use t.Parallel-incompatible wallclock waits — so
	// the suite stays serial. ~2.1s.
	if testing.Short() {
		t.Skip("skipping 2s timeout test in -short mode")
	}
	stubSleep := func(ctx context.Context, name string, args ...string) *exec.Cmd {
		_ = name
		_ = args
		return exec.CommandContext(ctx, "sleep", "5")
	}
	start := time.Now()
	got, err := detectWith(stubSleep)
	elapsed := time.Since(start)

	if !errors.Is(err, ErrTimeout) {
		t.Fatalf("err = %v, want ErrTimeout", err)
	}
	if got != ProfileUnknown {
		t.Errorf("got = %v, want ProfileUnknown", got)
	}
	if elapsed > 3*time.Second {
		t.Errorf("detectWith blocked %s past timeout (want ≤3s)", elapsed)
	}
}
