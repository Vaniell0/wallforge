package steam

import (
	"os"
	"path/filepath"
	"testing"
)

func setupFakeSteam(t *testing.T, items map[string]string) string {
	t.Helper()
	root := t.TempDir()
	contentDir := filepath.Join(root, "steamapps", "workshop", "content", WallpaperEngineAppID)
	if err := os.MkdirAll(contentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for id, projectJSON := range items {
		itemDir := filepath.Join(contentDir, id)
		if err := os.MkdirAll(itemDir, 0o755); err != nil {
			t.Fatal(err)
		}
		if projectJSON != "" {
			if err := os.WriteFile(filepath.Join(itemDir, "project.json"),
				[]byte(projectJSON), 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	return root
}

func TestFindWorkshopDir_Override(t *testing.T) {
	root := setupFakeSteam(t, map[string]string{"123": ""})
	got, err := FindWorkshopDir(root)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "steamapps", "workshop", "content", WallpaperEngineAppID)
	if got != want {
		t.Errorf("FindWorkshopDir = %q, want %q", got, want)
	}
}

func TestFindWorkshopDir_Missing(t *testing.T) {
	if _, err := FindWorkshopDir(t.TempDir()); err == nil {
		t.Error("expected error when workshop dir is absent")
	}
}

func TestList(t *testing.T) {
	root := setupFakeSteam(t, map[string]string{
		"111": `{"type":"scene","title":"Aurora"}`,
		"222": `{"type":"video","file":"rain.mp4","title":"Rain"}`,
		"333": "",                                     // dir without project.json
		"444": `{ broken`,                             // malformed
	})
	items, err := List(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 4 {
		t.Fatalf("expected 4 items, got %d", len(items))
	}
	// Sorted by ID ascending.
	ids := []string{items[0].ID, items[1].ID, items[2].ID, items[3].ID}
	wantIDs := []string{"111", "222", "333", "444"}
	for i := range ids {
		if ids[i] != wantIDs[i] {
			t.Errorf("List()[%d].ID = %q, want %q", i, ids[i], wantIDs[i])
		}
	}
	// Items with invalid or missing project.json should carry Project == nil.
	if items[2].Project != nil {
		t.Error("item 333 should have nil Project (no project.json)")
	}
	if items[3].Project != nil {
		t.Error("item 444 should have nil Project (malformed)")
	}
	if items[0].Project == nil || items[0].Project.Title != "Aurora" {
		t.Errorf("item 111 project not parsed: %+v", items[0].Project)
	}
}

func TestResolve(t *testing.T) {
	root := setupFakeSteam(t, map[string]string{"3682370294": `{"type":"scene"}`})

	got, err := Resolve(root, "3682370294")
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "steamapps", "workshop", "content",
		WallpaperEngineAppID, "3682370294")
	if got != want {
		t.Errorf("Resolve = %q, want %q", got, want)
	}

	if _, err := Resolve(root, "9999"); err == nil {
		t.Error("expected error for missing ID")
	}
	if _, err := Resolve(root, ""); err == nil {
		t.Error("expected error for empty ID")
	}
}
