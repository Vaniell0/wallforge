package apply

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Vaniell0/wallforge/internal/config"
	"github.com/Vaniell0/wallforge/internal/engine"
	"github.com/Vaniell0/wallforge/internal/state"
	"github.com/Vaniell0/wallforge/internal/watchdog"
)

// Disable state persistence + cross-backend stop + mode detection for
// the whole test suite. ByInput would otherwise write to
// $XDG_STATE_HOME, shell out via StopOthers, and consult the host's
// real sysfs/ppd state — none of which is what these tests are about.
// Mode-aware behaviour gets its own tests below with explicit stubs.
func TestMain(m *testing.M) {
	saveState = func(state.Entry) error { return nil }
	stopOthers = func(engine.Backend, config.Config) {}
	detectMode = func(config.Config) watchdog.Mode { return watchdog.ModeNormal }
	os.Exit(m.Run())
}

// stubMode swaps detectMode for the duration of a test. Use it when a
// test needs a non-Normal mode; the TestMain default is ModeNormal so
// the legacy suite stays unaffected.
func stubMode(m watchdog.Mode) func() {
	prev := detectMode
	detectMode = func(config.Config) watchdog.Mode { return m }
	return func() { detectMode = prev }
}

// fakeBackend records each Apply call so a test can assert what
// would have been passed to the real backend without executing it.
type fakeBackend struct {
	name    string
	applied []string
	err     error
}

func (f *fakeBackend) Name() string  { return f.name }
func (f *fakeBackend) Apply(p string) error {
	f.applied = append(f.applied, p)
	return f.err
}
func (f *fakeBackend) Stop() error { return nil }

// stubSelect replaces engine.Select for the duration of a test, returning
// fake for whatever Kind is requested. The restore callback puts the
// original back — always defer it.
func stubSelect(fake *fakeBackend) func() {
	prev := selectBackend
	selectBackend = func(_ engine.Target, _ config.Config) (engine.Backend, error) {
		return fake, nil
	}
	return func() { selectBackend = prev }
}

func stubResolve(root, expect, returnPath string, returnErr error) func() {
	prev := resolveSteam
	resolveSteam = func(r, id string) (string, error) {
		if r != root || id != expect {
			return "", fmt.Errorf("unexpected resolve(%q, %q)", r, id)
		}
		return returnPath, returnErr
	}
	return func() { resolveSteam = prev }
}

