package workshop

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Downloader fetches a Steam Workshop item by its published file ID and
// returns the local directory containing the extracted content.
type Downloader interface {
	Download(ctx context.Context, workshopID string) (string, error)
}

// ProxyDownloader uses a steamworkshopdownloader.io-compatible HTTP API
// to fetch Wallpaper Engine workshop items without a Steam client.
//
// The API surface in use is:
//
//	POST {base}/api/download/request   → {"uuid": "..."}
//	POST {base}/api/download/status    → {"<uuid>": {"status": "prepared"|"queued"|"transmitting"|"error"}}
//	GET  {base}/api/download/transmit?uuid=...
//
// This is a third-party service — availability and response shape are not
// under our control. When it breaks, the fix is either a patch here or
// swapping the Downloader implementation (e.g. for DepotDownloader).
type ProxyDownloader struct {
	BaseURL      string
	CacheDir     string
	HTTPClient   *http.Client
	PollInterval time.Duration
	Timeout      time.Duration
}

// NewProxyDownloader returns a ProxyDownloader with the default endpoint
// and a cache directory. CacheDir is created on first Download.
func NewProxyDownloader(cacheDir string) *ProxyDownloader {
	return &ProxyDownloader{
		BaseURL:      "https://api.steamworkshopdownloader.io",
		CacheDir:     cacheDir,
		HTTPClient:   &http.Client{Timeout: 30 * time.Second},
		PollInterval: 2 * time.Second,
		Timeout:      5 * time.Minute,
	}
}

// Download fetches the workshop item and returns the path to the extracted
// directory. If the item is already present in the cache, the existing
// directory is returned without a network call.
func (d *ProxyDownloader) Download(ctx context.Context, workshopID string) (string, error) {
	if workshopID == "" {
		return "", errors.New("workshopID is empty")
	}
	itemDir := ItemDir(d.CacheDir, workshopID)
	if _, err := os.Stat(filepath.Join(itemDir, "project.json")); err == nil {
		return itemDir, nil
	}
	if err := os.MkdirAll(d.CacheDir, 0o755); err != nil {
		return "", fmt.Errorf("create cache dir: %w", err)
	}

	uuid, err := d.requestDownload(ctx, workshopID)
	if err != nil {
		return "", fmt.Errorf("request download: %w", err)
	}
	if err := d.waitReady(ctx, uuid); err != nil {
		return "", fmt.Errorf("wait ready: %w", err)
	}
	zipBytes, err := d.transmit(ctx, uuid)
	if err != nil {
		return "", fmt.Errorf("transmit: %w", err)
	}
	if err := extractZip(zipBytes, itemDir); err != nil {
		return "", fmt.Errorf("extract: %w", err)
	}
	return itemDir, nil
}

func (d *ProxyDownloader) requestDownload(ctx context.Context, id string) (string, error) {
	body, _ := json.Marshal(map[string]any{
		"publishedFileId": parseID(id),
		"collectionId":    nil,
		"extract":         true,
		"hidden":          false,
		"direct":          false,
		"autodownload":    false,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		d.BaseURL+"/api/download/request", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out struct {
		UUID string `json:"uuid"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	if out.UUID == "" {
		return "", errors.New("empty UUID in response")
	}
	return out.UUID, nil
}

func (d *ProxyDownloader) waitReady(ctx context.Context, uuid string) error {
	deadline := time.Now().Add(d.Timeout)
	for {
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout after %s", d.Timeout)
		}
		status, err := d.checkStatus(ctx, uuid)
		if err != nil {
			return err
		}
		switch status {
		case "prepared":
			return nil
		case "error":
			return errors.New("server reported error status")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(d.PollInterval):
		}
	}
}

func (d *ProxyDownloader) checkStatus(ctx context.Context, uuid string) (string, error) {
	body, _ := json.Marshal(map[string]any{"uuids": []string{uuid}})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		d.BaseURL+"/api/download/status", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var out map[string]struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", err
	}
	entry, ok := out[uuid]
	if !ok {
		return "", fmt.Errorf("UUID %q missing from status response", uuid)
	}
	return entry.Status, nil
}

func (d *ProxyDownloader) transmit(ctx context.Context, uuid string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		d.BaseURL+"/api/download/transmit?uuid="+uuid, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.HTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}

// parseID accepts either a bare numeric id or a full workshop URL.
func parseID(s string) string {
	if !strings.Contains(s, "?id=") {
		return s
	}
	parts := strings.SplitN(s, "?id=", 2)
	id := parts[1]
	if i := strings.IndexAny(id, "&#"); i >= 0 {
		id = id[:i]
	}
	return id
}

// extractZip unpacks a zip archive into dst. File mode is preserved for
// executables; directories are created on demand. Zip-slip attempts (paths
// escaping dst) are rejected.
func extractZip(data []byte, dst string) error {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return err
	}
	for _, f := range r.File {
		target := filepath.Join(dst, f.Name)
		if !strings.HasPrefix(target, filepath.Clean(dst)+string(os.PathSeparator)) &&
			target != filepath.Clean(dst) {
			return fmt.Errorf("zip slip rejected: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, f.Mode()); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		if _, err := io.Copy(out, rc); err != nil {
			rc.Close()
			out.Close()
			return err
		}
		rc.Close()
		out.Close()
	}
	return nil
}
