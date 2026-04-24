package library

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScan_ClassifiesByExtension(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "a.png"))
	mustTouch(t, filepath.Join(root, "b.JPG"))     // case-insensitive
	mustTouch(t, filepath.Join(root, "clip.mp4"))
	mustTouch(t, filepath.Join(root, "readme.txt")) // skipped
	mustTouch(t, filepath.Join(root, "no-ext"))     // skipped

	items, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("got %d items, want 3: %+v", len(items), items)
	}

	seen := map[string]string{}
	for _, it := range items {
		seen[it.Title] = it.Kind
	}
	cases := map[string]string{
		"a":    "image",
		"b":    "image",
		"clip": "video",
	}
	for title, wantKind := range cases {
		if seen[title] != wantKind {
			t.Errorf("item %q: kind=%q, want %q", title, seen[title], wantKind)
		}
	}
}

func TestScan_RecursiveWithinDepthLimit(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "top.png"))
	mustTouch(t, filepath.Join(root, "sub", "mid.png"))
	mustTouch(t, filepath.Join(root, "a", "b", "c", "d", "deep.png")) // at depth 5, should be skipped

	items, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	titles := map[string]bool{}
	for _, it := range items {
		titles[it.Title] = true
	}
	if !titles["top"] || !titles["mid"] {
		t.Errorf("missing shallow items: %v", titles)
	}
	if titles["deep"] {
		t.Errorf("depth-6 item should be skipped: %v", titles)
	}
}

func TestScan_MissingRootIgnored(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "real.png"))

	// Mix a non-existent root with a real one — non-existent must be
	// skipped without erroring out the whole scan.
	items, err := Scan([]string{"/does/not/exist", root})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item from real root, got %d", len(items))
	}
}

func TestScan_StableIDAcrossScans(t *testing.T) {
	root := t.TempDir()
	mustTouch(t, filepath.Join(root, "one.png"))

	first, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	second, err := Scan([]string{root})
	if err != nil {
		t.Fatal(err)
	}
	if first[0].ID != second[0].ID {
		t.Errorf("ID drifted between scans: %q vs %q", first[0].ID, second[0].ID)
	}
}

func TestExpandHome(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Skip("no home dir on this runner")
	}
	tests := []struct {
		in   string
		want string
	}{
		{"~", home},
		{"~/", home + "/"}, // filepath.Join removes trailing slash; check prefix below
		{"~/pictures", filepath.Join(home, "pictures")},
		{"/absolute", "/absolute"},
		{"", ""},
		{"relative", "relative"},
	}
	for _, tc := range tests {
		got := ExpandHome(tc.in)
		if tc.in == "~/" {
			// filepath.Join normalises trailing slash — accept either form.
			if got != home && got != home+"/" {
				t.Errorf("ExpandHome(%q) = %q, want home or home+/", tc.in, got)
			}
			continue
		}
		if got != tc.want {
			t.Errorf("ExpandHome(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func mustTouch(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}