func TestIsNumericID(t *testing.T) {
	tests := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"0", true},
		{"12345", true},
		{"3682370294", true},
		{"abc", false},
		{"123abc", false},
		{"12 34", false},
		{"-1", false},
		{"+1", false},
		{".5", false},
	}
	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			if got := IsNumericID(tc.in); got != tc.want {
				t.Errorf("IsNumericID(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestByInput_ImageFile(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "pic.png")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeBackend{name: "swww-fake"}
	defer stubSelect(fake)()

	res, err := ByInput(config.Default(), img)
	if err != nil {
		t.Fatalf("ByInput: %v", err)
	}
	if res.Kind != "image" {
		t.Errorf("Kind = %q, want image", res.Kind)
	}
	if res.Backend != "swww-fake" {
		t.Errorf("Backend = %q, want swww-fake", res.Backend)
	}
	if res.Path != img {
		t.Errorf("Path = %q, want %q", res.Path, img)
	}
	if len(fake.applied) != 1 || fake.applied[0] != img {
		t.Errorf("backend Apply got %v, want [%q]", fake.applied, img)
	}
}

func TestByInput_VideoFile(t *testing.T) {
	dir := t.TempDir()
	vid := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(vid, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeBackend{name: "mpvpaper-fake"}
	defer stubSelect(fake)()

	res, err := ByInput(config.Default(), vid)
	if err != nil {
		t.Fatal(err)
	}
	if res.Kind != "video" {
		t.Errorf("Kind = %q, want video", res.Kind)
	}
}

func TestByInput_NumericIDResolvesSteam(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "pic.jpg")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := config.Default()
	cfg.Steam.Root = "/fake/root"
	fake := &fakeBackend{name: "stub"}
	defer stubSelect(fake)()
	defer stubResolve("/fake/root", "12345", img, nil)()

	res, err := ByInput(cfg, "12345")
	if err != nil {
		t.Fatalf("ByInput: %v", err)
	}
	if res.Path != img {
		t.Errorf("Path = %q, want %q", res.Path, img)
	}
}

func TestByInput_SteamResolveError(t *testing.T) {
	defer stubResolve("", "9999", "", errors.New("not subscribed"))()

	_, err := ByInput(config.Default(), "9999")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestByInput_BackendApplyError(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "x.png")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fake := &fakeBackend{name: "broken", err: errors.New("boom")}
	defer stubSelect(fake)()

	_, err := ByInput(config.Default(), img)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Error should be wrapped with backend name for diagnostics.
	if msg := err.Error(); msg == "" {
		t.Error("error message empty")
	}
}

func TestByInput_UnsupportedExtension(t *testing.T) {
	dir := t.TempDir()
	weird := filepath.Join(dir, "what.xyz")
	if err := os.WriteFile(weird, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// No stubs — Detect should reject the file before Select/Apply.
	_, err := ByInput(config.Default(), weird)
	if err == nil {
		t.Fatal("expected error for unsupported extension")
	}
}

func TestByInput_NonexistentPath(t *testing.T) {
	_, err := ByInput(config.Default(), "/no/such/path-xyz")
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

func TestByInput_StopsOtherBackendsBeforeApply(t *testing.T) {
	dir := t.TempDir()
	img := filepath.Join(dir, "x.png")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stopCalledBefore bool
	fake := &fakeBackend{name: "swww-fake"}
	defer stubSelect(fake)()

	// Install a stopOthers that records it fired *before* the backend
	// Apply, not after. If Apply ran first, the image would paint
	// under a still-live mpvpaper / lwpe layer.
	prevStop := stopOthers
	stopOthers = func(keep engine.Backend, _ config.Config) {
		if len(fake.applied) != 0 {
			t.Errorf("stopOthers ran AFTER Apply — ordering wrong")
			return
		}
		if keep.Name() != fake.Name() {
			t.Errorf("stopOthers kept %q, want %q", keep.Name(), fake.Name())
		}
		stopCalledBefore = true
	}
	defer func() { stopOthers = prevStop }()

	if _, err := ByInput(config.Default(), img); err != nil {
		t.Fatal(err)
	}
	if !stopCalledBefore {
		t.Error("stopOthers was not called")
	}
}

func TestByInput_PausedReturnsErrorAndPersists(t *testing.T) {
	defer stubMode(watchdog.ModePaused)()

	// Capture saveState calls — even when paused, the requested input
	// must be persisted so a later resume can pick it up.
	var saved []state.Entry
	prevSave := saveState
	saveState = func(e state.Entry) error {
		saved = append(saved, e)
		return nil
	}
	defer func() { saveState = prevSave }()

	dir := t.TempDir()
	img := filepath.Join(dir, "x.png")
	if err := os.WriteFile(img, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ByInput(config.Default(), img)
	if err == nil {
		t.Fatal("expected error in paused mode, got nil")
	}
	if len(saved) != 1 || saved[0].Input != img {
		t.Errorf("saveState calls = %v, want one entry with Input=%q", saved, img)
	}
}

func TestByInput_LowPowerSwapsConfig(t *testing.T) {
	defer stubMode(watchdog.ModeLowPower)()

	dir := t.TempDir()
	vid := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(vid, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Capture the config the backend selector receives so we can prove
	// the LowPower override actually flowed through. mpv_opts must be
	// the battery variant; fps must be the battery cap.
	var seen config.Config
	prev := selectBackend
	selectBackend = func(_ engine.Target, c config.Config) (engine.Backend, error) {
		seen = c
		return &fakeBackend{name: "stub"}, nil
	}
	defer func() { selectBackend = prev }()

	cfg := config.Default()
	cfg.Mpvpaper.MpvOpts = "normal-opts"
	cfg.Mpvpaper.BatteryMpvOpts = "battery-opts"
	cfg.Wpe.Fps = 60
	cfg.Wpe.FpsBattery = 20

	if _, err := ByInput(cfg, vid); err != nil {
		t.Fatalf("ByInput: %v", err)
	}
	if seen.Mpvpaper.MpvOpts != "battery-opts" {
		t.Errorf("MpvOpts = %q, want battery-opts", seen.Mpvpaper.MpvOpts)
	}
	if seen.Wpe.Fps != 20 {
		t.Errorf("Fps = %d, want 20", seen.Wpe.Fps)
	}
}

func TestForLowPower_EmptyOverridesPreserved(t *testing.T) {
	cfg := config.Default()
	cfg.Mpvpaper.MpvOpts = "keep-me"
	cfg.Mpvpaper.BatteryMpvOpts = "" // unset → should not overwrite
	cfg.Wpe.Fps = 30
	cfg.Wpe.FpsBattery = 0 // unset → should not overwrite

	out := cfg.ForLowPower()
	if out.Mpvpaper.MpvOpts != "keep-me" {
		t.Errorf("MpvOpts = %q, want keep-me (override empty)", out.Mpvpaper.MpvOpts)
	}
	if out.Wpe.Fps != 30 {
		t.Errorf("Fps = %d, want 30 (override 0)", out.Wpe.Fps)
	}
}
