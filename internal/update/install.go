package update

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const helperScript = `#!/bin/bash
set -euo pipefail
APP_PID="$1"
TARGET_BUNDLE="$2"
STAGED_BUNDLE="$3"
wait "$APP_PID" 2>/dev/null || true
sleep 1
rm -rf "$TARGET_BUNDLE"
ditto "$STAGED_BUNDLE" "$TARGET_BUNDLE"
xattr -cr "$TARGET_BUNDLE" 2>/dev/null || true
open "$TARGET_BUNDLE"
`

// Info describes an available update from GitHub Releases.
type Info struct {
	Current    string
	Latest     string
	TagName    string
	ReleaseURL string
	ZipName    string
	ZipURL     string
	SumsURL    string
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

	zipName, zipURL, err := macOSZipAsset(rel)
	if err != nil {
		return nil, err
	}
	sumsURL, err := checksumsAsset(rel)
	if err != nil {
		return nil, err
	}

	return &Info{
		Current:    currentVersion,
		Latest:     latest,
		TagName:    rel.TagName,
		ReleaseURL: rel.HTMLURL,
		ZipName:    zipName,
		ZipURL:     zipURL,
		SumsURL:    sumsURL,
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

// DownloadAndStage downloads the release zip, verifies SHA256SUMS, and extracts the .app to stagingDir.
func DownloadAndStage(ctx context.Context, info *Info, stagingDir string, progress Progress, client *http.Client) (stagedApp string, err error) {
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

	zipPath := filepath.Join(stagingDir, info.ZipName)
	dlProgress := throttleProgress(scaleProgress(progress, 0, 750, stageTotal), 200*time.Millisecond)
	if err := downloadURL(ctx, client, info.ZipURL, zipPath, dlProgress); err != nil {
		return "", err
	}
	report(760)

	sums, err := downloadBytes(ctx, client, info.SumsURL)
	if err != nil {
		return "", fmt.Errorf("download SHA256SUMS: %w", err)
	}
	report(800)

	report(820)

	if err := verifyZipChecksum(zipPath, info.ZipName, sums); err != nil {
		return "", err
	}
	report(850)

	extractDir := filepath.Join(stagingDir, "extract")
	unzipProgress := throttleProgress(scaleProgress(progress, 850, 150, stageTotal), 100*time.Millisecond)
	if err := unzip(zipPath, extractDir, unzipProgress); err != nil {
		return "", err
	}
	report(stageTotal)

	return findAppBundleInDir(extractDir)
}

// LaunchReplacer starts a helper that replaces targetBundle with stagedApp after this process exits.
func LaunchReplacer(targetBundle, stagedApp string) error {
	cache, err := os.UserCacheDir()
	if err != nil {
		return err
	}
	helperDir := filepath.Join(cache, "com.svpchain.agent-gui", "update")
	if err := os.MkdirAll(helperDir, 0o755); err != nil {
		return err
	}
	helperPath := filepath.Join(helperDir, "apply-update.sh")
	if err := os.WriteFile(helperPath, []byte(helperScript), 0o755); err != nil {
		return err
	}

	cmd := exec.Command("/bin/bash", helperPath,
		strconv.Itoa(os.Getpid()),
		targetBundle,
		stagedApp,
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start update helper: %w", err)
	}
	go cmd.Wait()
	return nil
}

func unzip(src, dest string, progress Progress) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return err
	}

	totalFiles := int64(0)
	for _, f := range r.File {
		if !f.FileInfo().IsDir() {
			totalFiles++
		}
	}

	var done int64
	for _, f := range r.File {
		path := filepath.Join(dest, f.Name)
		if !filepath.HasPrefix(path, filepath.Clean(dest)+string(os.PathSeparator)) {
			return fmt.Errorf("invalid zip entry: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, copyErr := io.Copy(out, rc)
		closeErr := rc.Close()
		cerr := out.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if cerr != nil {
			return cerr
		}
		done++
		if progress != nil && totalFiles > 0 {
			progress(done, totalFiles)
		}
	}
	return nil
}
