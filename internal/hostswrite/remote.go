package hostswrite

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/oldj/switchhosts/internal/store"
)

// HTTPClient returns the client remote hosts are fetched with, honoring the
// proxy preferences.
func HTTPClient(cfg *store.Config) *http.Client {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	if cfg.UseProxy && cfg.ProxyHost != "" {
		addr := fmt.Sprintf("%s:%d", cfg.ProxyHost, cfg.ProxyPort)
		if proxy, err := url.Parse(cfg.ProxyProtocol + "://" + addr); err == nil {
			transport.Proxy = http.ProxyURL(proxy)
		}
	}
	return &http.Client{Transport: transport, Timeout: 30 * time.Second}
}

const maxRemoteBytes = 32 << 20

// FetchRemote downloads a remote hosts URL, or reads it from disk for
// file:// and plain paths.
func FetchRemote(client *http.Client, rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}
	switch strings.ToLower(parsed.Scheme) {
	case "file":
		return readFileLimited(parsed.Path)
	case "http", "https":
	case "":
		return readFileLimited(rawURL)
	default:
		return "", fmt.Errorf("unsupported URL scheme %q", parsed.Scheme)
	}
	resp, err := client.Get(rawURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxRemoteBytes))
	if err != nil {
		return "", err
	}
	return store.NormalizeToLF(string(data)), nil
}

func readFileLimited(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if len(data) > maxRemoteBytes {
		return "", fmt.Errorf("file is too large")
	}
	return store.NormalizeToLF(string(data)), nil
}

// OperationResult is what the app shows after an action on an entry.
type OperationResult struct {
	Success bool           `json:"success"`
	Code    string         `json:"code,omitempty"`
	Message string         `json:"message,omitempty"`
	Changed bool           `json:"changed,omitempty"`
	Data    store.NodeJSON `json:"data,omitempty"`
}

// RefreshNode refreshes one remote entry and returns its updated metadata.
func RefreshNode(client *http.Client, p store.Paths, id string) (*OperationResult, error) {
	m, err := store.LoadManifest(p)
	if err != nil {
		return nil, err
	}
	node := store.FindNode(m.Root, id)
	if node == nil {
		return &OperationResult{Success: false, Code: "invalid_id", Message: "node not found"}, nil
	}
	if store.NodeType(node) != "remote" {
		return &OperationResult{Success: false, Code: "not_remote", Message: "node is not a remote hosts"}, nil
	}
	rawURL, _ := node["url"].(string)
	if rawURL == "" {
		return &OperationResult{Success: false, Code: "no_url", Message: "remote node has no URL"}, nil
	}

	content, err := FetchRemote(client, rawURL)
	if err != nil {
		return &OperationResult{Success: false, Code: "fetch_failed", Message: err.Error()}, nil
	}

	now := time.Now()
	node["last_refresh"] = now.Format("2006-01-02 15:04:05")
	node["last_refresh_ms"] = now.UnixMilli()
	previous, err := store.ReadEntry(p, id)
	if err != nil {
		return nil, err
	}
	err = store.Tx(p, []store.TxTarget{
		{Kind: "state"},
		{Kind: "manifest"},
		{Kind: "entry", ID: id},
	}, func() error {
		if previous != content {
			if err := store.WriteEntry(p, id, content); err != nil {
				return err
			}
		}
		return store.SaveManifest(p, m)
	})
	if err != nil {
		return nil, err
	}
	updated := store.FindNode(m.Root, id)
	return &OperationResult{Success: true, Changed: previous != content, Data: updated}, nil
}

// SearchHit is one match of a find query.
type SearchHit struct {
	ItemID    string      `json:"item_id"`
	ItemTitle string      `json:"item_title"`
	ItemType  string      `json:"item_type"`
	Positions []FindMatch `json:"positions"`
}

// FindMatch is one match inside an entry.
type FindMatch struct {
	Start      int    `json:"start"`
	End        int    `json:"end"`
	Line       int    `json:"line"`
	LinePos    int    `json:"line_pos"`
	EndLine    int    `json:"end_line"`
	EndLinePos int    `json:"end_line_pos"`
	Before     string `json:"before"`
	Match      string `json:"match"`
	After      string `json:"after"`
}

// FindOptions are the search options of the find window.
type FindOptions struct {
	IsRegexp     bool `json:"is_regexp"`
	IsIgnoreCase bool `json:"is_ignore_case"`
}

var _ = json.Marshal
