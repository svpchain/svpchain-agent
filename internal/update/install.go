package update

import (
	"context"
	"fmt"
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
	DmgName    string
	DmgURL     string
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

	dmgName, dmgURL, err := macOSDmgAsset(rel)
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
		DmgName:    dmgName,
		DmgURL:     dmgURL,
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

// DownloadAndStage downloads the release DMG, verifies SHA256SUMS, and stages the .app under stagingDir.
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

	dmgPath := filepath.Join(stagingDir, info.DmgName)
	dlProgress := throttleProgress(scaleProgress(progress, 0, 750, stageTotal), 200*time.Millisecond)
	if err := downloadURL(ctx, client, info.DmgURL, dmgPath, dlProgress); err != nil {
		return "", err
	}
	report(760)

	sums, err := downloadBytes(ctx, client, info.SumsURL)
	if err != nil {
		return "", fmt.Errorf("download SHA256SUMS: %w", err)
	}
	report(800)

	report(820)

	if err := verifyReleaseChecksum(dmgPath, info.DmgName, sums); err != nil {
		return "", err
	}
	report(850)

	extractProgress := throttleProgress(scaleProgress(progress, 850, 150, stageTotal), 100*time.Millisecond)
	stagedApp, err = stageAppFromDMG(dmgPath, stagingDir, extractProgress)
	if err != nil {
		return "", err
	}
	report(stageTotal)

	return stagedApp, nil
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

func stageAppFromDMG(dmgPath, stagingDir string, progress Progress) (string, error) {
	mountPoint, err := os.MkdirTemp(stagingDir, "dmg-mount-*")
	if err != nil {
		return "", err
	}
	defer func() {
		_ = exec.Command("hdiutil", "detach", mountPoint, "-quiet").Run()
		_ = os.RemoveAll(mountPoint)
	}()

	cmd := exec.Command("hdiutil", "attach", "-nobrowse", "-readonly", "-mountpoint", mountPoint, dmgPath)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("hdiutil attach: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if progress != nil {
		progress(1, 2)
	}

	appSrc, err := findAppBundleInDir(mountPoint)
	if err != nil {
		return "", err
	}

	stagedApp := filepath.Join(stagingDir, filepath.Base(appSrc))
	cmd = exec.Command("ditto", appSrc, stagedApp)
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("copy app from dmg: %w: %s", err, strings.TrimSpace(string(out)))
	}
	if progress != nil {
		progress(2, 2)
	}
	return stagedApp, nil
}
