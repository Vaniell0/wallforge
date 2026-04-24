// Package steam resolves local Wallpaper Engine Workshop content from
// the installed Steam client. The user's Steam install already keeps every
// subscribed item's project.json and assets up to date, so we work against
// that cache instead of fetching through third-party proxies.
package steam

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/Vaniell0/wallforge/internal/workshop"
)

// WallpaperEngineAppID is the Steam app ID for Wallpaper Engine.
const WallpaperEngineAppID = "431960"

// candidates returns the Steam installation roots to try, in priority
// order. An override (from config) short-circuits the list.
func candidates(override string) []string {
	if override != "" {
		return []string{override}
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, ".local", "share", "Steam"),
		filepath.Join(home, ".steam", "steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", "data", "Steam"),
		filepath.Join(home, ".var", "app", "com.valvesoftware.Steam", ".local", "share", "Steam"),
	}
}

// FindWorkshopDir returns the path to the WE workshop content directory
// (.../steamapps/workshop/content/431960). Returns fs.ErrNotExist when
// no Steam install is found at any of the standard locations.
func FindWorkshopDir(override string) (string, error) {
	for _, root := range candidates(override) {
		dir := filepath.Join(root, "steamapps", "workshop", "content", WallpaperEngineAppID)
		info, err := os.Stat(dir)
		if err == nil && info.IsDir() {
			return dir, nil
		}
	}
	return "", fmt.Errorf("no Steam Wallpaper Engine workshop dir found: %w", fs.ErrNotExist)
}

// Item is a single subscribed Wallpaper Engine item.
type Item struct {
	ID      string
	Path    string
	Project *workshop.Project // nil if project.json is missing or invalid
}

// List enumerates every subdirectory of the Workshop content dir and
// decorates each with its parsed project.json when possible. Items with
// an unreadable project.json are still returned, with Project == nil, so
// the caller can flag them instead of silently hiding them.
func List(override string) ([]Item, error) {
	dir, err := FindWorkshopDir(override)
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", dir, err)
	}
	var items []Item
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		itemPath := filepath.Join(dir, e.Name())
		proj, _ := workshop.ParseDir(itemPath) // ignore parse errors; surface via nil
		items = append(items, Item{
			ID:      e.Name(),
			Path:    itemPath,
			Project: proj,
		})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	return items, nil
}

// Resolve turns a bare numeric workshop ID into the full path of the
// downloaded item. Useful for `wallforge apply <id>`.
func Resolve(override, id string) (string, error) {
	if id == "" {
		return "", errors.New("empty workshop ID")
	}
	dir, err := FindWorkshopDir(override)
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, id)
	info, err := os.Stat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("workshop item %s not found — is it subscribed in Steam?", id)
		}
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a directory", path)
	}
	return path, nil
}
