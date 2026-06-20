package update

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	githubOwner = "svpchain"
	githubRepo  = "svpchain-mcp"
	zipStem     = "svpchain-agent"
)

var latestReleaseURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", githubOwner, githubRepo)

type releaseAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

type githubRelease struct {
	TagName string         `json:"tag_name"`
	HTMLURL string         `json:"html_url"`
	Assets  []releaseAsset `json:"assets"`
}

func fetchLatestRelease(ctx context.Context, client *http.Client) (*githubRelease, error) {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, latestReleaseURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "svpchain-gui")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return nil, fmt.Errorf("fetch release: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}
	if strings.TrimSpace(rel.TagName) == "" {
		return nil, fmt.Errorf("release missing tag_name")
	}
	return &rel, nil
}

func macOSZipAsset(rel *githubRelease) (name, url string, err error) {
	version := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	want := fmt.Sprintf("%s-%s-macos.zip", zipStem, version)
	for _, a := range rel.Assets {
		if a.Name == want && a.BrowserDownloadURL != "" {
			return a.Name, a.BrowserDownloadURL, nil
		}
	}
	return "", "", fmt.Errorf("release asset %q not found", want)
}

func checksumsAsset(rel *githubRelease) (url string, err error) {
	for _, a := range rel.Assets {
		if a.Name == "SHA256SUMS" && a.BrowserDownloadURL != "" {
			return a.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("release asset SHA256SUMS not found")
}
