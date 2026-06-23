package update

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Info describes an available update from GitHub Releases.
type Info struct {
	Current     string
	Latest      string
	TagName     string
	ReleaseURL  string
	PackageName string
	PackageURL  string
	SumsURL     string
}

// Check queries GitHub for a newer stable release. skipVersion suppresses that exact tag.
// Returns nil when no update is available, updates are disabled, or the build is a dev version.
func Check(ctx context.Context, currentVersion, skipVersion string, client *http.Client) (*Info, error) {
	if !Enabled() {
		return nil, nil
	}
	if IsDevVersion(currentVersion) {
		return nil, nil
	}
	return checkAvailable(ctx, currentVersion, skipVersion, client)
}

func checkAvailable(ctx context.Context, currentVersion, skipVersion string, client *http.Client) (*Info, error) {
	rel, err := fetchLatestRelease(ctx, client)
	if err != nil {
		return nil, err
	}
	if skipVersion != "" && stringsEqualTag(rel.TagName, skipVersion) {
		return nil, nil
	}

	latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	if !IsNewer(latest, currentVersion) {
		return nil, nil
	}

	packageName, packageURL, err := platformReleaseAsset(rel)
	if err != nil {
		return nil, err
	}
	sumsURL, err := checksumsAsset(rel)
	if err != nil {
		return nil, err
	}

	return &Info{
		Current:     currentVersion,
		Latest:      latest,
		TagName:     rel.TagName,
		ReleaseURL:  rel.HTMLURL,
		PackageName: packageName,
		PackageURL:  packageURL,
		SumsURL:     sumsURL,
	}, nil
}

func stringsEqualTag(a, b string) bool {
	a = strings.TrimSpace(a)
	b = strings.TrimSpace(b)
	if a == b {
		return true
	}
	return strings.TrimPrefix(a, "v") == strings.TrimPrefix(b, "v")
}

// DownloadAndStage downloads the release package, verifies SHA256SUMS, and stages the update payload.
func DownloadAndStage(ctx context.Context, info *Info, stagingDir string, progress Progress, client *http.Client) (staged string, err error) {
	if info == nil {
		return "", fmt.Errorf("update info is nil")
	}
	const stageTotal int64 = 1000

	report := func(done int64) {
		if progress != nil {
			progress(done, stageTotal)
		}
	}

	if err := os.RemoveAll(stagingDir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(stagingDir, 0o755); err != nil {
		return "", err
	}

	packagePath := filepath.Join(stagingDir, info.PackageName)
	dlProgress := throttleProgress(scaleProgress(progress, 0, 750, stageTotal), 200*time.Millisecond)
	if err := downloadURL(ctx, client, info.PackageURL, packagePath, dlProgress); err != nil {
		return "", err
	}
	report(760)

	sums, err := downloadBytes(ctx, client, info.SumsURL)
	if err != nil {
		return "", fmt.Errorf("download SHA256SUMS: %w", err)
	}
	report(800)

	report(820)

	if err := verifyReleaseChecksum(packagePath, info.PackageName, sums); err != nil {
		return "", err
	}
	report(850)

	extractProgress := throttleProgress(scaleProgress(progress, 850, 150, stageTotal), 100*time.Millisecond)
	staged, err = stageReleasePackage(packagePath, stagingDir, extractProgress)
	if err != nil {
		return "", err
	}
	report(stageTotal)

	return staged, nil
}
