package engine

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetect(t *testing.T) {
	dir := t.TempDir()

	img := filepath.Join(dir, "wall.jpg")
	if err := os.WriteFile(img, []byte{0xff, 0xd8}, 0o644); err != nil {
		t.Fatal(err)
	}
	vid := filepath.Join(dir, "wall.mp4")
	if err := os.WriteFile(vid, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	scene := filepath.Join(dir, "scene")
	if err := os.MkdirAll(scene, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(scene, "project.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		path string
		want Kind
	}{
		{img, KindImage},
		{vid, KindVideo},
		{scene, KindScene},
	}
	for _, c := range cases {
		got, err := Detect(c.path)
		if err != nil {
			t.Errorf("Detect(%s): %v", c.path, err)
			continue
		}
		if got != c.want {
			t.Errorf("Detect(%s) = %s, want %s", c.path, got, c.want)
		}
	}
}

func TestDetectUnknown(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "note.txt")
	if err := os.WriteFile(bad, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Detect(bad); err == nil {
		t.Error("expected error for .txt, got nil")
	}
}
