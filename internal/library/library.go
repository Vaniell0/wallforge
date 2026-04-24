// Package library indexes local wallpaper directories (anything the user
// keeps in ~/Pictures/Wallpapers/ and friends) so the web-UI can show
// them alongside Steam Workshop subscriptions. Pure filesystem walk — no
// metadata parsing, no thumbnails. The browser can render the file
// directly as a preview for both image and video items.
package library

import (
	"crypto/sha1"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// maxDepth keeps the walk bounded. Four levels is more than enough for
// typical layouts (e.g. `Wallpapers/artist/series/file.png`) but still
// cheap on a poorly-organised 50k-photo drive.
const maxDepth = 4

// Item is one scanned entry. ID is stable across scans so the UI can
// identify "same file" between refreshes and the preview handler can
// map it back to an absolute path without trusting a raw query string.
type Item struct {
	ID    string `json:"id"`
	Path  string `json:"path"`
	Kind  string `json:"kind"`  // image | video
	Title string `json:"title"` // filename without extension
	Root  string `json:"root"`  // which configured root it came from
}

var (
	imageExts = map[string]struct{}{
		".png":  {},
		".jpg":  {},
		".jpeg": {},
		".webp": {},
		".bmp":  {},
	}
	videoExts = map[string]struct{}{
		".mp4":  {},
		".webm": {},
		".mkv":  {},
		".mov":  {},
		".gif":  {},
	}
)

// Scan walks every configured root and returns the indexed items.
// Roots that don't exist or aren't directories are skipped silently —
// a wallforge user who lists ~/Pictures/Wallpapers/ but doesn't have
// that directory shouldn't get an error, they should just get zero
// library items.
//
// Home-relative paths (leading "~/") are expanded against the current
// user's home directory.
func Scan(roots []string) ([]Item, error) {
	var items []Item
	for _, root := range roots {
		expanded := ExpandHome(root)
		info, err := os.Stat(expanded)
		if err != nil || !info.IsDir() {
			continue
		}
		rootItems, err := scanRoot(expanded)
		if err != nil {
			return nil, err
		}
		items = append(items, rootItems...)
	}
	sort.Slice(items, func(i, j int) bool {
		if items[i].Root != items[j].Root {
			return items[i].Root < items[j].Root
		}
		return items[i].Title < items[j].Title
	})
	return items, nil
}

func scanRoot(root string) ([]Item, error) {
	var items []Item
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			// Unreadable subtree — log-and-continue semantics via nil.
			// We'd rather index what we can than fail the whole scan.
			return nil
		}
		if depthBelow(root, path) > maxDepth {
			if d.IsDir() {
				return fs.SkipDir
			}
			// File too deep — also skip. We don't bail on the whole
			// walk because sibling branches may be within the limit.
			return nil
		}
		if d.IsDir() {
			return nil
		}
		ext := strings.ToLower(filepath.Ext(path))
		kind := classify(ext)
		if kind == "" {
			return nil
		}
		name := d.Name()
		title := strings.TrimSuffix(name, filepath.Ext(name))
		items = append(items, Item{
			ID:    "lib_" + shortHash(path),
			Path:  path,
			Kind:  kind,
			Title: title,
			Root:  root,
		})
		return nil
	})
	return items, err
}

func classify(ext string) string {
	if _, ok := imageExts[ext]; ok {
		return "image"
	}
	if _, ok := videoExts[ext]; ok {
		return "video"
	}
	return ""
}

func depthBelow(root, path string) int {
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." {
		return 0
	}
	return strings.Count(rel, string(os.PathSeparator)) + 1
}

func shortHash(s string) string {
	sum := sha1.Sum([]byte(s))
	return hex.EncodeToString(sum[:8]) // 16 chars, plenty to avoid collisions in a personal index
}

// ExpandHome replaces a leading "~" or "~/" with the user's home
// directory. Returns the input unchanged if HOME can't be determined.
func ExpandHome(p string) string {
	if p == "" || p[0] != '~' {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if len(p) == 1 {
		return home
	}
	if p[1] == '/' {
		return filepath.Join(home, p[2:])
	}
	return p
}
