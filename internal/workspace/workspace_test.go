package workspace

import (
	"context"
	"io"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	in := Bindings{ByWorkspace: map[string]string{
		"1":   "/path/to/pic.png",
		"web": "1234567890",
	}}
	if err := Save(in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.ByWorkspace) != 2 {
		t.Fatalf("want 2 bindings, got %d", len(got.ByWorkspace))
	}
	if got.ByWorkspace["1"] != "/path/to/pic.png" || got.ByWorkspace["web"] != "1234567890" {
		t.Errorf("bindings mismatch: %+v", got.ByWorkspace)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.ByWorkspace == nil {
		t.Fatal("ByWorkspace map should be non-nil even when file is missing")
	}
	if len(got.ByWorkspace) != 0 {
		t.Errorf("got %d bindings, want 0", len(got.ByWorkspace))
	}
}

func TestParseEvent(t *testing.T) {
	tests := []struct {
		line, event, data string
	}{
		{"workspace>>1", "workspace", "1"},
		{"workspacev2>>3,web", "workspacev2", "3,web"},
		{"activewindow>>", "activewindow", ""},
		{"noarrow", "noarrow", ""},
		{"", "", ""},
	}
	for _, tc := range tests {
		ev, data := ParseEvent(tc.line)
		if ev != tc.event || data != tc.data {
			t.Errorf("ParseEvent(%q) = (%q, %q), want (%q, %q)", tc.line, ev, data, tc.event, tc.data)
		}
	}
}

func TestWorkspaceIDFromEvent(t *testing.T) {
	tests := []struct {
		event, data string
		want        string
		ok          bool
	}{
		{"workspace", "1", "1", true},
		{"workspace", "web", "web", true},
		{"workspacev2", "3,web", "web", true},
		{"workspacev2", "3", "3", true}, // malformed but don't crash
		{"activewindow", "foo", "", false},
		{"", "anything", "", false},
	}
	for _, tc := range tests {
		got, ok := WorkspaceIDFromEvent(tc.event, tc.data)
		if got != tc.want || ok != tc.ok {
			t.Errorf("WorkspaceIDFromEvent(%q, %q) = (%q, %v), want (%q, %v)",
				tc.event, tc.data, got, ok, tc.want, tc.ok)
		}
	}
}

func TestRunner_AppliesOnWorkspaceSwitch(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := Save(Bindings{ByWorkspace: map[string]string{
		"1":   "/pic1.png",
		"web": "/pic-web.png",
	}}); err != nil {
		t.Fatal(err)
	}

	var (
		mu      sync.Mutex
		applied []string
	)
	runner := NewRunner(func(input string) error {
		mu.Lock()
		defer mu.Unlock()
		applied = append(applied, input)
		return nil
	})

	events := strings.Join([]string{
		"workspace>>1",
		"activewindow>>ignored,ignored",
		"workspacev2>>3,web",
		"workspace>>unknown", // no binding — should be skipped
		"workspace>>1",       // same as last applied for "1" — deduped
	}, "\n") + "\n"

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if err := runner.Run(ctx, io.NopCloser(strings.NewReader(events))); err != nil {
		t.Fatalf("Run: %v", err)
	}

	mu.Lock()
	got := append([]string(nil), applied...)
	mu.Unlock()

	want := []string{"/pic1.png", "/pic-web.png", "/pic1.png"}
	// dedup kicks in only when last input equals new — applying /pic1,
	// then /pic-web, then /pic1 again is allowed (last was /pic-web).
	if len(got) != len(want) {
		t.Fatalf("applied %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("applied[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestRunner_IgnoresApplyErrors(t *testing.T) {
	t.Setenv("XDG_STATE_HOME", t.TempDir())

	if err := Save(Bindings{ByWorkspace: map[string]string{"1": "bad"}}); err != nil {
		t.Fatal(err)
	}

	runner := NewRunner(func(input string) error {
		return io.EOF // arbitrary non-nil
	})

	// Single event; Run should return normally on EOF of reader even
	// after the apply function errors.
	err := runner.Run(context.Background(), strings.NewReader("workspace>>1\n"))
	if err != nil {
		t.Errorf("Run returned %v, want nil (apply errors should be swallowed)", err)
	}
}
